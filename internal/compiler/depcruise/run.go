package depcruise

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
	cruiseTimeout  = 10 * time.Minute
	commandWait    = 5 * time.Second
	maxStdoutBytes = 256 << 20
	stderrLimit    = 1024
	versionLimit   = 32
)

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
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: versionLimit, Stderr: stderrLimit, Wait: commandWait})
	if err != nil {
		return Tool{}, &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("depcruise --version: %s", toolrun.Excerpt(toolrun.Stderr(err), "depcruise failed"))}
	}
	version := strings.TrimSpace(string(out.Stdout))
	if _, _, _, ok := toolrun.ParseSemver(version); !ok {
		return Tool{}, &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("version %q", version)}
	}
	return Tool{Version: version}, nil
}

// Cruise runs depcruise on a temporary copy of config.
// The copy is the artifact LoadBuild verified, so a later change under .ipso/build is not what runs.
func (t Tool) Cruise(ctx context.Context, root string, config []byte, paths []string) ([]byte, error) {
	cleaned, err := toolrun.NormalizePaths(paths)
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	if err := toolrun.WithinRoot(root, cleaned); err != nil {
		return nil, &compiler.Error{Code: CodePath, Err: err}
	}
	dir, err := os.MkdirTemp("", "ipso-depcruise-")
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
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: t.outputMax(), Stderr: stderrLimit, Wait: t.waitDelay()})
	if err != nil {
		return nil, cruiseErr(err, t.outputMax())
	}
	return out.Stdout, nil
}

func cruiseErr(err error, max int) error {
	var timeout *toolrun.TimeoutError
	var over *toolrun.OutputError
	switch {
	case errors.As(err, &timeout):
		return &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
	case errors.As(err, &over):
		return &compiler.Error{Code: CodeOutput, Err: fmt.Errorf("output is larger than %d bytes", max)}
	default:
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", toolrun.Excerpt(toolrun.Stderr(err), "depcruise failed"))}
	}
}

// Memo runs one cruise per distinct config. Checks that compiled to the same bytes share that result.
type Memo struct {
	Tool   Tool
	Root   string
	Paths  []string
	shared toolrun.Memo
}

// Cruise returns the cruise for config, reusing an earlier cruise of the same bytes.
func (m *Memo) Cruise(ctx context.Context, config []byte) ([]byte, error) {
	if m.shared.Run == nil {
		tool := m.Tool
		root := m.Root
		paths := append([]string(nil), m.Paths...)
		m.shared.Run = func(ctx context.Context, config []byte) ([]byte, error) {
			return tool.Cruise(ctx, root, config, paths)
		}
	}
	return m.shared.Do(ctx, config)
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
