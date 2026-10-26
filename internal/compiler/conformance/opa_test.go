//go:build conformance

package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/opa"
	"github.com/ergoprobat/ergo/internal/edr"
)

func TestOPA(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range templateIDs(t, opaTemplates, ".rego", "lib/") {
		t.Run(id, func(t *testing.T) {
			artifacts := compileOPA(t, id)
			root := filepath.Join(opaCorpus, filepath.FromSlash(id))
			for _, name := range jsonFiles(t, filepath.Join(root, "pass")) {
				outcome := evaluateOPA(t, artifacts, filepath.Join(root, "pass", name), now)
				if outcome.Err != nil || outcome.Waived || len(outcome.Violations) != 0 {
					t.Errorf("pass %s err %v waived %v violations %d", name, outcome.Err, outcome.Waived, len(outcome.Violations))
				}
			}
			for _, name := range jsonFiles(t, filepath.Join(root, "fail")) {
				outcome := evaluateOPA(t, artifacts, filepath.Join(root, "fail", name), now)
				if outcome.Err != nil || outcome.Waived || len(outcome.Violations) == 0 {
					t.Errorf("fail %s err %v waived %v violations %d", name, outcome.Err, outcome.Waived, len(outcome.Violations))
				}
			}
		})
	}
}

func compileOPA(t *testing.T, id string) map[string][]byte {
	t.Helper()
	engine := opa.New()
	in := compiler.Input{
		Record: edr.Record{
			ID:          "01M5WNCV78TVJ7WKE5BV520C9Z",
			Slug:        "conformance",
			Status:      "accepted",
			Criticality: "blocking",
			ValidFrom:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Check: edr.Check{Engine: "opa", Target: "terraform-plan", Ref: "ergo:" + id, Mode: "block"},
	}
	artifacts, err := engine.Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, artifact := range artifacts {
		out[artifact.Name] = artifact.Data
	}
	if _, ok := out["policy.rego"]; !ok {
		t.Fatal("missing policy.rego")
	}
	if _, ok := out["terraform.rego"]; !ok {
		t.Fatal("missing terraform.rego")
	}
	return out
}

func evaluateOPA(t *testing.T, artifacts map[string][]byte, path string, now time.Time) compiler.Outcome {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := opa.ReadPlan(bytes.NewReader(withFormatVersion(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return opa.EvalCheck(compiler.Entry{}, artifacts, plan, now)
}

// withFormatVersion lets ReadPlan accept a corpus document.
// The files are resource_changes objects. The splice adds the version field and leaves every other byte in place.
func withFormatVersion(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Contains(trimmed, []byte(`"format_version"`)) || !bytes.HasPrefix(trimmed, []byte("{")) {
		return trimmed
	}
	out := make([]byte, 0, len(trimmed)+len(`"format_version":"1.0",`))
	out = append(out, '{')
	out = append(out, `"format_version":"1.0",`...)
	out = append(out, trimmed[1:]...)
	return out
}
