package semgrep

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestFindToolMissing(t *testing.T) {
	skipFakeSemgrep(t)
	t.Setenv("PATH", t.TempDir())
	_, err := FindTool(context.Background())
	if compiler.CodeOf(err) != CodeMissing {
		t.Fatalf("code %s err %v", compiler.CodeOf(err), err)
	}
}

func TestFindToolVersion(t *testing.T) {
	skipFakeSemgrep(t)
	installFakeSemgrep(t)

	t.Setenv("FAKE_SEMGREP_VERSION", "nope")
	_, err := FindTool(context.Background())
	if compiler.CodeOf(err) != CodeVersion {
		t.Fatalf("bad version code %s err %v", compiler.CodeOf(err), err)
	}

	t.Setenv("FAKE_SEMGREP_VERSION", "1.182.1")
	tool, err := FindTool(context.Background())
	if err != nil || tool.Version != "1.182.1" {
		t.Fatalf("tool %+v err %v", tool, err)
	}
}

func TestFakeScan(t *testing.T) {
	skipFakeSemgrep(t)
	installFakeSemgrep(t)
	root := t.TempDir()
	ctx := context.Background()

	t.Run("flags", func(t *testing.T) {
		argsPath := filepath.Join(t.TempDir(), "args")
		envPath := filepath.Join(t.TempDir(), "env")
		copyPath := filepath.Join(t.TempDir(), "copied.yaml")
		t.Setenv("FAKE_SEMGREP_ARGS", argsPath)
		t.Setenv("FAKE_SEMGREP_ENV", envPath)
		t.Setenv("FAKE_SEMGREP_COPY", copyPath)
		t.Setenv("FAKE_SEMGREP_STDOUT", `{"version":"1.182.1","results":[],"errors":[],"paths":{"scanned":["a.py"]}}`)
		t.Setenv("SEMGREP_BASELINE_COMMIT", "abc")
		t.Setenv("SEMGREP_APP_TOKEN", "secret")
		config := []byte("rules: []\n")
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tool.Scan(ctx, root, config, []string{"src"}); err != nil {
			t.Fatal(err)
		}
		args := readText(t, argsPath)
		for _, flag := range []string{
			"scan",
			"--json",
			"--metrics=off",
			"--disable-version-check",
			"--oss-only",
			"--no-rewrite-rule-ids",
			"--disable-nosem",
			"--x-ignore-semgrepignore-files",
			"--max-target-bytes=0",
			"src",
		} {
			if !strings.Contains(args, flag+"\n") && !strings.HasSuffix(strings.TrimRight(args, "\n"), flag) {
				t.Fatalf("args missing %s:\n%s", flag, args)
			}
		}
		got, err := os.ReadFile(copyPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(config) {
			t.Fatalf("config %s", got)
		}
		used := strings.TrimSpace(readText(t, copyPath+".path"))
		rootClean := filepath.Clean(root)
		if !filepath.IsAbs(used) || used == rootClean || strings.HasPrefix(used, rootClean+string(os.PathSeparator)) {
			t.Fatalf("config path %s is inside %s", used, root)
		}
		envText := readText(t, envPath)
		if !strings.Contains(envText, "metrics=off\n") || !strings.Contains(envText, "version_check=0\n") {
			t.Fatalf("env %s", envText)
		}
		if strings.Contains(envText, "abc") || strings.Contains(envText, "secret") {
			t.Fatalf("env kept a SEMGREP variable: %s", envText)
		}
	})

	t.Run("exit", func(t *testing.T) {
		t.Setenv("FAKE_SEMGREP_EXIT", "2")
		t.Setenv("FAKE_SEMGREP_STDERR", "boom")
		t.Setenv("FAKE_SEMGREP_STDOUT", `{"version":"1.182.1"}`)
		_, err := scanFake(t, ctx, root)
		if compiler.CodeOf(err) != CodeRun || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Setenv("FAKE_SEMGREP_EXIT", "0")
		t.Setenv("FAKE_SEMGREP_SLEEP", "30")
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tool.timeout = 500 * time.Millisecond
		tool.wait = 200 * time.Millisecond
		_, err = tool.Scan(ctx, root, []byte("rules: []\n"), []string{"src"})
		if compiler.CodeOf(err) != CodeRun || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("oversize", func(t *testing.T) {
		t.Setenv("FAKE_SEMGREP_EXIT", "0")
		t.Setenv("FAKE_SEMGREP_SLEEP", "")
		t.Setenv("FAKE_SEMGREP_STDOUT", strings.Repeat("x", 64))
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tool.outputLimit = 16
		_, err = tool.Scan(ctx, root, []byte("rules: []\n"), []string{"src"})
		if compiler.CodeOf(err) != CodeOutput {
			t.Fatalf("err %v", err)
		}
	})
}

func TestMemoReusesScan(t *testing.T) {
	skipFakeSemgrep(t)
	installFakeSemgrep(t)
	count := filepath.Join(t.TempDir(), "count")
	t.Setenv("FAKE_SEMGREP_COUNT", count)
	t.Setenv("FAKE_SEMGREP_STDOUT", `{"version":"1.182.1"}`)
	tool, err := FindTool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	memo := Memo{Tool: tool, Root: t.TempDir(), Paths: []string{"src"}}
	ctx := context.Background()
	if _, err := memo.Scan(ctx, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if _, err := memo.Scan(ctx, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(readText(t, count), "x"); got != 1 {
		t.Fatalf("scans %d", got)
	}
	if _, err := memo.Scan(ctx, []byte("other")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(readText(t, count), "x"); got != 2 {
		t.Fatalf("scans %d", got)
	}
}

func TestScanRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	_, err := (Tool{}).Scan(context.Background(), root, []byte("rules: []\n"), []string{"escape"})
	if compiler.CodeOf(err) != CodePath {
		t.Fatalf("escape %v", err)
	}
}

func scanFake(t *testing.T, ctx context.Context, root string) ([]byte, error) {
	t.Helper()
	tool, err := FindTool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Scan(ctx, root, []byte("rules: []\n"), []string{"src"})
}

func installFakeSemgrep(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' "${FAKE_SEMGREP_VERSION:-1.182.1}"
  exit 0
fi
if [ -n "${FAKE_SEMGREP_SLEEP:-}" ]; then
  /bin/sleep "$FAKE_SEMGREP_SLEEP"
fi
if [ -n "${FAKE_SEMGREP_ARGS:-}" ]; then
  printf '%s\n' "$@" > "$FAKE_SEMGREP_ARGS"
fi
if [ -n "${FAKE_SEMGREP_ENV:-}" ]; then
  {
    printf 'metrics=%s\n' "$SEMGREP_SEND_METRICS"
    printf 'version_check=%s\n' "$SEMGREP_ENABLE_VERSION_CHECK"
    printf 'baseline=%s\n' "$SEMGREP_BASELINE_COMMIT"
    printf 'token=%s\n' "$SEMGREP_APP_TOKEN"
  } > "$FAKE_SEMGREP_ENV"
fi
prev=""
cfg=""
for arg in "$@"; do
  if [ "$prev" = "--config" ]; then
    cfg=$arg
  fi
  prev=$arg
done
if [ -n "${FAKE_SEMGREP_COPY:-}" ] && [ -n "$cfg" ]; then
  /bin/cp "$cfg" "$FAKE_SEMGREP_COPY"
  printf '%s' "$cfg" > "${FAKE_SEMGREP_COPY}.path"
fi
if [ "${FAKE_SEMGREP_EXIT:-0}" != "0" ]; then
  printf '%s' "${FAKE_SEMGREP_STDERR:-failed}" >&2
  exit "$FAKE_SEMGREP_EXIT"
fi
if [ -n "${FAKE_SEMGREP_COUNT:-}" ]; then
  printf 'x\n' >> "$FAKE_SEMGREP_COUNT"
fi
printf '%s' "${FAKE_SEMGREP_STDOUT:-}"
`
	if err := os.WriteFile(filepath.Join(dir, "semgrep"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_SEMGREP_VERSION", "1.182.1")
	t.Setenv("FAKE_SEMGREP_EXIT", "0")
	t.Setenv("FAKE_SEMGREP_SLEEP", "")
	t.Setenv("FAKE_SEMGREP_STDERR", "")
	t.Setenv("FAKE_SEMGREP_STDOUT", "")
	t.Setenv("FAKE_SEMGREP_COPY", "")
	t.Setenv("FAKE_SEMGREP_ARGS", "")
	t.Setenv("FAKE_SEMGREP_ENV", "")
	t.Setenv("FAKE_SEMGREP_COUNT", "")
	t.Setenv("SEMGREP_BASELINE_COMMIT", "")
	t.Setenv("SEMGREP_APP_TOKEN", "")
}

func skipFakeSemgrep(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake semgrep is a shell script")
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
