package toolrun

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
)

// semverPattern is MAJOR.MINOR.PATCH with no leading zeros and no prerelease.
var semverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Limits bounds one command.
// Stdout is the most bytes of standard output that still count as a result. One more is an output error.
// Stderr is the most bytes of standard error that are kept for the excerpt.
// Wait is how long a child may hold a pipe after the context ends.
type Limits struct {
	Stdout int
	Stderr int
	Wait   time.Duration
}

// Result is the output of a command that exited 0 inside its cap.
type Result struct {
	Stdout []byte
}

// TimeoutError means the context ended before the command finished.
type TimeoutError struct {
	Stderr []byte
}

func (e *TimeoutError) Error() string { return "timed out" }

// OutputError means standard output exceeded Limits.Stdout.
type OutputError struct {
	Max    int
	Stderr []byte
}

func (e *OutputError) Error() string {
	return fmt.Sprintf("output is larger than %d bytes", e.Max)
}

// ExitError means the command finished with a non-zero status.
type ExitError struct {
	Stderr []byte
	Err    error
}

func (e *ExitError) Error() string { return "command failed" }

func (e *ExitError) Unwrap() error { return e.Err }

// Stderr returns the standard error captured for err, or nil.
func Stderr(err error) []byte {
	switch e := err.(type) {
	case *TimeoutError:
		return e.Stderr
	case *OutputError:
		return e.Stderr
	case *ExitError:
		return e.Stderr
	default:
		return nil
	}
}

// ParseSemver reports the components of a MAJOR.MINOR.PATCH version.
func ParseSemver(value string) (major, minor, patch int, ok bool) {
	parts := semverPattern.FindStringSubmatch(value)
	if parts == nil {
		return 0, 0, 0, false
	}
	major, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, false
	}
	minor, err = strconv.Atoi(parts[2])
	if err != nil {
		return 0, 0, 0, false
	}
	patch, err = strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, 0, false
	}
	return major, minor, patch, true
}

// Excerpt returns a one-line excerpt of raw, or fallback when raw is empty.
// Control characters are escaped. The excerpt is at most 200 bytes.
func Excerpt(raw []byte, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	text := compiler.EscapeControls(string(raw))
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}

// Run executes cmd. The caller sets Dir, Env, and a context deadline.
// Standard input is empty. The process tree is killed when the context ends.
func Run(ctx context.Context, cmd *exec.Cmd, limits Limits) (Result, error) {
	if limits.Stdout <= 0 || limits.Stderr <= 0 {
		return Result{}, errors.New("output cap is unset")
	}
	cmd.Stdin = bytes.NewReader(nil)
	cmd.WaitDelay = limits.Wait
	prepare(cmd)
	stdout := &CappedBuffer{max: limits.Stdout + 1}
	stderr := &CappedBuffer{max: limits.Stderr}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return Result{}, &TimeoutError{Stderr: stderr.Bytes()}
	}
	if stdout.Over() {
		return Result{}, &OutputError{Max: limits.Stdout, Stderr: stderr.Bytes()}
	}
	if runErr != nil {
		return Result{}, &ExitError{Stderr: stderr.Bytes(), Err: runErr}
	}
	return Result{Stdout: stdout.Bytes()}, nil
}

// CappedBuffer keeps at most max bytes and records when a write does not fit.
type CappedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

// Write implements io.Writer. A write past the cap is counted and discarded.
func (c *CappedBuffer) Write(p []byte) (int, error) {
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

// Bytes returns the bytes kept.
func (c *CappedBuffer) Bytes() []byte { return c.buf.Bytes() }

// Over reports whether a write did not fit.
func (c *CappedBuffer) Over() bool { return c.over }
