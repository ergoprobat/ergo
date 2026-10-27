package kyverno

import (
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestCELAllowsStringLiteral(t *testing.T) {
	err := checkExpression(`object.metadata.name == "http"`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCELImageFunction(t *testing.T) {
	if err := checkExpression(`image("nginx:1")`); err != nil {
		t.Fatal(err)
	}
	err := checkExpression(`image.GetMetadata("nginx:1")`)
	if compiler.CodeOf(err) != CodeCEL {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func TestCELRejectsContext(t *testing.T) {
	err := checkExpression(`http.Get("https://example.invalid")`)
	if compiler.CodeOf(err) != CodeCEL {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}
