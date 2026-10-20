package edr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidExample(t *testing.T) {
	doc := parseTestdata(t, "valid.md")
	if doc.Slug() != "tls-on-public-ingress" {
		t.Fatalf("slug = %q", doc.Slug())
	}
	problems := Validate(doc)
	if len(problems) != 0 {
		t.Fatalf("valid file: %s", strings.Join(problems, "; "))
	}
	text, ok := DecisionText(doc.Body)
	if !ok || !strings.Contains(text, "terminates TLS") {
		t.Fatalf("decision text = %q ok=%v", text, ok)
	}
}

func TestInvalidExampleMissingHeading(t *testing.T) {
	doc := parseTestdata(t, "invalid.md")
	problems := Validate(doc)
	found := false
	for _, problem := range problems {
		if strings.Contains(problem, "missing heading ## Verification") {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems = %v", problems)
	}
}

func TestMatchGlob(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docs", "decisions", "nested", "one.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "README.md")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	matches, err := Match(root, []string{"docs/decisions/**/*.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0] != path {
		t.Fatalf("matches = %v", matches)
	}
}

func parseTestdata(t *testing.T, name string) Document {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
