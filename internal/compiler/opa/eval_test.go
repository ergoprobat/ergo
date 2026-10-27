package opa

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestReadPlan(t *testing.T) {
	plan, err := ReadPlan(strings.NewReader(`{"format_version":"1.2","resource_changes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.SHA256()) != 64 {
		t.Fatalf("hash %q", plan.SHA256())
	}
	again, err := ReadPlan(strings.NewReader(`{"format_version":"1.2","resource_changes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.SHA256() != again.SHA256() {
		t.Fatal("the same bytes hashed differently")
	}

	cases := map[string]string{
		"not json":         "hello",
		"no version":       `{"resource_changes":[]}`,
		"version zero":     `{"format_version":"0.2","resource_changes":[]}`,
		"no changes":       `{"format_version":"1.2"}`,
		"changes not list": `{"format_version":"1.2","resource_changes":{}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ReadPlan(strings.NewReader(body))
			if compiler.CodeOf(err) != CodePlanFormat {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}

func TestReadPlanTooLarge(t *testing.T) {
	body := append([]byte(`{"format_version":"1.2","resource_changes":["`), bytes.Repeat([]byte("x"), MaxPlanBytes)...)
	_, err := ReadPlan(bytes.NewReader(body))
	if compiler.CodeOf(err) != CodePlanTooLarge {
		t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
	}
}

func TestEvalCheckShapes(t *testing.T) {
	plan := planWith(t, `{"format_version":"1.2","resource_changes":[]}`)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	entry := compiler.Entry{}

	cases := []struct {
		name   string
		policy string
		want   int
	}{
		{name: "no denials", policy: `package demo

deny contains msg if { false; msg := "no" }`, want: 0},
		{name: "string", policy: `package demo

deny contains msg if { msg := "blocked" }`, want: 1},
		{name: "object", policy: `package demo

deny contains v if { v := {"msg": "blocked", "address": "aws_s3_bucket.logs"} }`, want: 1},
		{name: "object without address", policy: `package demo

deny contains v if { v := {"msg": "blocked"} }`, want: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			outcome := EvalCheck(entry, map[string][]byte{"policy.rego": []byte(test.policy)}, plan, now)
			if outcome.Err != nil {
				t.Fatal(outcome.Err)
			}
			if len(outcome.Violations) != test.want {
				t.Fatalf("violations %+v", outcome.Violations)
			}
		})
	}

	bad := []string{
		`package demo

deny contains v if { v := {"address": "aws_s3_bucket.logs"} }`,
		`package demo

deny contains v if { v := {"msg": 7} }`,
		`package demo

deny contains v if { v := 7 }`,
		`package demo

deny contains msg if { msg := "" }`,
	}
	for _, policy := range bad {
		outcome := EvalCheck(entry, map[string][]byte{"policy.rego": []byte(policy)}, plan, now)
		if compiler.CodeOf(outcome.Err) != CodeEval {
			t.Fatalf("code %q err %v for %s", compiler.CodeOf(outcome.Err), outcome.Err, policy)
		}
	}
}

func TestEvalCheckWaiver(t *testing.T) {
	plan := planWith(t, `{"format_version":"1.2","resource_changes":[]}`)
	policy := []byte(`package demo

deny contains msg if { msg := "blocked" }`)
	entry := compiler.Entry{Waivers: []compiler.ManifestWaiver{{
		Approver:  "team:platform",
		ValidFrom: "2026-10-01",
		ValidTo:   "2026-11-01",
	}}}
	inside := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	outcome := EvalCheck(entry, map[string][]byte{"policy.rego": policy}, plan, inside)
	if outcome.Err != nil || !outcome.Waived || len(outcome.Violations) != 1 {
		t.Fatalf("waiver did not cover the violation: %+v", outcome)
	}
	outside := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	outcome = EvalCheck(entry, map[string][]byte{"policy.rego": policy}, plan, outside)
	if outcome.Err != nil || len(outcome.Violations) != 1 {
		t.Fatalf("waiver covered a day it does not bind: %+v", outcome)
	}
}

func TestEvalCheckWaiverKeepsErrors(t *testing.T) {
	plan := planWith(t, `{"format_version":"1.2","resource_changes":[]}`)
	policy := []byte(`package demo

deny contains v if { v := 7 }`)
	entry := compiler.Entry{Waivers: []compiler.ManifestWaiver{{
		Approver:  "team:platform",
		ValidFrom: "2026-10-01",
		ValidTo:   "2026-11-01",
	}}}
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	outcome := EvalCheck(entry, map[string][]byte{"policy.rego": policy}, plan, now)
	if compiler.CodeOf(outcome.Err) != CodeEval {
		t.Fatalf("code %q err %v", compiler.CodeOf(outcome.Err), outcome.Err)
	}
}

func TestEvalCheckTimeout(t *testing.T) {
	plan := planWith(t, `{"format_version":"1.2","resource_changes":[]}`)
	policy := []byte(`package demo

deny contains msg if {
	msg := numbers.range(1, 1000000000)[_]
}`)
	start := time.Now()
	outcome := EvalCheck(compiler.Entry{}, map[string][]byte{"policy.rego": policy}, plan, start)
	if compiler.CodeOf(outcome.Err) != CodeEval {
		t.Fatalf("code %q err %v", compiler.CodeOf(outcome.Err), outcome.Err)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatalf("evaluation ran for %s", time.Since(start))
	}
}

func TestEvalCheckDeniedBuiltin(t *testing.T) {
	plan := planWith(t, `{"format_version":"1.2","resource_changes":[]}`)
	policy := []byte("package demo\n\ndeny contains msg if {\n\tresp := http.send({\"method\": \"GET\", \"url\": \"https://example.com\"})\n\tmsg := sprintf(\"%v\", [resp])\n}\n")
	outcome := EvalCheck(compiler.Entry{}, map[string][]byte{"policy.rego": policy}, plan, time.Now())
	if compiler.CodeOf(outcome.Err) != CodeEval {
		t.Fatalf("code %q err %v", compiler.CodeOf(outcome.Err), outcome.Err)
	}
}

func planWith(t *testing.T, body string) Plan {
	t.Helper()
	plan, err := ReadPlan(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
