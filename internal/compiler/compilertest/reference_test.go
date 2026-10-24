package compilertest

import (
	"flag"
	"os"
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestReference(t *testing.T) {
	Run(t, referenceCompiler{})
}

// referenceCompiler packages the file named by ref. It is not linked into the binary.
// Engine is opa so the fixture stays a valid EDR file. The built-in OPA compiler is internal/compiler/opa.
type referenceCompiler struct{}

func (referenceCompiler) Engine() string  { return "opa" }
func (referenceCompiler) Version() string { return "0" }
func (referenceCompiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	data, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: in.Check.Ref, Data: data}}, nil
}
