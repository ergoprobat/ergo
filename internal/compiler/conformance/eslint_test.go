//go:build conformance

package conformance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler/eslint"
)

const eslintTool = "../eslint/testdata/tool"

func TestESLint(t *testing.T) {
	stdout, stderr, exit := eslintOutput(t, 30*time.Second, "", "--version")
	if exit != 0 {
		t.Fatalf("version exit %d\n%s\n%s", exit, stdout, stderr)
	}
	requireVersion(t, stdout, "v"+eslint.New().Version())

	clean := compiledChecks(t, eslintClean, "eslint")
	bad := compiledChecks(t, eslintBad, "eslint")
	if len(clean) != 2 || len(bad) != 2 {
		t.Fatalf("checks %d %d", len(clean), len(bad))
	}
	sameArtifacts(t, clean[0], bad[0])
	sameArtifacts(t, clean[1], bad[1])

	ts := artifactData(t, clean[0], "eslint.config.mjs")
	js := artifactData(t, clean[1], "eslint.config.mjs")
	if got := eslintFindings(t, ts, eslintClean); len(got) != 0 {
		t.Fatalf("clean typescript %#v", got)
	}
	if got := eslintFindings(t, js, eslintClean); len(got) != 0 {
		t.Fatalf("clean javascript %#v", got)
	}
	got := eslintFindings(t, ts, eslintBad)
	want := []eslintFinding{{Rule: "@typescript-eslint/no-floating-promises", File: "services/float.ts", Line: 3}}
	if !sameFindings(got, want) {
		t.Fatalf("violating typescript %#v", got)
	}
	got = eslintFindings(t, js, eslintBad)
	want = []eslintFinding{{Rule: "eqeqeq", File: "web/eq.js", Line: 2}}
	if !sameFindings(got, want) {
		t.Fatalf("violating javascript %#v", got)
	}
}

func TestESLintRuleLists(t *testing.T) {
	script := `
const { builtinRules } = require("eslint/use-at-your-own-risk");
const plugin = require("@typescript-eslint/eslint-plugin");
const core = [];
for (const [id, rule] of builtinRules) {
  if (!(rule.meta && rule.meta.deprecated)) core.push(id);
}
const ts = [];
for (const [id, rule] of Object.entries(plugin.rules)) {
  if (!(rule.meta && rule.meta.deprecated)) ts.push("@typescript-eslint/" + id);
}
console.log(JSON.stringify({ core, ts }));
`
	stdout, stderr, exit := nodeOutput(t, 60*time.Second, eslintTool, "-e", script)
	if exit != 0 {
		t.Fatalf("rules exit %d\n%s\n%s", exit, stdout, stderr)
	}
	var parsed struct {
		Core []string `json:"core"`
		TS   []string `json:"ts"`
	}
	if err := json.Unmarshal(stdout, &parsed); err != nil {
		t.Fatal(err)
	}
	if got, want := ruleIDs(eslint.CoreRules()), sortedCopy(parsed.Core); got != want {
		t.Fatalf("core rules\n%s\ninstall\n%s", got, want)
	}
	if got, want := ruleIDs(eslint.TypeScriptRules()), sortedCopy(parsed.TS); got != want {
		t.Fatalf("typescript rules\n%s\ninstall\n%s", got, want)
	}
}

type eslintFinding struct {
	Rule string
	File string
	Line int
}

func eslintFindings(t *testing.T, config []byte, subject string) []eslintFinding {
	t.Helper()
	dir := copyFixtureTree(t, filepath.Join(subject, "repo"))
	tool, err := filepath.Abs(eslintTool)
	if err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(tool, "runs", strings.ReplaceAll(t.Name(), "/", "_"))
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(configDir) })
	configPath := filepath.Join(configDir, "eslint.config.mjs")
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	suppressions := filepath.Join(t.TempDir(), "eslint-suppressions.json")
	if err := os.WriteFile(suppressions, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, _ := eslintOutput(t, 60*time.Second, dir,
		"--config", configPath,
		"--format", "json",
		"--no-warn-ignored",
		"--suppressions-location", suppressions,
		".",
	)
	var results []struct {
		FilePath        string `json:"filePath"`
		FatalErrorCount int    `json:"fatalErrorCount"`
		Messages        []struct {
			RuleID   *string `json:"ruleId"`
			Line     int     `json:"line"`
			Fatal    bool    `json:"fatal"`
			Severity int     `json:"severity"`
		} `json:"messages"`
		SuppressedMessages []json.RawMessage `json:"suppressedMessages"`
	}
	if err := json.Unmarshal(stdout, &results); err != nil {
		t.Fatalf("decode eslint: %v\n%s\n%s", err, stdout, stderr)
	}
	var findings []eslintFinding
	for _, result := range results {
		if result.FatalErrorCount > 0 || len(result.SuppressedMessages) > 0 {
			t.Fatalf("fatal or suppressed %#v", result)
		}
		filePath, err := filepath.EvalSymlinks(result.FilePath)
		if err != nil {
			t.Fatal(err)
		}
		root, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, filePath)
		if err != nil {
			t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		for _, message := range result.Messages {
			if message.Fatal || message.RuleID == nil {
				if message.Fatal {
					t.Fatalf("fatal %#v", message)
				}
				continue
			}
			findings = append(findings, eslintFinding{Rule: *message.RuleID, File: rel, Line: message.Line})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	return findings
}

func sameFindings(got, want []eslintFinding) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func ruleIDs(rules map[string]struct{}) string {
	ids := make([]string, 0, len(rules))
	for id := range rules {
		ids = append(ids, id)
	}
	return sortedCopy(ids)
}

func sortedCopy(ids []string) string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return strings.Join(out, "\n")
}
