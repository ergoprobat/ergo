//go:build unix

package importlinter

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStageRefusesSymlinks(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"evil.py": "raise SystemExit"})
	cases := map[string]func(root string) error{
		"link below the package": func(root string) error {
			return os.Symlink(outside, filepath.Join(root, "shop", "link"))
		},
		"link to a file named like a module": func(root string) error {
			return os.Symlink(filepath.Join(outside, "evil.py"), filepath.Join(root, "shop", "evil.py"))
		},
		"link with another name": func(root string) error {
			return os.Symlink(filepath.Join(outside, "evil.py"), filepath.Join(root, "shop", "notes.txt"))
		},
		"link to a sibling inside the repository": func(root string) error {
			return os.Symlink("api", filepath.Join(root, "shop", "alias"))
		},
	}
	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"shop/__init__.py": "", "shop/api/__init__.py": ""})
			if err := link(root); err != nil {
				t.Skipf("no symlinks: %v", err)
			}
			tree := newTree(t, root, ".")
			requireCode(t, tree.stage(context.Background(), "shop"), CodePath)
			if _, err := os.Stat(filepath.Join(tree.dir, "shop")); err == nil {
				t.Fatal("a failed copy was left behind")
			}
		})
	}
}

func TestStageRefusesALinkedRootAndALinkedInit(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"real/__init__.py": "", "linkedinit/real.py": "", "linkedinit/other.py": ""})
	if err := os.Symlink("real", filepath.Join(root, "shop")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := os.Symlink("other.py", filepath.Join(root, "linkedinit", "__init__.py")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	requireCode(t, newTree(t, root, ".").stage(ctx, "shop"), CodePath)
	requireCode(t, newTree(t, root, ".").stage(ctx, "linkedinit"), CodePath)
}

func TestStageRefusesAFileThatIsNotRegular(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"shop/__init__.py": ""})
	if err := syscall.Mkfifo(filepath.Join(root, "shop", "pipe.py"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	requireCode(t, newTree(t, root, ".").stage(context.Background(), "shop"), CodePath)

	// A file that is not a module is never opened, so it cannot block the copy.
	other := t.TempDir()
	writeTree(t, other, map[string]string{"shop/__init__.py": ""})
	if err := syscall.Mkfifo(filepath.Join(other, "shop", "pipe.txt"), 0o600); err != nil {
		t.Skipf("no fifo: %v", err)
	}
	if err := newTree(t, other, ".").stage(context.Background(), "shop"); err != nil {
		t.Fatal(err)
	}
}
