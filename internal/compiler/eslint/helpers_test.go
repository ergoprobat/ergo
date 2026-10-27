package eslint

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

var evalNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

const (
	floatingPromises = "@typescript-eslint/no-floating-promises"
	captureRoot      = "/repo"
)

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

func entryFor(target, mode string) compiler.Entry {
	return compiler.Entry{
		DecisionID:      "0ZCK57ASYTV9HMERY112N3ER0W",
		Slug:            "app-violating",
		Engine:          "eslint",
		Mode:            mode,
		Target:          target,
		ValidFrom:       "2026-01-15",
		Compiler:        "eslint",
		CompilerVersion: version,
	}
}

// capture is a real `eslint --format json` output from testdata/results, with the repository root of
// the capture replaced by root.
func capture(t *testing.T, name, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "results", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(raw), captureRoot, filepath.ToSlash(root))
}

func ruleSet(ids ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set
}

// goldenArtifact is the compiled config of check index of testdata/app-violating: 0 is typescript and 1 is javascript.
func goldenArtifact(t *testing.T, index string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "app-violating", "golden", index, artifactName))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// listing returns the sorted relative paths below dir.
func listing(t *testing.T, dir string) string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, _ fs.DirEntry, walkErr error) error {
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
