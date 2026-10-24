package compiler

import (
	"bytes"
	"strings"
	"testing"

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
		Record: edr.Record{ID: "01M5C5QEHHNXJ3D7X2ZCMGXQT7", Slug: "tls", Criticality: "warning"},
		Check:  edr.Check{Engine: "opa"},
		Index:  1,
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
	if !strings.Contains(text, `"entries": []`) && strings.Contains(text, "null") {
		t.Fatalf("nil artifacts became null: %s", text)
	}
}

func TestRegistryFirstWins(t *testing.T) {
	reg := NewRegistry(stubCompiler{engine: "opa", version: "1"}, stubCompiler{engine: "opa", version: "2"})
	got, ok := reg.Lookup("opa")
	if !ok || got.Version() != "1" {
		t.Fatalf("lookup %+v %v", got, ok)
	}
	if _, ok := Builtin().Lookup("opa"); ok {
		t.Fatal("builtin registry should be empty")
	}
}
