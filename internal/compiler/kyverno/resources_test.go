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

// The copy rules are tested in internal/compiler/manifests. These tests pin the reason code each kind
// of failure gets for Kyverno.
func TestStageCopiesTheManifestsOfTheNamedPaths(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pods/api.yaml": "api", "k8s/readme.md": "docs"})
	dest, count, err := stage(t, context.Background(), root, "k8s")
	if err != nil || count != 1 || strings.Join(copied(t, dest), " ") != "api" {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestStageMapsEachFailureToItsCode(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod", "docs/readme.md": "docs"})
	_, _, err := stage(t, context.Background(), root, "nope")
	requireCode(t, err, CodePath)
	_, _, err = stage(t, context.Background(), root, "docs")
	requireCode(t, err, CodeEmpty)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = stage(t, ctx, root, "k8s")
	requireCode(t, err, CodeRun)
}
