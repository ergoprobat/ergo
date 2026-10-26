package importlinter

import (
	"os"
	"strings"
	"testing"
)

func TestPinnedVersion(t *testing.T) {
	for _, path := range []string{
		"testdata/tool/requirements.in",
		"testdata/tool/requirements.txt",
	} {
		pin, err := importLinterPin(path)
		if err != nil {
			t.Fatal(err)
		}
		if pin != version {
			t.Fatalf("%s import-linter %q, compiler %s", path, pin, version)
		}
	}
}

func importLinterPin(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "import-linter==") {
			continue
		}
		pin := strings.TrimPrefix(line, "import-linter==")
		pin, _, _ = strings.Cut(pin, " ")
		pin, _, _ = strings.Cut(pin, "\\")
		return pin, nil
	}
	return "", os.ErrNotExist
}
