//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeKyverno answers the version probe and otherwise prints the report in FAKE_OUT.
const fakeKyverno = `#!/bin/sh
if [ "$1" = "version" ]; then
  printf 'Version: 1.19.2\nTime: x\n'
  exit 0
fi
/bin/cat "$FAKE_OUT"
exit "${FAKE_EXIT:-0}"
`

func useFakeKyverno(t *testing.T, report string, exit string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kyverno"), []byte(fakeKyverno), 0o755); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join(kyvernoTestdata(), "output", report+".json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("FAKE_OUT", abs)
	t.Setenv("FAKE_EXIT", exit)
}

func TestCheckKyvernoFakeViolating(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := kyvernoRepo(t, now, kyvernoCheck, false)
	writePod(t, root, "cluster-violating")
	useFakeKyverno(t, "mixed", "1")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := kyvernoAt(t, now, []string{"k8s", "--report", report})
	if code != 1 || strings.Count(stdout, "(block): ") != 2 {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"pods: checks[0] (block): team-a/Pod/bad: runAsNonRoot must be true",
		"pods: checks[0] (block): default/Deployment/dep: runAsNonRoot must be true",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout misses %q:\n%s", want, stdout)
		}
	}
	decoded, body := readKyvernoReport(t, report)
	for _, leaked := range []string{"team-a", "Deployment", "dep", "run-as-non-root", "runAsNonRoot"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("report carries %q: %s", leaked, body)
		}
	}
	if decoded.KyvernoVersion != "1.19.2" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "fail" || decoded.Verdicts[0].Violations != 2 {
		t.Fatalf("report %+v", decoded)
	}
	if info, err := os.Stat(report); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestCheckKyvernoFakeCleanWarnAndWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	root := kyvernoRepo(t, now, kyvernoCheck, false)
	writePod(t, root, "cluster-clean")
	useFakeKyverno(t, "pass", "0")
	stdout, stderr, code := kyvernoAt(t, now, []string{"k8s"})
	if code != 0 || !strings.Contains(stdout, "pods: checks[0] (block): pass") {
		t.Fatalf("clean code %d\n%s\n%s", code, stdout, stderr)
	}

	root = kyvernoRepo(t, now, "  - engine: kyverno\n    ref: policies/run-as-non-root.yaml\n    mode: warn\n", false)
	writePod(t, root, "cluster-violating")
	useFakeKyverno(t, "fail", "1")
	stdout, stderr, code = kyvernoAt(t, now, []string{"k8s"})
	if code != 0 || !strings.Contains(stdout, "(warn): team-a/Pod/bad: runAsNonRoot must be true") {
		t.Fatalf("warn code %d\n%s\n%s", code, stdout, stderr)
	}

	root = kyvernoRepo(t, now, kyvernoCheck, true)
	writePod(t, root, "cluster-violating")
	useFakeKyverno(t, "fail", "1")
	report := filepath.Join(root, "waived.json")
	stdout, stderr, code = kyvernoAt(t, now, []string{"k8s", "--report", report})
	if code != 0 || !strings.Contains(stdout, "pods: checks[0] (block): waived") || strings.Contains(stdout, "team-a") {
		t.Fatalf("waiver code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readKyvernoReport(t, report)
	if len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "waived" || decoded.Verdicts[0].Violations != 1 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckKyvernoFakeErrorResultStaysAnErrorWithAWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := kyvernoRepo(t, now, kyvernoCheck, true)
	writePod(t, root, "cluster-violating")
	useFakeKyverno(t, "error", "1")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := kyvernoAt(t, now, []string{"k8s", "--report", report})
	if code != 1 || !strings.Contains(stdout, "kyverno-incomplete") || strings.Contains(stdout, "waived") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readKyvernoReport(t, report)
	if decoded.KyvernoVersion != "1.19.2" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "error" || decoded.Verdicts[0].Violations != 0 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckKyvernoFakePathProblemsAreErrors(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := kyvernoRepo(t, now, kyvernoCheck, true)
	writePod(t, root, "cluster-violating")
	useFakeKyverno(t, "fail", "1")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	for dir, code := range map[string]string{"nope": "kyverno-path", "escape": "kyverno-path", "empty": "kyverno-empty"} {
		stdout, stderr, exit := kyvernoAt(t, now, []string{dir})
		if exit != 1 || !strings.Contains(stdout, code) || strings.Contains(stdout, "waived") {
			t.Fatalf("%s: code %d\n%s\n%s", dir, exit, stdout, stderr)
		}
	}
}

func TestCheckKyvernoFakeAppliesInEveryEnvironment(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := kyvernoRepo(t, now, kyvernoCheck, false)
	writePod(t, root, "cluster-violating")
	useFakeKyverno(t, "mixed", "1")
	// The record has no environment list, so it applies in every environment.
	stdout, _, code := kyvernoAt(t, now, []string{"k8s", "--env", "prod"})
	if code != 1 || strings.Count(stdout, "(block): ") != 2 {
		t.Fatalf("code %d\n%s", code, stdout)
	}
}
