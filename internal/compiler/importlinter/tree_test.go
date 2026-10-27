package importlinter

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		target := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func newTree(t *testing.T, root string, named ...string) *sourceTree {
	t.Helper()
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return &sourceTree{repo: repo, named: named, dir: t.TempDir(), staged: map[string]bool{}}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var coded *compiler.Error
	if !errors.As(err, &coded) || coded.Code != code {
		t.Fatalf("err %v, want %s", err, code)
	}
}

func copied(t *testing.T, dir string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(dir, p)
		files = append(files, filepath.ToSlash(rel))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestStageCopiesOnlyTheFilesGrimpReads(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"shop/__init__.py":               "",
		"shop/api/__init__.py":           "import shop.domain\n",
		"shop/api/views.py":              "",
		"shop/api/views.py.orig":         "not a module",
		"shop/api/.hidden.py":            "hidden",
		"shop/api/some.module.py":        "too many dots",
		"shop/api/README.md":             "docs",
		"shop/api/data.json":             "{}",
		"shop/__pycache__/x.cpython.pyc": "bytecode",
		"shop/api/sub/mod.py":            "",
		"other/__init__.py":              "not named",
		"setup.py":                       "outside the package",
	})
	tree := newTree(t, root, ".")
	if err := tree.stage(context.Background(), "shop"); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(copied(t, tree.dir), " ")
	want := "shop/__init__.py shop/api/__init__.py shop/api/sub/mod.py shop/api/views.py"
	if got != want {
		t.Fatalf("copied %s, want %s", got, want)
	}
	info, err := os.Stat(filepath.Join(tree.dir, "shop", "api", "views.py"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
	dirInfo, err := os.Stat(filepath.Join(tree.dir, "shop", "api"))
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v %v", dirInfo, err)
	}
}

func TestStageFindsTheRootInANamedDirectory(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"src/shop/__init__.py": "", "src/shop/a.py": ""})
	tree := newTree(t, root, "src")
	if err := tree.stage(context.Background(), "shop"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(copied(t, tree.dir), " "); got != "shop/__init__.py shop/a.py" {
		t.Fatalf("copied %s", got)
	}
}

func TestStageNeedsExactlyOneHome(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"a/shop/__init__.py":  "",
		"b/shop/__init__.py":  "",
		"c/shop/mod.py":       "no __init__.py",
		"d/shop":              "a file, not a package",
		"e/other/__init__.py": "",
	})
	ctx := context.Background()

	requireCode(t, newTree(t, root, "a", "b").stage(ctx, "shop"), CodePath)
	requireCode(t, newTree(t, root, "c").stage(ctx, "shop"), CodeEmpty)
	requireCode(t, newTree(t, root, "d").stage(ctx, "shop"), CodeEmpty)
	requireCode(t, newTree(t, root, "e").stage(ctx, "shop"), CodeEmpty)
	if err := newTree(t, root, "a", "c", "d", "e").stage(ctx, "shop"); err != nil {
		t.Fatalf("one home among several directories: %v", err)
	}
}

func TestStageCopiesOnceAndKeepsTheFirstCopy(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"shop/__init__.py": "first"})
	tree := newTree(t, root, ".")
	ctx := context.Background()
	if err := tree.stage(ctx, "shop"); err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{"shop/__init__.py": "second", "shop/late.py": ""})
	if err := tree.stage(ctx, "shop"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(tree.dir, "shop", "__init__.py"))
	if err != nil || string(body) != "first" || len(copied(t, tree.dir)) != 1 {
		t.Fatalf("body %q err %v files %v", body, err, copied(t, tree.dir))
	}
}

func TestStageRefusesPythonSourceOverTheCap(t *testing.T) {
	old := maxSourceBytes
	maxSourceBytes = 10
	t.Cleanup(func() { maxSourceBytes = old })
	root := t.TempDir()
	writeTree(t, root, map[string]string{"shop/__init__.py": "12345", "shop/big.py": "1234567890"})
	tree := newTree(t, root, ".")
	requireCode(t, tree.stage(context.Background(), "shop"), CodePath)
	if _, err := os.Stat(filepath.Join(tree.dir, "shop")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a failed copy was left behind: %v", err)
	}
	if tree.total != 0 {
		t.Fatalf("total %d after a failed copy", tree.total)
	}
}

func TestStageStopsWhenTheContextEnds(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"shop/__init__.py": ""})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requireCode(t, newTree(t, root, ".").stage(ctx, "shop"), CodeRun)
}
