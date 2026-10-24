package checkov

import (
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/compilertest"
)

func TestCompile(t *testing.T) {
	compilertest.Run(t, New())
}

func TestEval(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	entry := compiler.Entry{}
	artifacts := map[string][]byte{"binding.json": []byte("{\n  \"check_id\": \"CKV_AWS_24\"\n}\n")}
	pass, err := ReadResults(strings.NewReader(`{"check_type":"terraform","results":{"passed_checks":[{"check_id":"CKV_AWS_24","resource":"aws_security_group.admin","file_path":"/main.tf"}],"failed_checks":[],"skipped_checks":[],"parsing_errors":[]},"summary":{"passed":1,"failed":0,"skipped":0,"parsing_errors":0,"checkov_version":"3.2.495"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := EvalCheck(entry, artifacts, pass, now); outcome.Err != nil || len(outcome.Violations) != 0 {
		t.Fatalf("pass %+v", outcome)
	}
	failed, err := ReadResults(strings.NewReader(`{"check_type":"terraform","results":{"passed_checks":[],"failed_checks":[{"check_id":"CKV_AWS_24","resource":"aws_security_group.admin","file_path":"/main.tf"}],"skipped_checks":[{"check_id":"CKV_AWS_24","resource":"aws_security_group.other","file_path":"/skip.tf"}],"parsing_errors":[]},"summary":{"parsing_errors":0,"checkov_version":"3.2.495"}}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome := EvalCheck(entry, artifacts, failed, now)
	if outcome.Err != nil || len(outcome.Violations) != 2 || !strings.Contains(outcome.Violations[0].Message, "failed CKV_AWS_24") {
		t.Fatalf("failed %+v", outcome)
	}
	absent, err := ReadResults(strings.NewReader(`{"passed":0,"failed":0,"skipped":0,"parsing_errors":0,"checkov_version":"3.2.495"}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome = EvalCheck(entry, artifacts, absent, now)
	if outcome.Err != nil || len(outcome.Violations) != 1 || !strings.Contains(outcome.Violations[0].Message, "not reported") {
		t.Fatalf("absent %+v", outcome)
	}
	parsed, err := ReadResults(strings.NewReader(`{"check_type":"terraform","results":{"passed_checks":[],"failed_checks":[],"skipped_checks":[],"parsing_errors":["main.tf"]},"summary":{"parsing_errors":1,"checkov_version":"3.2.495"}}`))
	if err != nil {
		t.Fatal(err)
	}
	outcome = EvalCheck(entry, artifacts, parsed, now)
	if compiler.CodeOf(outcome.Err) != CodeFormat {
		t.Fatalf("parse %+v", outcome)
	}
	waived := entry
	waived.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-06-01", ValidTo: "2026-06-30"}}
	inside := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	outcome = EvalCheck(waived, artifacts, failed, inside)
	if !outcome.Waived {
		t.Fatalf("waiver %+v", outcome)
	}
	outcome = EvalCheck(waived, artifacts, parsed, inside)
	if outcome.Err == nil || outcome.Waived {
		t.Fatalf("waiver covered an error: %+v", outcome)
	}
}

func TestControlCharacters(t *testing.T) {
	got := escapeControls("a\nb")
	if got != "a\\u000ab" {
		t.Fatalf("escaped %q", got)
	}
}
