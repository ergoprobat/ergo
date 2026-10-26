// Package importlinter compiles an import-linter INI file into one canonical config.
package importlinter

import (
	"fmt"
	"path"
	"strings"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const (
	// CodeFormat means ref is not an .importlinter file.
	CodeFormat = "importlinter-format"
	// CodeINI means the file is not the INI subset this compiler accepts.
	CodeINI = "importlinter-ini"
	// CodeRoot means a root package is missing, dotted, or not a top-level name.
	CodeRoot = "importlinter-root"
	// CodeContract means a contract is missing, duplicated, or the wrong shape.
	CodeContract = "importlinter-contract"
	// CodeKey means a key is unknown or rejected.
	CodeKey = "importlinter-key"
	// CodeValue means a value does not match the field it belongs to.
	CodeValue = "importlinter-value"

	artifactName = "importlinter.ini"
	version      = "2.15"
)

// Compiler is the built-in import-linter engine.
type Compiler struct{}

// New returns the import-linter compiler.
func New() Compiler { return Compiler{} }

// Engine returns the EDR engine value this compiler handles.
func (Compiler) Engine() string { return "importlinter" }

// Version returns the import-linter release this compiler validates against.
func (Compiler) Version() string { return version }

// Compile reads the INI file named by ref and writes importlinter.ini.
// The artifact does not depend on the check mode. target is not read.
// The framework stamps both on the manifest unchanged.
func (Compiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	if _, err := in.Check.EffectiveMode(in.Record.Criticality); err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(in.Check.Ref), ".importlinter") {
		return nil, fail(CodeFormat, "ref %q is not an import-linter file", in.Check.Ref)
	}
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	out, err := compileConfig(src)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: artifactName, Data: out}}, nil
}

func fail(code, format string, args ...any) error {
	return &compiler.Error{Code: code, Err: fmt.Errorf(format, args...)}
}
