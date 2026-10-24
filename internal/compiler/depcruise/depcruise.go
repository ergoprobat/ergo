// Package depcruise compiles a dependency-cruiser JSON rule file into one canonical config.
package depcruise

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const (
	// CodeJSON means ref is not a JSON file or the file is not one JSON object.
	CodeJSON = "depcruise-json"
	// CodeSchema means the object does not match the dependency-cruiser configuration schema.
	CodeSchema = "depcruise-schema"
	// CodeOption means the file sets extends or an option this compiler rejects.
	CodeOption = "depcruise-option"
	// CodePath means a path option leaves the repository or does not name an existing path.
	CodePath = "depcruise-path"
	// CodeNoRules means the file has no forbidden, allowed, or required rule.
	CodeNoRules = "depcruise-no-rules"
	// CodeRuleName means a forbidden or required rule has no name, or two rules share a name.
	CodeRuleName = "depcruise-rule-name"

	artifactName = "depcruise.json"
	version      = "18.7.0"
)

// Compiler is the built-in dependency-cruiser engine.
type Compiler struct{}

// New returns the dependency-cruiser compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "depcruise" }

// Version returns the dependency-cruiser release this compiler validates against.
func (Compiler) Version() string { return version }

// Compile reads the JSON rule file named by ref and writes depcruise.json.
// Severity follows the effective check mode. A severity of ignore is left as ignore.
// target is not read. The framework stamps it on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	mode, err := in.Check.EffectiveMode(in.Record.Criticality)
	if err != nil {
		return nil, err
	}
	out, err := compileConfig(src, in.Check.Ref, in.Repo, mode)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: out}}, nil
}

func compileConfig(src []byte, ref string, repo fs.FS, mode string) ([]byte, error) {
	if path.Ext(ref) != ".json" {
		return nil, &compiler.Error{Code: CodeJSON, Err: fmt.Errorf("ref %q is not a JSON file", ref)}
	}
	doc, err := decodeObject(src)
	if err != nil {
		return nil, err
	}
	if err := validateSchema(doc); err != nil {
		return nil, err
	}
	if err := rejectOptions(doc); err != nil {
		return nil, err
	}
	if err := checkPaths(doc, repo); err != nil {
		return nil, err
	}
	if err := checkRules(doc); err != nil {
		return nil, err
	}
	if err := rewriteSeverity(doc, mode); err != nil {
		return nil, err
	}
	out, err := canonical(doc)
	if err != nil {
		return nil, fmt.Errorf("encode depcruise config: %w", err)
	}
	return out, nil
}

func severityFor(mode string) (string, error) {
	switch mode {
	case "block":
		return "error", nil
	case "warn":
		return "warn", nil
	case "advise":
		return "info", nil
	default:
		return "", fmt.Errorf("mode %q", mode)
	}
}

func canonical(doc map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
