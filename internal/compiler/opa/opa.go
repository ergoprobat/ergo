// Package opa compiles Rego policies and the built-in Terraform plan templates.
package opa

import (
	"fmt"
	"strings"

	"github.com/open-policy-agent/opa/v1/version"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const (
	// CodeTemplateUnknown means ref names an ergo: template that is not built in.
	CodeTemplateUnknown = "template-unknown"
	// CodeTemplateTarget means an ergo: template check does not target a Terraform plan.
	CodeTemplateTarget = "template-target"
	// CodeRegoParse means the Rego module could not be parsed as Rego v1.
	CodeRegoParse = "rego-parse"
	// CodeRegoCompile means the Rego module failed OPA's compile checks.
	CodeRegoCompile = "rego-compile"
	// CodeRegoImport means the module imports a data package other than its own.
	CodeRegoImport = "rego-import"
	// CodeRegoNoDeny means the module has no multi-value deny rule.
	CodeRegoNoDeny = "rego-no-deny"

	templatePrefix = "ergo:"
	planTarget     = "terraform-plan"
	policyName     = "policy.rego"
	libName        = "terraform.rego"
)

// Compiler is the built-in OPA engine.
type Compiler struct{}

// New returns the OPA compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "opa" }

// Version returns the embedded OPA version.
func (Compiler) Version() string { return version.Version }

// Compile writes policy.rego for one Rego module.
// A ref that starts with ergo: selects a built-in template and also writes terraform.rego.
// Any other ref is one repository file.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	if strings.HasPrefix(in.Check.Ref, templatePrefix) {
		return compileTemplate(in)
	}
	return compileRepo(in)
}

func compileTemplate(in compiler.Input) ([]compiler.Artifact, error) {
	if in.Check.Target != planTarget {
		return nil, &compiler.Error{Code: CodeTemplateTarget, Err: fmt.Errorf("target %q", in.Check.Target)}
	}
	id := strings.TrimPrefix(in.Check.Ref, templatePrefix)
	src, ok := lookup(id)
	if !ok {
		return nil, &compiler.Error{Code: CodeTemplateUnknown, Err: fmt.Errorf("template %q", id)}
	}
	lib := libSource()
	if err := vet(id+".rego", src, true, lib); err != nil {
		return nil, err
	}
	return []compiler.Artifact{
		{Name: policyName, Data: src},
		{Name: libName, Data: lib},
	}, nil
}

func compileRepo(in compiler.Input) ([]compiler.Artifact, error) {
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	if err := vet(in.Check.Ref, src, false, nil); err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: policyName, Data: src}}, nil
}
