package importlinter

import (
	"bytes"
	"errors"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// Contract is one contract of the compiled config.
type Contract struct {
	ID   string
	Name string
	Type string
}

// Canonical is what evaluation needs to know about a compiled importlinter.ini.
type Canonical struct {
	// Roots are the root packages, in file order.
	Roots []string
	// Contracts are the contracts, in file order. import-linter reports them in that order.
	Contracts []Contract
}

var errNotCanonical = errors.New("importlinter.ini is not the config ipso compiles")

// ReadCanonical checks that src is exactly what compile writes and recovers the roots and contracts.
// Compile is idempotent on its own output, so compiling src again must give the same bytes. Anything
// else, such as a hand-edited file with a contract class or a dotted root, is a build-manifest error
// and is never run. The digest check alone would accept a manifest and an artifact edited together.
func ReadCanonical(src []byte) (Canonical, error) {
	again, err := compileConfig(src)
	if err != nil || !bytes.Equal(again, src) {
		return Canonical{}, notCanonical()
	}
	file, err := parseINI(src)
	if err != nil {
		return Canonical{}, notCanonical()
	}
	roots, err := listItems(file.session["root_packages"])
	if err != nil {
		return Canonical{}, notCanonical()
	}
	contracts := make([]Contract, len(file.contracts))
	for i, contract := range file.contracts {
		contracts[i] = Contract{
			ID:   contract.id,
			Name: contract.values["name"].scalar,
			Type: contract.values["type"].scalar,
		}
	}
	return Canonical{Roots: roots, Contracts: contracts}, nil
}

func notCanonical() error {
	return &compiler.Error{Code: compiler.CodeBuildManifest, Err: errNotCanonical}
}
