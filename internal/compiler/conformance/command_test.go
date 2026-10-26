//go:build conformance

package conformance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
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
