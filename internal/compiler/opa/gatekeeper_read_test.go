package opa

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

type goldenPair struct {
	template   string
	constraint string
	mode       string
	dir        string
}

// goldenPairs returns every Gatekeeper artifact pair that compile writes for a fixture, with the mode it
// was compiled for.
func goldenPairs(t *testing.T) []goldenPair {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "*", "golden", "0", templateArtifact))
	if err != nil || len(matches) == 0 {
		t.Fatalf("goldens %v %v", matches, err)
	}
	var pairs []goldenPair
	for _, path := range matches {
		dir := filepath.Dir(path)
		template, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		constraint, err := os.ReadFile(filepath.Join(dir, constraintArtifact))
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "manifest.json"))
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
		pairs = append(pairs, goldenPair{string(template), string(constraint), decoded.Entries[0].Mode, dir})
	}
	return pairs
}

func ownerPair(t *testing.T, mode string) (template, constraint string) {
	t.Helper()
	for _, pair := range goldenPairs(t) {
		if strings.Contains(pair.template, "kind: RequireOwner") && pair.mode == mode {
			return pair.template, pair.constraint
		}
	}
	t.Fatalf("no RequireOwner golden for mode %s", mode)
	return "", ""
}

func requireGatekeeperNotCanonical(t *testing.T, template, constraint, mode string) {
	t.Helper()
	_, err := ReadGatekeeper([]byte(template), []byte(constraint), mode)
	var coded *compiler.Error
	if !errors.As(err, &coded) || coded.Code != compiler.CodeBuildManifest {
		t.Fatalf("err %v for\n%s\n---\n%s", err, template, constraint)
	}
}

// Every artifact pair compile writes for a fixture is its own canonical form, in the mode it was compiled for.
func TestReadGatekeeperAcceptsEveryGolden(t *testing.T) {
	for _, pair := range goldenPairs(t) {
		if _, err := ReadGatekeeper([]byte(pair.template), []byte(pair.constraint), pair.mode); err != nil {
			t.Fatalf("%s: %v", pair.dir, err)
		}
	}
}

func TestReadGatekeeperReturnsTheConstraint(t *testing.T) {
	for mode, action := range map[string]string{"block": "deny", "warn": "warn", "advise": "dryrun"} {
		template, constraint := ownerPair(t, mode)
		got, err := ReadGatekeeper([]byte(template), []byte(constraint), mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if got != (Gatekeeper{Kind: "RequireOwner", Name: "owner", Action: action}) {
			t.Fatalf("%s: %#v", mode, got)
		}
	}
}

// A constraint compiled for one mode is not the canonical form for another.
func TestReadGatekeeperRefusesAnotherMode(t *testing.T) {
	template, constraint := ownerPair(t, "block")
	requireGatekeeperNotCanonical(t, template, constraint, "warn")
	requireGatekeeperNotCanonical(t, template, constraint, "advise")
	requireGatekeeperNotCanonical(t, template, constraint, "unknown")
}

func TestReadGatekeeperRefusesAHandEdit(t *testing.T) {
	template, constraint := ownerPair(t, "block")
	cases := map[string][2]string{
		"unsorted keys":        {template, strings.Replace(constraint, "kind: RequireOwner\nmetadata:", "metadata:", 1) + "kind: RequireOwner\n"},
		"comment":              {template, "# edited\n" + constraint},
		"extra constraint key": {template, strings.Replace(constraint, "spec:\n", "spec:\n  parameters:\n    x: y\n", 1)},
		"enforcement action":   {template, strings.Replace(constraint, "enforcementAction: deny", "enforcementAction: dryrun", 1)},
		"scoped action":        {template, strings.Replace(constraint, "spec:\n", "spec:\n  scopedEnforcementActions: []\n", 1)},
		"another kind":         {template, strings.Replace(constraint, "kind: RequireOwner", "kind: Other", 1)},
		"no trailing newline":  {template, strings.TrimSuffix(constraint, "\n")},
		"empty template":       {"", constraint},
		"empty constraint":     {template, ""},
		"swapped artifacts":    {constraint, template},
		"two documents":        {template + "---\n" + template, constraint},
		"not yaml":             {template, "{"},
	}
	for name, pair := range cases {
		t.Run(name, func(t *testing.T) {
			requireGatekeeperNotCanonical(t, pair[0], pair[1], "block")
		})
	}
}

// A template that calls the network or reads cluster state is never run, even when its digest is in the manifest.
func TestReadGatekeeperRefusesTemplatesCompileRejects(t *testing.T) {
	template, constraint := ownerPair(t, "block")
	for name, rego := range map[string]string{
		"http.send":      "msg := http.send({\"method\": \"get\", \"url\": \"https://example.invalid\"}).status",
		"data.inventory": "data.inventory.cluster",
	} {
		t.Run(name, func(t *testing.T) {
			edited := strings.Replace(template, "not input.review.object.metadata.labels.owner", rego, 1)
			if edited == template {
				t.Fatal("the golden template changed")
			}
			requireGatekeeperNotCanonical(t, edited, constraint, "block")
		})
	}
}
