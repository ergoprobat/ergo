package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/edr"
)

func TestReadRef(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "policies"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "policies", "rule.txt"), []byte("yes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(root, "policies", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "policies", "rule.txt"), filepath.Join(root, "policies", "inside")); err != nil {
		t.Fatal(err)
	}
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	tests := []struct {
		name string
		ref  string
		code string
		want string
	}{
		{name: "file", ref: "policies/rule.txt", want: "yes"},
		{name: "missing", ref: "policies/missing.txt", code: CodeRefMissing},
		{name: "empty", ref: "", code: CodeRefRequired},
		{name: "parent", ref: "../secret", code: CodeRefOutside},
		{name: "absolute", ref: "/etc/passwd", code: CodeRefOutside},
		{name: "symlink escape", ref: "policies/escape", code: CodeRefOutside},
		{name: "symlink inside", ref: "policies/inside", code: CodeRefOutside},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := ReadRef(Input{Check: edr.Check{Ref: test.ref}, Repo: repo.FS()})
			if test.code != "" {
				if CodeOf(err) != test.code {
					t.Fatalf("code %q err %v", CodeOf(err), err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != test.want {
				t.Fatalf("data %q", data)
			}
		})
	}
}

func TestReadRefTooLarge(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "policies"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := make([]byte, MaxRefBytes+1)
	if err := os.WriteFile(filepath.Join(root, "policies", "big.rego"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	_, err = ReadRef(Input{Check: edr.Check{Ref: "policies/big.rego"}, Repo: repo.FS()})
	if CodeOf(err) != CodeRefTooLarge {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}

func TestReadRefNilRepo(t *testing.T) {
	_, err := ReadRef(Input{Check: edr.Check{Ref: "policies/rule.txt"}})
	if CodeOf(err) != CodeRefMissing {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}
