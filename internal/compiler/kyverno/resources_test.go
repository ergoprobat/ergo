package kyverno

import (
	"context"
	"errors"
	"fmt"
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

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var coded *compiler.Error
	if !errors.As(err, &coded) || coded.Code != code {
		t.Fatalf("err %v, want %s", err, code)
	}
}

// stage copies the named paths of root and returns the destination and the count.
func stage(t *testing.T, ctx context.Context, root string, named ...string) (string, int, error) {
	t.Helper()
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	dest := filepath.Join(t.TempDir(), "resources")
	count, err := stageManifests(ctx, repo, named, dest)
	return dest, count, err
}

// copied returns the body of each copied file, in file name order.
func copied(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for i, entry := range entries {
		if want := fmt.Sprintf("%06d.yaml", i+1); entry.Name() != want {
			t.Fatalf("file %d is %s, want %s", i+1, entry.Name(), want)
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, string(body))
	}
	return bodies
}

func listing(t *testing.T, dir string) string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, p)
		files = append(files, rel)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return strings.Join(files, "\n")
}

func TestStageCopiesOnlyManifestsInWalkOrder(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"k8s/pods/api.yaml":        "api",
		"k8s/pods/web.YML":         "web",
		"k8s/deep/er/x.yaml":       "x",
		"k8s/readme.md":            "docs",
		"k8s/values.json":          "{}",
		"k8s/pods/api.yaml.orig":   "backup",
		"k8s/.hidden.yaml":         "hidden file",
		"k8s/.git/config.yaml":     "hidden directory",
		"k8s/pods/.cache/zzz.yaml": "hidden below",
		"other/skipped.yaml":       "not named",
	})
	dest, count, err := stage(t, context.Background(), root, "k8s")
	if err != nil || count != 3 {
		t.Fatalf("count %d err %v", count, err)
	}
	if got := strings.Join(copied(t, dest), " "); got != "x api web" {
		t.Fatalf("copied %q", got)
	}
	info, err := os.Stat(filepath.Join(dest, "000001.yaml"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
	dirInfo, err := os.Stat(dest)
	if err != nil || dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v %v", dirInfo, err)
	}
}

func TestStageSkipsHiddenDirectoriesOfTheRepositoryRoot(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"deploy.yaml":                  "deploy",
		".ipso/build/x/0/kyverno.yaml": "build output",
		".github/workflows/ci.yml":     "workflow",
	})
	dest, count, err := stage(t, context.Background(), root, ".")
	if err != nil || count != 1 || strings.Join(copied(t, dest), " ") != "deploy" {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestStageAcceptsANamedHiddenPathAndANamedFile(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		".github/policies/a.yaml": "a",
		"pod.yml":                 "pod",
		"notes.txt":               "notes",
	})
	dest, count, err := stage(t, context.Background(), root, ".github/policies", "pod.yml")
	if err != nil || count != 2 || strings.Join(copied(t, dest), " ") != "a pod" {
		t.Fatalf("count %d err %v", count, err)
	}
	_, _, err = stage(t, context.Background(), root, "notes.txt")
	requireCode(t, err, CodePath)
}

// A named file follows the same case rule as a file found in a directory.
func TestStageTakesANamedFileInAnyCase(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"manifests/Pod.YAML": "pod", "manifests/Other.Yml": "other"})
	dest, count, err := stage(t, context.Background(), root, "manifests/Pod.YAML", "manifests/Other.Yml")
	if err != nil || count != 2 || strings.Join(copied(t, dest), " ") != "pod other" {
		t.Fatalf("count %d err %v", count, err)
	}
	if _, count, err := stage(t, context.Background(), root, "manifests"); err != nil || count != 2 {
		t.Fatalf("directory: count %d err %v", count, err)
	}
}

func TestStageKeepsArgumentOrderAndCopiesAFileOnce(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"a/one.yaml": "a1", "b/two.yaml": "b2", "b/sub/three.yaml": "b3"})
	dest, count, err := stage(t, context.Background(), root, "b", "a", "b/sub", "a/one.yaml", "b")
	if err != nil || count != 3 || strings.Join(copied(t, dest), " ") != "b3 b2 a1" {
		t.Fatalf("count %d err %v bodies %v", count, err, copied(t, dest))
	}
}

func TestStageNeedsAManifest(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"docs/readme.md":      "docs",
		"hidden/.a.yaml":      "hidden",
		"hidden/.dir/b.yaml":  "hidden directory",
		"empty/.gitkeep":      "",
		"mixed/readme.md":     "docs",
		"mixed/deep/pod.yaml": "pod",
	})
	for _, named := range []string{"docs", "hidden", "empty"} {
		_, _, err := stage(t, context.Background(), root, named)
		requireCode(t, err, CodeEmpty)
	}
	if _, count, err := stage(t, context.Background(), root, "docs", "mixed"); err != nil || count != 1 {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestStageRefusesAPathItCannotRead(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod"})
	_, _, err := stage(t, context.Background(), root, "nope")
	requireCode(t, err, CodePath)
	_, _, err = stage(t, context.Background(), root, "k8s", "k8s/missing.yaml")
	requireCode(t, err, CodePath)
}

func TestStageRefusesManifestsOverTheCap(t *testing.T) {
	old := maxResourceBytes
	maxResourceBytes = 10
	t.Cleanup(func() { maxResourceBytes = old })
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/a.yaml": "123456", "k8s/b.yaml": "123456"})
	_, _, err := stage(t, context.Background(), root, "k8s")
	requireCode(t, err, CodePath)
	if _, count, err := stage(t, context.Background(), root, "k8s/a.yaml"); err != nil || count != 1 {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestStageStopsWhenTheContextEnds(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := stage(t, ctx, root, "k8s")
	requireCode(t, err, CodeRun)
}

func TestIsManifest(t *testing.T) {
	for name, want := range map[string]bool{
		"a.yaml": true, "a.yml": true, "A.YAML": true, "a.Yml": true, "a.json": false,
		"a.yaml.orig": false, "yaml": false, ".yaml": true, "a.yaml.fixture": false, "": false,
	} {
		if got := isManifest(name); got != want {
			t.Errorf("isManifest(%q) = %v", name, got)
		}
	}
}
