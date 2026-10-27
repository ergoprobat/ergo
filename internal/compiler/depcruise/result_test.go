package depcruise

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestVersionFits(t *testing.T) {
	if !VersionFits("18.7.0", "18.7.0") || !VersionFits("18.8.0", "18.7.0") || !VersionFits("18.7.1", "18.7.0") {
		t.Fatal("expected compatible versions")
	}
	for _, found := range []string{"19.0.0", "18.6.9", "18.7", "v18.7.0", "nope"} {
		if VersionFits(found, "18.7.0") {
			t.Fatalf("accepted %s", found)
		}
	}
}

func TestParseCaptured(t *testing.T) {
	clean := readOutput(t, "clean.json")
	got, err := ParseResult(clean, version)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != version || got.Cruised == 0 || len(got.Violations) != 0 {
		t.Fatalf("clean %+v", got)
	}

	bad := readOutput(t, "violating.json")
	got, err = ParseResult(bad, version)
	if err != nil {
		t.Fatal(err)
	}
	want := []Violation{
		{Rule: "no-circular", From: "packages/ui/src/a.ts", To: "packages/ui/src/b.ts", Severity: "error"},
		{Rule: "ui-not-to-db", From: "packages/ui/src/index.ts", To: "packages/db/src/index.ts", Severity: "error"},
	}
	if len(got.Violations) != len(want) {
		t.Fatalf("violations %+v", got.Violations)
	}
	for i := range want {
		if got.Violations[i] != want[i] {
			t.Fatalf("violation %d %+v", i, got.Violations[i])
		}
	}

	_, err = ParseResult(readOutput(t, "environment.json"), version)
	if compiler.CodeOf(err) != CodeEnvironment {
		t.Fatalf("environment err %v", err)
	}
}

func TestParseResultEdges(t *testing.T) {
	report := func(body string) []byte {
		return []byte(`{"summary":{"violations":` + body + `,"totalCruised":2,"environment":{"version":"18.7.0"}}}`)
	}
	got, err := ParseResult(report(`[{"from":"a\u0001b","to":"a\u0001b","rule":{"name":"solo","severity":"warn"}},{"from":"c","to":"d","rule":{"name":"old","severity":"ignore"}}]`), "18.7.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Violations) != 1 || got.Violations[0].message() != `solo a\u0001b` {
		t.Fatalf("violations %+v message %q", got.Violations, got.Violations[0].message())
	}

	_, err = ParseResult([]byte(`{"summary":{"violations":[],"totalCruised":0,"environment":{"version":"18.7.0"}}}`), "18.7.0")
	if compiler.CodeOf(err) != CodeEmpty {
		t.Fatalf("empty err %v", err)
	}
	_, err = ParseResult([]byte(`{"summary":{"violations":[],"totalCruised":1,"environment":{"version":"18.8.0"}}}`), "18.7.0")
	if compiler.CodeOf(err) != CodeVersion {
		t.Fatalf("version err %v", err)
	}
	_, err = ParseResult([]byte(`not-json`), "18.7.0")
	if compiler.CodeOf(err) != CodeOutput {
		t.Fatalf("json err %v", err)
	}
	_, err = ParseResult([]byte(`{"summary":{"totalCruised":1,"environment":{"version":"18.7.0"}}}`), "18.7.0")
	if compiler.CodeOf(err) != CodeOutput {
		t.Fatalf("missing violations err %v", err)
	}
}

func TestEvalCheck(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	raw := []byte(`{"summary":{"violations":[{"from":"secret-module.ts","to":"other.ts","rule":{"name":"secret-rule","severity":"error"}}],"totalCruised":2,"environment":{"version":"18.7.0"}}}`)
	entry := compiler.Entry{CompilerVersion: version, Mode: "block"}
	artifacts := map[string][]byte{artifactName: []byte("{}")}

	outcome := EvalCheck(entry, artifacts, "19.0.0", raw, now)
	if compiler.CodeOf(outcome.Err) != CodeVersion || outcome.Waived {
		t.Fatalf("version %+v", outcome)
	}
	outcome = EvalCheck(entry, artifacts, version, raw, now)
	if outcome.Err != nil || outcome.Waived || len(outcome.Violations) != 1 || outcome.Violations[0].Message != "secret-rule secret-module.ts -> other.ts" {
		t.Fatalf("fail %+v", outcome)
	}

	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	outcome = EvalCheck(entry, artifacts, version, raw, now)
	if outcome.Err != nil || !outcome.Waived || len(outcome.Violations) != 1 {
		t.Fatalf("waived %+v", outcome)
	}

	outcome = EvalCheck(entry, artifacts, version, readOutput(t, "environment.json"), now)
	if compiler.CodeOf(outcome.Err) != CodeEnvironment || outcome.Waived {
		t.Fatalf("environment %+v", outcome)
	}
	outcome = EvalCheck(entry, map[string][]byte{}, version, raw, now)
	if compiler.CodeOf(outcome.Err) != compiler.CodeBuildManifest {
		t.Fatalf("artifact %+v", outcome)
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
