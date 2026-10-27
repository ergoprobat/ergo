// Package eslint compiles an ESLint JSON rule file into one flat config.
package eslint

import (
	"fmt"
	"path"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

const (
	// CodeJSON means ref is not a JSON file, or the file is not one JSON value.
	CodeJSON = "eslint-json"
	// CodeSchema means the object does not match the ESLint rule schema.
	CodeSchema = "eslint-schema"
	// CodeTarget means target is not javascript or typescript.
	CodeTarget = "eslint-target"
	// CodeRule means a rule id is unknown, deprecated, or turned off.
	CodeRule = "eslint-rule"
	// CodePath means a files or ignores pattern leaves the repository.
	CodePath = "eslint-path"

	artifactName     = "eslint.config.mjs"
	version          = "10.13.0"
	targetJavaScript = "javascript"
	targetTypeScript = "typescript"
)

// Compiler is the built-in ESLint engine.
type Compiler struct{}

// New returns the ESLint compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "eslint" }

// Version returns the ESLint release the conformance suite runs.
func (Compiler) Version() string { return version }

// Compile reads the JSON rule file named by ref and writes eslint.config.mjs.
// Severity follows the effective check mode. The config sets noInlineConfig,
// so an inline disable comment cannot turn a rule off.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	mode, err := in.Check.EffectiveMode(in.Record.Criticality)
	if err != nil {
		return nil, err
	}
	if in.Check.Target != targetJavaScript && in.Check.Target != targetTypeScript {
		return nil, fail(CodeTarget, "target %q", in.Check.Target)
	}
	if !strings.EqualFold(path.Ext(in.Check.Ref), ".json") {
		return nil, fail(CodeJSON, "ref %q is not a JSON file", in.Check.Ref)
	}
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	out, err := compileRules(src, in.Check.Target, mode)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: out}}, nil
}

func severityFor(mode string) (string, error) {
	switch mode {
	case "block":
		return "error", nil
	case "warn", "advise":
		return "warn", nil
	default:
		return "", fmt.Errorf("mode %q", mode)
	}
}

func fail(code, format string, args ...any) error {
	return &compiler.Error{Code: code, Err: fmt.Errorf(format, args...)}
}
