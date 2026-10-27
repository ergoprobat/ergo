package semgrep

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/toolrun"
)

const (
	versionTimeout = 30 * time.Second
	scanTimeout    = 10 * time.Minute
	commandWait    = 5 * time.Second
	maxStdoutBytes = 256 << 20
	stderrLimit    = 1024
	versionLimit   = 32
)

// Tool is the semgrep binary resolved from PATH.
type Tool struct {
	Version string
	// timeout, outputLimit, and wait override the production bounds. Zero means the default.
	timeout     time.Duration
	outputLimit int
	wait        time.Duration
}

// FindTool resolves semgrep on PATH and reads its version.
// The executable name is a literal, so LookPath refuses a binary found relative to the current directory.
func FindTool(ctx context.Context) (Tool, error) {
	if _, err := exec.LookPath("semgrep"); err != nil {
		return Tool{}, &compiler.Error{Code: CodeMissing, Err: errors.New("semgrep is not on PATH")}
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "semgrep", "--version")
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: versionLimit, Stderr: stderrLimit, Wait: commandWait})
	if err != nil {
		return Tool{}, &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("semgrep --version: %s", toolrun.Excerpt(toolrun.Stderr(err), "semgrep failed"))}
	}
	version := strings.TrimSpace(string(out.Stdout))
	if _, _, _, ok := toolrun.ParseSemver(version); !ok {
		return Tool{}, &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("version %q", version)}
	}
	return Tool{Version: version}, nil
}

// Scan runs semgrep on a temporary copy of config.
// The copy is the artifact LoadBuild verified, so a later change under .ipso/build is not what runs.
func (t Tool) Scan(ctx context.Context, root string, config []byte, paths []string) ([]byte, error) {
	cleaned, err := toolrun.NormalizePaths(paths)
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	if err := toolrun.WithinRoot(root, cleaned); err != nil {
		return nil, &compiler.Error{Code: CodePath, Err: err}
	}
	dir, err := os.MkdirTemp("", "ipso-semgrep-")
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	configPath := filepath.Join(dir, ArtifactName)
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, t.scanLimit())
	defer cancel()
	args := make([]string, 0, 12+len(cleaned))
	args = append(args,
		"scan",
		"--config", configPath,
		"--json",
		"--metrics=off",
		"--disable-version-check",
		"--oss-only",
		"--no-rewrite-rule-ids",
		"--disable-nosem",
		"--x-ignore-semgrepignore-files",
		"--max-target-bytes=0",
	)
	args = append(args, cleaned...)
	cmd := exec.CommandContext(ctx, "semgrep", args...)
	cmd.Dir = root
	cmd.Env = scanEnv()
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: t.outputMax(), Stderr: stderrLimit, Wait: t.waitDelay()})
	if err != nil {
		return nil, scanErr(err, t.outputMax())
	}
	return out.Stdout, nil
}

// scanEnv is the runner environment without SEMGREP_* variables.
// Those variables can narrow the scan, point it at a token, or select other rules.
// Metrics and the version check are then turned off explicitly.
func scanEnv() []string {
	out := make([]string, 0, len(os.Environ())+2)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(key), "SEMGREP_") {
			continue
		}
		out = append(out, item)
	}
	return append(out, "SEMGREP_SEND_METRICS=off", "SEMGREP_ENABLE_VERSION_CHECK=0")
}

func scanErr(err error, max int) error {
	var timeout *toolrun.TimeoutError
	var over *toolrun.OutputError
	switch {
	case errors.As(err, &timeout):
		return &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
	case errors.As(err, &over):
		return &compiler.Error{Code: CodeOutput, Err: fmt.Errorf("output is larger than %d bytes", max)}
	default:
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", toolrun.Excerpt(toolrun.Stderr(err), "semgrep failed"))}
	}
}

// Memo runs one scan per distinct config. Checks that compiled to the same bytes share that result.
type Memo struct {
	Tool   Tool
	Root   string
	Paths  []string
	shared toolrun.Memo
}

// Scan returns the scan for config, reusing an earlier scan of the same bytes.
func (m *Memo) Scan(ctx context.Context, config []byte) ([]byte, error) {
	if m.shared.Run == nil {
		tool := m.Tool
		root := m.Root
		paths := append([]string(nil), m.Paths...)
		m.shared.Run = func(ctx context.Context, config []byte) ([]byte, error) {
			return tool.Scan(ctx, root, config, paths)
		}
	}
	return m.shared.Do(ctx, config)
}

func (t Tool) scanLimit() time.Duration {
	if t.timeout > 0 {
		return t.timeout
	}
	return scanTimeout
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
