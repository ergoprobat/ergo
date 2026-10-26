//go:build conformance

package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/importlinter"
	"github.com/ergoprobat/ergo/internal/edr"
)

var contractSummary = regexp.MustCompile(`Contracts:\s+(\d+) kept,\s+(\d+) broken`)

func TestImportLinter(t *testing.T) {
	stdout, stderr, exit := lintImportsOutput(t, 30*time.Second, "", "--version")
	if exit != 0 {
		t.Fatalf("version exit %d\n%s\n%s", exit, stdout, stderr)
	}
	requireVersion(t, stdout, "import-linter "+importlinter.New().Version())

	clean := compileImportLinter(t, importlinterClean)
	bad := compileImportLinter(t, importlinterBad)
	if !bytes.Equal(clean, bad) {
		t.Fatal("clean and violating configs compiled to different artifacts")
	}
	for _, id := range []string{"forbidden", "independence", "layers"} {
		if got := lintContract(t, clean, importlinterClean, id); got != "kept" {
			t.Fatalf("clean %s %s", id, got)
		}
		if got := lintContract(t, clean, importlinterBad, id); got != "broken" {
			t.Fatalf("violating %s %s", id, got)
		}
	}
}

func compileImportLinter(t *testing.T, dir string) []byte {
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
		t.Fatal(strings.Join(problems, "; "))
	}
	rec, err := edr.RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Join(dir, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	var in compiler.Input
	found := false
	for index, check := range rec.Checks {
		if check.Engine != "importlinter" {
			continue
		}
		in = compiler.Input{Record: rec, Check: check, Index: index, Repo: root.FS()}
		found = true
		break
	}
	if !found {
		t.Fatal("no importlinter check")
	}
	artifacts, err := importlinter.New().Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "importlinter.ini" {
		t.Fatalf("artifacts %#v", artifacts)
	}
	return artifacts[0].Data
}

func lintContract(t *testing.T, config []byte, subject, id string) string {
	t.Helper()
	dir := copyFixtureTree(t, filepath.Join(subject, "repo"))
	path := filepath.Join(dir, "importlinter.ini")
	if err := os.WriteFile(path, config, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := lintImportsOutput(t, 60*time.Second, dir,
		"--config", path,
		"--contract", id,
		"--no-cache",
		"--no-logo",
	)
	match := contractSummary.FindSubmatch(stripANSI(stdout))
	if match == nil {
		t.Fatalf("contract %s exit %d\n%s\n%s", id, exit, stdout, stderr)
	}
	kept, broken := string(match[1]), string(match[2])
	switch {
	case kept == "1" && broken == "0" && exit == 0:
		return "kept"
	case kept == "0" && broken == "1" && exit != 0:
		return "broken"
	default:
		t.Fatalf("contract %s exit %d kept %s broken %s\n%s\n%s", id, exit, kept, broken, stdout, stderr)
		return ""
	}
}

func stripANSI(text []byte) []byte {
	return regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`).ReplaceAll(text, nil)
}
