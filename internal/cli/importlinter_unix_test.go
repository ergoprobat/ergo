//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeImportLinterPython answers the version probe, which has six arguments, and otherwise prints the report in FAKE_OUT.
const fakeImportLinterPython = `#!/bin/sh
if [ "$#" -eq 6 ]; then
  printf '2.15\n3.18\n'
  exit 0
fi
/bin/cat "$FAKE_OUT"
exit "${FAKE_EXIT:-0}"
`

func useFakeImportLinterPython(t *testing.T, report string, exit string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "python"), []byte(fakeImportLinterPython), 0o755); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join(importlinterTestdata(), "output", report+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("FAKE_OUT", abs)
	t.Setenv("FAKE_EXIT", exit)
}

func TestCheckImportlinterFakeViolating(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := importlinterRepo(t, now, importlinterCheck, false)
	writeShop(t, root, "shop-violating")
	useFakeImportLinterPython(t, "shop-violating", "1")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := importlinterAt(t, now, []string{".", "--report", report})
	if code != 1 || strings.Count(stdout, "(block): ") != 3 {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"shop: checks[0] (block): layers shop.domain -> shop.api (l.1)",
		"shop: checks[0] (block): independence shop.billing -> shop.shipping (l.1)",
		"shop: checks[0] (block): forbidden shop.domain -> shop.infra (l.2)",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout misses %q:\n%s", want, stdout)
		}
	}
	decoded, body := readImportLinterReport(t, report)
	for _, leaked := range []string{"Layers", "Independence", "Forbidden", "shop.domain", "layers"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("report carries %q: %s", leaked, body)
		}
	}
	if decoded.ImportLinterVersion != "2.15" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "fail" || decoded.Verdicts[0].Violations != 3 {
		t.Fatalf("report %+v", decoded)
	}
	if info, err := os.Stat(report); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestCheckImportlinterFakeCleanWarnAndWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	root := importlinterRepo(t, now, importlinterCheck, false)
	writeShop(t, root, "shop-clean")
	useFakeImportLinterPython(t, "shop-clean", "0")
	stdout, stderr, code := importlinterAt(t, now, []string{"."})
	if code != 0 || !strings.Contains(stdout, "shop: checks[0] (block): pass") {
		t.Fatalf("clean code %d\n%s\n%s", code, stdout, stderr)
	}

	root = importlinterRepo(t, now, "  - engine: importlinter\n    ref: .importlinter\n    mode: warn\n", false)
	writeShop(t, root, "shop-violating")
	useFakeImportLinterPython(t, "shop-violating", "1")
	stdout, stderr, code = importlinterAt(t, now, []string{"."})
	if code != 0 || !strings.Contains(stdout, "(warn): layers shop.domain -> shop.api (l.1)") {
		t.Fatalf("warn code %d\n%s\n%s", code, stdout, stderr)
	}

	root = importlinterRepo(t, now, importlinterCheck, true)
	writeShop(t, root, "shop-violating")
	useFakeImportLinterPython(t, "shop-violating", "1")
	report := filepath.Join(root, "waived.json")
	stdout, stderr, code = importlinterAt(t, now, []string{".", "--report", report})
	if code != 0 || !strings.Contains(stdout, "shop: checks[0] (block): waived") || strings.Contains(stdout, "shop.domain") {
		t.Fatalf("waiver code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readImportLinterReport(t, report)
	if len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "waived" || decoded.Verdicts[0].Violations != 3 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckImportlinterFakeEngineErrorStaysAnErrorWithAWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := importlinterRepo(t, now, importlinterCheck, true)
	writeShop(t, root, "shop-violating")
	useFakeImportLinterPython(t, "error-module", "1")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := importlinterAt(t, now, []string{".", "--report", report})
	if code != 1 || !strings.Contains(stdout, "importlinter-run") || strings.Contains(stdout, "waived") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readImportLinterReport(t, report)
	if decoded.ImportLinterVersion != "2.15" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "error" || decoded.Verdicts[0].Violations != 0 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckImportlinterFakeAppliesInEveryEnvironment(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := importlinterRepo(t, now, importlinterCheck, false)
	writeShop(t, root, "shop-violating")
	useFakeImportLinterPython(t, "shop-violating", "1")
	// The record has no environment list, so it applies in every environment.
	stdout, _, code := importlinterAt(t, now, []string{".", "--env", "prod"})
	if code != 1 || strings.Count(stdout, "(block): ") != 3 {
		t.Fatalf("code %d\n%s", code, stdout)
	}
}
