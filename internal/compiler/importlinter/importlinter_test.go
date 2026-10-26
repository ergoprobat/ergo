package importlinter

import (
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler/compilertest"
)

func TestCompile(t *testing.T) {
	compilertest.Run(t, New())
}
