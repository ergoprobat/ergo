//go:build conformance

package conformance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func semgrepOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return runCommand(t, ctx, exec.CommandContext(ctx, "semgrep", args...), dir)
}

func nodeOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return runCommand(t, ctx, exec.CommandContext(ctx, "node", args...), dir)
}

func javacOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return runCommand(t, ctx, exec.CommandContext(ctx, "javac", args...), dir)
}

func javaOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return runCommand(t, ctx, exec.CommandContext(ctx, "java", args...), dir)
}

func kyvernoOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "kyverno", args...)
	pinCommand(t, cmd, "../kyverno/testdata/tool/bin/kyverno")
	return runCommand(t, ctx, cmd, dir)
}

func gatorOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "gator", args...)
	pinCommand(t, cmd, "../opa/testdata/tool/bin/gator")
	return runCommand(t, ctx, cmd, dir)
}

func eslintOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "eslint", args...)
	pinCommand(t, cmd, "../eslint/testdata/tool/node_modules/.bin/eslint")
	return runCommand(t, ctx, cmd, dir)
}

// pinCommand runs the pinned binary. The command name stays a literal so the
// call is not a variable executable. LookPath may fail when that name is not
// on PATH, so the absolute pin replaces it.
func pinCommand(t *testing.T, cmd *exec.Cmd, bin string) {
	t.Helper()
	abs, err := filepath.Abs(bin)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Path = abs
	cmd.Err = nil
}

func lintImportsOutput(t *testing.T, timeout time.Duration, dir string, args ...string) (stdout, stderr []byte, exit int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return runCommand(t, ctx, exec.CommandContext(ctx, "lint-imports", args...), dir)
}

func copyFixtureTree(t *testing.T, src string) string {
	t.Helper()
	dest := t.TempDir()
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".fixture") {
			rel = filepath.Join(filepath.Dir(rel), strings.TrimSuffix(entry.Name(), ".fixture"))
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

func runCommand(t *testing.T, ctx context.Context, cmd *exec.Cmd, dir string) (stdout, stderr []byte, exit int) {
	t.Helper()
	if cmd.Err != nil {
		t.Fatal(cmd.Err)
	}
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "SEMGREP_SEND_METRICS=off", "SEMGREP_ENABLE_VERSION_CHECK=0")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	if runErr != nil && ctx.Err() != nil {
		t.Fatalf("%s timed out\n%s", cmd.Path, errBuf.String())
	}
	if runErr == nil {
		return out.Bytes(), errBuf.Bytes(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return out.Bytes(), errBuf.Bytes(), exitErr.ExitCode()
	}
	t.Fatal(runErr)
	return nil, nil, 0
}

func requireVersion(t *testing.T, stdout []byte, want string) {
	t.Helper()
	got := strings.TrimSpace(string(stdout))
	if got != want {
		t.Fatalf("version %q, want %s", got, want)
	}
}
