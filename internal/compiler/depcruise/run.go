package depcruise

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const (
	versionTimeout = 30 * time.Second
	cruiseTimeout  = 10 * time.Minute
	commandWait    = 5 * time.Second
	maxStdoutBytes = 256 << 20
	stderrLimit    = 1024
	versionLimit   = 32
)

// pathSegment is one slash-separated piece of a cruise path.
// A hyphen is allowed inside a segment. A path that starts with a hyphen is rejected separately,
// because dependency-cruiser would read it as a flag.
var pathSegment = regexp.MustCompile(`^[A-Za-z0-9._@+-]+$`)

// Tool is the depcruise binary resolved from PATH.
type Tool struct {
	Version string
	// timeout, outputLimit, and wait override the production bounds. Zero means the default.
	timeout     time.Duration
	outputLimit int
	wait        time.Duration
}

// FindTool resolves depcruise on PATH and reads its version.
// The executable name is a literal, so LookPath refuses a binary found relative to the current directory.
func FindTool(ctx context.Context) (Tool, error) {
	if _, err := exec.LookPath("depcruise"); err != nil {
		return Tool{}, &compiler.Error{Code: CodeMissing, Err: errors.New("depcruise is not on PATH")}
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "depcruise", "--version")
	cmd.Stdin = bytes.NewReader(nil)
	cmd.WaitDelay = commandWait
	prepareCommand(cmd)
	stdout := &cappedBuffer{max: versionLimit + 1}
	stderr := &cappedBuffer{max: stderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil || ctx.Err() != nil || stdout.over {
		return Tool{}, &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("depcruise --version: %s", stderrExcerpt(stderr.buf.Bytes()))}
	}
	version := strings.TrimSpace(stdout.buf.String())
	if _, _, _, ok := parseSemver(version); !ok {
		return Tool{}, &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("version %q", version)}
	}
	return Tool{Version: version}, nil
}

// Cruise runs depcruise on a temporary copy of config.
// The copy is the artifact LoadBuild verified, so a later change under .ergo/build is not what runs.
func (t Tool) Cruise(ctx context.Context, root string, config []byte, paths []string) ([]byte, error) {
	cleaned, err := NormalizePaths(paths)
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	if err := withinRoot(root, cleaned); err != nil {
		if compiler.CodeOf(err) != "" {
			return nil, err
		}
		return nil, &compiler.Error{Code: CodePath, Err: err}
	}
	dir, err := os.MkdirTemp("", "ergo-depcruise-")
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	configPath := filepath.Join(dir, artifactName)
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, t.cruiseLimit())
	defer cancel()
	args := make([]string, 0, 4+len(cleaned))
	args = append(args, "--config", configPath, "--output-type", "json")
	args = append(args, cleaned...)
	cmd := exec.CommandContext(ctx, "depcruise", args...)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(nil)
	cmd.WaitDelay = t.waitDelay()
	prepareCommand(cmd)
	stdout := &cappedBuffer{max: t.outputMax() + 1}
	stderr := &cappedBuffer{max: stderrLimit}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
	}
	if stdout.over {
		return nil, &compiler.Error{Code: CodeOutput, Err: fmt.Errorf("output is larger than %d bytes", t.outputMax())}
	}
	// A non-zero exit is a failed cruise, even when stdout looks like JSON.
	// The JSON reporter exits 0 and records violations in the summary.
	// A crash can print a partial report, and that report must not become a verdict.
	if runErr != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", stderrExcerpt(stderr.buf.Bytes()))}
	}
	return stdout.buf.Bytes(), nil
}

// Memo runs one cruise per distinct config. Checks that compiled to the same bytes share that result.
type Memo struct {
	Tool  Tool
	Root  string
	Paths []string
	hits  map[[sha256.Size]byte]memoResult
}

type memoResult struct {
	raw []byte
	err error
}

// Cruise returns the cruise for config, reusing an earlier cruise of the same bytes.
func (m *Memo) Cruise(ctx context.Context, config []byte) ([]byte, error) {
	if m.hits == nil {
		m.hits = map[[sha256.Size]byte]memoResult{}
	}
	key := sha256.Sum256(config)
	if hit, ok := m.hits[key]; ok {
		return append([]byte(nil), hit.raw...), hit.err
	}
	raw, err := m.Tool.Cruise(ctx, m.Root, config, m.Paths)
	m.hits[key] = memoResult{raw: append([]byte(nil), raw...), err: err}
	return raw, err
}

func (t Tool) cruiseLimit() time.Duration {
	if t.timeout > 0 {
		return t.timeout
	}
	return cruiseTimeout
}

func (t Tool) waitDelay() time.Duration {
	if t.wait > 0 {
		return t.wait
	}
	return commandWait
}

func (t Tool) outputMax() int {
	if t.outputLimit > 0 {
		return t.outputLimit
	}
	return maxStdoutBytes
}

// withinRoot reports whether every path stays inside root after symlink resolution.
// A missing path is left for dependency-cruiser. A symlink that leaves root is depcruise-path.
func withinRoot(root string, paths []string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rootResolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	for _, rel := range paths {
		// "." is the repository root. Walking it for every symlink is the cruise itself.
		// A named path is rejected when that path resolves outside root.
		if rel == "." {
			continue
		}
		current := rootResolved
		for _, segment := range strings.Split(rel, "/") {
			next := filepath.Join(current, segment)
			info, err := os.Lstat(next)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				current = next
				continue
			}
			resolved, err := filepath.EvalSymlinks(next)
			if err != nil {
				return err
			}
			if !insideRoot(rootResolved, resolved) {
				return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s leaves the repository", rel)}
			}
			current = resolved
		}
	}
	return nil
}

func insideRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

// NormalizePath checks one cruise path and returns its cleaned form.
// A path that starts with a hyphen is rejected so it cannot be read as a flag.
func NormalizePath(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return "", fmt.Errorf("path %q", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == ".." || !pathSegment.MatchString(segment) {
			return "", fmt.Errorf("path %q", value)
		}
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "-") {
		return "", fmt.Errorf("path %q", value)
	}
	return cleaned, nil
}

// NormalizePaths checks every cruise path. The order is kept.
func NormalizePaths(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("no paths")
	}
	out := make([]string, len(values))
	for i, value := range values {
		cleaned, err := NormalizePath(value)
		if err != nil {
			return nil, err
		}
		out[i] = cleaned
	}
	return out, nil
}

func stderrExcerpt(raw []byte) string {
	if len(raw) == 0 {
		return "depcruise failed"
	}
	text := compiler.EscapeControls(string(raw))
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

type cappedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.max <= 0 {
		return 0, errors.New("output cap is unset")
	}
	if c.buf.Len() >= c.max {
		c.over = true
		return len(p), nil
	}
	room := c.max - c.buf.Len()
	if len(p) > room {
		if _, err := c.buf.Write(p[:room]); err != nil {
			return 0, err
		}
		c.over = true
		return len(p), nil
	}
	if _, err := c.buf.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}
