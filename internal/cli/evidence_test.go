package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/builtin"
)

const (
	evidenceSHA = "0123456789abcdef0123456789abcdef01234567"
	otherSHA    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
)

func TestCheckCI(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	withDir(t, root)
	record := compileRecord(idA, "s3-kms", "accepted", "blocking", "2026-01-01", "", "  - engine: ci-binding\n    job: checkov / CKV_AWS_145\n")
	writeRepoFile(t, root, "docs/decisions/s3.md", record)
	if _, stderr, code := compileAt(t, now, builtin.Registry()); code != 0 {
		t.Fatalf("compile: %s", stderr)
	}

	writeRepoFile(t, root, "evidence.json", ciEvidence(evidenceSHA, "checkov / CKV_AWS_145", "success"))
	stdout, stderr, code := ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA})
	if code != 0 || !strings.Contains(stdout, "s3-kms: checks[0] (block): pass") {
		t.Fatalf("pass code %d\n%s\n%s", code, stdout, stderr)
	}

	writeRepoFile(t, root, "evidence.json", ciEvidence(evidenceSHA, "other", "success"))
	stdout, stderr, code = ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA})
	if code != 1 || !strings.Contains(stdout, "job not in evidence") {
		t.Fatalf("missing code %d\n%s\n%s", code, stdout, stderr)
	}

	writeRepoFile(t, root, "evidence.json", ciEvidence(evidenceSHA, "checkov / CKV_AWS_145", "failure"))
	stdout, _, code = ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA})
	if code != 1 || !strings.Contains(stdout, "job conclusion failure") {
		t.Fatalf("failed code %d\n%s", code, stdout)
	}

	writeRepoFile(t, root, "evidence.json", ciEvidence(otherSHA, "checkov / CKV_AWS_145", "success"))
	_, stderr, code = ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA})
	if code != 1 || !strings.Contains(stderr, "evidence-sha") {
		t.Fatalf("sha code %d stderr %s", code, stderr)
	}

	_, stderr, code = ciAt(t, now, []string{"evidence.json"})
	if code != 2 {
		t.Fatalf("usage code %d stderr %s", code, stderr)
	}
	_, stderr, code = ciAt(t, now, []string{"evidence.json", "--sha", "ABC"})
	if code != 2 {
		t.Fatalf("bad sha code %d stderr %s", code, stderr)
	}
}

func TestCheckCIWarnAndWaiver(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	withDir(t, root)
	record := compileRecord(idA, "s3-kms", "accepted", "blocking", "2026-01-01", "", "  - engine: ci-binding\n    job: unit\n    mode: warn\n")
	writeRepoFile(t, root, "docs/decisions/s3.md", record)
	writeRepoFile(t, root, "evidence.json", ciEvidence(evidenceSHA, "other", "success"))
	if _, stderr, code := compileAt(t, now, builtin.Registry()); code != 0 {
		t.Fatalf("compile: %s", stderr)
	}
	stdout, _, code := ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA})
	if code != 0 || !strings.Contains(stdout, "(warn):") {
		t.Fatalf("warn code %d\n%s", code, stdout)
	}

	waived := compileRecord(idA, "s3-kms", "accepted", "blocking", "2026-01-01", "", "  - engine: ci-binding\n    job: unit\n")
	waived = strings.Replace(waived, "\nwaivers: []\n", "\nwaivers:\n  - reason: cutover\n    approver: Ada Lovelace\n    valid_from: 2026-01-01\n    valid_to: 2026-03-01\n", 1)
	writeRepoFile(t, root, "docs/decisions/s3.md", waived)
	if stdout, stderr, code := compileAt(t, now, builtin.Registry()); code != 0 {
		t.Fatalf("compile waiver:\n%s\n%s\n%s", waived, stdout, stderr)
	}
	stdout, stderr, code := ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA})
	if code != 0 || !strings.Contains(stdout, "waived") {
		t.Fatalf("waiver code %d\n%s\n%s", code, stdout, stderr)
	}
}

func TestCheckCIReport(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	withDir(t, root)
	record := compileRecord(idA, "s3-kms", "accepted", "blocking", "2026-01-01", "", "  - engine: ci-binding\n    job: checkov / CKV_AWS_145\n")
	record = strings.Replace(record, "  - org: acme\n", "  - org: acme\n    env: [prod]\n", 1)
	writeRepoFile(t, root, "docs/decisions/s3.md", record)
	writeRepoFile(t, root, "evidence.json", ciEvidence(evidenceSHA, "missing", "success"))
	if _, stderr, code := compileAt(t, now, builtin.Registry()); code != 0 {
		t.Fatalf("compile: %s", stderr)
	}
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA, "--env", "dev"})
	if code != 0 || !strings.Contains(stdout, "no checks selected") {
		t.Fatalf("none code %d\n%s\n%s", code, stdout, stderr)
	}
	stdout, stderr, code = ciAt(t, now, []string{"evidence.json", "--sha", evidenceSHA, "--report", report})
	if code != 1 {
		t.Fatalf("report code %d\n%s\n%s", code, stdout, stderr)
	}
	body, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "CKV_AWS_145") || strings.Contains(string(body), "job not in evidence") {
		t.Fatalf("report carries the finding: %s", body)
	}
	var decoded compiler.CIReport
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.CommitSHA != evidenceSHA || len(decoded.EvidenceSHA256) != 64 || len(decoded.BuildSHA256) != 64 || decoded.Verdicts[0].Result != "fail" {
		t.Fatalf("report %+v", decoded)
	}
	info, err := os.Stat(report)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v %v", info, err)
	}
}

func TestCheckCheckov(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	withDir(t, root)
	record := compileRecord(idA, "admin-port", "accepted", "blocking", "2026-06-01", "", "  - engine: checkov\n    ref: CKV_AWS_24\n")
	writeRepoFile(t, root, "docs/decisions/sg.md", record)
	if _, stderr, code := compileAt(t, now, builtin.Registry()); code != 0 {
		t.Fatalf("compile: %s", stderr)
	}
	marker := "secret-group-name"
	writeRepoFile(t, root, "results.json", `{"check_type":"terraform","results":{"passed_checks":[],"failed_checks":[{"check_id":"CKV_AWS_24","resource":"`+marker+`","file_path":"/main.tf"}],"skipped_checks":[],"parsing_errors":[]},"summary":{"parsing_errors":0,"checkov_version":"3.2.495"}}`)
	report := filepath.Join(root, "report.json")
	stdout, stderr, code := checkovAt(t, now, []string{"results.json", "--report", report})
	if code != 1 || !strings.Contains(stdout, marker) {
		t.Fatalf("fail code %d\n%s\n%s", code, stdout, stderr)
	}
	body, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), marker) || strings.Contains(string(body), "CKV_AWS_24") {
		t.Fatalf("report carries the finding: %s", body)
	}
	var decoded compiler.CheckovReport
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.EvidenceSHA256) != 64 || decoded.Verdicts[0].Violations != 1 {
		t.Fatalf("report %+v", decoded)
	}
}

func ciEvidence(sha, name, conclusion string) string {
	return `{"evidence_version":1,"sha":"` + sha + `","jobs":[{"name":"` + name + `","conclusion":"` + conclusion + `"}]}`
}

func ciAt(t *testing.T, now time.Time, args []string) (string, string, int) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run(append([]string{"check", "ci"}, args...), &stdout, &stderr, deps{now: func() time.Time { return now }, registry: builtin.Registry()})
	return stdout.String(), stderr.String(), code
}

func checkovAt(t *testing.T, now time.Time, args []string) (string, string, int) {
	t.Helper()
	var stdout, stderr strings.Builder
	code := run(append([]string{"check", "checkov"}, args...), &stdout, &stderr, deps{now: func() time.Time { return now }, registry: builtin.Registry()})
	return stdout.String(), stderr.String(), code
}
