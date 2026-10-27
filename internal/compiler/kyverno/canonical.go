package kyverno

import (
	"bytes"
	"errors"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/strictyaml"
)

var errNotCanonical = errors.New("kyverno.yaml is not the policy ipso compiles")

// ReadCanonical checks that src is exactly what compile writes for mode and returns the policy name.
// Compile is idempotent on its own output, so compiling src again with the same mode must give the
// same bytes. Anything else, such as a hand-edited policy that calls http or reads the cluster, is a
// build-manifest error and is never run. The digest check alone would accept a manifest and an
// artifact edited together.
func ReadCanonical(src []byte, mode string) (string, error) {
	again, err := compilePolicy(src, mode)
	if err != nil || !bytes.Equal(again, src) {
		return "", notCanonical()
	}
	docs, err := strictyaml.Documents(src, CodeYAML, 1)
	if err != nil {
		return "", notCanonical()
	}
	name := child(child(docs[0], "metadata"), "name")
	if !isString(name) {
		return "", notCanonical()
	}
	return name.Value, nil
}

func notCanonical() error {
	return &compiler.Error{Code: compiler.CodeBuildManifest, Err: errNotCanonical}
}
