package kyverno

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func readGolden(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name, "golden", "0", artifactName))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func requireNotCanonical(t *testing.T, src, mode string) {
	t.Helper()
	_, err := ReadCanonical([]byte(src), mode)
	var coded *compiler.Error
	if !errors.As(err, &coded) || coded.Code != compiler.CodeBuildManifest {
		t.Fatalf("err %v for\n%s", err, src)
	}
}

func TestReadCanonicalReturnsThePolicyName(t *testing.T) {
	name, err := ReadCanonical([]byte(readGolden(t, "example-10")), "block")
	if err != nil {
		t.Fatal(err)
	}
	if name != "run-as-non-root" {
		t.Fatalf("name %q", name)
	}
}

// Every artifact compile writes for a fixture is its own canonical form, in the mode it was compiled for.
func TestReadCanonicalAcceptsEveryGolden(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join("testdata", "*", "golden", "0", artifactName))
	if err != nil || len(goldens) == 0 {
		t.Fatalf("goldens %v %v", goldens, err)
	}
	for _, path := range goldens {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(path)), "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Entries []struct {
				Mode string `json:"mode"`
			} `json:"entries"`
		}
		if err := json.Unmarshal(manifest, &decoded); err != nil || len(decoded.Entries) != 1 {
			t.Fatalf("%s: %v", path, err)
		}
		if _, err := ReadCanonical(raw, decoded.Entries[0].Mode); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

func TestReadCanonicalRefusesAnythingElse(t *testing.T) {
	good := readGolden(t, "example-10")
	expression := "has(object.spec.securityContext) && has(object.spec.securityContext.runAsNonRoot) && object.spec.securityContext.runAsNonRoot == true"
	for name, src := range map[string]string{
		"empty":                "",
		"network call":         strings.Replace(good, expression, "http.get('https://example.invalid').status == 200", 1),
		"cluster state":        strings.Replace(good, expression, "resource.List('v1', 'pods', '').items.size() == 0", 1),
		"image data":           strings.Replace(good, expression, "image.GetMetadata('example.invalid/a:1').config.architecture == 'amd64'", 1),
		"cluster policy":       strings.Replace(good, "apiVersion: policies.kyverno.io/v1\nkind: ValidatingPolicy", "apiVersion: kyverno.io/v1\nkind: ClusterPolicy", 1),
		"other action":         strings.Replace(good, "    - Deny\n", "    - Warn\n", 1),
		"no action":            strings.Replace(good, "  validationActions:\n    - Deny\n", "", 1),
		"extra key":            strings.Replace(good, "spec:\n", "spec:\n  evaluation:\n    mode: JSON\n", 1),
		"unsorted keys":        strings.Replace(good, "kind: ValidatingPolicy\nmetadata:\n  name: run-as-non-root\n", "metadata:\n  name: run-as-non-root\nkind: ValidatingPolicy\n", 1),
		"comment":              "# note\n" + good,
		"crlf":                 strings.ReplaceAll(good, "\n", "\r\n"),
		"no final newline":     strings.TrimSuffix(good, "\n"),
		"extra blank line":     good + "\n",
		"two documents":        good + "---\n" + good,
		"anchor":               strings.Replace(good, "- apiGroups:", "- &a apiGroups:", 1),
		"flow style":           strings.Replace(good, "          - pods\n", "          - [pods]\n", 1),
		"quoted name":          strings.Replace(good, "name: run-as-non-root", "name: 'run-as-non-root'", 1),
		"upper case name":      strings.Replace(good, "name: run-as-non-root", "name: Run-As-Non-Root", 1),
		"expression is a list": strings.Replace(good, "    - expression: "+expression, "    - expression:\n        - x", 1),
	} {
		t.Run(name, func(t *testing.T) { requireNotCanonical(t, src, "block") })
	}
}

// The mode decides validationActions, so an artifact compiled for one mode is not the canonical
// artifact of an entry that names another.
func TestReadCanonicalFollowsTheMode(t *testing.T) {
	for mode, golden := range map[string]string{"warn": "mode-warn", "advise": "mode-advise", "block": "example-10"} {
		if _, err := ReadCanonical([]byte(readGolden(t, golden)), mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		for _, other := range []string{"warn", "advise", "block"} {
			if other != mode {
				requireNotCanonical(t, readGolden(t, golden), other)
			}
		}
	}
	requireNotCanonical(t, readGolden(t, "example-10"), "")
	requireNotCanonical(t, readGolden(t, "example-10"), "deny")
}
