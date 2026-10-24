// Package builtin is the compiler registry linked into the ergo binary.
package builtin

import (
	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/opa"
)

// Registry returns the compilers linked into the ergo binary.
// Adding an engine means appending its constructor here.
func Registry() compiler.Registry {
	return compiler.NewRegistry(opa.New())
}
