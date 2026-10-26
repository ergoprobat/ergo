package opa

import (
	"os"
	"strings"
	"testing"
)

func TestGatorPinnedVersion(t *testing.T) {
	raw, err := os.ReadFile("testdata/tool/bin.lock")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, name := range []string{
		"gator-" + GatorVersion + "-linux-amd64 ",
		"gator-" + GatorVersion + "-darwin-arm64 ",
	} {
		if !strings.Contains(text, name) {
			t.Fatalf("bin.lock does not pin %s", name)
		}
	}
	if GatorOPA != "v1.17.1" {
		t.Fatalf("capabilities pin %s", GatorOPA)
	}
}
