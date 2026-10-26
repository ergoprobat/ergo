package opa

import (
	"strings"
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const ownerRego = `package requireowner

violation contains {"msg": msg} if {
	not input.review.object.metadata.labels.owner
	msg := "missing owner label"
}
`

func TestGatekeeperRego(t *testing.T) {
	if err := vetGatekeeper(ownerRego, nil); err != nil {
		t.Fatal(err)
	}
}

func TestGatekeeperRejectsRegoV1Import(t *testing.T) {
	src := strings.Replace(ownerRego, "package requireowner\n", "package requireowner\n\nimport rego.v1\n", 1)
	err := vetGatekeeper(src, nil)
	if compiler.CodeOf(err) != CodeRegoImport {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func TestGatekeeperRejectsInventory(t *testing.T) {
	src := strings.Replace(ownerRego, "not input.review.object.metadata.labels.owner", "data.inventory.cluster", 1)
	err := vetGatekeeper(src, nil)
	if compiler.CodeOf(err) != CodeRegoImport {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}
