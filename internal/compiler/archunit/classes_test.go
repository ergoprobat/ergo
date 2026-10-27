package archunit

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler"
)

func relativeFiles(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(found)
	return found
}

func TestStageClassesCopiesOnlyClassFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root,
		"target/classes/com/acme/billing/domain/Order.class",
		"target/classes/com/acme/billing/web/OrderController.class",
		"target/classes/archunit.properties",
		"target/classes/archunit_ignore_patterns.txt",
		"target/classes/junit-platform.properties",
		"target/classes/META-INF/services/org.junit.platform.launcher.TestExecutionListener",
		"target/classes/com/acme/billing/Order.java",
		"target/classes/lib/dep.jar",
		"target/classes/Notes.class.txt",
		"src/Other.class",
	)
	dest := t.TempDir()
	tree, err := stageClasses(context.Background(), root, []string{"target/classes"}, dest)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"target/classes/com/acme/billing/domain/Order.class",
		"target/classes/com/acme/billing/web/OrderController.class",
	}
	if got := relativeFiles(t, dest); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("copied %v, want %v", got, want)
	}
	if len(tree.roots) != 1 || tree.roots[0] != filepath.Join(dest, "target", "classes") {
		t.Fatalf("roots %v", tree.roots)
	}
	if got := readText(t, filepath.Join(dest, "target/classes/com/acme/billing/domain/Order.class")); got != "x target/classes/com/acme/billing/domain/Order.class" {
		t.Fatalf("copy has %q", got)
	}
}

func TestStageClassesKeepsTheNamedPath(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "build/classes/java/main/a/A.class", "build/classes/java/test/a/ATest.class")
	dest := t.TempDir()
	tree, err := stageClasses(context.Background(), root, []string{"build/classes/java/main", "build/classes/java/test", "build/classes/java/main"}, dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.roots) != 2 {
		t.Fatalf("a repeated path is copied once: %v", tree.roots)
	}
	want := []string{"build/classes/java/main/a/A.class", "build/classes/java/test/a/ATest.class"}
	if got := relativeFiles(t, dest); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("copied %v", got)
	}
}

func TestStageClassesFromTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "target/classes/a/A.class", "README.md")
	dest := t.TempDir()
	tree, err := stageClasses(context.Background(), root, []string{"."}, dest)
	if err != nil {
		t.Fatal(err)
	}
	if got := relativeFiles(t, dest); strings.Join(got, "\n") != "target/classes/a/A.class" {
		t.Fatalf("copied %v", got)
	}
	if tree.hasClassesUnder([]string{"a"}) {
		t.Fatal("the classpath root is the repository, so package a is not at its top")
	}
}

func TestHasClassesUnder(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "classes/com/acme/billing/domain/Order.class", "classes/Top.class")
	tree, err := stageClasses(context.Background(), root, []string{"classes"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		packages []string
		want     bool
	}{
		"the package itself":     {[]string{"com.acme.billing.domain"}, true},
		"a parent package":       {[]string{"com.acme.billing"}, true},
		"one of several":         {[]string{"org.other", "com.acme"}, true},
		"a sibling":              {[]string{"com.acme.billing.web"}, false},
		"a prefix of a segment":  {[]string{"com.acme.bill"}, false},
		"a child that is absent": {[]string{"com.acme.billing.domain.model"}, false},
		"nothing":                {nil, false},
		"another root":           {[]string{"org"}, false},
	} {
		if got := tree.hasClassesUnder(c.packages); got != c.want {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

// A caller who names a relative symlink to a directory inside the repository gets that directory.
// An absolute symlink is refused, because os.Root refuses it. Symlinks below the named directory are refused.
func TestStageClassesNamedSymlink(t *testing.T) {
	skipSymlinks(t)
	root := t.TempDir()
	writeFiles(t, root, "real/a/A.class")
	if err := os.Symlink("real", filepath.Join(root, "relative")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "absolute")); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	if _, err := stageClasses(context.Background(), root, []string{"relative"}, dest); err != nil {
		t.Fatal(err)
	}
	if got := relativeFiles(t, dest); strings.Join(got, "\n") != "relative/a/A.class" {
		t.Fatalf("copied %v", got)
	}

	if _, err := stageClasses(context.Background(), root, []string{"absolute"}, t.TempDir()); compiler.CodeOf(err) != CodePath {
		t.Fatalf("an absolute symlink: %v", err)
	}

	if err := os.Symlink("..", filepath.Join(root, "real", "again")); err != nil {
		t.Fatal(err)
	}
	if _, err := stageClasses(context.Background(), root, []string{"relative"}, t.TempDir()); compiler.CodeOf(err) != CodePath {
		t.Fatalf("a symlink below the named directory: %v", err)
	}
}

func TestStageClassesRefusesBadPaths(t *testing.T) {
	skipSymlinks(t)
	root := t.TempDir()
	outside := t.TempDir()
	writeFiles(t, root, "ok/A.class", "file.txt", "tree/a/A.class", "inner/a/A.class")
	writeFiles(t, outside, "leak/Leak.class")
	for name, link := range map[string][2]string{
		"escape":      {"escape", outside},
		"inner":       {"tree/link", filepath.Join(root, "ok")},
		"file link":   {"inner/a/B.class", filepath.Join(root, "file.txt")},
		"escape file": {"inner/a/C.class", filepath.Join(outside, "leak", "Leak.class")},
	} {
		if err := os.Symlink(link[1], filepath.Join(root, filepath.FromSlash(link[0]))); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for name, path := range map[string]string{
		"missing":         "nope",
		"a file":          "file.txt",
		"symlink escape":  "escape",
		"inside the tree": "tree",
		"a class symlink": "inner",
		"below an escape": "escape/leak",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := stageClasses(context.Background(), root, []string{path}, t.TempDir())
			if compiler.CodeOf(err) != CodePath {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}

func TestStageClassesCap(t *testing.T) {
	saved := maxClassBytes
	maxClassBytes = 16
	t.Cleanup(func() { maxClassBytes = saved })
	root := t.TempDir()
	writeFiles(t, root, "c/a/A.class", "c/a/B.class")
	if err := os.WriteFile(filepath.Join(root, "c/a/A.class"), []byte("12345678"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "c/a/B.class"), []byte("12345678"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := stageClasses(context.Background(), root, []string{"c"}, t.TempDir()); err != nil {
		t.Fatalf("at the cap: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "c/a/B.class"), []byte("123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := stageClasses(context.Background(), root, []string{"c"}, t.TempDir())
	if compiler.CodeOf(err) != CodePath {
		t.Fatalf("over the cap: code %q err %v", compiler.CodeOf(err), err)
	}
}

func TestStageClassesStopsWhenTheContextEnds(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, "c/A.class")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := stageClasses(ctx, root, []string{"c"}, t.TempDir())
	if compiler.CodeOf(err) != CodeRun {
		t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
	}
}

func skipSymlinks(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks and fifos")
	}
}
