package gatekeeper

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/opa"
)

var evalNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

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

func requireOutcomeCode(t *testing.T, out compiler.Outcome, code string) {
	t.Helper()
	requireCode(t, out.Err, code)
	if len(out.Violations) != 0 || out.Waived {
		t.Fatalf("outcome %+v", out)
	}
}

// listing returns the sorted relative paths below dir.
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

// golden returns the artifacts compile wrote for an opa fixture: the owner label policy in one mode.
func golden(t *testing.T, fixture string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, name := range []string{artifactTemplate, artifactConstraint} {
		raw, err := os.ReadFile(filepath.Join("..", "opa", "testdata", fixture, "golden", "0", name))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = raw
	}
	return out
}

var fixtureModes = map[string]string{
	"gatekeeper-clean": "block",
	"gk-mode-warn":     "warn",
	"gk-mode-advise":   "advise",
}

func entryFor(mode string) compiler.Entry {
	return compiler.Entry{
		DecisionID:      "0P7F1J5VGY38SNDK78KXAV1357",
		Slug:            "owner",
		Engine:          "opa",
		Target:          "gatekeeper",
		Mode:            mode,
		CompilerVersion: "1.21.4",
		ValidFrom:       "2026-01-01",
	}
}

// ownerID is the constraint the owner label fixtures compile to, for an enforcement action.
func ownerID(action string) opa.Gatekeeper {
	return opa.Gatekeeper{Kind: "RequireOwner", Name: "owner", Action: action}
}

func deployment(name string, owned bool) string {
	text := "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: " + name + "\n"
	if owned {
		text += "  labels:\n    owner: team\n"
	}
	return text + "spec:\n  replicas: 1\n"
}
