package eslint

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// The tests below run the pinned ESLint toolchain. They need IPSO_ESLINT=1 and eslint 10.13.0 on PATH,
// installed with `npm ci --ignore-scripts` in testdata/tool.

func requireESLint(t *testing.T) {
	t.Helper()
	if os.Getenv("IPSO_ESLINT") == "" {
		t.Skip("IPSO_ESLINT is not set")
	}
	if _, err := exec.LookPath("eslint"); err != nil {
		t.Fatal("eslint is not on PATH")
	}
}

// subject copies the repository of one fixture subject into a new directory. A .fixture suffix is dropped.
func subject(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", name, "repo")
	dest := t.TempDir()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeTree(t, dest, map[string]string{filepath.ToSlash(strings.TrimSuffix(rel, ".fixture")): string(raw)})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}

func realEnv(t *testing.T, root string, named ...string) *Env {
	t.Helper()
	requireESLint(t)
	env, err := Setup(context.Background(), root, named)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env
}

func realEval(env *Env, target, mode string, artifact []byte) compiler.Outcome {
	return env.Evaluate(context.Background(), entryFor(target, mode), map[string][]byte{artifactName: artifact}, evalNow)
}

func TestRealCleanSubjectPasses(t *testing.T) {
	env := realEnv(t, subject(t, "app-clean"), "services", "web")
	if env.Version() != version {
		t.Fatalf("version %s", env.Version())
	}
	for target, index := range map[string]string{"typescript": "0", "javascript": "1"} {
		out := realEval(env, target, "block", goldenArtifact(t, index))
		if out.Err != nil || len(out.Violations) != 0 || out.Waived {
			t.Fatalf("%s: %+v", target, out)
		}
	}
}

// The violating subject has an eslint-disable comment and an eslint-suppressions.json that would hide
// both findings. ipso reports them anyway.
func TestRealViolatingSubjectIgnoresRepositorySuppressions(t *testing.T) {
	root := subject(t, "app-violating")
	for _, name := range []string{"eslint-suppressions.json", "services/float.ts"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("the subject lost %s: %v", name, err)
		}
	}
	env := realEnv(t, root, "services", "web")
	cases := []struct {
		target, index, line string
	}{
		{"typescript", "0", floatingPromises + " services/float.ts:3:3: "},
		{"javascript", "1", "eqeqeq web/eq.js:2:15: "},
	}
	for _, tc := range cases {
		out := realEval(env, tc.target, "block", goldenArtifact(t, tc.index))
		if out.Err != nil || out.Waived || len(out.Violations) != 1 || !strings.HasPrefix(out.Violations[0].Message, tc.line) {
			t.Fatalf("%s: %+v", tc.target, out)
		}
	}
}

func TestRealModeAndWaiver(t *testing.T) {
	env := realEnv(t, subject(t, "app-violating"), "services")
	source, err := os.ReadFile(filepath.Join("testdata", "app-violating", "repo", "eslint-ts.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"warn", "advise"} {
		artifact, err := compileRules(source, targetTypeScript, mode)
		if err != nil {
			t.Fatal(err)
		}
		out := realEval(env, "typescript", mode, artifact)
		if out.Err != nil || out.Waived || len(out.Violations) != 1 {
			t.Fatalf("%s: %+v", mode, out)
		}
	}
	entry := entryFor("typescript", "block")
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	artifacts := map[string][]byte{artifactName: goldenArtifact(t, "0")}
	out := env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 1 {
		t.Fatalf("waiver: %+v", out)
	}
}

// Nothing in the repository decides what runs. Its own config file is not loaded, and a directive
// comment cannot turn a rule off.
func TestRealRepositoryConfigAndDirectivesAreNotHonored(t *testing.T) {
	root := subject(t, "app-violating")
	writeTree(t, root, map[string]string{
		"eslint.config.js":  "import fs from 'node:fs';\nfs.writeFileSync('loaded', 'x');\nexport default [];\n",
		"eslint.config.mjs": "import fs from 'node:fs';\nfs.writeFileSync('loaded-mjs', 'x');\nexport default [];\n",
		"web/off.js":        "/* eslint-disable */\n/* eslint eqeqeq: \"off\" */\nexport const f = (a, b) => a == b;\n",
	})
	env := realEnv(t, root, "web")
	out := realEval(env, "javascript", "block", goldenArtifact(t, "1"))
	if out.Err != nil || len(out.Violations) != 2 {
		t.Fatalf("outcome %+v", out)
	}
	for _, marker := range []string{"loaded", "loaded-mjs"} {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			t.Fatalf("the repository config was loaded: %s exists", marker)
		}
	}
}

func TestRealUnfinishedRunsAreErrors(t *testing.T) {
	root := subject(t, "app-clean")
	writeTree(t, root, map[string]string{
		"web/bad.js": "export function (\n",
		"docs/a.txt": "no source\n",
		"other/x.ts": "export const x = 1;\n",
	})
	env := realEnv(t, root, "web", "docs", "other")
	artifacts := map[string][]byte{artifactName: goldenArtifact(t, "1")}
	out := env.Evaluate(context.Background(), entryFor("javascript", "block"), artifacts, evalNow)
	requireOutcomeCode(t, out, CodeIncomplete)

	empty := realEnv(t, root, "docs")
	requireOutcomeCode(t, realEval(empty, "javascript", "block", goldenArtifact(t, "1")), CodeEmpty)

	// A typescript file that no tsconfig includes cannot be type checked, so it is not checked.
	everywhere, err := compileRules([]byte(`{"files":["**/*.ts"],"rules":{"`+floatingPromises+`":"error"}}`), targetTypeScript, "block")
	if err != nil {
		t.Fatal(err)
	}
	other := realEnv(t, root, "other")
	requireOutcomeCode(t, realEval(other, "typescript", "block", everywhere), CodeIncomplete)
}

func TestRealRunLeavesTheRepositoryAndTheToolchainClean(t *testing.T) {
	root := subject(t, "app-violating")
	before := listing(t, root)
	env := realEnv(t, root, "services", "web")
	realEval(env, "typescript", "block", goldenArtifact(t, "0"))
	if after := listing(t, root); after != before {
		t.Fatalf("the repository changed:\n%s\nthen\n%s", before, after)
	}
	scratch := env.scratch
	env.Close()
	if _, err := os.Stat(scratch); !os.IsNotExist(err) {
		t.Fatalf("the scratch directory is still there: %v", err)
	}
}

func TestRealRunTimesOut(t *testing.T) {
	env := realEnv(t, subject(t, "app-violating"), "services", "web")
	env.timeout = time.Nanosecond
	out := realEval(env, "typescript", "block", goldenArtifact(t, "0"))
	requireOutcomeCode(t, out, CodeRun)
	if !strings.Contains(out.Err.Error(), "timed out") {
		t.Fatalf("err %v", out.Err)
	}
}
