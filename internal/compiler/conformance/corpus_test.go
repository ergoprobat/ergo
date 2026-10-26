// Package conformance checks that each compiled engine agrees with the expected verdict.
// The engine runs are behind the conformance build tag. This file checks the subjects without those tools.
package conformance

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	opaTemplates     = "../opa/templates"
	opaCorpus        = "../opa/testdata/corpus"
	semgrepTemplates = "../semgrep/templates"
	semgrepFixtures  = "../semgrep/testdata/fixtures"
	depcruiseClean   = "../depcruise/testdata/monorepo-clean"
	depcruiseBad     = "../depcruise/testdata/monorepo-violating"
)

func TestSubjects(t *testing.T) {
	opaIDs := templateIDs(t, opaTemplates, ".rego", "lib/")
	if len(opaIDs) != 20 {
		t.Fatalf("opa templates %d", len(opaIDs))
	}
	for _, id := range opaIDs {
		pass := jsonFiles(t, filepath.Join(opaCorpus, filepath.FromSlash(id), "pass"))
		fail := jsonFiles(t, filepath.Join(opaCorpus, filepath.FromSlash(id), "fail"))
		if len(pass) == 0 || len(fail) == 0 {
			t.Errorf("%s pass %d fail %d", id, len(pass), len(fail))
		}
	}
	corpus := templateIDs(t, opaCorpus, "", "")
	if strings.Join(corpus, "\n") != strings.Join(opaIDs, "\n") {
		t.Fatalf("corpus\n%s\ntemplates\n%s", strings.Join(corpus, "\n"), strings.Join(opaIDs, "\n"))
	}

	semgrepIDs := templateIDs(t, semgrepTemplates, ".yaml", "")
	if len(semgrepIDs) != 10 {
		t.Fatalf("semgrep templates %d", len(semgrepIDs))
	}
	for _, id := range semgrepIDs {
		files := semgrepFixtureFiles(t, id)
		if len(files) == 0 {
			t.Errorf("%s has no fixtures", id)
		}
		var negative, positive int
		for _, file := range files {
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			neg, pos := annotatedLines(string(body), strings.ReplaceAll(id, "/", "."))
			if len(neg) == 0 && len(pos) == 0 {
				t.Errorf("%s has no subjects", file)
			}
			negative += len(neg)
			positive += len(pos)
		}
		if negative == 0 || positive == 0 {
			t.Errorf("%s ruleid %d ok %d", id, negative, positive)
		}
	}

	for _, dir := range []string{depcruiseClean, depcruiseBad} {
		for _, name := range []string{"record.md", filepath.Join("repo", ".dependency-cruiser.json")} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				t.Errorf("%s/%s: %v", dir, name, err)
			}
		}
	}
}

func templateIDs(t *testing.T, root, suffix, skip string) []string {
	t.Helper()
	var ids []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skip != "" && strings.HasPrefix(rel, skip) {
			return nil
		}
		if entry.IsDir() {
			if rel != "." && suffix == "" && strings.Count(rel, "/") == 1 {
				ids = append(ids, rel)
			}
			return nil
		}
		if suffix == "" || !strings.HasSuffix(entry.Name(), suffix) {
			return nil
		}
		id := strings.TrimSuffix(rel, suffix)
		if strings.Count(id, "/") != 1 {
			return nil
		}
		ids = append(ids, id)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	return ids
}

func jsonFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func semgrepFixtureFiles(t *testing.T, id string) []string {
	t.Helper()
	dir, name, ok := strings.Cut(id, "/")
	if !ok {
		t.Fatalf("template id %q", id)
	}
	entries, err := os.ReadDir(filepath.Join(semgrepFixtures, dir))
	if err != nil {
		t.Fatal(err)
	}
	prefix := name + "."
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".fixture") {
			continue
		}
		files = append(files, filepath.Join(semgrepFixtures, dir, entry.Name()))
	}
	sort.Strings(files)
	return files
}

func annotatedLines(text, ruleID string) (negative, positive []int) {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		kind, id, ok := annotation(line)
		if !ok || id != ruleID || i+1 >= len(lines) {
			continue
		}
		switch kind {
		case "ruleid":
			negative = append(negative, i+2)
		case "ok":
			positive = append(positive, i+2)
		}
	}
	return negative, positive
}

func annotation(line string) (kind, id string, ok bool) {
	trimmed := strings.TrimSpace(line)
	trimmed = strings.TrimPrefix(trimmed, "//")
	trimmed = strings.TrimPrefix(trimmed, "#")
	trimmed = strings.TrimSpace(trimmed)
	kind, id, found := strings.Cut(trimmed, ":")
	if !found {
		return "", "", false
	}
	kind = strings.TrimSpace(kind)
	id = strings.TrimSpace(id)
	if (kind != "ruleid" && kind != "ok") || id == "" || strings.ContainsAny(id, " \t") {
		return "", "", false
	}
	return kind, id, true
}
