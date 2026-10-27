//go:build unix

package kyverno

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStageRefusesSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"evil.yaml": "evil"})
	cases := map[string]func(root string) error{
		"link to a manifest named like one": func(root string) error {
			return os.Symlink(filepath.Join(outside, "evil.yaml"), filepath.Join(root, "k8s", "evil.yaml"))
		},
		"link to a directory": func(root string) error {
			return os.Symlink(outside, filepath.Join(root, "k8s", "linked"))
		},
		"link with another name": func(root string) error {
			return os.Symlink(filepath.Join(outside, "evil.yaml"), filepath.Join(root, "k8s", "notes.txt"))
		},
		"link to a sibling inside the repository": func(root string) error {
			return os.Symlink("deep", filepath.Join(root, "k8s", "alias"))
		},
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod", "k8s/deep/x.yaml": "x"})
			if err := link(root); err != nil {
				t.Skipf("no symlinks: %v", err)
			}
			_, _, err := stage(t, context.Background(), root, "k8s")
			requireCode(t, err, CodePath)
		})
	}
}

// A named path is the caller's choice, so a symlink in the name itself is followed when it stays inside
// the repository. A symlink below a named directory is refused, and Setup refuses one that leaves it.
func TestStageFollowsANamedSymlinkInsideTheRepository(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"real/pod.yaml": "pod", "single.yaml": "single"})
	if err := os.Symlink("real", filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := os.Symlink("single.yaml", filepath.Join(root, "linkfile.yaml")); err != nil {
		t.Fatal(err)
	}
	dest, count, err := stage(t, context.Background(), root, "linkdir", "linkfile.yaml")
	if err != nil || count != 2 || len(copied(t, dest)) != 2 {
		t.Fatalf("count %d err %v", count, err)
	}
}

// A hidden entry is skipped before anything else is asked of it, so a symlink in a hidden directory,
// such as node_modules/.bin, does not stop a run.
func TestStageIgnoresAHiddenSymlink(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod", "k8s/.bin/real": "x"})
	if err := os.Symlink("real", filepath.Join(root, "k8s", ".bin", "alias")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "k8s", ".linked")); err != nil {
		t.Fatal(err)
	}
	if _, count, err := stage(t, context.Background(), root, "k8s"); err != nil || count != 1 {
		t.Fatalf("count %d err %v", count, err)
	}
}

func TestStageRefusesAFileThatIsNotRegular(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod"})
	if err := syscall.Mkfifo(filepath.Join(root, "k8s", "pipe.yaml"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	_, _, err := stage(t, context.Background(), root, "k8s")
	requireCode(t, err, CodePath)
	_, _, err = stage(t, context.Background(), root, "k8s/pipe.yaml")
	requireCode(t, err, CodePath)

	// A file that is not a manifest is never opened, so it cannot block the copy.
	other := t.TempDir()
	writeTree(t, other, map[string]string{"k8s/pod.yaml": "pod"})
	if err := syscall.Mkfifo(filepath.Join(other, "k8s", "pipe.txt"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	if _, count, err := stage(t, context.Background(), other, "k8s"); err != nil || count != 1 {
		t.Fatalf("count %d err %v", count, err)
	}
}
