package eslint

import (
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestNestedProtoKey(t *testing.T) {
	src := []byte(`{"files":["a.js"],"rules":{"eqeqeq":["error",{"__proto__":{"x":1}}]}}`)
	_, err := compileRules(src, "javascript", "block")
	if compiler.CodeOf(err) != CodeJSON {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}
