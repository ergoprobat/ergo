// Package checkov compiles a Checkov check id into a binding ergo evaluates offline.
package checkov

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const (
	// CodeID means ref is not a Checkov check id.
	CodeID = "checkov-id"
	// CodeFormat means the Checkov JSON is not a report ergo can read.
	CodeFormat = "checkov-format"
	// CodeTooLarge means the Checkov JSON is larger than MaxReportBytes.
	CodeTooLarge = "checkov-too-large"

	// MaxReportBytes is the largest Checkov report the evaluator reads.
	MaxReportBytes = 256 << 20

	artifactName = "binding.json"
	// version is the Checkov release whose JSON output is captured under testdata.
	version = "3.2.495"
)

var checkIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*(_[A-Z0-9]+)+$`)

// Compiler is the built-in Checkov engine.
type Compiler struct{}

// New returns the Checkov compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "checkov" }

// Version returns the Checkov release this compiler's captured reports came from.
func (Compiler) Version() string { return version }

// Compile writes binding.json with the check id in ref.
// target and job are ignored. The framework stamps target on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	id := in.Check.Ref
	if id == "" {
		return nil, &compiler.Error{Code: compiler.CodeRefRequired, Err: fmt.Errorf("checkov requires ref")}
	}
	if len(id) > 64 || !checkIDPattern.MatchString(id) {
		return nil, &compiler.Error{Code: CodeID, Err: fmt.Errorf("ref %q", id)}
	}
	raw, err := marshalBinding(binding{CheckID: id})
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: raw}}, nil
}

type binding struct {
	CheckID string `json:"check_id"`
}

func marshalBinding(value binding) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
