package cibinding

import (
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/compilertest"
)

func TestCompile(t *testing.T) {
	compilertest.Run(t, New())
}

func TestEval(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	entry := compiler.Entry{}
	artifacts := map[string][]byte{"binding.json": []byte("{\n  \"job\": \"checkov / CKV_AWS_145\"\n}\n")}
	sha := "0123456789abcdef0123456789abcdef01234567"
	pass := evidence(sha, `{"name":"checkov / CKV_AWS_145","conclusion":"success"}`)
	evidencePass, err := ReadEvidence(strings.NewReader(pass), sha)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := EvalCheck(entry, artifacts, evidencePass, now); outcome.Err != nil || len(outcome.Violations) != 0 {
		t.Fatalf("pass %+v", outcome)
	}
	missing := evidence(sha, `{"name":"other","conclusion":"success"}`)
	evidenceMissing, err := ReadEvidence(strings.NewReader(missing), sha)
	if err != nil {
		t.Fatal(err)
	}
	outcome := EvalCheck(entry, artifacts, evidenceMissing, now)
	if outcome.Err != nil || len(outcome.Violations) != 1 {
		t.Fatalf("missing %+v", outcome)
	}
	failed := evidence(sha, `{"name":"checkov / CKV_AWS_145","conclusion":"failure"},{"name":"checkov / CKV_AWS_145","conclusion":null}`)
	evidenceFailed, err := ReadEvidence(strings.NewReader(failed), sha)
	if err != nil {
		t.Fatal(err)
	}
	outcome = EvalCheck(entry, artifacts, evidenceFailed, now)
	if outcome.Err != nil || len(outcome.Violations) != 2 {
		t.Fatalf("failed %+v", outcome)
	}
	waived := entry
	waived.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-10-01", ValidTo: "2026-11-01"}}
	outcome = EvalCheck(waived, artifacts, evidenceMissing, now)
	if !outcome.Waived || len(outcome.Violations) != 1 {
		t.Fatalf("waiver %+v", outcome)
	}
}

func TestEvidenceRejects(t *testing.T) {
	sha := "0123456789abcdef0123456789abcdef01234567"
	other := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := ReadEvidence(strings.NewReader(evidence(sha, `{"name":"unit","conclusion":"success"}`)), other); compiler.CodeOf(err) != CodeEvidenceSHA {
		t.Fatalf("sha %v", err)
	}
	cases := []string{
		`{"evidence_version":1,"sha":"` + sha + `","jobs":[],"extra":1}`,
		`{"evidence_version":2,"sha":"` + sha + `","jobs":[]}`,
		`{"evidence_version":1,"sha":"ABC","jobs":[]}`,
		`{"evidence_version":1,"sha":"` + sha + `","jobs":[{"name":"unit","conclusion":"FAILED"}]}`,
		`{"evidence_version":1,"sha":"` + sha + `","jobs":[{"name":"bad\nname","conclusion":"success"}]}`,
		`{"evidence_version":1,"sha":"` + sha + `","jobs":[{"name":"unit"}]}`,
	}
	for _, body := range cases {
		if _, err := ReadEvidence(strings.NewReader(body), sha); compiler.CodeOf(err) != CodeEvidenceFormat {
			t.Fatalf("code %q for %s (%v)", compiler.CodeOf(err), body, err)
		}
	}
}

func evidence(sha, jobs string) string {
	return `{"evidence_version":1,"sha":"` + sha + `","jobs":[` + jobs + `]}`
}
