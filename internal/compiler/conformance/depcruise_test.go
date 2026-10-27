//go:build conformance

package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/depcruise"
	"github.com/ipsofacto-io/ipso/internal/edr"
)

const cruiseScript = "../depcruise/testdata/tool/node_modules/dependency-cruiser/bin/dependency-cruiser.mjs"

func TestDepcruise(t *testing.T) {
	if _, err := os.Stat(cruiseScript); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs(cruiseScript)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := nodeOutput(t, 30*time.Second, "", script, "--version")
	if exit != 0 {
		t.Fatalf("dependency-cruiser --version exit %d\n%s\n%s", exit, stdout, stderr)
	}
	requireVersion(t, stdout, depcruise.New().Version())

	clean := compileDepcruise(t, depcruiseClean)
	bad := compileDepcruise(t, depcruiseBad)
	if !bytes.Equal(clean, bad) {
		t.Fatal("clean and violating configs compiled to different artifacts")
	}
	if got := cruise(t, script, clean, filepath.Join(depcruiseClean, "repo")); len(got) != 0 {
		t.Fatalf("clean violations %#v", got)
	}
	got := cruise(t, script, clean, filepath.Join(depcruiseBad, "repo"))
	want := []cruiseHit{
		{Rule: "no-circular", From: "packages/ui/src/a.ts", To: "packages/ui/src/b.ts", Severity: "error"},
		{Rule: "ui-not-to-db", From: "packages/ui/src/index.ts", To: "packages/db/src/index.ts", Severity: "error"},
	}
	if len(got) != len(want) {
		t.Fatalf("violations %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("violation %d %#v, want %#v", i, got[i], want[i])
		}
	}
}

type cruiseHit struct {
	Rule     string
	From     string
	To       string
	Severity string
}

func compileDepcruise(t *testing.T, dir string) []byte {
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
	var in compiler.Input
	found := false
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
	artifacts, err := depcruise.New().Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "depcruise.json" {
		t.Fatalf("artifacts %#v", artifacts)
	}
	return artifacts[0].Data
}

func cruise(t *testing.T, script string, config []byte, repo string) []cruiseHit {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "depcruise.json")
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, exit := nodeOutput(t, 60*time.Second, repo, script, "--config", configPath, "--output-type", "json", "packages")
	if exit != 0 {
		t.Fatalf("depcruise exit %d\n%s\n%s", exit, stdout, stderr)
	}
	var parsed struct {
		Summary struct {
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
	if err := json.Unmarshal(stdout, &parsed); err != nil {
		t.Fatalf("decode cruise json: %v\n%s", err, stdout)
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
	return hits
}
