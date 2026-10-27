package depcruise

import (
	"errors"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/compilertest"
)

func TestCompile(t *testing.T) {
	compilertest.Run(t, New())
}

func TestSchemaLoadError(t *testing.T) {
	err := schemaLoadError(errors.New("depcruise schema: broken"))
	if compiler.CodeOf(err) != CodeSchema {
		t.Fatalf("code %q", compiler.CodeOf(err))
	}
	coded := &compiler.Error{Code: CodeSchema, Err: errors.New("already coded")}
	if schemaLoadError(coded) != coded {
		t.Fatal("coded error was replaced")
	}
	if schemaLoadError(nil) != nil {
		t.Fatal("nil error became an error")
	}
}
