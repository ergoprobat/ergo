// Package semgrep compiles a Semgrep YAML rule file into one canonical rule file.
package semgrep

import (
	"fmt"
	"path"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

const (
	// CodeYAML means ref is not a YAML file, or the file is not one plain YAML document.
	CodeYAML = "semgrep-yaml"
	// CodeRules means the document root is not one non-empty rules list.
	CodeRules = "semgrep-rules"
	// CodeRule means a rule is missing a required field or has the wrong shape.
	CodeRule = "semgrep-rule"
	// CodeRuleID means a rule id is missing, invalid, or duplicated.
	CodeRuleID = "semgrep-rule-id"
	// CodeKey means a rule sets a key or mode this compiler rejects.
	CodeKey = "semgrep-key"

	version = "1.182.1"
)

// ArtifactName is the compiled Semgrep rule file.
const ArtifactName = "semgrep.yaml"

// Compiler is the built-in Semgrep engine.
type Compiler struct{}

// New returns the Semgrep compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "semgrep" }

// Version returns the Semgrep release the fixture job runs.
func (Compiler) Version() string { return version }

// Compile writes semgrep.yaml for one rule file.
// A ref that starts with ipso: selects a built-in template.
// Any other ref is one repository .yaml or .yml file.
// Severity follows the effective check mode. target is not read.
// The framework stamps it on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	mode, err := in.Check.EffectiveMode(in.Record.Criticality)
	if err != nil {
		return nil, err
	}
	src, err := source(in)
	if err != nil {
		return nil, err
	}
	out, err := compileConfig(src, mode)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: ArtifactName, Data: out}}, nil
}

func source(in compiler.Input) ([]byte, error) {
	if strings.HasPrefix(in.Check.Ref, compiler.TemplatePrefix) {
		id := strings.TrimPrefix(in.Check.Ref, compiler.TemplatePrefix)
		src, ok := lookup(id)
		if !ok {
			return nil, &compiler.Error{Code: compiler.CodeTemplateUnknown, Err: fmt.Errorf("template %q", id)}
		}
		return src, nil
	}
	ext := strings.ToLower(path.Ext(in.Check.Ref))
	if ext != ".yaml" && ext != ".yml" {
		return nil, &compiler.Error{Code: CodeYAML, Err: fmt.Errorf("ref %q is not a YAML file", in.Check.Ref)}
	}
	return compiler.ReadRef(in)
}

func severityFor(mode string) (string, error) {
	switch mode {
	case "block":
		return "ERROR", nil
	case "warn":
		return "WARNING", nil
	case "advise":
		return "INFO", nil
	default:
		return "", fmt.Errorf("mode %q", mode)
	}
}
