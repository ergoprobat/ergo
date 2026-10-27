package strictyaml

import (
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestTwoDocuments(t *testing.T) {
	src := []byte("a: 1\n---\nb: 2\n")
	docs, err := Documents(src, "yaml", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].Content[1].Value != "1" || docs[1].Content[1].Value != "2" {
		t.Fatalf("documents %#v", docs)
	}
	if _, err := Documents(src, "yaml", 1); compiler.CodeOf(err) != "yaml" {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
	many := []byte("a: 1\n---\nb: 2\n---\nc: 3\n")
	if _, err := Documents(many, "yaml", 2); compiler.CodeOf(err) != "yaml" {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func TestRejectsAlias(t *testing.T) {
	src := []byte("a: &x 1\nb: *x\n")
	_, err := Documents(src, "yaml", 1)
	if compiler.CodeOf(err) != "yaml" {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
	if !strings.Contains(err.Error(), "alias") && !strings.Contains(err.Error(), "anchor") {
		t.Fatalf("error %v", err)
	}
}
