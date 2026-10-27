package semgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/edr"
)

func TestSemgrepFixtures(t *testing.T) {
	if os.Getenv("IPSO_SEMGREP") == "" {
		t.Skip("IPSO_SEMGREP is not set")
	}
	if _, err := exec.LookPath("semgrep"); err != nil {
		t.Fatal(err)
	}
	t.Run("templates", func(t *testing.T) {
		testTemplates(t)
	})
	t.Run("load", func(t *testing.T) {
		testLoad(t)
	})
}

func testTemplates(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	rulesDir := filepath.Join(root, "rules")
	targetsDir := filepath.Join(root, "targets")
	for _, id := range templateIDs() {
		compiled := compileTemplate(t, id)
		rulePath := filepath.Join(rulesDir, filepath.FromSlash(id+".yaml"))
		if err := os.MkdirAll(filepath.Dir(rulePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rulePath, compiled, 0o644); err != nil {
			t.Fatal(err)
		}
		dir, name, _ := strings.Cut(id, "/")
		fixtureDir := filepath.Join("testdata", "fixtures", dir)
		entries, err := os.ReadDir(fixtureDir)
		if err != nil {
			t.Fatal(err)
		}
		prefix := name + "."
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".fixture") {
				continue
			}
			targetName := strings.TrimSuffix(entry.Name(), ".fixture")
			body, err := os.ReadFile(filepath.Join(fixtureDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			targetPath := filepath.Join(targetsDir, dir, targetName)
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(targetPath, body, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	stdout, stderr, err := runSemgrep(t, 3*time.Minute, "--test", "--json", "--metrics=off", "--disable-version-check", "--config", rulesDir, targetsDir)
	if err != nil {
		t.Fatalf("semgrep --test: %v\n%s\n%s", err, stdout, stderr)
	}
	var report semgrepTestReport
	if err := json.Unmarshal(stdout, &report); err != nil {
		t.Fatalf("parse test report: %v\n%s", err, stdout)
	}
	if len(report.ConfigWithErrors) != 0 {
		t.Fatalf("config errors %s", stdout)
	}
	passed := map[string]struct{}{}
	for _, file := range report.Results {
		for id, check := range file.Checks {
			if !check.Passed || len(check.Errors) != 0 {
				t.Errorf("rule %s did not pass\n%s", id, stdout)
				continue
			}
			passed[id] = struct{}{}
		}
	}
	for _, id := range templateIDs() {
		ruleID := strings.ReplaceAll(id, "/", ".")
		if _, ok := passed[ruleID]; !ok {
			t.Errorf("template %s was not reported as passed\n%s", id, stdout)
		}
	}
}

type semgrepTestReport struct {
	ConfigWithErrors []json.RawMessage `json:"config_with_errors"`
	Results          map[string]struct {
		Checks map[string]struct {
			Passed bool              `json:"passed"`
			Errors []json.RawMessage `json:"errors"`
		} `json:"checks"`
	} `json:"results"`
}

func testLoad(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		config := filepath.Join("testdata", entry.Name(), "golden", "0", "semgrep.yaml")
		if _, err := os.Stat(config); err != nil {
			continue
		}
		repo := filepath.Join("testdata", entry.Name(), "repo")
		t.Run(entry.Name(), func(t *testing.T) {
			stdout, stderr, err := runSemgrep(t, time.Minute, "scan", "--config", config, "--json", "--strict", "--metrics=off", "--disable-version-check", repo)
			if err != nil {
				t.Fatalf("semgrep scan: %v\n%s\n%s", err, stdout, stderr)
			}
			var report struct {
				Errors []json.RawMessage `json:"errors"`
			}
			if err := json.Unmarshal(stdout, &report); err != nil {
				t.Fatalf("parse scan report: %v\n%s", err, stdout)
			}
			if len(report.Errors) != 0 {
				t.Fatalf("errors %s", stdout)
			}
		})
	}
}

func compileTemplate(t *testing.T, id string) []byte {
	t.Helper()
	artifacts, err := New().Compile(compiler.Input{
		Record: edr.Record{Criticality: "blocking"},
		Check:  edr.Check{Engine: "semgrep", Ref: "ipso:" + id, Mode: "block"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != ArtifactName {
		t.Fatalf("artifacts %#v", artifacts)
	}
	return artifacts[0].Data
}

func runSemgrep(t *testing.T, timeout time.Duration, args ...string) ([]byte, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "semgrep", args...)
	cmd.Env = append(os.Environ(), "SEMGREP_SEND_METRICS=off", "SEMGREP_ENABLE_VERSION_CHECK=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}
