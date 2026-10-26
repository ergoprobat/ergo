//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler/opa"
)

func TestGatekeeper(t *testing.T) {
	stdout, stderr, exit := gatorOutput(t, 30*time.Second, "", "--version")
	if exit != 0 {
		t.Fatalf("version exit %d\n%s\n%s", exit, stdout, stderr)
	}
	text := string(stdout)
	if !strings.Contains(text, "gator version v"+opa.GatorVersion) || !strings.Contains(text, "opa/"+opa.GatorOPA) {
		t.Fatalf("version %s", stdout)
	}

	clean := compiledChecks(t, gatekeeperClean, "opa")
	bad := compiledChecks(t, gatekeeperBad, "opa")
	sameArtifacts(t, clean[0], bad[0])
	template := artifactData(t, clean[0], "constraint-template.yaml")
	constraint := artifactData(t, clean[0], "constraint.yaml")
	if got := gatorViolations(t, template, constraint, gatekeeperClean); len(got) != 0 {
		t.Fatalf("clean %#v", got)
	}
	got := gatorViolations(t, template, constraint, gatekeeperBad)
	if len(got) != 1 || got[0].Msg != "missing owner label" || got[0].Kind != "RequireOwner" || got[0].Name != "owner" || got[0].Action != "deny" {
		t.Fatalf("violating %#v", got)
	}
}

type gatorViolation struct {
	Msg    string
	Kind   string
	Name   string
	Action string
}

func gatorViolations(t *testing.T, template, constraint []byte, subject string) []gatorViolation {
	t.Helper()
	dir := copyFixtureTree(t, filepath.Join(subject, "repo"))
	templatePath := filepath.Join(dir, "constraint-template.yaml")
	constraintPath := filepath.Join(dir, "constraint.yaml")
	if err := os.WriteFile(templatePath, template, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(constraintPath, constraint, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := gatorOutput(t, 60*time.Second, dir,
		"test",
		"-f", templatePath,
		"-f", constraintPath,
		"-f", filepath.Join(dir, "apps", "api.yaml"),
		"-o", "json",
	)
	if bytes.Equal(bytes.TrimSpace(stdout), []byte("null")) {
		if exit != 0 {
			t.Fatalf("no violations exit %d\n%s", exit, stderr)
		}
		return nil
	}
	var parsed []struct {
		Msg        string `json:"msg"`
		Action     string `json:"enforcementAction"`
		Constraint struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"constraint"`
	}
	if err := json.Unmarshal(stdout, &parsed); err != nil {
		t.Fatalf("decode gator: %v\n%s\n%s", err, stdout, stderr)
	}
	if len(parsed) > 0 && exit == 0 {
		t.Fatalf("violations exited 0 %#v", parsed)
	}
	out := make([]gatorViolation, len(parsed))
	for i, item := range parsed {
		out[i] = gatorViolation{Msg: item.Msg, Kind: item.Constraint.Kind, Name: item.Constraint.Metadata.Name, Action: item.Action}
	}
	return out
}
