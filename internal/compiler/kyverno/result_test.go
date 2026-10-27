package kyverno

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const policyName = "run-as-non-root"

// readOutput returns a report captured from Kyverno CLI 1.19.2.
func readOutput(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "output", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestParseReportReadsCapturedOutput(t *testing.T) {
	cases := []struct {
		name string
		exit int
		want []Finding
	}{
		{"pass", 0, nil},
		{"skip", 0, nil},
		{"fail", 1, []Finding{{Resource: "team-a/Pod/bad", Message: "runAsNonRoot must be true"}}},
		{"mixed", 1, []Finding{
			{Resource: "team-a/Pod/bad", Message: "runAsNonRoot must be true"},
			{Resource: "default/Deployment/dep", Message: "runAsNonRoot must be true"},
		}},
		{"multiline", 1, []Finding{{Resource: "default/Pod/api", Message: "line1\nline2 api"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReport([]byte(readOutput(t, tc.name)), tc.exit, policyName)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseReportErrorResultIsIncomplete(t *testing.T) {
	_, err := ParseReport([]byte(readOutput(t, "error")), 1, policyName)
	requireCode(t, err, CodeIncomplete)
	if !strings.Contains(err.Error(), "default/Pod/api: error: no such key: securityContext") {
		t.Fatalf("err %v", err)
	}
}

// A run that prints nothing and exits 0 loaded resources that the policy does not match.
// Exit 1 with nothing printed is a load error, and any other status is not a verdict.
func TestParseReportEmptyOutput(t *testing.T) {
	if got, err := ParseReport(nil, 0, policyName); err != nil || got != nil {
		t.Fatalf("got %v %v", got, err)
	}
	for _, exit := range []int{1, 2, 3, 137, -1} {
		_, err := ParseReport(nil, exit, policyName)
		requireCode(t, err, CodeRun)
	}
	for _, exit := range []int{2, 137, -1} {
		_, err := ParseReport([]byte(readOutput(t, "pass")), exit, policyName)
		requireCode(t, err, CodeRun)
	}
}

func TestParseReportAcceptsATrailingNewlineOnly(t *testing.T) {
	pass := readOutput(t, "pass")
	if !strings.HasSuffix(pass, "}\n") {
		t.Fatalf("fixture does not end in a newline: %q", pass[len(pass)-10:])
	}
	if _, err := ParseReport([]byte(strings.TrimSuffix(pass, "\n")), 0, policyName); err != nil {
		t.Fatal(err)
	}
}

func TestParseReportRefusesAnythingElse(t *testing.T) {
	pass := readOutput(t, "pass")
	fail := readOutput(t, "fail")
	mixed := readOutput(t, "mixed")
	cases := []struct {
		name string
		out  string
		exit int
		code string
	}{
		{"banner before the report", "Skipped invalid policy x\n" + pass, 0, CodeOutput},
		{"text after the report", pass + "Error: late\n", 0, CodeOutput},
		{"two reports", pass + pass, 0, CodeOutput},
		{"blank output", "\n", 0, CodeOutput},
		{"terminal escape", "\x1b[0m" + pass, 0, CodeOutput},
		{"not json", "{not json", 0, CodeOutput},
		{"not an object", "[]", 0, CodeOutput},
		{"text report", "pass: 1, fail: 0, warn: 0, error: 0, skip: 0\n", 0, CodeOutput},
		{"other kind", strings.Replace(pass, `"kind":"ClusterReport"`, `"kind":"PolicyReport"`, 1), 0, CodeOutput},
		{"other api version", strings.Replace(pass, "openreports.io/v1alpha1", "wgpolicyk8s.io/v1alpha2", 1), 0, CodeOutput},
		{"no kind", strings.Replace(pass, `"kind":"ClusterReport",`, "", 1), 0, CodeOutput},
		{"no results", strings.Replace(pass, `"results":[`, `"nothing":[`, 1), 0, CodeOutput},
		{"empty results", `{"kind":"ClusterReport","apiVersion":"openreports.io/v1alpha1","summary":{"pass":0,"fail":0,"warn":0,"error":0,"skip":0},"results":[]}`, 0, CodeOutput},
		{"no summary", strings.Replace(pass, `"summary":{"pass":1,"fail":0,"warn":0,"error":0,"skip":0},`, "", 1), 0, CodeOutput},
		{"partial summary", strings.Replace(pass, `"skip":0}`, `"other":0}`, 1), 0, CodeOutput},
		{"summary counts more passes", strings.Replace(pass, `"pass":1`, `"pass":2`, 1), 0, CodeOutput},
		{"summary hides a failure", strings.Replace(fail, `"fail":1`, `"fail":0`, 1), 1, CodeOutput},
		{"summary warn", strings.Replace(pass, `"warn":0`, `"warn":1`, 1), 0, CodeOutput},
		{"other policy", strings.Replace(pass, `"policy":"run-as-non-root"`, `"policy":"other"`, 1), 0, CodeOutput},
		{"no policy", strings.Replace(pass, `"policy":"run-as-non-root",`, "", 1), 0, CodeOutput},
		{"other source", strings.Replace(pass, `"source":"KyvernoValidatingPolicy"`, `"source":"KyvernoPolicy"`, 1), 0, CodeOutput},
		{"warn result", strings.Replace(strings.Replace(pass, `"result":"pass"`, `"result":"warn"`, 1), `"pass":1,"fail":0,"warn":0`, `"pass":0,"fail":0,"warn":1`, 1), 0, CodeOutput},
		{"unknown result", strings.Replace(pass, `"result":"pass"`, `"result":"maybe"`, 1), 0, CodeOutput},
		{"no result", strings.Replace(pass, `"result":"pass",`, "", 1), 0, CodeOutput},
		{"no resource list", strings.Replace(pass, `"resources":[{"kind":"Pod","namespace":"default","name":"good","apiVersion":"v1"}],`, "", 1), 0, CodeOutput},
		{"empty resource list", strings.Replace(pass, `"resources":[{"kind":"Pod","namespace":"default","name":"good","apiVersion":"v1"}]`, `"resources":[]`, 1), 0, CodeOutput},
		{"resource without a name", strings.Replace(pass, `"name":"good"`, `"name":""`, 1), 0, CodeOutput},
		{"resource without a kind", strings.Replace(pass, `"kind":"Pod"`, `"kind":""`, 1), 0, CodeOutput},
		{"two resources in one result", strings.Replace(pass, `"apiVersion":"v1"}]`, `"apiVersion":"v1"},{"kind":"Pod","name":"x"}]`, 1), 0, CodeOutput},
		{"a failure with exit 0", fail, 0, CodeOutput},
		{"a pass with exit 1", pass, 1, CodeOutput},
		{"a skip with exit 1", readOutput(t, "skip"), 1, CodeOutput},
		{"mixed with exit 0", mixed, 0, CodeOutput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReport([]byte(tc.out), tc.exit, policyName)
			requireCode(t, err, tc.code)
			if got != nil {
				t.Fatalf("findings %+v with an error", got)
			}
		})
	}
}

func TestParseReportOnePolicyAmongOthersIsRefused(t *testing.T) {
	// Evaluation runs one policy, so a result for another one means the output is not from this run.
	mixed := strings.Replace(readOutput(t, "mixed"), `"policy":"run-as-non-root"`, `"policy":"other"`, 1)
	_, err := ParseReport([]byte(mixed), 1, policyName)
	requireCode(t, err, CodeOutput)
}

func TestFindingViolationEscapesControls(t *testing.T) {
	got := Finding{Resource: "ns/Pod/a\nb", Message: "line1\nline2\x1b[31m"}.violation()
	if got.Address != `ns/Pod/a\u000ab` || got.Message != `line1\u000aline2\u001b[31m` {
		t.Fatalf("violation %+v", got)
	}
}

func TestResourceLabel(t *testing.T) {
	if got := (resourceItem{Kind: "Namespace", Name: "team-a"}).label(); got != "Namespace/team-a" {
		t.Fatalf("label %q", got)
	}
	if got := (resourceItem{Kind: "Pod", Namespace: "team-a", Name: "api"}).label(); got != "team-a/Pod/api" {
		t.Fatalf("label %q", got)
	}
}
