package toolrun

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
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

func TestWithinRoot(t *testing.T) {
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
	err := WithinRoot(root, []string{"escape"})
	var escape *EscapeError
	if !errors.As(err, &escape) || escape.Path != "escape" {
		t.Fatalf("escape %v", err)
	}
	if err := WithinRoot(root, []string{"inside", "missing", "."}); err != nil {
		t.Fatal(err)
	}
}
