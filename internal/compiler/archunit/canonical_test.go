package archunit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// goldenTests are the fixtures whose golden directory holds a generated test.
var goldenTests = []string{"billing-clean", "billing-violating", "canonical", "example-15", "layers", "mode-warn"}

func goldenTest(t *testing.T, name string) (decisionID string, source []byte) {
	t.Helper()
	dir := filepath.Join("testdata", name, "golden")
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Entries []struct {
			DecisionID string `json:"decision_id"`
			CheckIndex int    `json:"check_index"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Entries) != 1 || manifest.Entries[0].CheckIndex != 0 {
		t.Fatalf("%s: entries %+v", name, manifest.Entries)
	}
	source, err = os.ReadFile(filepath.Join(dir, "0", "ArchitectureTest.java.fixture"))
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Entries[0].DecisionID, source
}

func TestReadCanonicalGolden(t *testing.T) {
	for _, name := range goldenTests {
		t.Run(name, func(t *testing.T) {
			id, source := goldenTest(t, name)
			got, err := ReadCanonical(id, 0, source)
			if err != nil {
				t.Fatal(err)
			}
			if want := "ipso.d" + strings.ToLower(id) + ".c0.ArchitectureTest"; got.Class != want {
				t.Fatalf("class %q, want %q", got.Class, want)
			}
			if len(got.Packages) == 0 || len(got.Rules) == 0 {
				t.Fatalf("canonical %+v", got)
			}
		})
	}
}

func TestReadCanonicalBilling(t *testing.T) {
	id, source := goldenTest(t, "billing-violating")
	got, err := ReadCanonical(id, 0, source)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Packages, ",") != "com.acme.billing" {
		t.Fatalf("packages %v", got.Packages)
	}
	if strings.Join(got.Rules, ",") != "domain-not-to-adapters,layers" {
		t.Fatalf("rules %v", got.Rules)
	}
	if len(got.Layers) != 1 || got.Layers["layers"] != 3 {
		t.Fatalf("layers %v", got.Layers)
	}
}

// A different package list is still a file the generator writes, so it is canonical.
// The check refuses Java the generator would not write. It does not pin the rule set:
// the manifest digest does that.
func TestReadCanonicalAcceptsAnotherGeneratedRuleSet(t *testing.T) {
	id, source := goldenTest(t, "billing-violating")
	other := strings.Replace(string(source), `.importPackages("com.acme.billing")`, `.importPackages("com.acme.other", "com.acme.more")`, 1)
	got, err := ReadCanonical(id, 0, []byte(other))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Packages, ",") != "com.acme.other,com.acme.more" {
		t.Fatalf("packages %v", got.Packages)
	}
}

func TestReadCanonicalRejectsTamperedSource(t *testing.T) {
	id, source := goldenTest(t, "billing-violating")
	text := string(source)
	tampered := map[string]string{
		"empty":            "",
		"junk":             "class ArchitectureTest { }\n",
		"trailing comment": text + "// extra\n",
		"missing newline":  strings.TrimSuffix(text, "\n"),
		"injected call": strings.Replace(text, "    @BeforeAll\n",
			"    static { Runtime.getRuntime().exit(0); }\n\n    @BeforeAll\n", 1),
		"injected import": strings.Replace(text, "import org.junit.jupiter.api.Test;\n",
			"import org.junit.jupiter.api.Test;\nimport java.io.File;\n", 1),
		"string breakout": strings.Replace(text, `.importPackages("com.acme.billing")`,
			`.importPackages("com.acme.billing"); Runtime.getRuntime().exit(0); ("")`, 1),
		"other decision": strings.Replace(text, "ipso.d01m5wvyech306k6ddjxr2vapcs.c0",
			"ipso.d01m5wvyech306k6ddjxr2vapct.c0", 1),
		"allow empty": strings.Replace(text, `                .as("layers")`,
			"                .allowEmptyShould(true)\n                .as(\"layers\")", 1),
		"second import": strings.Replace(text, "    }\n\n    @Test\n",
			"        classes = new ClassFileImporter().importPackages(\"x\");\n    }\n\n    @Test\n", 1),
		"no rule name": strings.Replace(text, `                .as("layers")`+"\n", "", 1),
		"no rules":     text[:strings.Index(text, "    @Test\n")] + "}\n",
		"bad kind":     strings.Replace(text, "        layeredArchitecture()\n", "        somethingElse()\n", 1),
		"crlf":         strings.ReplaceAll(text, "\n", "\r\n"),
	}
	for name, src := range tampered {
		t.Run(name, func(t *testing.T) {
			if src == text {
				t.Fatal("tampering left the source unchanged")
			}
			_, err := ReadCanonical(id, 0, []byte(src))
			if compiler.CodeOf(err) != compiler.CodeBuildManifest {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
			if strings.Contains(err.Error(), "Runtime") {
				t.Fatalf("error repeats the artifact: %v", err)
			}
		})
	}
}

func TestReadCanonicalRejectsOtherIdentity(t *testing.T) {
	id, source := goldenTest(t, "billing-violating")
	for name, c := range map[string]struct {
		id    string
		index int
	}{
		"other index":      {id, 1},
		"negative index":   {id, -1},
		"lowercase id":     {strings.ToLower(id), 0},
		"not a ulid":       {"../../etc/passwd", 0},
		"empty id":         {"", 0},
		"id with newline":  {id + "\n", 0},
		"other valid ulid": {"01M5WVYECH306K6DDJXR2VAPCT", 0},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ReadCanonical(c.id, c.index, source)
			if compiler.CodeOf(err) != compiler.CodeBuildManifest {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}
