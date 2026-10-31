package edr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPremiseRecord(t *testing.T) {
	doc := withPremises(t, `
premises:
  - id: postgres-major-is-16
    statement: The platform still runs PostgreSQL 16.
    every: 1
    probe:
      url: https://example.com/platform/postgres.json
      pointer: /major
      equals: 16
`)
	if problems := Validate(doc); len(problems) != 0 {
		t.Fatalf("problems: %s", strings.Join(problems, "; "))
	}
	rec, err := RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Premises) != 1 {
		t.Fatalf("premises = %d", len(rec.Premises))
	}
	got := rec.Premises[0]
	if got.ID != "postgres-major-is-16" || got.Every != 1 || !got.HasEquals || got.Semver != "" {
		t.Fatalf("premise = %+v", got)
	}
	if got.URL != "https://example.com/platform/postgres.json" || got.Pointer != "/major" || got.Equals != float64(16) {
		t.Fatalf("probe = %+v", got)
	}
}

func TestSemverPremise(t *testing.T) {
	doc := withPremises(t, `
premises:
  - id: postgres-at-least-16
    statement: Production PostgreSQL is at least 16.0.0.
    every: 365
    probe:
      url: https://example.com/platform/postgres.json
      pointer: /version
      semver: ">=16.0.0"
`)
	if problems := Validate(doc); len(problems) != 0 {
		t.Fatalf("problems: %s", strings.Join(problems, "; "))
	}
	rec, err := RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	got := rec.Premises[0]
	if got.HasEquals || got.Semver != ">=16.0.0" || got.Pointer != "/version" {
		t.Fatalf("premise = %+v", got)
	}
}

func TestDraft01StillValid(t *testing.T) {
	doc := parseTestdata(t, "valid.md")
	if problems := Validate(doc); len(problems) != 0 {
		t.Fatalf("problems: %s", strings.Join(problems, "; "))
	}
	rec, err := RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Premises) != 0 {
		t.Fatalf("premises = %+v", rec.Premises)
	}
}

func TestDuplicatePremiseID(t *testing.T) {
	doc := withPremises(t, `
premises:
  - id: same-id
    statement: One fact.
    every: 1
    probe:
      url: https://example.com/a.json
      pointer: /a
      equals: 1
  - id: same-id
    statement: The same fact again.
    every: 1
    probe:
      url: https://example.com/a.json
      pointer: /a
      equals: 1
`)
	if !hasProblem(Validate(doc), "premises: duplicate premise id") {
		t.Fatalf("problems = %v", Validate(doc))
	}
}

func TestEqualsTooLarge(t *testing.T) {
	doc := withPremises(t, `
premises:
  - id: long-equals
    statement: The value is a long string.
    every: 1
    probe:
      url: https://example.com/a.json
      pointer: /a
      equals: "`+strings.Repeat("a", 1023)+`"
`)
	if !hasProblem(Validate(doc), "premises: equals exceeds 1024 bytes") {
		t.Fatalf("problems = %v", Validate(doc))
	}
}

func TestEqualsAtLimit(t *testing.T) {
	doc := withPremises(t, `
premises:
  - id: long-equals
    statement: The value is a long string.
    every: 1
    probe:
      url: https://example.com/a.json
      pointer: /a
      equals: "`+strings.Repeat("a", 1022)+`"
`)
	if problems := Validate(doc); len(problems) != 0 {
		t.Fatalf("problems: %s", strings.Join(problems, "; "))
	}
}

func TestUnknownPremiseKey(t *testing.T) {
	doc := withPremises(t, `
premises:
  - id: scripted
    statement: A script is not a probe.
    every: 1
    probe:
      url: https://example.com/a.json
      pointer: /a
      equals: 1
      script: echo
`)
	if problems := Validate(doc); len(problems) == 0 {
		t.Fatal("expected a schema error")
	}
}

func TestEqualsKeyOrder(t *testing.T) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte("b: 1\na: 2\n"), &root); err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeJSON(&root)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"b":1,"a":2}` {
		t.Fatalf("encoded = %s", encoded)
	}
}

func withPremises(t *testing.T, premises string) Document {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "valid.md"))
	if err != nil {
		t.Fatal(err)
	}
	const needle = "tags: [tls]\n"
	text := string(raw)
	if !strings.Contains(text, needle) {
		t.Fatal("fixture has no tags line")
	}
	if !strings.HasSuffix(premises, "\n") {
		premises += "\n"
	}
	doc, err := Parse(strings.Replace(text, needle, needle+premises, 1))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func hasProblem(problems []string, want string) bool {
	for _, problem := range problems {
		if problem == want || strings.Contains(problem, want) {
			return true
		}
	}
	return false
}
