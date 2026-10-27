//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeESLint answers the version probe and otherwise prints the result list in FAKE_OUT.
const fakeESLint = `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'v10.13.0\n'
  exit 0
fi
/bin/cat "$FAKE_OUT"
exit "${FAKE_EXIT:-0}"
`

// installFakeESLint writes a fake eslint and the packages next to it under dir, and puts it first on PATH.
func installFakeESLint(t *testing.T, dir string) {
	t.Helper()
	modules := filepath.Join(dir, "node_modules")
	writeRepoFile(t, modules, "eslint/package.json", `{"version":"10.13.0"}`)
	writeRepoFile(t, modules, "typescript-eslint/package.json", `{"version":"8.72.1"}`)
	writeRepoFile(t, modules, "typescript/package.json", `{"version":"6.0.3"}`)
	bin := filepath.Join(modules, ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "eslint"), []byte(fakeESLint), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
}

// useFakeESLint installs the fake outside the repository and makes it print a real capture, with the
// repository root of the capture replaced by root.
func useFakeESLint(t *testing.T, root, results, exit string) {
	t.Helper()
	installFakeESLint(t, t.TempDir())
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(eslintTestdata(), "results", results+".json"))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), results+".json")
	if err := os.WriteFile(out, []byte(strings.ReplaceAll(string(raw), "/repo", filepath.ToSlash(resolved))), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_OUT", out)
	t.Setenv("FAKE_EXIT", exit)
}

func TestCheckESLintFakeViolating(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := eslintRepo(t, now, eslintTSCheck, false, "app-violating")
	useFakeESLint(t, root, "block", "1")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := eslintAt(t, now, []string{"services", "--report", report})
	if code != 1 || strings.Count(stdout, "(block): ") != 1 {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	if want := "app: checks[0] (block): @typescript-eslint/no-floating-promises services/float.ts:3:3: Promises must be awaited"; !strings.Contains(stdout, want) {
		t.Fatalf("stdout misses %q:\n%s", want, stdout)
	}
	decoded, body := readESLintReport(t, report)
	for _, leaked := range []string{"no-floating-promises", "float.ts", "Promises must", "typescript-eslint", "source"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("report carries %q: %s", leaked, body)
		}
	}
	if decoded.ESLintVersion != "10.13.0" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "fail" || decoded.Verdicts[0].Violations != 1 {
		t.Fatalf("report %+v", decoded)
	}
	if len(decoded.Paths) != 1 || decoded.Paths[0] != "services" || len(decoded.BuildSHA256) != 64 {
		t.Fatalf("report %+v", decoded)
	}
	if info, err := os.Stat(report); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestCheckESLintFakeCleanWarnAndWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

	root := eslintRepo(t, now, eslintTSCheck, false, "app-clean")
	useFakeESLint(t, root, "clean", "0")
	stdout, stderr, code := eslintAt(t, now, []string{"services"})
	if code != 0 || !strings.Contains(stdout, "app: checks[0] (block): pass") {
		t.Fatalf("clean code %d\n%s\n%s", code, stdout, stderr)
	}

	warn := strings.Replace(eslintTSCheck, "mode: block", "mode: warn", 1)
	root = eslintRepo(t, now, warn, false, "app-violating")
	useFakeESLint(t, root, "warn", "0")
	stdout, stderr, code = eslintAt(t, now, []string{"services"})
	if code != 0 || !strings.Contains(stdout, "(warn): @typescript-eslint/no-floating-promises services/float.ts:3:3") {
		t.Fatalf("warn code %d\n%s\n%s", code, stdout, stderr)
	}

	root = eslintRepo(t, now, eslintTSCheck, true, "app-violating")
	useFakeESLint(t, root, "block", "1")
	report := filepath.Join(root, "waived.json")
	stdout, stderr, code = eslintAt(t, now, []string{"services", "--report", report})
	if code != 0 || !strings.Contains(stdout, "app: checks[0] (block): waived") || strings.Contains(stdout, "float.ts") {
		t.Fatalf("waiver code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readESLintReport(t, report)
	if len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "waived" || decoded.Verdicts[0].Violations != 1 {
		t.Fatalf("report %+v", decoded)
	}
}

func TestCheckESLintFakeDisagreeingOutputStaysAnErrorWithAWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := eslintRepo(t, now, eslintTSCheck, true, "app-violating")
	useFakeESLint(t, root, "block", "0")
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := eslintAt(t, now, []string{"services", "--report", report})
	if code != 1 || !strings.Contains(stdout, "eslint-output") || strings.Contains(stdout, "waived") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	decoded, _ := readESLintReport(t, report)
	if decoded.ESLintVersion != "10.13.0" || len(decoded.Verdicts) != 1 || decoded.Verdicts[0].Result != "error" || decoded.Verdicts[0].Violations != 0 {
		t.Fatalf("report %+v", decoded)
	}
}

// A suppression that hides a finding is an error: the run reports exit 0 and nothing else.
func TestCheckESLintFakeSuppressedFindingIsAnErrorEvenWithAWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := eslintRepo(t, now, eslintJSCheck, true, "app-violating")
	useFakeESLint(t, root, "suppressed", "0")
	stdout, stderr, code := eslintAt(t, now, []string{"web"})
	if code != 1 || !strings.Contains(stdout, "eslint-output") || strings.Contains(stdout, "waived") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
}

func TestCheckESLintFakeParseErrorIsIncompleteEvenWithAWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := eslintRepo(t, now, eslintJSCheck, true, "app-violating")
	useFakeESLint(t, root, "fatal", "1")
	stdout, stderr, code := eslintAt(t, now, []string{"web"})
	if code != 1 || !strings.Contains(stdout, "eslint-incomplete") || strings.Contains(stdout, "waived") {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
}

func TestCheckESLintFakePathAndToolchainProblemsAreErrors(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := eslintRepo(t, now, eslintTSCheck, true, "app-violating")
	useFakeESLint(t, root, "block", "1")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	for dir, code := range map[string]string{
		"nope":   "eslint-path",
		"escape": "eslint-path",
	} {
		stdout, stderr, exit := eslintAt(t, now, []string{dir})
		if exit != 1 || !strings.Contains(stdout, code) || strings.Contains(stdout, "waived") {
			t.Fatalf("%s: code %d\n%s\n%s", dir, exit, stdout, stderr)
		}
	}

	// The packages that decide the verdict come from the runner, not from the repository under review.
	installFakeESLint(t, root)
	stdout, stderr, exit := eslintAt(t, now, []string{"services"})
	if exit != 1 || !strings.Contains(stdout, "eslint-missing") || !strings.Contains(stdout, "inside the repository") || strings.Contains(stdout, "waived") {
		t.Fatalf("toolchain in the repository: code %d\n%s\n%s", exit, stdout, stderr)
	}
}

func TestCheckESLintFakeAppliesInEveryEnvironment(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := eslintRepo(t, now, eslintTSCheck, false, "app-violating")
	useFakeESLint(t, root, "block", "1")
	// The record has no environment list, so it applies in every environment.
	stdout, _, code := eslintAt(t, now, []string{"services", "--env", "prod"})
	if code != 1 || strings.Count(stdout, "(block): ") != 1 {
		t.Fatalf("code %d\n%s", code, stdout)
	}
}
