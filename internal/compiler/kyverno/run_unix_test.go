//go:build unix

package kyverno

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// The tests below put a fake kyverno first on PATH. It answers the version probe and, for an apply run,
// records how it was started and prints a captured report.
const fakeKyverno = `#!/bin/sh
if [ "$1" = "version" ]; then
  printf '%s\n' "$FAKE_VERSION_OUT"
  exit "${FAKE_PROBE_EXIT:-0}"
fi
{
  echo "pwd=$(/bin/pwd -P)"
  echo "entries=[$(/bin/ls -A)]"
  echo "args=$1 $3 $5 $6 $7 $8 n=$#"
  echo "policy=$(/usr/bin/basename "$2")"
  echo "resources=[$(/bin/ls "$4" | /usr/bin/tr '\n' ' ')]"
  echo "env=${KYVERNO_FOO-unset} ${KYVERNO_NAMESPACE-unset} ${KEEP_ME-unset}"
} > "$FAKE_LOG"
/bin/cp "$2" "$FAKE_POLICY"
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
	log    string
	policy string
}

// useFakeKyverno installs the script and sets the environment it reads. A test changes a value with t.Setenv.
func useFakeKyverno(t *testing.T) fake {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kyverno"), []byte(fakeKyverno), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	f := fake{log: filepath.Join(t.TempDir(), "log"), policy: filepath.Join(t.TempDir(), "policy")}
	t.Setenv("FAKE_LOG", f.log)
	t.Setenv("FAKE_POLICY", f.policy)
	t.Setenv("FAKE_VERSION_OUT", "Version: "+version+"\nTime: 2026-10-20T22:13:49Z\nGit commit ID: e63308f")
	for _, name := range []string{"FAKE_PROBE_EXIT", "FAKE_SLEEP", "FAKE_OUT", "FAKE_ERR", "FAKE_EXIT"} {
		t.Setenv(name, "")
	}
	return f
}

func (f fake) ran() bool {
	_, err := os.Stat(f.log)
	return err == nil
}

func outputPath(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "output", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/api.yaml": "api", "k8s/web.yml": "web", "notes.txt": "not copied"})
	return root
}

func fakeEnv(t *testing.T) (*Env, map[string][]byte) {
	t.Helper()
	env, err := Setup(context.Background(), fakeRepo(t), []string{"k8s"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env, map[string][]byte{artifactName: []byte(readGolden(t, "example-10"))}
}

func evaluate(env *Env, artifacts map[string][]byte) compiler.Outcome {
	return env.Evaluate(context.Background(), entryFor("block"), artifacts, evalNow)
}

func TestSetupProbe(t *testing.T) {
	root := fakeRepo(t)
	ctx := context.Background()

	t.Run("kyverno is not on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := Setup(ctx, root, []string{"k8s"})
		requireCode(t, err, CodeMissing)
	})
	t.Run("kyverno cannot report its version", func(t *testing.T) {
		useFakeKyverno(t)
		t.Setenv("FAKE_PROBE_EXIT", "1")
		_, err := Setup(ctx, root, []string{"k8s"})
		requireCode(t, err, CodeMissing)
	})
	for name, out := range map[string]string{
		"no output":         "",
		"no prefix":         version,
		"a v prefix":        "Version: v" + version,
		"two components":    "Version: 1.19",
		"a prerelease":      "Version: 1.19.2-rc.1",
		"a leading zero":    "Version: 01.19.2",
		"the wrong line":    "Time: 2026-10-20\nVersion: " + version,
		"trailing text":     "Version: 1.19.2 extra",
		"lower case prefix": "version: " + version,
	} {
		t.Run(name, func(t *testing.T) {
			useFakeKyverno(t)
			t.Setenv("FAKE_VERSION_OUT", out)
			_, err := Setup(ctx, root, []string{"k8s"})
			requireCode(t, err, CodeVersion)
		})
	}
	t.Run("the pinned release", func(t *testing.T) {
		useFakeKyverno(t)
		env, err := Setup(ctx, root, []string{"k8s", "k8s"})
		if err != nil {
			t.Fatal(err)
		}
		defer env.Close()
		if env.Version() != version {
			t.Fatalf("version %s", env.Version())
		}
		if got := copied(t, env.resources); len(got) != 2 {
			t.Fatalf("copied %v", got)
		}
	})
	t.Run("a carriage return after the version", func(t *testing.T) {
		useFakeKyverno(t)
		t.Setenv("FAKE_VERSION_OUT", "Version: "+version+"\r\nTime: x")
		env, err := Setup(ctx, root, []string{"k8s"})
		if err != nil {
			t.Fatal(err)
		}
		env.Close()
	})
}

func TestSetupRefusesPathsBeforeAnythingRuns(t *testing.T) {
	f := useFakeKyverno(t)
	root := fakeRepo(t)
	ctx := context.Background()
	_, err := Setup(ctx, root, []string{"nope"})
	requireCode(t, err, CodePath)

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	_, err = Setup(ctx, root, []string{"escape"})
	requireCode(t, err, CodePath)

	writeTree(t, root, map[string]string{"docs/readme.md": "docs"})
	_, err = Setup(ctx, root, []string{"docs"})
	requireCode(t, err, CodeEmpty)
	if f.ran() {
		t.Fatal("kyverno applied a policy for a run that failed in setup")
	}
}

func TestSetupLeavesNoDirectoryWhenItFails(t *testing.T) {
	useFakeKyverno(t)
	root := fakeRepo(t)
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "ipso-kyverno-*"))
	t.Setenv("FAKE_PROBE_EXIT", "1")
	if _, err := Setup(context.Background(), root, []string{"k8s"}); err == nil {
		t.Fatal("setup passed")
	}
	t.Setenv("FAKE_PROBE_EXIT", "")
	if _, err := Setup(context.Background(), root, []string{"nope"}); err == nil {
		t.Fatal("setup passed")
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "ipso-kyverno-*"))
	if len(after) != len(before) {
		t.Fatalf("temporary directories %v then %v", before, after)
	}
}

func TestEvaluateStartsKyvernoInIsolation(t *testing.T) {
	f := useFakeKyverno(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "mixed"))
	t.Setenv("FAKE_EXIT", "1")
	t.Setenv("KYVERNO_FOO", "secret")
	t.Setenv("kyverno_namespace", "lower")
	t.Setenv("KEEP_ME", "yes")

	out := evaluate(env, artifacts)
	if out.Err != nil || out.Waived || len(out.Violations) != 2 {
		t.Fatalf("outcome %+v", out)
	}
	if out.Violations[0].Address != "team-a/Pod/bad" || out.Violations[0].Message != "runAsNonRoot must be true" ||
		out.Violations[1].Address != "default/Deployment/dep" {
		t.Fatalf("violations %+v", out.Violations)
	}
	logged, err := os.ReadFile(f.log)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logged)
	for _, want := range []string{
		"entries=[kyverno.yaml]",
		"args=apply --resource --policy-report --output-format json --remove-color n=8",
		"policy=kyverno.yaml",
		"resources=[000001.yaml 000002.yaml ]",
		"env=unset unset yes",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("log misses %q:\n%s", want, log)
		}
	}
	pwd, _, _ := strings.Cut(strings.TrimPrefix(log, "pwd="), "\n")
	if !strings.HasPrefix(log, "pwd=") || !strings.HasPrefix(filepath.Base(pwd), "check-") {
		t.Fatalf("kyverno did not run in the check directory:\n%s", log)
	}
	policy, err := os.ReadFile(f.policy)
	if err != nil || string(policy) != string(artifacts[artifactName]) {
		t.Fatalf("policy %q err %v", policy, err)
	}
	if got := listing(t, env.dir); strings.Contains(got, "check-") {
		t.Fatalf("a check directory was left behind:\n%s", got)
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
		{"load error", "", "1", "Error: failed to load resources (yaml: line 4)", CodeRun, "status 1: Error: failed to load resources"},
		{"load error without a message", "", "1", "", CodeRun, "status 1: no message"},
		{"panic", "", "2", "panic: pods \"api\" already exists", CodeRun, "status 2: panic: pods"},
		{"killed", "", "137", "", CodeRun, "status 137"},
		{"a runtime error in the policy", "error", "1", "", CodeIncomplete, "no such key"},
		{"a report with exit 0 and a failure", "fail", "0", "", CodeOutput, "disagree"},
		{"a report with exit 1 and no failure", "pass", "1", "", CodeOutput, "disagree"},
		{"a report with exit 2", "pass", "2", "", CodeRun, "status 2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFakeKyverno(t)
			env, artifacts := fakeEnv(t)
			if tc.out != "" {
				t.Setenv("FAKE_OUT", outputPath(t, tc.out))
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

func TestEvaluateEmptyOutputIsAPassOnlyWithExitZero(t *testing.T) {
	useFakeKyverno(t)
	env, artifacts := fakeEnv(t)
	if out := evaluate(env, artifacts); out.Err != nil || len(out.Violations) != 0 || out.Waived {
		t.Fatalf("nothing matched: %+v", out)
	}
	t.Setenv("FAKE_OUT", outputPath(t, "skip"))
	if out := evaluate(env, artifacts); out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("skipped: %+v", out)
	}
}

func TestEvaluateErrorIsNeverWaived(t *testing.T) {
	useFakeKyverno(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "error"))
	t.Setenv("FAKE_EXIT", "1")
	entry := entryFor("block")
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	out := env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err == nil || out.Waived {
		t.Fatalf("outcome %+v", out)
	}

	t.Setenv("FAKE_OUT", outputPath(t, "fail"))
	out = env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 1 {
		t.Fatalf("waiver outcome %+v", out)
	}
}

func TestEvaluateStopsAtTheDeadlineAndTheOutputCap(t *testing.T) {
	useFakeKyverno(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "mixed"))
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
}

// Everything that can be refused is refused before kyverno applies a policy.
func TestEvaluateRefusesBeforeStartingKyverno(t *testing.T) {
	f := useFakeKyverno(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "pass"))
	good := string(artifacts[artifactName])

	entry := entryFor("block")
	entry.CompilerVersion = "1.19.1"
	requireOutcomeCode(t, env.Evaluate(context.Background(), entry, artifacts, evalNow), CodeVersion)

	requireCode(t, env.Evaluate(context.Background(), entryFor("block"), map[string][]byte{}, evalNow).Err, compiler.CodeBuildManifest)

	// A manifest and an artifact edited together pass the digest check. The policy still has to be
	// the one compile writes, so a network call in it is never run.
	network := strings.Replace(good, "has(object.spec.securityContext)", "http.get('https://example.invalid').status == 200", 1)
	if network == good {
		t.Fatal("the fixture changed shape")
	}
	requireCode(t, evaluate(env, map[string][]byte{artifactName: []byte(network)}).Err, compiler.CodeBuildManifest)

	// An artifact compiled for another mode is not the canonical artifact of this entry.
	requireCode(t, env.Evaluate(context.Background(), entryFor("warn"), artifacts, evalNow).Err, compiler.CodeBuildManifest)
	requireCode(t, env.Evaluate(context.Background(), entryFor(""), artifacts, evalNow).Err, compiler.CodeBuildManifest)

	if f.ran() {
		t.Fatal("kyverno applied a policy for a check that is refused before the run")
	}
}

func TestEvaluateRefusesAnotherBinaryRelease(t *testing.T) {
	f := useFakeKyverno(t)
	t.Setenv("FAKE_VERSION_OUT", "Version: 1.20.0")
	env, artifacts := fakeEnv(t)
	if env.Version() != "1.20.0" {
		t.Fatalf("version %s", env.Version())
	}
	out := evaluate(env, artifacts)
	requireOutcomeCode(t, out, CodeVersion)
	if !strings.Contains(out.Err.Error(), "1.20.0") || f.ran() {
		t.Fatalf("err %v ran %v", out.Err, f.ran())
	}
}

func TestChildEnvDropsKyvernoVariables(t *testing.T) {
	for _, name := range []string{"KYVERNO_FOO", "KYVERNO_NAMESPACE", "kyverno_lower"} {
		t.Setenv(name, "x")
	}
	t.Setenv("KEEP_ME", "yes")
	t.Setenv("KYVERNOX", "kept")
	got := map[string]int{}
	for _, item := range childEnv() {
		got[item]++
	}
	for _, want := range []string{"KEEP_ME=yes", "KYVERNOX=kept"} {
		if got[want] != 1 {
			t.Fatalf("%s appears %d times", want, got[want])
		}
	}
	for _, gone := range []string{"KYVERNO_FOO=x", "KYVERNO_NAMESPACE=x", "kyverno_lower=x"} {
		if got[gone] != 0 {
			t.Fatalf("%s survived", gone)
		}
	}
}
