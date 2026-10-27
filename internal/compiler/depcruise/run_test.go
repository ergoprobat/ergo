package depcruise

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
)

func TestNormalizePath(t *testing.T) {
	ok := []struct {
		in, want string
	}{
		{".", "."},
		{"packages", "packages"},
		{"./packages", "packages"},
		{"packages/ui", "packages/ui"},
		{"foo/./bar", "foo/bar"},
		{"src/foo-bar", "src/foo-bar"},
		{"lib/@scope/pkg", "lib/@scope/pkg"},
		{"a+b", "a+b"},
	}
	for _, tc := range ok {
		got, err := NormalizePath(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("NormalizePath(%q) = %q, %v", tc.in, got, err)
		}
	}
	bad := []string{"", "..", "../x", "/abs", `src\x`, "src/*", "foo//bar", "packages/", "--config", "foo/../../etc", "a b", "src/../x"}
	for _, in := range bad {
		if _, err := NormalizePath(in); err == nil {
			t.Fatalf("NormalizePath(%q) accepted", in)
		}
	}
	if _, err := NormalizePaths(nil); err == nil {
		t.Fatal("empty path list accepted")
	}
}

func TestFindToolMissing(t *testing.T) {
	skipFakeCruise(t)
	t.Setenv("PATH", t.TempDir())
	_, err := FindTool(context.Background())
	if compiler.CodeOf(err) != CodeMissing {
		t.Fatalf("code %s err %v", compiler.CodeOf(err), err)
	}
}

func TestFindToolVersion(t *testing.T) {
	skipFakeCruise(t)
	installFakeDepcruise(t)

	t.Setenv("FAKE_DEPCRUISE_VERSION", "nope")
	_, err := FindTool(context.Background())
	if compiler.CodeOf(err) != CodeVersion {
		t.Fatalf("bad version code %s err %v", compiler.CodeOf(err), err)
	}

	t.Setenv("FAKE_DEPCRUISE_VERSION", "18.8.0")
	tool, err := FindTool(context.Background())
	if err != nil || tool.Version != "18.8.0" {
		t.Fatalf("tool %+v err %v", tool, err)
	}
}

func TestFakeCruise(t *testing.T) {
	skipFakeCruise(t)
	installFakeDepcruise(t)
	root := t.TempDir()
	ctx := context.Background()

	t.Run("exit", func(t *testing.T) {
		t.Setenv("FAKE_DEPCRUISE_EXIT", "2")
		t.Setenv("FAKE_DEPCRUISE_STDERR", "boom")
		t.Setenv("FAKE_DEPCRUISE_STDOUT", `{"summary":{"violations":[],"totalCruised":1,"environment":{"version":"18.7.0"}}}`)
		_, err := cruiseFake(t, ctx, root)
		if compiler.CodeOf(err) != CodeRun || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		t.Setenv("FAKE_DEPCRUISE_EXIT", "0")
		t.Setenv("FAKE_DEPCRUISE_SLEEP", "30")
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tool.timeout = 500 * time.Millisecond
		tool.wait = 200 * time.Millisecond
		_, err = tool.Cruise(ctx, root, []byte("{}"), []string{"packages"})
		if compiler.CodeOf(err) != CodeRun || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("oversize", func(t *testing.T) {
		t.Setenv("FAKE_DEPCRUISE_EXIT", "0")
		t.Setenv("FAKE_DEPCRUISE_SLEEP", "")
		t.Setenv("FAKE_DEPCRUISE_STDOUT", strings.Repeat("x", 64))
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		tool.outputLimit = 16
		_, err = tool.Cruise(ctx, root, []byte("{}"), []string{"packages"})
		if compiler.CodeOf(err) != CodeOutput {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("bad json", func(t *testing.T) {
		t.Setenv("FAKE_DEPCRUISE_STDOUT", "not-json")
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		out, err := tool.Cruise(ctx, root, []byte("{}"), []string{"packages"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = ParseResult(out, tool.Version)
		if compiler.CodeOf(err) != CodeOutput {
			t.Fatalf("err %v", err)
		}
	})

	t.Run("config copy", func(t *testing.T) {
		copyPath := filepath.Join(t.TempDir(), "copied.json")
		t.Setenv("FAKE_DEPCRUISE_COPY", copyPath)
		t.Setenv("FAKE_DEPCRUISE_STDOUT", `{"summary":{"violations":[],"totalCruised":1,"environment":{"version":"18.7.0"}}}`)
		t.Setenv("FAKE_DEPCRUISE_VERSION", "18.7.0")
		config := []byte(`{"forbidden":[]}`)
		tool, err := FindTool(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tool.Cruise(ctx, root, config, []string{"packages"}); err != nil {
			t.Fatal(err)
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
	})
}

func TestMemoReusesCruise(t *testing.T) {
	skipFakeCruise(t)
	installFakeDepcruise(t)
	count := filepath.Join(t.TempDir(), "count")
	t.Setenv("FAKE_DEPCRUISE_COUNT", count)
	t.Setenv("FAKE_DEPCRUISE_STDOUT", `{"summary":{"violations":[],"totalCruised":1,"environment":{"version":"18.7.0"}}}`)
	tool, err := FindTool(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	memo := Memo{Tool: tool, Root: t.TempDir(), Paths: []string{"packages"}}
	ctx := context.Background()
	if _, err := memo.Cruise(ctx, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if _, err := memo.Cruise(ctx, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(readText(t, count), "x"); got != 1 {
		t.Fatalf("cruises %d", got)
	}
	if _, err := memo.Cruise(ctx, []byte("other")); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(readText(t, count), "x"); got != 2 {
		t.Fatalf("cruises %d", got)
	}
}

func TestCruiseRejectsEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub", filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	_, err := (Tool{}).Cruise(context.Background(), root, []byte("{}"), []string{"escape"})
	if compiler.CodeOf(err) != CodePath {
		t.Fatalf("escape %v", err)
	}
	if err := withinRoot(root, []string{"inside", "missing"}); err != nil {
		t.Fatal(err)
	}
}

func TestCruiseRejectsFlagPath(t *testing.T) {
	_, err := (Tool{Version: "18.7.0"}).Cruise(context.Background(), t.TempDir(), []byte("{}"), []string{"--config"})
	if compiler.CodeOf(err) != CodeRun {
		t.Fatalf("err %v", err)
	}
}

func cruiseFake(t *testing.T, ctx context.Context, root string) ([]byte, error) {
	t.Helper()
	tool, err := FindTool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return tool.Cruise(ctx, root, []byte("{}"), []string{"packages"})
}

func installFakeDepcruise(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' "${FAKE_DEPCRUISE_VERSION:-18.7.0}"
  exit 0
fi
if [ -n "${FAKE_DEPCRUISE_SLEEP:-}" ]; then
  /bin/sleep "$FAKE_DEPCRUISE_SLEEP"
fi
prev=""
cfg=""
for arg in "$@"; do
  if [ "$prev" = "--config" ]; then
    cfg=$arg
  fi
  prev=$arg
done
if [ -n "${FAKE_DEPCRUISE_COPY:-}" ] && [ -n "$cfg" ]; then
  /bin/cp "$cfg" "$FAKE_DEPCRUISE_COPY"
  printf '%s' "$cfg" > "${FAKE_DEPCRUISE_COPY}.path"
fi
if [ "${FAKE_DEPCRUISE_EXIT:-0}" != "0" ]; then
  printf '%s' "${FAKE_DEPCRUISE_STDERR:-failed}" >&2
  exit "$FAKE_DEPCRUISE_EXIT"
fi
if [ -n "${FAKE_DEPCRUISE_COUNT:-}" ]; then
  printf 'x\n' >> "$FAKE_DEPCRUISE_COUNT"
fi
printf '%s' "${FAKE_DEPCRUISE_STDOUT:-}"
`
	if err := os.WriteFile(filepath.Join(dir, "depcruise"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_DEPCRUISE_VERSION", "18.7.0")
	t.Setenv("FAKE_DEPCRUISE_EXIT", "0")
	t.Setenv("FAKE_DEPCRUISE_SLEEP", "")
	t.Setenv("FAKE_DEPCRUISE_STDERR", "")
	t.Setenv("FAKE_DEPCRUISE_STDOUT", "")
	t.Setenv("FAKE_DEPCRUISE_COPY", "")
}

func skipFakeCruise(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake depcruise is a shell script")
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
