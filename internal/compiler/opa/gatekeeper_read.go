package opa

import (
	"bytes"
	"errors"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/strictyaml"
)

var errGatekeeperNotCanonical = errors.New("the template and constraint are not what ipso compiles")

// Gatekeeper names the constraint a verified build holds, as gator reports it.
type Gatekeeper struct {
	// Kind is the constraint kind, the same as the template's CRD kind.
	Kind string
	// Name is the constraint's metadata.name.
	Name string
	// Action is the enforcementAction the mode gives: deny, warn, or dryrun.
	Action string
}

// ReadGatekeeper checks that template and constraint are exactly what compile writes for mode and
// returns the constraint's identity. Compile is idempotent on its own output, so compiling the two
// artifacts again with the same mode must give the same bytes. Anything else, such as a hand-edited
// template that calls http.send or reads data.inventory, is a build-manifest error and is never run.
// The digest check alone would accept a manifest and an artifact edited together.
func ReadGatekeeper(template, constraint []byte, mode string) (Gatekeeper, error) {
	templateDocs, err := strictyaml.Documents(template, CodeGatekeeperYAML, 1)
	if err != nil {
		return Gatekeeper{}, gatekeeperNotCanonical()
	}
	constraintDocs, err := strictyaml.Documents(constraint, CodeGatekeeperYAML, 1)
	if err != nil {
		return Gatekeeper{}, gatekeeperNotCanonical()
	}
	built, err := buildGatekeeper(templateDocs[0], constraintDocs[0], mode)
	if err != nil || !bytes.Equal(built.template, template) || !bytes.Equal(built.constraint, constraint) {
		return Gatekeeper{}, gatekeeperNotCanonical()
	}
	return Gatekeeper{Kind: built.kind, Name: built.name, Action: built.action}, nil
}

func gatekeeperNotCanonical() error {
	return &compiler.Error{Code: compiler.CodeBuildManifest, Err: errGatekeeperNotCanonical}
}
