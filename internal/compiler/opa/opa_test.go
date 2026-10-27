package opa

import (
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler/compilertest"
)

func TestCompile(t *testing.T) {
	compilertest.Run(t, New())
}
