//go:build unix

package eslint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// The tests below put a fake eslint first on PATH, in a fake node_modules/.bin. It answers the version
// probe and, for a lint run, records how it was started and prints a captured result list.
const fakeESLint = `#!/bin/sh
if [ "$1" = "--version" ]; then
  if [ -n "$FAKE_PROBED" ]; then
    echo "pwd=$(/bin/pwd -P) env=${ESLINT_FLAGS-unset} ${TIMING-unset}" > "$FAKE_PROBED"
  fi
  printf '%s\n' "$FAKE_VERSION_OUT"
  exit "${FAKE_PROBE_EXIT:-0}"
fi
/bin/cp "$2" "$FAKE_CONFIG"
{
  echo "pwd=$(/bin/pwd -P)"
  echo "flags=$1 $3 $4 $5 $6 $7 $8 n=$#"
  echo "config=$(/usr/bin/basename "$2")"
  echo "configdir=$(/usr/bin/dirname "$2")"
  echo "suppressions=$(/usr/bin/basename "$9") $(/bin/cat "$9")"
  if [ "$(/usr/bin/dirname "$2")" = "$(/usr/bin/dirname "$9")" ]; then echo "samedir=yes"; else echo "samedir=no"; fi
  echo "env=${ESLINT_FLAGS-unset} ${TIMING-unset} ${eslint_lower-unset} ${KEEP_ME-unset}"
  echo "stdin=$(/bin/cat | /usr/bin/wc -c | /usr/bin/tr -d ' ')"
  shift 9
  echo "paths=$*"
} > "$FAKE_LOG"
if [ -n "$FAKE_SLEEP" ]; then
  /bin/sleep "$FAKE_SLEEP"
fi
if [ -n "$FAKE_OUT" ]; then
  /bin/cat "$FAKE_OUT"
fi
printf '%s' "$FAKE_ERR" >&2
exit "${FAKE_EXIT:-0}"
`

type fake struct {
	// modules is the node_modules directory with symlinks resolved.
	modules string
	log     string
	config  string
	probed  string
}

func (f fake) ran() bool {
	_, err := os.Stat(f.log)
	return err == nil
}

func (f fake) probeRan() bool {
	_, err := os.Stat(f.probed)
	return err == nil
}

// installFake writes a fake eslint and the packages next to it under dir, puts it first on PATH, and
// sets the environment it reads. A test changes a value with t.Setenv.
func installFake(t *testing.T, dir string) fake {
	t.Helper()
	modules := filepath.Join(dir, "node_modules")
	writeTree(t, modules, map[string]string{
		"eslint/package.json":            `{"name":"eslint","version":"` + version + `"}`,
		"typescript-eslint/package.json": `{"name":"typescript-eslint","version":"` + typescriptESLintVersion + `"}`,
		"typescript/package.json":        `{"name":"typescript","version":"` + typescriptVersion + `"}`,
	})
	bin := filepath.Join(modules, ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "eslint"), []byte(fakeESLint), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	resolved, err := filepath.EvalSymlinks(modules)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	f := fake{modules: resolved, log: filepath.Join(out, "log"), config: filepath.Join(out, "config"), probed: filepath.Join(out, "probed")}
	t.Setenv("FAKE_LOG", f.log)
	t.Setenv("FAKE_CONFIG", f.config)
	t.Setenv("FAKE_PROBED", f.probed)
	t.Setenv("FAKE_VERSION_OUT", "v"+version)
	for _, name := range []string{"FAKE_PROBE_EXIT", "FAKE_SLEEP", "FAKE_OUT", "FAKE_ERR", "FAKE_EXIT"} {
		t.Setenv(name, "")
	}
	return f
}

func useFakeESLint(t *testing.T) fake {
	t.Helper()
	return installFake(t, t.TempDir())
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"services/float.ts": "x", "web/eq.js": "y", "README.md": "z"})
	return root
}

// outputFile writes a capture for the repository root of env and returns its path.
func outputFile(t *testing.T, name string, env *Env) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+".json")
	if err := os.WriteFile(path, []byte(capture(t, name, env.root)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeEnv sets up a run over services and web, and returns it with the compiled typescript config.
func fakeEnv(t *testing.T) (*Env, map[string][]byte) {
	t.Helper()
	env, err := Setup(context.Background(), fakeRepo(t), []string{"services", "web"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env, map[string][]byte{artifactName: goldenArtifact(t, "0")}
}

func evaluate(env *Env, artifacts map[string][]byte) compiler.Outcome {
	return env.Evaluate(context.Background(), entryFor("typescript", "block"), artifacts, evalNow)
}

func TestSetupProbe(t *testing.T) {
	root := fakeRepo(t)
	ctx := context.Background()

	t.Run("eslint is not on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
	})
	t.Run("eslint cannot report its version", func(t *testing.T) {
		useFakeESLint(t)
		t.Setenv("FAKE_PROBE_EXIT", "1")
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
	})
	for name, out := range map[string]string{
		"no output":        "",
		"no v":             version,
		"two components":   "v10.13",
		"a prerelease":     "v10.13.0-rc.1",
		"a leading zero":   "v010.13.0",
		"trailing text":    "v10.13.0 extra",
		"a capital V":      "V10.13.0",
		"a banner":         "ESLint\nv10.13.0",
		"two lines":        "v10.13.0\nv10.13.0",
		"a timing table":   "v10.13.0\nRule | Time (ms)",
		"just the v":       "v",
		"a plus build tag": "v10.13.0+build",
	} {
		t.Run(name, func(t *testing.T) {
			useFakeESLint(t)
			t.Setenv("FAKE_VERSION_OUT", out)
			_, err := Setup(ctx, root, []string{"web"})
			requireCode(t, err, CodeVersion)
		})
	}
	t.Run("the pinned release", func(t *testing.T) {
		useFakeESLint(t)
		env, err := Setup(ctx, root, []string{"web", "services", "web"})
		if err != nil {
			t.Fatal(err)
		}
		defer env.Close()
		if env.Version() != version {
			t.Fatalf("version %s", env.Version())
		}
		if strings.Join(env.paths, " ") != "web services web" {
			t.Fatalf("paths %v", env.paths)
		}
	})
	t.Run("a carriage return after the version", func(t *testing.T) {
		useFakeESLint(t)
		t.Setenv("FAKE_VERSION_OUT", "v"+version+"\r")
		env, err := Setup(ctx, root, []string{"web"})
		if err != nil {
			t.Fatal(err)
		}
		env.Close()
	})
	t.Run("another release is still read", func(t *testing.T) {
		useFakeESLint(t)
		t.Setenv("FAKE_VERSION_OUT", "v10.14.0")
		env, err := Setup(ctx, root, []string{"web"})
		if err != nil {
			t.Fatal(err)
		}
		defer env.Close()
		if env.Version() != "10.14.0" {
			t.Fatalf("version %s", env.Version())
		}
	})
}

func TestProbeRunsOutsideTheRepositoryWithoutEngineVariables(t *testing.T) {
	f := useFakeESLint(t)
	t.Setenv("ESLINT_FLAGS", "x")
	t.Setenv("TIMING", "1")
	root := fakeRepo(t)
	env, err := Setup(context.Background(), root, []string{"web"})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Close()
	raw, err := os.ReadFile(f.probed)
	if err != nil {
		t.Fatal(err)
	}
	probe := string(raw)
	if !strings.Contains(probe, "env=unset unset") {
		t.Fatalf("probe %q", probe)
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(probe, "pwd="+resolved) {
		t.Fatalf("the probe ran in the repository: %q", probe)
	}
}

func TestSetupRefusesPathsBeforeAnythingRuns(t *testing.T) {
	f := useFakeESLint(t)
	root := fakeRepo(t)
	ctx := context.Background()
	_, err := Setup(ctx, root, []string{"nope"})
	requireCode(t, err, CodePath)
	_, err = Setup(ctx, root, []string{"web", "services/missing.ts"})
	requireCode(t, err, CodePath)

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	_, err = Setup(ctx, root, []string{"escape"})
	requireCode(t, err, CodePath)
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	_, err = Setup(ctx, root, []string{"dangling"})
	requireCode(t, err, CodePath)
	if f.probeRan() || f.ran() {
		t.Fatal("eslint was started for a run that failed on its paths")
	}
}

func TestSetupAcceptsAFileAndTheRoot(t *testing.T) {
	useFakeESLint(t)
	root := fakeRepo(t)
	env, err := Setup(context.Background(), root, []string{"web/eq.js", "."})
	if err != nil {
		t.Fatal(err)
	}
	env.Close()
}

// ESLint, typescript-eslint, and TypeScript decide the verdict, so they must come from the runner.
func TestSetupRefusesAToolchainThatIsNotTheRunners(t *testing.T) {
	ctx := context.Background()

	t.Run("installed inside the repository", func(t *testing.T) {
		root := fakeRepo(t)
		f := installFake(t, filepath.Join(root, "tooling"))
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if !strings.Contains(err.Error(), "inside the repository") || f.probeRan() {
			t.Fatalf("err %v probe ran %v", err, f.probeRan())
		}
	})
	t.Run("installed in the repository root", func(t *testing.T) {
		root := fakeRepo(t)
		f := installFake(t, root)
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if f.probeRan() {
			t.Fatal("the probe ran")
		}
	})
	t.Run("a binary link that resolves into the repository", func(t *testing.T) {
		root := fakeRepo(t)
		f := useFakeESLint(t)
		evil := filepath.Join(root, "evil.sh")
		if err := os.WriteFile(evil, []byte(fakeESLint), 0o755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(f.modules, ".bin", "eslint")
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(evil, link); err != nil {
			t.Skipf("no symlinks: %v", err)
		}
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if f.probeRan() {
			t.Fatal("the probe ran")
		}
	})
	t.Run("a binary that is not in node_modules/.bin", func(t *testing.T) {
		root := fakeRepo(t)
		f := useFakeESLint(t)
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "eslint"), []byte(fakeESLint), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+":/usr/bin:/bin")
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if f.probeRan() {
			t.Fatal("the probe ran")
		}
	})
	t.Run("a binary in a directory that is not node_modules", func(t *testing.T) {
		root := fakeRepo(t)
		f := useFakeESLint(t)
		other := filepath.Join(t.TempDir(), "modules", ".bin")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(other, "eslint"), []byte(fakeESLint), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", other+":/usr/bin:/bin")
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if f.probeRan() {
			t.Fatal("the probe ran")
		}
	})
	t.Run("no eslint package next to the link", func(t *testing.T) {
		root := fakeRepo(t)
		f := useFakeESLint(t)
		if err := os.Remove(filepath.Join(f.modules, "eslint", "package.json")); err != nil {
			t.Fatal(err)
		}
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if f.probeRan() {
			t.Fatal("the probe ran")
		}
	})
	t.Run("a link that is found relative to the working directory", func(t *testing.T) {
		root := fakeRepo(t)
		f := useFakeESLint(t)
		t.Setenv("PATH", "node_modules/.bin:/usr/bin:/bin")
		t.Chdir(filepath.Dir(f.modules))
		_, err := Setup(ctx, root, []string{"web"})
		requireCode(t, err, CodeMissing)
		if f.probeRan() {
			t.Fatal("the probe ran")
		}
	})
}

func TestSetupLeavesNoDirectoryWhenItFails(t *testing.T) {
	f := useFakeESLint(t)
	root := fakeRepo(t)
	// Other test binaries run at the same time and make their own ipso-eslint-* directories in the
	// system temporary directory, so this test counts only the ones below its own.
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	pattern := filepath.Join(tmp, "ipso-eslint-*")
	before, _ := filepath.Glob(pattern)
	t.Setenv("FAKE_PROBE_EXIT", "1")
	if _, err := Setup(context.Background(), root, []string{"web"}); err == nil {
		t.Fatal("setup passed")
	}
	t.Setenv("FAKE_PROBE_EXIT", "")
	if _, err := Setup(context.Background(), root, []string{"nope"}); err == nil {
		t.Fatal("setup passed")
	}
	after, _ := filepath.Glob(pattern)
	if len(after) != len(before) {
		t.Fatalf("temporary directories %v then %v", before, after)
	}
	if left, _ := filepath.Glob(filepath.Join(f.modules, scratchPrefix+"*")); len(left) != 0 {
		t.Fatalf("scratch directories %v", left)
	}

	env, err := Setup(context.Background(), root, []string{"web"})
	if err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(f.modules, scratchPrefix+"*")); len(left) != 1 {
		t.Fatalf("scratch directories %v", left)
	}
	env.Close()
	if left, _ := filepath.Glob(filepath.Join(f.modules, scratchPrefix+"*")); len(left) != 0 {
		t.Fatalf("scratch directories after Close %v", left)
	}
	if left, _ := filepath.Glob(pattern); len(left) != len(before) {
		t.Fatalf("temporary directories after Close %v", left)
	}
}

func TestEvaluateStartsESLintInIsolation(t *testing.T) {
	f := useFakeESLint(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputFile(t, "block", env))
	t.Setenv("FAKE_EXIT", "1")
	t.Setenv("ESLINT_FLAGS", "x")
	t.Setenv("eslint_lower", "x")
	t.Setenv("TIMING", "1")
	t.Setenv("KEEP_ME", "yes")
	before := listing(t, env.root)

	out := evaluate(env, artifacts)
	if out.Err != nil || out.Waived || len(out.Violations) != 1 {
		t.Fatalf("outcome %+v", out)
	}
	if got := out.Violations[0].Message; !strings.HasPrefix(got, floatingPromises+" services/float.ts:3:3: ") {
		t.Fatalf("violation %q", got)
	}
	raw, err := os.ReadFile(f.log)
	if err != nil {
		t.Fatal(err)
	}
	log := string(raw)
	for _, want := range []string{
		"pwd=" + env.root + "\n",
		"flags=--config --format json --no-inline-config --no-warn-ignored --no-error-on-unmatched-pattern --suppressions-location n=11",
		"config=eslint.config.mjs",
		"suppressions=suppressions.json {}",
		"samedir=yes",
		"env=unset unset unset yes",
		"stdin=0",
		"paths=services web",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("log misses %q:\n%s", want, log)
		}
	}
	var configDir string
	for _, line := range strings.Split(log, "\n") {
		if value, ok := strings.CutPrefix(line, "configdir="); ok {
			configDir = value
		}
	}
	// The check directory is gone by now, so resolve the scratch directory that held it.
	scratch, err := filepath.EvalSymlinks(filepath.Dir(configDir))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(scratch) != f.modules || !strings.HasPrefix(filepath.Base(scratch), scratchPrefix) || !strings.HasPrefix(filepath.Base(configDir), "check-") {
		t.Fatalf("the config was written to %s, not below %s<random>/check-<random>", configDir, filepath.Join(f.modules, scratchPrefix))
	}
	config, err := os.ReadFile(f.config)
	if err != nil || string(config) != string(artifacts[artifactName]) {
		t.Fatalf("config %q err %v", config, err)
	}
	if got := listing(t, env.scratch); got != "." {
		t.Fatalf("a check directory was left behind:\n%s", got)
	}
	if after := listing(t, env.root); after != before {
		t.Fatalf("the repository changed:\n%s\nthen\n%s", before, after)
	}
}

func TestEvaluateKeepsWhatEachExitStatusMeans(t *testing.T) {
	cases := []struct {
		name   string
		out    string
		exit   string
		stderr string
		code   string
		in     string
	}{
		{"a configuration error", "", "2", "Oops! Something went wrong! :(", CodeRun, "status 2: Oops"},
		{"exit 2 with a list on stdout", "block", "2", "There are suppressions left", CodeRun, "status 2: There are suppressions"},
		{"exit 1 with no output", "", "1", "boom", CodeRun, "status 1: boom"},
		{"exit 1 with no output and no message", "", "1", "", CodeRun, "status 1: no message"},
		{"killed", "", "137", "", CodeRun, "status 137"},
		{"an empty list", "empty", "0", "", CodeEmpty, "no file"},
		{"no output and exit 0", "", "0", "", CodeOutput, "empty"},
		{"a parse error", "fatal", "1", "", CodeIncomplete, "Parsing error"},
		{"a suppressed finding", "suppressed", "0", "", CodeOutput, "suppression"},
		{"a list with exit 0 and an error", "block", "0", "", CodeOutput, "disagree"},
		{"a clean list with exit 1", "clean", "1", "", CodeOutput, "disagree"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFakeESLint(t)
			env, artifacts := fakeEnv(t)
			if tc.out != "" {
				t.Setenv("FAKE_OUT", outputFile(t, tc.out, env))
			}
			t.Setenv("FAKE_EXIT", tc.exit)
			t.Setenv("FAKE_ERR", tc.stderr)
			out := evaluate(env, artifacts)
			requireOutcomeCode(t, out, tc.code)
			if !strings.Contains(out.Err.Error(), tc.in) {
				t.Fatalf("err %v, want %q", out.Err, tc.in)
			}
		})
	}
}

func TestEvaluateCleanRunPasses(t *testing.T) {
	useFakeESLint(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputFile(t, "clean", env))
	if out := evaluate(env, artifacts); out.Err != nil || len(out.Violations) != 0 || out.Waived {
		t.Fatalf("outcome %+v", out)
	}
}

func TestEvaluateRunsAJavaScriptCheckAndReadsItsRules(t *testing.T) {
	useFakeESLint(t)
	env, _ := fakeEnv(t)
	js := map[string][]byte{artifactName: goldenArtifact(t, "1")}
	raw := strings.ReplaceAll(capture(t, "block", env.root), floatingPromises, "eqeqeq")
	path := filepath.Join(t.TempDir(), "js.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_OUT", path)
	t.Setenv("FAKE_EXIT", "1")
	out := env.Evaluate(context.Background(), entryFor("javascript", "block"), js, evalNow)
	if out.Err != nil || len(out.Violations) != 1 || !strings.HasPrefix(out.Violations[0].Message, "eqeqeq services/float.ts:3:3: ") {
		t.Fatalf("outcome %+v", out)
	}
	// The typescript rule is not one of this check's rules.
	t.Setenv("FAKE_OUT", outputFile(t, "block", env))
	requireOutcomeCode(t, env.Evaluate(context.Background(), entryFor("javascript", "block"), js, evalNow), CodeOutput)
}

func TestEvaluateAppliesTheModeToTheSeverity(t *testing.T) {
	useFakeESLint(t)
	env, artifacts := fakeEnv(t)
	warn := map[string][]byte{artifactName: []byte(strings.Replace(string(artifacts[artifactName]), `"error"`, `"warn"`, 1))}
	t.Setenv("FAKE_OUT", outputFile(t, "warn", env))
	for _, mode := range []string{"warn", "advise"} {
		out := env.Evaluate(context.Background(), entryFor("typescript", mode), warn, evalNow)
		if out.Err != nil || len(out.Violations) != 1 {
			t.Fatalf("%s: outcome %+v", mode, out)
		}
	}
	// The warn config is not the config of a block entry.
	requireOutcomeCode(t, env.Evaluate(context.Background(), entryFor("typescript", "block"), warn, evalNow), compiler.CodeBuildManifest)
}

func TestEvaluateErrorIsNeverWaived(t *testing.T) {
	useFakeESLint(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputFile(t, "fatal", env))
	t.Setenv("FAKE_EXIT", "1")
	entry := entryFor("typescript", "block")
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	out := env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err == nil || out.Waived {
		t.Fatalf("outcome %+v", out)
	}

	t.Setenv("FAKE_OUT", outputFile(t, "block", env))
	out = env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 1 {
		t.Fatalf("waiver outcome %+v", out)
	}
	late := env.Evaluate(context.Background(), entry, artifacts, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if late.Err != nil || late.Waived || len(late.Violations) != 1 {
		t.Fatalf("expired waiver outcome %+v", late)
	}
}

func TestEvaluateStopsAtTheDeadlineAndTheOutputCap(t *testing.T) {
	useFakeESLint(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputFile(t, "block", env))
	t.Setenv("FAKE_EXIT", "1")

	env.outputLimit = 64
	requireOutcomeCode(t, evaluate(env, artifacts), CodeOutput)
	env.outputLimit = 0

	t.Setenv("FAKE_SLEEP", "30")
	env.timeout = 300 * time.Millisecond
	env.wait = 200 * time.Millisecond
	started := time.Now()
	out := evaluate(env, artifacts)
	requireOutcomeCode(t, out, CodeRun)
	if !strings.Contains(out.Err.Error(), "timed out") || time.Since(started) > 10*time.Second {
		t.Fatalf("err %v after %s", out.Err, time.Since(started))
	}
	if got := listing(t, env.scratch); got != "." {
		t.Fatalf("a check directory was left behind:\n%s", got)
	}
}

// Everything that can be refused is refused before eslint lints anything.
func TestEvaluateRefusesBeforeStartingESLint(t *testing.T) {
	f := useFakeESLint(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputFile(t, "clean", env))
	good := string(artifacts[artifactName])
	ts := func(config string) map[string][]byte { return map[string][]byte{artifactName: []byte(config)} }

	entry := entryFor("typescript", "block")
	entry.CompilerVersion = "10.12.0"
	requireOutcomeCode(t, env.Evaluate(context.Background(), entry, artifacts, evalNow), CodeVersion)

	requireCode(t, env.Evaluate(context.Background(), entryFor("typescript", "block"), map[string][]byte{}, evalNow).Err, compiler.CodeBuildManifest)

	// A manifest and an artifact edited together pass the digest check. The config still has to be the
	// one compile writes, so code in it never reaches ESLint.
	injected := "import fs from \"node:fs\";\n" + good
	requireCode(t, evaluate(env, ts(injected)).Err, compiler.CodeBuildManifest)
	code := strings.Replace(good, `files: ["services/**/*.ts"]`, `files: ["services/**/*.ts"], ...await import("node:fs")`, 1)
	if code == good {
		t.Fatal("the fixture changed shape")
	}
	requireCode(t, evaluate(env, ts(code)).Err, compiler.CodeBuildManifest)
	off := strings.Replace(good, "noInlineConfig: true", "noInlineConfig: false", 1)
	requireCode(t, evaluate(env, ts(off)).Err, compiler.CodeBuildManifest)

	// An artifact compiled for another mode or target is not the canonical artifact of this entry.
	requireCode(t, env.Evaluate(context.Background(), entryFor("typescript", "warn"), artifacts, evalNow).Err, compiler.CodeBuildManifest)
	requireCode(t, env.Evaluate(context.Background(), entryFor("typescript", ""), artifacts, evalNow).Err, compiler.CodeBuildManifest)
	requireCode(t, env.Evaluate(context.Background(), entryFor("javascript", "block"), artifacts, evalNow).Err, compiler.CodeBuildManifest)

	if f.ran() {
		t.Fatal("eslint linted for a check that is refused before the run")
	}
}

func TestEvaluateRefusesAnotherBinaryRelease(t *testing.T) {
	f := useFakeESLint(t)
	t.Setenv("FAKE_VERSION_OUT", "v10.14.0")
	env, artifacts := fakeEnv(t)
	out := evaluate(env, artifacts)
	requireOutcomeCode(t, out, CodeVersion)
	if !strings.Contains(out.Err.Error(), "10.14.0") || f.ran() {
		t.Fatalf("err %v ran %v", out.Err, f.ran())
	}
}

func TestEvaluateChecksTheTypeScriptPackagesForATypeScriptCheckOnly(t *testing.T) {
	cases := []struct {
		name   string
		damage func(t *testing.T, modules string)
		code   string
	}{
		{"another typescript", func(t *testing.T, m string) {
			writeTree(t, m, map[string]string{"typescript/package.json": `{"version":"6.0.2"}`})
		}, CodeVersion},
		{"another typescript-eslint", func(t *testing.T, m string) {
			writeTree(t, m, map[string]string{"typescript-eslint/package.json": `{"version":"8.72.0"}`})
		}, CodeVersion},
		{"no typescript", func(t *testing.T, m string) {
			if err := os.RemoveAll(filepath.Join(m, "typescript")); err != nil {
				t.Fatal(err)
			}
		}, CodeMissing},
		{"no typescript-eslint", func(t *testing.T, m string) {
			if err := os.RemoveAll(filepath.Join(m, "typescript-eslint")); err != nil {
				t.Fatal(err)
			}
		}, CodeMissing},
		{"a package.json that is not JSON", func(t *testing.T, m string) {
			writeTree(t, m, map[string]string{"typescript/package.json": `not json`})
		}, CodeMissing},
		{"no version", func(t *testing.T, m string) {
			writeTree(t, m, map[string]string{"typescript/package.json": `{}`})
		}, CodeVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := useFakeESLint(t)
			tc.damage(t, f.modules)
			env, artifacts := fakeEnv(t)
			t.Setenv("FAKE_OUT", outputFile(t, "clean", env))
			requireOutcomeCode(t, evaluate(env, artifacts), tc.code)
			if f.ran() {
				t.Fatal("eslint linted a typescript check without the pinned packages")
			}
			// A javascript check does not import them.
			js := map[string][]byte{artifactName: goldenArtifact(t, "1")}
			out := env.Evaluate(context.Background(), entryFor("javascript", "block"), js, evalNow)
			if out.Err != nil {
				t.Fatalf("javascript check: %v", out.Err)
			}
		})
	}
}

func TestChildEnvDropsEngineVariables(t *testing.T) {
	for _, name := range []string{"ESLINT_FLAGS", "ESLINT_USE_FLAT_CONFIG", "eslint_lower", "TIMING", "timing"} {
		t.Setenv(name, "x")
	}
	t.Setenv("KEEP_ME", "yes")
	t.Setenv("ESLINTX", "kept")
	t.Setenv("MY_TIMING", "kept")
	t.Setenv("NODE_OPTIONS", "--max-old-space-size=4096")
	got := map[string]int{}
	for _, item := range childEnv() {
		got[item]++
	}
	for _, want := range []string{"KEEP_ME=yes", "ESLINTX=kept", "MY_TIMING=kept", "NODE_OPTIONS=--max-old-space-size=4096"} {
		if got[want] != 1 {
			t.Fatalf("%s appears %d times", want, got[want])
		}
	}
	for _, gone := range []string{"ESLINT_FLAGS=x", "ESLINT_USE_FLAT_CONFIG=x", "eslint_lower=x", "TIMING=x", "timing=x"} {
		if got[gone] != 0 {
			t.Fatalf("%s survived", gone)
		}
	}
}
