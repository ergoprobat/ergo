// Package compiler turns in-force EDR checks into engine artifacts.
package compiler

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/ipsofacto-io/ipso/internal/edr"
)

const (
	// CodeRefRequired means the check has no ref.
	CodeRefRequired = "ref-required"
	// CodeRefOutside means ref leaves the repository, by .., an absolute path, or a symlink.
	CodeRefOutside = "ref-outside-repo"
	// CodeRefMissing means ref names a file that is not in the repository.
	CodeRefMissing = "ref-missing"
	// CodeRefTooLarge means ref is larger than MaxRefBytes.
	CodeRefTooLarge = "ref-too-large"

	// MaxRefBytes is the largest file a ref may read.
	// The cap is the compile budget for a repository policy: the parser never sees a larger file.
	MaxRefBytes = 1 << 20
	// CodeArtifactName means an artifact path is empty, duplicated, or not a relative path.
	CodeArtifactName = "artifact-name"
	// CodeDuplicateID means two in-force records share a decision id.
	CodeDuplicateID = "duplicate-id"

	// ReportCompiled is a check an engine compiled.
	ReportCompiled = "compiled"
	// ReportAdvisory is an llm-advisory check, which is never compiled.
	ReportAdvisory = "advisory"
	// ReportNoCompiler means no compiler is registered for the engine.
	ReportNoCompiler = "no-compiler"

	advisoryEngine = "llm-advisory"
)

// Compiler is one engine. Adding an engine is a package, a registry line, and fixtures.
type Compiler interface {
	Engine() string
	Version() string
	Compile(in Input) ([]Artifact, error)
}

// Input is one check from one record. Repo is rooted at the repository and cannot escape it.
type Input struct {
	Record edr.Record
	Check  edr.Check
	Index  int
	Repo   fs.FS
}

// Artifact is one file a compiler emits. Name is relative to the check directory.
type Artifact struct {
	Name string
	Data []byte
}

// Error is a compile failure with a stable reason code.
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// CodeOf returns the reason code carried by err, or an empty string.
func CodeOf(err error) string {
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Code
	}
	return ""
}

// Report is the outcome of one check, in check order.
type Report struct {
	Index  int
	Engine string
	Kind   string
	Count  int
}

// Compiled is one check that produced artifacts.
type Compiled struct {
	Entry     Entry
	Artifacts []Artifact
}

// DecisionRef identifies an in-force record. Path is the display path.
type DecisionRef struct {
	Path string
	ID   string
}

// CompileRecord compiles every check on rec. llm-advisory is reported and not compiled.
// A check whose engine has no compiler is reported and skipped. The first compile error stops the record.
func CompileRecord(reg Registry, rec edr.Record, repo fs.FS) ([]Compiled, []Report, error) {
	var compiled []Compiled
	reports := make([]Report, 0, len(rec.Checks))
	for i, check := range rec.Checks {
		if check.Engine == advisoryEngine {
			reports = append(reports, Report{Index: i, Engine: check.Engine, Kind: ReportAdvisory})
			continue
		}
		engine, ok := reg.Lookup(check.Engine)
		if !ok {
			reports = append(reports, Report{Index: i, Engine: check.Engine, Kind: ReportNoCompiler})
			continue
		}
		in := Input{Record: rec, Check: check, Index: i, Repo: repo}
		artifacts, err := engine.Compile(in)
		if err != nil {
			return nil, nil, fmt.Errorf("checks[%d] (%s): %w", i, check.Engine, err)
		}
		entry, sorted, err := Assemble(engine, in, artifacts)
		if err != nil {
			return nil, nil, fmt.Errorf("checks[%d] (%s): %w", i, check.Engine, err)
		}
		compiled = append(compiled, Compiled{Entry: entry, Artifacts: sorted})
		reports = append(reports, Report{Index: i, Engine: check.Engine, Kind: ReportCompiled, Count: len(sorted)})
	}
	return compiled, reports, nil
}
