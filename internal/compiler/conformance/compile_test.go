//go:build conformance

package conformance

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/builtin"
	"github.com/ipsofacto-io/ipso/internal/edr"
)

// compiledChecks returns the artifacts of every check for engine, in check order.
func compiledChecks(t *testing.T, dir, engine string) [][]compiler.Artifact {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "record.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := edr.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if problems := edr.Validate(doc); len(problems) != 0 {
		t.Fatal(strings.Join(problems, "; "))
	}
	rec, err := edr.RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Join(dir, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	tool, ok := builtin.Registry().Lookup(engine)
	if !ok {
		t.Fatalf("no %s compiler", engine)
	}
	var out [][]compiler.Artifact
	for index, check := range rec.Checks {
		if check.Engine != engine {
			continue
		}
		in := compiler.Input{Record: rec, Check: check, Index: index, Repo: root.FS()}
		artifacts, err := tool.Compile(in)
		if err != nil {
			t.Fatal(err)
		}
		if len(artifacts) == 0 {
			t.Fatalf("check %d has no artifacts", index)
		}
		out = append(out, artifacts)
	}
	if len(out) == 0 {
		t.Fatalf("no %s check", engine)
	}
	return out
}

func artifactData(t *testing.T, artifacts []compiler.Artifact, name string) []byte {
	t.Helper()
	for _, artifact := range artifacts {
		if artifact.Name == name {
			return artifact.Data
		}
	}
	t.Fatalf("missing artifact %s", name)
	return nil
}

func sameArtifacts(t *testing.T, left, right []compiler.Artifact) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("artifacts %d and %d", len(left), len(right))
	}
	for i := range left {
		if left[i].Name != right[i].Name || !bytes.Equal(left[i].Data, right[i].Data) {
			t.Fatalf("artifact %s differs", left[i].Name)
		}
	}
}
