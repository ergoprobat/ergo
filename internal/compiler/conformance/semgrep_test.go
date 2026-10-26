//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/semgrep"
	"github.com/ergoprobat/ergo/internal/edr"
)

func TestSemgrep(t *testing.T) {
	stdout, stderr, exit := commandOutput(t, 30*time.Second, "", "semgrep", "--version")
	if exit != 0 {
		t.Fatalf("semgrep --version exit %d\n%s\n%s", exit, stdout, stderr)
	}
	requireVersion(t, stdout, semgrep.New().Version())

	for _, id := range templateIDs(t, semgrepTemplates, ".yaml", "") {
		t.Run(id, func(t *testing.T) {
			config := compileSemgrep(t, id)
			ruleID := strings.ReplaceAll(id, "/", ".")
			for _, file := range semgrepFixtureFiles(t, id) {
				body, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				negative, positive := annotatedLines(string(body), ruleID)
				if len(negative) == 0 && len(positive) == 0 {
					t.Fatalf("%s has no subjects", file)
				}
				lines := scanSemgrep(t, config, file, ruleID)
				for _, line := range negative {
					if _, ok := lines[line]; !ok {
						t.Errorf("%s ruleid line %d was not a finding", filepath.Base(file), line)
					}
				}
				for _, line := range positive {
					if _, ok := lines[line]; ok {
						t.Errorf("%s ok line %d was a finding", filepath.Base(file), line)
					}
				}
			}
		})
	}
}

func compileSemgrep(t *testing.T, id string) []byte {
	t.Helper()
	artifacts, err := semgrep.New().Compile(compiler.Input{
		Record: edr.Record{Criticality: "blocking"},
		Check:  edr.Check{Engine: "semgrep", Ref: "ergo:" + id, Mode: "block"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "semgrep.yaml" {
		t.Fatalf("artifacts %#v", artifacts)
	}
	return artifacts[0].Data
}

func scanSemgrep(t *testing.T, config []byte, fixture, ruleID string) map[int]struct{} {
	t.Helper()
	root := t.TempDir()
	configPath := filepath.Join(root, "semgrep.yaml")
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	targetName := strings.TrimSuffix(filepath.Base(fixture), ".fixture")
	targetPath := filepath.Join(root, targetName)
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := commandOutput(t, 60*time.Second, root, "semgrep", "scan", "--config", configPath, "--json", "--metrics=off", "--disable-version-check", targetPath)
	if exit != 0 && exit != 1 {
		t.Fatalf("semgrep scan exit %d\n%s\n%s", exit, stdout, stderr)
	}
	var report struct {
		Results []struct {
			CheckID string `json:"check_id"`
			Start   struct {
				Line int `json:"line"`
			} `json:"start"`
		} `json:"results"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("parse scan report: %v\n%s", err, stdout)
	}
	if len(report.Errors) != 0 {
		t.Fatalf("errors %s", stdout)
	}
	lines := map[int]struct{}{}
	for _, result := range report.Results {
		if result.CheckID == ruleID || strings.HasSuffix(result.CheckID, "."+ruleID) || strings.HasSuffix(result.CheckID, "/"+ruleID) {
			lines[result.Start.Line] = struct{}{}
		}
	}
	return lines
}
