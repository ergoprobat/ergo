//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler/kyverno"
)

func TestKyverno(t *testing.T) {
	stdout, stderr, exit := kyvernoOutput(t, 30*time.Second, "", "version")
	if exit != 0 {
		t.Fatalf("version exit %d\n%s\n%s", exit, stdout, stderr)
	}
	if !strings.Contains(string(stdout), "Version: "+kyverno.New().Version()+"\n") {
		t.Fatalf("version %s", stdout)
	}

	clean := compiledChecks(t, kyvernoClean, "kyverno")
	bad := compiledChecks(t, kyvernoBad, "kyverno")
	sameArtifacts(t, clean[0], bad[0])
	policy := artifactData(t, clean[0], "kyverno.yaml")
	if got := kyvernoResult(t, policy, kyvernoClean); got != "pass" {
		t.Fatalf("clean %s", got)
	}
	if got := kyvernoResult(t, policy, kyvernoBad); got != "fail" {
		t.Fatalf("violating %s", got)
	}
}

func kyvernoResult(t *testing.T, policy []byte, subject string) string {
	t.Helper()
	dir := copyFixtureTree(t, filepath.Join(subject, "repo"))
	policyPath := filepath.Join(dir, "kyverno.yaml")
	if err := os.WriteFile(policyPath, policy, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := kyvernoOutput(t, 60*time.Second, dir,
		"apply", policyPath,
		"--resource", filepath.Join(dir, "pods", "api.yaml"),
		"--policy-report",
		"--output-format", "json",
	)
	var report struct {
		Results []struct {
			Policy string `json:"policy"`
			Result string `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("decode report: %v\n%s\n%s", err, stdout, stderr)
	}
	if len(report.Results) != 1 || report.Results[0].Policy != "run-as-non-root" {
		t.Fatalf("results %#v exit %d\n%s", report.Results, exit, stderr)
	}
	result := report.Results[0].Result
	switch result {
	case "pass":
		if exit != 0 {
			t.Fatalf("pass exit %d\n%s", exit, stderr)
		}
	case "fail":
		if exit == 0 {
			t.Fatal("fail exited 0")
		}
	default:
		t.Fatalf("result %s exit %d\n%s", result, exit, stderr)
	}
	return result
}
