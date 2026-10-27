package semgrep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestParseCaptured(t *testing.T) {
	clean, err := ParseResult(readOutput(t, "clean.json"), version)
	if err != nil || clean.Version != version || clean.Scanned == 0 || len(clean.Findings) != 0 {
		t.Fatalf("clean %+v err %v", clean, err)
	}
	bad, err := ParseResult(readOutput(t, "violating.json"), version)
	if err != nil || len(bad.Findings) != 2 {
		t.Fatalf("violating %+v err %v", bad, err)
	}
	for _, finding := range bad.Findings {
		if finding.Rule != "no-eval" || finding.Line < 1 {
			t.Fatalf("finding %+v", finding)
		}
	}
	_, err = ParseResult(readOutput(t, "errors.json"), version)
	if compiler.CodeOf(err) != CodeIncomplete {
		t.Fatalf("errors err %v", err)
	}
}

func readOutput(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "output", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestParseResultEdges(t *testing.T) {
	clean := `{"version":"1.182.1","results":[],"errors":[],"paths":{"scanned":["app.py"]},"skipped_rules":[]}`
	got, err := ParseResult([]byte(clean), "1.182.1")
	if err != nil || got.Scanned != 1 || len(got.Findings) != 0 {
		t.Fatalf("clean %+v err %v", got, err)
	}

	hit := `{"version":"1.182.1","results":[{"check_id":"no-eval","path":"a\u0001.py","start":{"line":4},"extra":{"message":"secret"}}],"errors":[],"paths":{"scanned":["a.py"]},"skipped_rules":[]}`
	got, err = ParseResult([]byte(hit), "1.182.1")
	if err != nil || len(got.Findings) != 1 || got.Findings[0].message() != `no-eval a\u0001.py:4` {
		t.Fatalf("hit %+v err %v", got, err)
	}

	_, err = ParseResult([]byte(`{"version":"1.183.0","results":[],"errors":[],"paths":{"scanned":["a.py"]},"skipped_rules":[]}`), "1.182.1")
	if compiler.CodeOf(err) != CodeVersion {
		t.Fatalf("version err %v", err)
	}
	_, err = ParseResult([]byte(`{"version":"1.182.1","results":[],"errors":[{"message":"parse"}],"paths":{"scanned":[]},"skipped_rules":[]}`), "1.182.1")
	if compiler.CodeOf(err) != CodeIncomplete {
		t.Fatalf("errors err %v", err)
	}
	_, err = ParseResult([]byte(`{"version":"1.182.1","results":[],"errors":[],"paths":{"scanned":[]},"skipped_rules":[{"rule_id":"no-eval"}]}`), "1.182.1")
	if compiler.CodeOf(err) != CodeIncomplete {
		t.Fatalf("skipped err %v", err)
	}
	_, err = ParseResult([]byte(`{"version":"1.182.1","results":[],"errors":[],"paths":{"scanned":[]},"skipped_rules":[]}`), "1.182.1")
	if compiler.CodeOf(err) != CodeEmpty {
		t.Fatalf("empty err %v", err)
	}
	_, err = ParseResult([]byte(`not-json`), "1.182.1")
	if compiler.CodeOf(err) != CodeOutput {
		t.Fatalf("json err %v", err)
	}
	_, err = ParseResult([]byte(`{"version":"1.182.1","results":[{"check_id":"","path":"a.py","start":{"line":1}}],"errors":[],"paths":{"scanned":["a.py"]},"skipped_rules":[]}`), "1.182.1")
	if compiler.CodeOf(err) != CodeOutput {
		t.Fatalf("finding err %v", err)
	}
	_, err = ParseResult([]byte(`{"version":"1.182.1","results":[{"check_id":"no-eval","path":"a.py","start":{"line":1}}],"errors":[],"paths":{"scanned":[]},"skipped_rules":[]}`), "1.182.1")
	if compiler.CodeOf(err) != CodeOutput {
		t.Fatalf("findings without scanned files err %v", err)
	}
	_, err = ParseResult([]byte(`{"version":"1.182.1","results":[],"errors":[],"paths":{"scanned":["a.py"]}}`), "1.182.1")
	if compiler.CodeOf(err) != CodeOutput {
		t.Fatalf("missing skipped_rules err %v", err)
	}
}

func TestEvalCheck(t *testing.T) {
	raw := []byte(`{"version":"1.182.1","results":[{"check_id":"no-eval","path":"app.py","start":{"line":2}}],"errors":[],"paths":{"scanned":["app.py"]},"skipped_rules":[]}`)
	entry := compiler.Entry{CompilerVersion: version, Mode: "block"}
	artifacts := map[string][]byte{ArtifactName: []byte("rules:\n")}
	outcome := EvalCheck(entry, artifacts, version, raw, time.Now())
	if outcome.Err != nil || len(outcome.Violations) != 1 || outcome.Violations[0].Message != "no-eval app.py:2" {
		t.Fatalf("outcome %+v", outcome)
	}

	outcome = EvalCheck(entry, artifacts, "1.183.0", raw, time.Now())
	if compiler.CodeOf(outcome.Err) != CodeVersion {
		t.Fatalf("version %+v", outcome)
	}
	outcome = EvalCheck(entry, nil, version, raw, time.Now())
	if compiler.CodeOf(outcome.Err) != compiler.CodeBuildManifest {
		t.Fatalf("artifact %+v", outcome)
	}

	waived := compiler.Entry{
		CompilerVersion: version,
		Mode:            "block",
		Waivers: []compiler.ManifestWaiver{{
			ValidFrom: "2026-01-01",
			ValidTo:   "2026-03-01",
		}},
	}
	now := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	outcome = EvalCheck(waived, artifacts, version, raw, now)
	if !outcome.Waived || len(outcome.Violations) != 1 {
		t.Fatalf("waiver %+v", outcome)
	}
	broken := []byte(`not-json`)
	outcome = EvalCheck(waived, artifacts, version, broken, now)
	if outcome.Waived || compiler.CodeOf(outcome.Err) != CodeOutput {
		t.Fatalf("error waiver %+v", outcome)
	}
}

func TestRejectedOptions(t *testing.T) {
	if len(allowedOptionKeys) != 39 {
		t.Fatalf("option keys %d", len(allowedOptionKeys))
	}
	base := "rules:\n  - id: sample\n    message: sample rule\n    languages:\n      - python\n    pattern: foo(...)\n    options:\n      %s: true\n"
	for _, key := range []string{"interfile", "taint_match_on", "commutative_compop", "not_a_real_option"} {
		_, err := compileConfig([]byte(strings.Replace(base, "%s", key, 1)), "warn")
		if compiler.CodeOf(err) != CodeKey || !strings.Contains(err.Error(), "options."+key) {
			t.Fatalf("%s err %v", key, err)
		}
	}
	_, err := compileConfig([]byte(strings.Replace(base, "%s: true", "constant_propagation: false", 1)), "warn")
	if err != nil {
		t.Fatal(err)
	}
}
