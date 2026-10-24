// Package compilertest runs golden fixtures for one compiler.
// A case is a directory under testdata/: record.md, repo/, and either golden/ or expect.
// Rewrite golden files with go test ./internal/compiler/compilertest -update.
package compilertest

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/edr"
)

var update = flag.Bool("update", false, "rewrite golden compiler fixtures")

// Run compiles every testdata case whose checks use c.
func Run(t *testing.T, c compiler.Compiler) {
	t.Helper()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("testdata has no cases")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			runCase(t, c, filepath.Join("testdata", entry.Name()))
		})
	}
}

func runCase(t *testing.T, c compiler.Compiler, dir string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "record.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := edr.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if problems := edr.Validate(doc); len(problems) != 0 {
		t.Fatalf("record: %s", strings.Join(problems, "; "))
	}
	rec, err := edr.RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := os.OpenRoot(filepath.Join(dir, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	first, err := compileMatching(c, rec, repo.FS())
	second, secondErr := compileMatching(c, rec, repo.FS())
	if compiler.CodeOf(err) != compiler.CodeOf(secondErr) || (err == nil) != (secondErr == nil) {
		t.Fatalf("compile was not deterministic: %v vs %v", err, secondErr)
	}
	if err == nil && !sameCompiled(first, second) {
		t.Fatal("artifact bytes were not deterministic")
	}

	expectRaw, expectErr := os.ReadFile(filepath.Join(dir, "expect"))
	if expectErr == nil {
		want := strings.TrimSpace(string(expectRaw))
		if err == nil {
			t.Fatalf("expected %s, compile succeeded", want)
		}
		if compiler.CodeOf(err) != want {
			t.Fatalf("reason %q, want %q (%v)", compiler.CodeOf(err), want, err)
		}
		return
	}
	if !os.IsNotExist(expectErr) {
		t.Fatal(expectErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("no checks for this engine")
	}
	compareGolden(t, dir, first)
}

func compileMatching(c compiler.Compiler, rec edr.Record, repo fs.FS) ([]compiler.Compiled, error) {
	var compiled []compiler.Compiled
	for i, check := range rec.Checks {
		if check.Engine != c.Engine() {
			continue
		}
		in := compiler.Input{Record: rec, Check: check, Index: i, Repo: repo}
		artifacts, err := c.Compile(in)
		if err != nil {
			return nil, err
		}
		entry, sorted, err := compiler.Assemble(c, in, artifacts)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, compiler.Compiled{Entry: entry, Artifacts: sorted})
	}
	return compiled, nil
}

func sameCompiled(left, right []compiler.Compiled) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if len(left[i].Artifacts) != len(right[i].Artifacts) {
			return false
		}
		for j := range left[i].Artifacts {
			if left[i].Artifacts[j].Name != right[i].Artifacts[j].Name {
				return false
			}
			if !bytes.Equal(left[i].Artifacts[j].Data, right[i].Artifacts[j].Data) {
				return false
			}
		}
	}
	return true
}

func compareGolden(t *testing.T, dir string, compiled []compiler.Compiled) {
	t.Helper()
	entries := make([]compiler.Entry, 0, len(compiled))
	for _, item := range compiled {
		entries = append(entries, item.Entry)
	}
	manifest, err := compiler.MarshalManifest(entries)
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join(dir, "golden")
	if *update {
		if err := os.RemoveAll(golden); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(golden, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(golden, "manifest.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
		for _, item := range compiled {
			for _, artifact := range item.Artifacts {
				path := filepath.Join(golden, strconv.Itoa(item.Entry.CheckIndex), filepath.FromSlash(artifact.Name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, artifact.Data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		return
	}
	got, err := os.ReadFile(filepath.Join(golden, "manifest.json"))
	if err != nil {
		t.Fatalf("read golden manifest: %v (go test ./internal/compiler/compilertest -update)", err)
	}
	if !bytes.Equal(got, manifest) {
		t.Fatalf("manifest\n%s\nwant\n%s", manifest, got)
	}
	wantFiles := map[string][]byte{"manifest.json": manifest}
	for _, item := range compiled {
		for _, artifact := range item.Artifacts {
			rel := filepath.ToSlash(filepath.Join(strconv.Itoa(item.Entry.CheckIndex), filepath.FromSlash(artifact.Name)))
			wantFiles[rel] = artifact.Data
		}
	}
	err = filepath.WalkDir(golden, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(golden, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, ok := wantFiles[rel]
		if !ok {
			t.Errorf("unexpected golden file %s", rel)
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(body, data) {
			t.Errorf("%s bytes differ", rel)
		}
		delete(wantFiles, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for rel := range wantFiles {
		t.Errorf("missing golden file %s", rel)
	}
}
