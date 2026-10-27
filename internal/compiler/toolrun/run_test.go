package toolrun

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExcerpt(t *testing.T) {
	if Excerpt(nil, "fallback") != "fallback" {
		t.Fatal("empty excerpt")
	}
	got := Excerpt([]byte("a\nb"), "fallback")
	if got != `a\u000ab` {
		t.Fatalf("excerpt %q", got)
	}
	long := Excerpt(bytes.Repeat([]byte("x"), 300), "fallback")
	if len(long) != 200 {
		t.Fatalf("len %d", len(long))
	}
}

func TestCappedBuffer(t *testing.T) {
	var buf CappedBuffer
	if _, err := buf.Write([]byte("x")); err == nil {
		t.Fatal("unset cap accepted a write")
	}
	buf.max = 4
	if _, err := buf.Write([]byte("abcdEF")); err != nil || !buf.Over() || string(buf.Bytes()) != "abcd" {
		t.Fatalf("over %v %q", buf.Over(), buf.Bytes())
	}
}

func TestRunBounds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake command is a shell script")
	}
	dir := t.TempDir()
	script := `#!/bin/sh
if [ -n "$FAKE_SLEEP" ]; then
  /bin/sleep "$FAKE_SLEEP"
fi
printf '%s' "$FAKE_STDOUT"
printf '%s' "$FAKE_STDERR" >&2
exit "${FAKE_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(dir, "fake-tool"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	t.Run("timeout", func(t *testing.T) {
		t.Setenv("FAKE_SLEEP", "30")
		t.Setenv("FAKE_STDOUT", "")
		t.Setenv("FAKE_STDERR", "")
		t.Setenv("FAKE_EXIT", "0")
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		cmd := exec.CommandContext(ctx, "fake-tool")
		_, err := Run(ctx, cmd, Limits{Stdout: 64, Stderr: 64, Wait: 200 * time.Millisecond})
		var timeout *TimeoutError
		if !errors.As(err, &timeout) {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("oversize", func(t *testing.T) {
		t.Setenv("FAKE_SLEEP", "")
		t.Setenv("FAKE_STDOUT", strings.Repeat("x", 64))
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "fake-tool")
		_, err := Run(ctx, cmd, Limits{Stdout: 16, Stderr: 64, Wait: time.Second})
		var over *OutputError
		if !errors.As(err, &over) || over.Max != 16 {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("exit", func(t *testing.T) {
		t.Setenv("FAKE_STDOUT", "")
		t.Setenv("FAKE_STDERR", "boom")
		t.Setenv("FAKE_EXIT", "2")
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "fake-tool")
		_, err := Run(ctx, cmd, Limits{Stdout: 64, Stderr: 64, Wait: time.Second})
		var exit *ExitError
		if !errors.As(err, &exit) || Excerpt(Stderr(err), "fallback") != "boom" {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("exit keeps stdout", func(t *testing.T) {
		t.Setenv("FAKE_STDOUT", "findings")
		t.Setenv("FAKE_STDERR", "")
		t.Setenv("FAKE_EXIT", "1")
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "fake-tool")
		_, err := Run(ctx, cmd, Limits{Stdout: 64, Stderr: 64, Wait: time.Second})
		var exit *ExitError
		if !errors.As(err, &exit) || string(exit.Stdout) != "findings" {
			t.Fatalf("err %v", err)
		}
	})
}

func TestMemoReuses(t *testing.T) {
	var calls int
	memo := Memo{Run: func(context.Context, []byte) ([]byte, error) {
		calls++
		return []byte("out"), nil
	}}
	ctx := context.Background()
	if _, err := memo.Do(ctx, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if _, err := memo.Do(ctx, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if _, err := memo.Do(ctx, []byte("other")); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls %d", calls)
	}
}

func TestMemoUnset(t *testing.T) {
	if _, err := (&Memo{}).Do(context.Background(), []byte("x")); err == nil {
		t.Fatal("unset run accepted")
	}
}

func TestParseSemver(t *testing.T) {
	major, minor, patch, ok := ParseSemver("1.182.1")
	if !ok || major != 1 || minor != 182 || patch != 1 {
		t.Fatalf("parsed %d.%d.%d ok %v", major, minor, patch, ok)
	}
	if _, _, _, ok := ParseSemver("v1.182.1"); ok {
		t.Fatal("accepted a prefix")
	}
}
