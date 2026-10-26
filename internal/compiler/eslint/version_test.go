package eslint

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
	if manifest.Dependencies["eslint"] != version {
		t.Fatalf("package.json eslint %q, compiler %s", manifest.Dependencies["eslint"], version)
	}
	if manifest.Dependencies["typescript-eslint"] != "8.72.1" || manifest.Dependencies["typescript"] != "6.0.3" {
		t.Fatalf("package.json typescript pins %#v", manifest.Dependencies)
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
	pins := map[string]string{
		"node_modules/eslint":            version,
		"node_modules/typescript-eslint": "8.72.1",
		"node_modules/typescript":        "6.0.3",
	}
	for name, want := range pins {
		if lock.Packages[name].Version != want {
			t.Fatalf("lockfile %s %q, want %s", name, lock.Packages[name].Version, want)
		}
	}
}

func TestRuleLists(t *testing.T) {
	if _, ok := coreRules["eqeqeq"]; !ok {
		t.Fatal("eqeqeq missing")
	}
	if _, ok := coreRules["indent"]; ok {
		t.Fatal("deprecated indent is allowed")
	}
	if _, ok := typescriptRules["@typescript-eslint/no-floating-promises"]; !ok {
		t.Fatal("no-floating-promises missing")
	}
	if _, ok := typescriptRules["@typescript-eslint/no-var-requires"]; ok {
		t.Fatal("deprecated no-var-requires is allowed")
	}
}
