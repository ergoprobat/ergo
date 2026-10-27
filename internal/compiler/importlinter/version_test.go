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
		pin, err := requirementPin(path, "import-linter")
		if err != nil {
			t.Fatal(err)
		}
		if pin != version {
			t.Fatalf("%s import-linter %q, compiler %s", path, pin, version)
		}
	}
}

// grimp finds every import, so the verdict depends on its release as much as on import-linter's.
// Only the lockfile names it, because requirements.in pins import-linter alone.
func TestPinnedGrimpVersion(t *testing.T) {
	pin, err := requirementPin("testdata/tool/requirements.txt", "grimp")
	if err != nil {
		t.Fatal(err)
	}
	if pin != grimpVersion {
		t.Fatalf("requirements.txt grimp %q, evaluation %s", pin, grimpVersion)
	}
}

func requirementPin(path, name string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	prefix := name + "=="
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		pin := strings.TrimPrefix(line, prefix)
		pin, _, _ = strings.Cut(pin, " ")
		pin, _, _ = strings.Cut(pin, "\\")
		return pin, nil
	}
	return "", os.ErrNotExist
}
