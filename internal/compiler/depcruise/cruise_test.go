package depcruise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/edr"
)

// The json reporter exits 0 and records the build result in summary.advisedExitCode.
// The err reporter exits with that number. This test reads the json report.
func TestCruise(t *testing.T) {
	if os.Getenv("ERGO_DEPCRUISE") == "" {
		t.Skip("ERGO_DEPCRUISE is not set")
	}
	if _, err := os.Stat(cruiseScript); err != nil {
		t.Fatal(err)
	}

	t.Run("monorepo-clean", func(t *testing.T) {
		got, err := cruise(t, "testdata/monorepo-clean")
		if err != nil {
			t.Fatal(err)
		}
		if got.Error != 0 || got.Advised != 0 || len(got.Violations) != 0 {
			t.Fatalf("error %d advised %d violations %#v", got.Error, got.Advised, got.Violations)
		}
	})

	t.Run("monorepo-violating", func(t *testing.T) {
		got, err := cruise(t, "testdata/monorepo-violating")
		if err != nil {
			t.Fatal(err)
		}
		want := []cruiseHit{
			{Rule: "no-circular", From: "packages/ui/src/a.ts", To: "packages/ui/src/b.ts", Severity: "error"},
			{Rule: "ui-not-to-db", From: "packages/ui/src/index.ts", To: "packages/db/src/index.ts", Severity: "error"},
		}
		if len(got.Violations) != len(want) {
			t.Fatalf("violations %#v", got.Violations)
		}
		for i := range want {
			if got.Violations[i] != want[i] {
				t.Fatalf("violation %d %#v, want %#v", i, got.Violations[i], want[i])
			}
		}
		if got.Error != len(want) || got.Advised != len(want) {
			t.Fatalf("error %d advised %d", got.Error, got.Advised)
		}
	})
}

type cruiseHit struct {
	Rule     string
	From     string
	To       string
	Severity string
}

type cruiseReport struct {
	Error      int
	Advised    int
	Violations []cruiseHit
}

func cruise(t *testing.T, dir string) (cruiseReport, error) {
	t.Helper()
	config := compileCase(t, dir)
	configPath := filepath.Join(t.TempDir(), artifactName)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	script, err := filepath.Abs(cruiseScript)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "node", script, "--config", configPath, "--output-type", "json", "packages")
	cmd.Dir = filepath.Join(dir, "repo")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr != nil {
		return cruiseReport{}, fmt.Errorf("depcruise: %w\n%s", runErr, stderr.String())
	}
	var parsed struct {
		Summary struct {
			Error      int `json:"error"`
			Advised    int `json:"advisedExitCode"`
			Violations []struct {
				From string `json:"from"`
				To   string `json:"to"`
				Rule struct {
					Name     string `json:"name"`
					Severity string `json:"severity"`
				} `json:"rule"`
			} `json:"violations"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		return cruiseReport{}, fmt.Errorf("decode cruise json: %w\n%s", err, stdout.String())
	}
	hits := make([]cruiseHit, 0, len(parsed.Summary.Violations))
	for _, item := range parsed.Summary.Violations {
		hits = append(hits, cruiseHit{
			Rule:     item.Rule.Name,
			From:     item.From,
			To:       item.To,
			Severity: item.Rule.Severity,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Rule != hits[j].Rule {
			return hits[i].Rule < hits[j].Rule
		}
		if hits[i].From != hits[j].From {
			return hits[i].From < hits[j].From
		}
		return hits[i].To < hits[j].To
	})
	return cruiseReport{Error: parsed.Summary.Error, Advised: parsed.Summary.Advised, Violations: hits}, nil
}

const cruiseScript = "testdata/tool/node_modules/dependency-cruiser/bin/dependency-cruiser.mjs"

func compileCase(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "record.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := edr.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if problems := edr.Validate(doc); len(problems) != 0 {
		t.Fatal(strings.Join(problems, "; "))
	}
	rec, err := edr.RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Join(dir, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	var found bool
	var in compiler.Input
	for index, check := range rec.Checks {
		if check.Engine != "depcruise" {
			continue
		}
		in = compiler.Input{Record: rec, Check: check, Index: index, Repo: root.FS()}
		found = true
		break
	}
	if !found {
		t.Fatal("no depcruise check")
	}
	artifacts, err := New().Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != artifactName {
		t.Fatalf("artifacts %#v", artifacts)
	}
	return artifacts[0].Data
}
