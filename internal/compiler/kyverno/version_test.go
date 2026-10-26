package kyverno

import (
	"os"
	"strings"
	"testing"
)

func TestPinnedVersion(t *testing.T) {
	raw, err := os.ReadFile("testdata/tool/bin.lock")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, name := range []string{
		"kyverno-" + version + "-linux-amd64 ",
		"kyverno-" + version + "-darwin-arm64 ",
	} {
		if !strings.Contains(text, name) {
			t.Fatalf("bin.lock does not pin %s", name)
		}
	}
}
