// Package archunit compiles an ArchUnit JSON rule file into one JUnit test.
package archunit

import (
	"fmt"
	"path"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

const (
	// CodeJSON means ref is not a JSON file, or the file is not one JSON object.
	CodeJSON = "archunit-json"
	// CodeSchema means the object does not match the ArchUnit rule schema.
	CodeSchema = "archunit-schema"
	// CodeRuleName means two rules share a name.
	CodeRuleName = "archunit-rule-name"
	// CodeLayerName means two layers in one rule share a name.
	CodeLayerName = "archunit-layer-name"

	artifactName = "ArchitectureTest.java"
	version      = "1.5.1"
)

// Compiler is the built-in ArchUnit engine.
type Compiler struct{}

// New returns the ArchUnit compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "archunit" }

// Version returns the ArchUnit release the generated test is written for.
func (Compiler) Version() string { return version }

// Compile reads the JSON rule file named by ref and writes ArchitectureTest.java.
// The artifact does not depend on the check mode. target is not read.
// The framework stamps both on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	if _, err := in.Check.EffectiveMode(in.Record.Criticality); err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(in.Check.Ref), ".json") {
		return nil, fail(CodeJSON, "ref %q is not a JSON file", in.Check.Ref)
	}
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	out, err := compileRules(src, in.Record.ID, in.Index)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: out}}, nil
}

func fail(code, format string, args ...any) error {
	return &compiler.Error{Code: code, Err: fmt.Errorf(format, args...)}
}
