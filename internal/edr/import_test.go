package edr

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportNygard(t *testing.T) {
	source := `# 1. Record architecture decisions

Date: 2016-02-01

## Status

Accepted

## Context

We need to record the architectural decisions made on this project.

## Decision

We will use Architecture Decision Records.

## Consequences

See Michael Nygard's article.
`
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	imported, err := Import("0001-record-architecture-decisions.md", source, now)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(imported)
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(doc); len(problems) > 0 {
		t.Fatalf("validate: %v\n%s", problems, imported)
	}
	if doc.Status() != "accepted" || doc.Front["valid_from"] != "2016-02-01" {
		t.Fatalf("frontmatter %+v", doc.Front)
	}
	if doc.Slug() != "record-architecture-decisions" {
		t.Fatalf("slug %q", doc.Slug())
	}
	if doc.Front["inferred"] != true {
		t.Fatal("expected inferred true")
	}
	if doc.Front["owner"] != "team:unknown" {
		t.Fatalf("owner %v", doc.Front["owner"])
	}
}

func TestImportMADRKeepsTeamOwner(t *testing.T) {
	source := `# Use Postgres

* Status: accepted
* Deciders: team:data
* Date: 2024-03-01

## Context and Problem Statement

The catalog needs a relational store.

## Decision Outcome

Chosen option: Postgres, because the queries are relational.

## Considered Options

* Postgres
* DynamoDB

## Links

* https://github.com/acme/catalog/pull/12
`
	imported, err := Import("0012-use-postgres.md", source, time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(imported)
	if err != nil {
		t.Fatal(err)
	}
	if problems := Validate(doc); len(problems) > 0 {
		t.Fatalf("validate: %v", problems)
	}
	if doc.Front["owner"] != "team:data" {
		t.Fatalf("owner %v", doc.Front["owner"])
	}
	if doc.Status() != "accepted" || doc.Front["valid_from"] != "2024-03-01" {
		t.Fatalf("front %+v", doc.Front)
	}
	sources, _ := doc.Front["sources"].([]any)
	if len(sources) != 1 {
		t.Fatalf("sources %#v", doc.Front["sources"])
	}
}

func TestImportCorpus(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	files := sampleCorpus()
	if len(files) != 50 {
		t.Fatalf("corpus size %d", len(files))
	}
	ok := 0
	for _, file := range files {
		imported, err := Import(file.name, file.body, now)
		if err != nil {
			continue
		}
		doc, err := Parse(imported)
		if err != nil {
			t.Fatalf("%s: parse %v", file.name, err)
		}
		if problems := Validate(doc); len(problems) > 0 {
			t.Fatalf("%s: %v", file.name, problems)
		}
		if doc.Front["inferred"] != true {
			t.Fatalf("%s: inferred not flagged", file.name)
		}
		ok++
	}
	if ok*100/len(files) < 95 {
		t.Fatalf("imported %d of %d", ok, len(files))
	}
}

func TestImportWritesCorpusFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	ok := 0
	for _, file := range sampleCorpus() {
		path := filepath.Join(dir, file.name)
		imported, err := Import(file.name, file.body, now)
		if err != nil {
			if err := os.WriteFile(path, []byte(file.body), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.WriteFile(path, []byte(imported), 0o644); err != nil {
			t.Fatal(err)
		}
		ok++
	}
	if ok*100/50 < 95 {
		t.Fatalf("wrote %d", ok)
	}
}

type corpusFile struct {
	name string
	body string
}

func sampleCorpus() []corpusFile {
	var files []corpusFile
	statuses := []string{"Accepted", "Proposed", "Rejected", "Deprecated", "Superseded by ADR-0009"}
	for i := 0; i < 22; i++ {
		date := ""
		if i%2 == 0 {
			date = fmt.Sprintf("Date: 2024-%02d-01\n\n", (i%12)+1)
		}
		owner := ""
		if i%3 == 0 {
			owner = "Owner: team:platform\n\n"
		}
		files = append(files, corpusFile{
			name: fmt.Sprintf("%04d-nygard-%d.md", i+1, i),
			body: fmt.Sprintf("# %d. Choose store %d\n\n%s%s## Status\n\n%s\n\n## Context\n\nThe service needs a store.\n\n## Decision\n\nUse the option numbered %d.\n\n## Consequences\n\nOperators learn one more tool.\n", i+1, i, date, owner, statuses[i%len(statuses)], i),
		})
	}
	for i := 0; i < 23; i++ {
		link := ""
		if i%2 == 0 {
			link = fmt.Sprintf("\n## Links\n\n* https://github.com/acme/repo/pull/%d\n", 100+i)
		}
		team := "* Deciders: Ada and Grace\n"
		if i%4 == 0 {
			team = "* Deciders: team:data\n"
		}
		files = append(files, corpusFile{
			name: fmt.Sprintf("adr-%04d-madr-%d.md", i+1, i),
			body: fmt.Sprintf("# Use option %d\n\n* Status: %s\n%s* Date: 2023-%02d-15\n\n## Context and Problem Statement\n\nWe need option %d.\n\n## Decision Drivers\n\n* cost\n\n## Considered Options\n\n* one\n* two\n\n## Decision Outcome\n\nChosen option: one, because it is smaller.\n\n## Pros and Cons of the Options\n\n### one\n\n* Good, because it is small\n\n### two\n\n* Bad, because it is large\n%s", i, strings.ToLower(statuses[i%len(statuses)]), team, (i%12)+1, i, link),
		})
	}
	for i := 0; i < 3; i++ {
		files = append(files, corpusFile{
			name: fmt.Sprintf("note-%d.md", i),
			body: fmt.Sprintf("# Loose note %d\n\nThis file is only a title and a paragraph.\n", i),
		})
	}
	files = append(files,
		corpusFile{name: "blank.md", body: "No heading here.\n"},
		corpusFile{name: "empty.md", body: "\n"},
	)
	return files
}
