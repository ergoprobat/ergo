package compiler

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/edr"
)

type stubCompiler struct {
	engine  string
	version string
}

func (s stubCompiler) Engine() string  { return s.engine }
func (s stubCompiler) Version() string { return s.version }
func (s stubCompiler) Compile(Input) ([]Artifact, error) {
	return nil, nil
}

func TestAssembleSortsAndStampsMode(t *testing.T) {
	in := Input{
		Record: edr.Record{
			ID: "01M5C5QEHHNXJ3D7X2ZCMGXQT7", Slug: "tls", Criticality: "warning",
			ValidFrom: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			Scope:     []edr.ScopeItem{{Env: []string{"prod"}}, {}},
			Waivers: []edr.Waiver{{
				Approver:  "team:platform",
				ValidFrom: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
				ValidTo:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			}},
		},
		Check: edr.Check{Engine: "opa", Target: "terraform-plan"},
		Index: 1,
	}
	entry, artifacts, err := Assemble(stubCompiler{engine: "opa", version: "test"}, in, []Artifact{
		{Name: "b.txt", Data: []byte("b")},
		{Name: "a.txt", Data: []byte("a")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.Mode != "warn" || entry.Compiler != "opa" || entry.CompilerVersion != "test" || entry.CheckIndex != 1 {
		t.Fatalf("entry %+v", entry)
	}
	if entry.Target != "terraform-plan" || entry.ValidFrom != "2026-01-15" || entry.ValidTo != nil {
		t.Fatalf("validity %+v", entry)
	}
	if entry.Env != nil {
		t.Fatalf("env %+v, a scope item with no env applies everywhere", entry.Env)
	}
	if len(entry.Waivers) != 1 || entry.Waivers[0].Approver != "team:platform" || entry.Waivers[0].ValidTo != "2026-04-01" {
		t.Fatalf("waivers %+v", entry.Waivers)
	}
	if artifacts[0].Name != "a.txt" || artifacts[1].Name != "b.txt" {
		t.Fatalf("order %+v", artifacts)
	}
	raw, err := MarshalManifest([]Entry{entry})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"mode": "warn"`)) || !strings.Contains(string(raw), "a.txt") {
		t.Fatalf("manifest %s", raw)
	}
	again, err := MarshalManifest([]Entry{entry})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatal("manifest was not stable")
	}
}

func TestAssembleRejectsNames(t *testing.T) {
	in := Input{Record: edr.Record{Criticality: "blocking"}, Check: edr.Check{Engine: "opa"}}
	engine := stubCompiler{engine: "opa", version: "0"}
	tests := []struct {
		name      string
		artifacts []Artifact
	}{
		{name: "parent", artifacts: []Artifact{{Name: "../secret", Data: []byte("x")}}},
		{name: "empty", artifacts: []Artifact{{Name: "", Data: []byte("x")}}},
		{name: "duplicate", artifacts: []Artifact{{Name: "a.txt", Data: []byte("x")}, {Name: "a.txt", Data: []byte("y")}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := Assemble(engine, in, test.artifacts)
			if CodeOf(err) != CodeArtifactName {
				t.Fatalf("code %q err %v", CodeOf(err), err)
			}
		})
	}
}

func TestMarshalManifestOrdersEntries(t *testing.T) {
	raw, err := MarshalManifest([]Entry{
		{DecisionID: "b", CheckIndex: 0, Artifacts: nil},
		{DecisionID: "a", CheckIndex: 1},
		{DecisionID: "a", CheckIndex: 0, Artifacts: []ManifestArtifact{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Index(text, `"decision_id": "a"`) > strings.Index(text, `"decision_id": "b"`) {
		t.Fatalf("order %s", text)
	}
	first := strings.Index(text, `"check_index": 0`)
	second := strings.Index(text, `"check_index": 1`)
	if first < 0 || second < 0 || first > second {
		t.Fatalf("check order %s", text)
	}
	if strings.Contains(text, `"artifacts": null`) {
		t.Fatalf("nil artifacts became null: %s", text)
	}
}

func TestEnvList(t *testing.T) {
	got := envList(edr.Record{Scope: []edr.ScopeItem{
		{Env: []string{"prod", "stage"}},
		{Env: []string{"prod", "dev"}},
	}})
	if strings.Join(got, ",") != "dev,prod,stage" {
		t.Fatalf("env %v", got)
	}
	if envList(edr.Record{}) != nil {
		t.Fatal("no scope should apply everywhere")
	}
}

func TestRegistryFirstWins(t *testing.T) {
	reg := NewRegistry(stubCompiler{engine: "opa", version: "1"}, stubCompiler{engine: "opa", version: "2"})
	got, ok := reg.Lookup("opa")
	if !ok || got.Version() != "1" {
		t.Fatalf("lookup %+v %v", got, ok)
	}
}
