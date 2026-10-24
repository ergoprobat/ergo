package depcruise

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPinnedVersion(t *testing.T) {
	raw, err := os.ReadFile("testdata/tool/package.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Dependencies["dependency-cruiser"] != version {
		t.Fatalf("package.json dependency-cruiser %q, compiler %s", manifest.Dependencies["dependency-cruiser"], version)
	}

	raw, err = os.ReadFile("testdata/tool/package-lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(raw, &lock); err != nil {
		t.Fatal(err)
	}
	got := lock.Packages["node_modules/dependency-cruiser"].Version
	if got != version {
		t.Fatalf("lockfile dependency-cruiser %q, compiler %s", got, version)
	}
}
