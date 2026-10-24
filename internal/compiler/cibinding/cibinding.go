// Package cibinding compiles a ci-binding check into the job name ergo verifies.
package cibinding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const (
	// CodeJobRequired means the check has no job.
	CodeJobRequired = "job-required"
	// CodeJobInvalid means job is not valid UTF-8, contains a control character, or has outer space.
	CodeJobInvalid = "job-invalid"
	// CodeEvidenceFormat means the evidence file is not evidence version 1.
	CodeEvidenceFormat = "evidence-format"
	// CodeEvidenceTooLarge means the evidence file is larger than compiler.MaxRefBytes.
	CodeEvidenceTooLarge = "evidence-too-large"
	// CodeEvidenceSHA means the evidence sha is not the sha the command was given.
	CodeEvidenceSHA = "evidence-sha"

	artifactName = "binding.json"
	version      = "1"
)

// Compiler is the built-in ci-binding engine.
type Compiler struct{}

// New returns the ci-binding compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "ci-binding" }

// Version returns the evidence contract version. Compile does not call a CI provider.
func (Compiler) Version() string { return version }

// Compile writes binding.json with the job name.
// target and ref are ignored. The framework stamps target on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	job := in.Check.Job
	if job == "" {
		return nil, &compiler.Error{Code: CodeJobRequired, Err: fmt.Errorf("ci-binding requires job")}
	}
	if !validJob(job) {
		return nil, &compiler.Error{Code: CodeJobInvalid, Err: fmt.Errorf("job %q", job)}
	}
	raw, err := marshalBinding(binding{Job: job})
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: raw}}, nil
}

type binding struct {
	Job string `json:"job"`
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

func validJob(job string) bool {
	if !utf8.ValidString(job) || job != strings.TrimSpace(job) {
		return false
	}
	for _, r := range job {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
