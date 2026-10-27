//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeGator answers the version probe and otherwise prints the results in FAKE_OUT.
const fakeGator = `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'gator version v3.23.1 (Feature State: beta), opa/v1.17.1, frameworks/v0.0.0-20260616163050-e1eaa1bf6d62\n'
  exit 0
fi
/bin/cat "$FAKE_OUT"
exit "${FAKE_EXIT:-0}"
`

func useFakeGator(t *testing.T, results string, exit string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gator"), []byte(fakeGator), 0o755); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join(gatekeeperTestdata(), "output", results+".json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("FAKE_OUT", abs)
	t.Setenv("FAKE_EXIT", exit)
}

func TestCheckGatekeeperFakeViolating(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := gatekeeperRepo(t, now, gatekeeperCheck, false)
	writeDeployment(t, root, "gatekeeper-violating")
	useFakeGator(t, "multi", "1")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := gatekeeperAt(t, now, []string{"apps", "--report", report})
	if code != 1 || strings.Count(stdout, "(block): ") != 2 {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	for _, want := range []string{
		"owner: checks[0] (block): team-a/Deployment/api: missing owner label",
		"owner: checks[0] (block): Deployment/web: missing owner label",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout misses %q:\n%s", want, stdout)
		}
	}
	decoded, body := readGatekeeperReport(t, report)
	for _, leaked := range []string{"team-a", "Deployment", "web", "RequireOwner", "missing owner"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("report carries %q: %s", leaked, body)
		}
	}
	if decoded.GatorVersion != "3.23.1" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "fail" || decoded.Verdicts[0].Violations != 2 {
		t.Fatalf("report %+v", decoded)
	}
	if info, err := os.Stat(report); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestCheckGatekeeperFakeCleanWarnAndWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	root := gatekeeperRepo(t, now, gatekeeperCheck, false)
	writeDeployment(t, root, "gatekeeper-clean")
	useFakeGator(t, "pass", "0")
	stdout, stderr, code := gatekeeperAt(t, now, []string{"apps"})
	if code != 0 || !strings.Contains(stdout, "owner: checks[0] (block): pass") {
		t.Fatalf("clean code %d\n%s\n%s", code, stdout, stderr)
	}

	root = gatekeeperRepo(t, now, "  - engine: opa\n    target: gatekeeper\n    ref: policies/gatekeeper.yaml\n    mode: warn\n", false)
	writeDeployment(t, root, "gatekeeper-violating")
	useFakeGator(t, "warn", "0")
	stdout, stderr, code = gatekeeperAt(t, now, []string{"apps"})
	if code != 0 || !strings.Contains(stdout, "(warn): Deployment/web: missing owner label") {
		t.Fatalf("warn code %d\n%s\n%s", code, stdout, stderr)
	}

	root = gatekeeperRepo(t, now, gatekeeperCheck, true)
	writeDeployment(t, root, "gatekeeper-violating")
	useFakeGator(t, "deny", "1")
	report := filepath.Join(root, "waived.json")
	stdout, stderr, code = gatekeeperAt(t, now, []string{"apps", "--report", report})
	if code != 0 || !strings.Contains(stdout, "owner: checks[0] (block): waived") || strings.Contains(stdout, "web") {
		t.Fatalf("waiver code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readGatekeeperReport(t, report)
	if len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "waived" || decoded.Verdicts[0].Violations != 1 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckGatekeeperFakeDisagreeingOutputStaysAnErrorWithAWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := gatekeeperRepo(t, now, gatekeeperCheck, true)
	writeDeployment(t, root, "gatekeeper-violating")
	useFakeGator(t, "deny", "0")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := gatekeeperAt(t, now, []string{"apps", "--report", report})
	if code != 1 || !strings.Contains(stdout, "gatekeeper-output") || strings.Contains(stdout, "waived") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readGatekeeperReport(t, report)
	if decoded.GatorVersion != "3.23.1" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "error" || decoded.Verdicts[0].Violations != 0 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckGatekeeperFakePathAndDocumentProblemsAreErrors(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := gatekeeperRepo(t, now, gatekeeperCheck, true)
	writeDeployment(t, root, "gatekeeper-violating")
	useFakeGator(t, "deny", "1")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, root, "nothing/a.yaml", "# no object\n")
	writeRepoFile(t, root, "lists/l.yaml", "apiVersion: v1\nkind: List\nitems:\n  - a\n")
	writeRepoFile(t, root, "policy/c.yaml", "apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: RequireOwner\nmetadata:\n  name: x\n")
	for dir, code := range map[string]string{
		"nope":    "gatekeeper-path",
		"escape":  "gatekeeper-path",
		"empty":   "gatekeeper-empty",
		"nothing": "gatekeeper-empty",
		"lists":   "gatekeeper-resource",
		"policy":  "gatekeeper-resource",
	} {
		stdout, stderr, exit := gatekeeperAt(t, now, []string{dir})
		if exit != 1 || !strings.Contains(stdout, code) || strings.Contains(stdout, "waived") {
			t.Fatalf("%s: code %d\n%s\n%s", dir, exit, stdout, stderr)
		}
	}
}

func TestCheckGatekeeperFakeAppliesInEveryEnvironment(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := gatekeeperRepo(t, now, gatekeeperCheck, false)
	writeDeployment(t, root, "gatekeeper-violating")
	useFakeGator(t, "multi", "1")
	// The record has no environment list, so it applies in every environment.
	stdout, _, code := gatekeeperAt(t, now, []string{"apps", "--env", "prod"})
	if code != 1 || strings.Count(stdout, "(block): ") != 2 {
		t.Fatalf("code %d\n%s", code, stdout)
	}
}
