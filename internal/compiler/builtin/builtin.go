// Package builtin is the compiler registry linked into the ergo binary.
package builtin

import (
	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/archunit"
	"github.com/ergoprobat/ergo/internal/compiler/checkov"
	"github.com/ergoprobat/ergo/internal/compiler/cibinding"
	"github.com/ergoprobat/ergo/internal/compiler/depcruise"
	"github.com/ergoprobat/ergo/internal/compiler/eslint"
	"github.com/ergoprobat/ergo/internal/compiler/importlinter"
	"github.com/ergoprobat/ergo/internal/compiler/kyverno"
	"github.com/ergoprobat/ergo/internal/compiler/opa"
	"github.com/ergoprobat/ergo/internal/compiler/semgrep"
)

// Registry returns the compilers linked into the ergo binary.
// Adding an engine means appending its constructor here.
func Registry() compiler.Registry {
	return compiler.NewRegistry(opa.New(), depcruise.New(), semgrep.New(), cibinding.New(), checkov.New(), archunit.New(), importlinter.New(), kyverno.New(), eslint.New())
}
