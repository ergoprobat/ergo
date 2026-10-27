// Package kyverno compiles a Kyverno ValidatingPolicy into one canonical policy.
package kyverno

import (
	"fmt"
	"path"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

const (
	// CodeYAML means ref is not a YAML file, or the file is not one plain YAML document.
	CodeYAML = "kyverno-yaml"
	// CodeKind means the document is not a policies.kyverno.io/v1 ValidatingPolicy.
	CodeKind = "kyverno-kind"
	// CodePolicy means the policy is missing a required field or has the wrong shape.
	CodePolicy = "kyverno-policy"
	// CodeKey means the policy sets a key this compiler rejects.
	CodeKey = "kyverno-key"
	// CodeCEL means a CEL expression does not parse, or it names a context variable
	// that reaches the network, a registry, or cluster state.
	CodeCEL = "kyverno-cel"

	artifactName = "kyverno.yaml"
	version      = "1.19.2"
)

// Compiler is the built-in Kyverno engine.
type Compiler struct{}

// New returns the Kyverno compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "kyverno" }

// Version returns the Kyverno CLI release the conformance suite runs.
func (Compiler) Version() string { return version }

// Compile reads the ValidatingPolicy named by ref and writes kyverno.yaml.
// validationActions follows the effective check mode. target is not read.
// The framework stamps it on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	mode, err := in.Check.EffectiveMode(in.Record.Criticality)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(path.Ext(in.Check.Ref))
	if ext != ".yaml" && ext != ".yml" {
		return nil, fail(CodeYAML, "ref %q is not a YAML file", in.Check.Ref)
	}
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	out, err := compilePolicy(src, mode)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: out}}, nil
}

func fail(code, format string, args ...any) error {
	return &compiler.Error{Code: code, Err: fmt.Errorf(format, args...)}
}
