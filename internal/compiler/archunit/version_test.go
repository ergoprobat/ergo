package archunit

import (
	"os"
	"strings"
	"testing"
)

func TestPinnedVersion(t *testing.T) {
	raw, err := os.ReadFile("testdata/tool/jars.lock")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "archunit-"+version+".jar ") {
		t.Fatalf("jars.lock does not pin archunit %s", version)
	}
	for _, name := range []string{
		"slf4j-api-2.0.19.jar ",
		"junit-platform-console-standalone-6.1.3.jar ",
	} {
		if !strings.Contains(text, name) {
			t.Fatalf("jars.lock does not pin %s", name)
		}
	}
}
