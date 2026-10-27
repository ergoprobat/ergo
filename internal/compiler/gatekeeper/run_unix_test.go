//go:build unix

package gatekeeper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/opa"
)

// The tests below put a fake gator first on PATH. It answers the version probe and, for a test run,
// records how it was started and prints a captured result.
const fakeGator = `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf '%s\n' "$FAKE_VERSION_OUT"
  exit "${FAKE_PROBE_EXIT:-0}"
fi
{
  echo "pwd=$(/bin/pwd -P)"
  echo "entries=[$(/bin/ls -A | /usr/bin/tr '\n' ' ')]"
  echo "args=$1 $2 $4 $6 $8 $9 ${10} n=$#"
  echo "resources=[$(/bin/ls "$3" | /usr/bin/tr '\n' ' ')]"
  echo "template=$(/usr/bin/basename "$5")"
  echo "constraint=$(/usr/bin/basename "$7")"
  echo "env=${GATOR_FOO-unset} ${KEEP_ME-unset}"
} > "$FAKE_LOG"
/bin/cp "$5" "$FAKE_TEMPLATE"
/bin/cp "$7" "$FAKE_CONSTRAINT"
if [ -n "$FAKE_SLEEP" ]; then
  /bin/sleep "$FAKE_SLEEP"
fi
if [ -n "$FAKE_OUT" ]; then
  /bin/cat "$FAKE_OUT"
fi
printf '%s' "$FAKE_ERR" >&2
exit "${FAKE_EXIT:-0}"
`

const versionOut = "gator version v" + opa.GatorVersion + " (Feature State: beta), opa/" + opa.GatorOPA + ", frameworks/v0.0.0-20260616163050-e1eaa1bf6d62"

type fake struct {
	log        string
	template   string
	constraint string
}

// useFakeGator installs the script and sets the environment it reads. A test changes a value with t.Setenv.
func useFakeGator(t *testing.T) fake {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gator"), []byte(fakeGator), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	out := t.TempDir()
	f := fake{log: filepath.Join(out, "log"), template: filepath.Join(out, "template"), constraint: filepath.Join(out, "constraint")}
	t.Setenv("FAKE_LOG", f.log)
	t.Setenv("FAKE_TEMPLATE", f.template)
	t.Setenv("FAKE_CONSTRAINT", f.constraint)
	t.Setenv("FAKE_VERSION_OUT", versionOut)
	for _, name := range []string{"FAKE_PROBE_EXIT", "FAKE_SLEEP", "FAKE_OUT", "FAKE_ERR", "FAKE_EXIT"} {
		t.Setenv(name, "")
	}
	return f
}

func (f fake) ran() bool {
	_, err := os.Stat(f.log)
	return err == nil
}

func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"k8s/api.yaml": deployment("api", false),
		"k8s/web.yml":  deployment("web", true),
		"notes.txt":    "not copied",
		"values.json":  "{}",
	})
	return root
}

func fakeEnv(t *testing.T) (*Env, map[string][]byte) {
	t.Helper()
	env, err := Setup(context.Background(), fakeRepo(t), []string{"k8s"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env, golden(t, "gatekeeper-clean")
}

func evaluate(env *Env, artifacts map[string][]byte) compiler.Outcome {
	return env.Evaluate(context.Background(), entryFor("block"), artifacts, evalNow)
}

func TestSetupProbe(t *testing.T) {
	root := fakeRepo(t)
	ctx := context.Background()

	t.Run("gator is not on PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := Setup(ctx, root, []string{"k8s"})
		requireCode(t, err, CodeMissing)
	})
	t.Run("gator cannot report its version", func(t *testing.T) {
		useFakeGator(t)
		t.Setenv("FAKE_PROBE_EXIT", "1")
		_, err := Setup(ctx, root, []string{"k8s"})
		requireCode(t, err, CodeMissing)
	})
	for name, out := range map[string]string{
		"no output":            "",
		"only the version":     "v" + opa.GatorVersion,
		"no v":                 strings.Replace(versionOut, "gator version v", "gator version ", 1),
		"no opa":               "gator version v" + opa.GatorVersion + " (Feature State: beta)",
		"no frameworks":        "gator version v" + opa.GatorVersion + " (Feature State: beta), opa/" + opa.GatorOPA,
		"two components":       strings.Replace(versionOut, "v"+opa.GatorVersion, "v3.23", 1),
		"a prerelease":         strings.Replace(versionOut, "v"+opa.GatorVersion, "v3.24.0-rc.1", 1),
		"a leading zero":       strings.Replace(versionOut, "v"+opa.GatorVersion, "v03.23.1", 1),
		"opa with a leading 0": strings.Replace(versionOut, opa.GatorOPA, "v01.17.1", 1),
		"another program":      strings.Replace(versionOut, "gator version", "kyverno version", 1),
		"trailing text":        versionOut + " extra",
		"the wrong line":       "usage: gator\n" + versionOut,
		"upper case":           strings.ToUpper(versionOut),
	} {
		t.Run(name, func(t *testing.T) {
			useFakeGator(t)
			t.Setenv("FAKE_VERSION_OUT", out)
			_, err := Setup(ctx, root, []string{"k8s"})
			requireCode(t, err, CodeVersion)
		})
	}
	t.Run("the pinned release", func(t *testing.T) {
		useFakeGator(t)
		env, err := Setup(ctx, root, []string{"k8s", "k8s"})
		if err != nil {
			t.Fatal(err)
		}
		defer env.Close()
		if env.Version() != opa.GatorVersion {
			t.Fatalf("version %s", env.Version())
		}
		if got := copied(t, env.resources); len(got) != 2 {
			t.Fatalf("copied %v", got)
		}
	})
	t.Run("a carriage return and more lines after the version", func(t *testing.T) {
		useFakeGator(t)
		t.Setenv("FAKE_VERSION_OUT", versionOut+"\r\nusage: gator [command]")
		env, err := Setup(ctx, root, []string{"k8s"})
		if err != nil {
			t.Fatal(err)
		}
		env.Close()
	})
}

func TestSetupRefusesPathsAndDocumentsBeforeAnythingRuns(t *testing.T) {
	f := useFakeGator(t)
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

	writeTree(t, root, map[string]string{
		"docs/readme.md":   "docs",
		"nothing/a.yaml":   "# no object\n",
		"policy/c.yaml":    "apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: RequireOwner\nmetadata:\n  name: x\n",
		"lists/l.yaml":     "apiVersion: v1\nkind: List\nitems:\n  - a\n",
		"mixed/ok.yaml":    deployment("ok", true),
		"mixed/policy.yml": "apiVersion: templates.gatekeeper.sh/v1\nkind: ConstraintTemplate\n",
	})
	for named, code := range map[string]string{
		"docs":    CodeEmpty,
		"nothing": CodeEmpty,
		"policy":  CodeResource,
		"lists":   CodeResource,
		"mixed":   CodeResource,
	} {
		_, err = Setup(ctx, root, []string{named})
		requireCode(t, err, code)
	}
	if f.ran() {
		t.Fatal("gator ran for a setup that failed")
	}
}

// The run's directory is made in the system temporary directory, which other test processes share,
// so this test gives itself a private one.
func TestSetupLeavesNoDirectoryWhenItFails(t *testing.T) {
	useFakeGator(t)
	root := fakeRepo(t)
	writeTree(t, root, map[string]string{"policy/c.yaml": "apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: X\n"})
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("FAKE_PROBE_EXIT", "1")
	if _, err := Setup(context.Background(), root, []string{"k8s"}); err == nil {
		t.Fatal("setup passed")
	}
	t.Setenv("FAKE_PROBE_EXIT", "")
	for _, named := range []string{"nope", "policy"} {
		if _, err := Setup(context.Background(), root, []string{named}); err == nil {
			t.Fatal("setup passed")
		}
	}
	left, err := filepath.Glob(filepath.Join(tmp, "ipso-gatekeeper-*"))
	if err != nil || len(left) != 0 {
		t.Fatalf("temporary directories left behind: %v %v", left, err)
	}
	env, err := Setup(context.Background(), root, []string{"k8s"})
	if err != nil {
		t.Fatal(err)
	}
	made, _ := filepath.Glob(filepath.Join(tmp, "ipso-gatekeeper-*"))
	if len(made) != 1 {
		t.Fatalf("a run that works makes one directory, found %v", made)
	}
	env.Close()
	if left, _ := filepath.Glob(filepath.Join(tmp, "ipso-gatekeeper-*")); len(left) != 0 {
		t.Fatalf("Close left %v", left)
	}
}

func TestEvaluateStartsGatorInIsolation(t *testing.T) {
	f := useFakeGator(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "multi"))
	t.Setenv("FAKE_EXIT", "1")
	t.Setenv("KEEP_ME", "yes")

	out := evaluate(env, artifacts)
	if out.Err != nil || out.Waived || len(out.Violations) != 2 {
		t.Fatalf("outcome %+v", out)
	}
	got := []string{out.Violations[0].Address, out.Violations[1].Address}
	if strings.Join(got, " ") != "team-a/Deployment/api Deployment/web" && strings.Join(got, " ") != "Deployment/web team-a/Deployment/api" {
		t.Fatalf("violations %+v", out.Violations)
	}
	logged, err := os.ReadFile(f.log)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logged)
	for _, want := range []string{
		"entries=[constraint-template.yaml constraint.yaml ]",
		"args=test -f -f -f -o json --enable-k8s-native-validation=false n=10",
		"resources=[000001.yaml 000002.yaml ]",
		"template=constraint-template.yaml",
		"constraint=constraint.yaml",
		"env=unset yes",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("log misses %q:\n%s", want, log)
		}
	}
	pwd, _, _ := strings.Cut(strings.TrimPrefix(log, "pwd="), "\n")
	if !strings.HasPrefix(log, "pwd=") || !strings.HasPrefix(filepath.Base(pwd), "check-") {
		t.Fatalf("gator did not run in the check directory:\n%s", log)
	}
	for path, want := range map[string]string{f.template: artifactTemplate, f.constraint: artifactConstraint} {
		body, err := os.ReadFile(path)
		if err != nil || string(body) != string(artifacts[want]) {
			t.Fatalf("%s %q err %v", want, body, err)
		}
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
		{"load error", "", "1", "reading: reading from filenames: yaml: line 4", CodeRun, "status 1: reading: reading from filenames"},
		{"load error without a message", "", "1", "", CodeRun, "status 1: no message"},
		{"panic", "", "2", "panic: nil pointer", CodeRun, "status 2: panic"},
		{"killed", "", "137", "", CodeRun, "status 137"},
		{"results with exit 0 and a deny", "deny", "0", "", CodeOutput, "disagree"},
		{"no result with exit 1", "pass", "1", "", CodeOutput, "disagree"},
		{"results with exit 2", "deny", "2", "", CodeRun, "status 2"},
		{"nothing printed and exit 0", "", "0", "", CodeOutput, "empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useFakeGator(t)
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

func TestEvaluateMapsTheEnforcementActionToTheExitStatus(t *testing.T) {
	for fixture, tc := range map[string]struct {
		out  string
		exit string
	}{
		"gatekeeper-clean": {"deny", "1"},
		"gk-mode-warn":     {"warn", "0"},
		"gk-mode-advise":   {"dryrun", "0"},
	} {
		t.Run(fixture, func(t *testing.T) {
			useFakeGator(t)
			env, _ := fakeEnv(t)
			t.Setenv("FAKE_OUT", outputPath(t, tc.out))
			t.Setenv("FAKE_EXIT", tc.exit)
			out := env.Evaluate(context.Background(), entryFor(fixtureModes[fixture]), golden(t, fixture), evalNow)
			if out.Err != nil || len(out.Violations) != 1 {
				t.Fatalf("outcome %+v", out)
			}
		})
	}
}

func TestEvaluateNullIsAPassOnlyWithExitZero(t *testing.T) {
	useFakeGator(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "pass"))
	if out := evaluate(env, artifacts); out.Err != nil || len(out.Violations) != 0 || out.Waived {
		t.Fatalf("nothing violated: %+v", out)
	}
}

func TestEvaluateErrorIsNeverWaived(t *testing.T) {
	useFakeGator(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "deny"))
	t.Setenv("FAKE_EXIT", "0")
	entry := entryFor("block")
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	out := env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err == nil || out.Waived {
		t.Fatalf("outcome %+v", out)
	}

	t.Setenv("FAKE_EXIT", "1")
	out = env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 1 {
		t.Fatalf("waiver outcome %+v", out)
	}
}

func TestEvaluateStopsAtTheDeadlineAndTheOutputCap(t *testing.T) {
	useFakeGator(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "multi"))
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

// Everything that can be refused is refused before gator runs a test.
func TestEvaluateRefusesBeforeStartingGator(t *testing.T) {
	f := useFakeGator(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "pass"))
	template := string(artifacts[artifactTemplate])
	constraint := string(artifacts[artifactConstraint])

	requireCode(t, env.Evaluate(context.Background(), entryFor("block"), map[string][]byte{artifactConstraint: artifacts[artifactConstraint]}, evalNow).Err, compiler.CodeBuildManifest)
	requireCode(t, env.Evaluate(context.Background(), entryFor("block"), map[string][]byte{artifactTemplate: artifacts[artifactTemplate]}, evalNow).Err, compiler.CodeBuildManifest)

	// A manifest and artifacts edited together pass the digest check. The template still has to be the
	// one compile writes, so a network call in it is never run.
	network := strings.Replace(template, "not input.review.object.metadata.labels.owner", `msg := http.send({"method": "get", "url": "https://example.invalid"}).status`, 1)
	if network == template {
		t.Fatal("the fixture changed shape")
	}
	requireCode(t, evaluate(env, map[string][]byte{artifactTemplate: []byte(network), artifactConstraint: []byte(constraint)}).Err, compiler.CodeBuildManifest)
	inventory := strings.Replace(template, "not input.review.object.metadata.labels.owner", "data.inventory.cluster", 1)
	requireCode(t, evaluate(env, map[string][]byte{artifactTemplate: []byte(inventory), artifactConstraint: []byte(constraint)}).Err, compiler.CodeBuildManifest)

	// A constraint compiled for one mode is not the canonical constraint of an entry in another.
	requireCode(t, env.Evaluate(context.Background(), entryFor("warn"), artifacts, evalNow).Err, compiler.CodeBuildManifest)
	requireCode(t, env.Evaluate(context.Background(), entryFor(""), artifacts, evalNow).Err, compiler.CodeBuildManifest)

	if f.ran() {
		t.Fatal("gator ran a test for a check that is refused before the run")
	}
}

func TestEvaluateRefusesAnotherBinaryRelease(t *testing.T) {
	for name, out := range map[string]string{
		"another gator":       strings.Replace(versionOut, "v"+opa.GatorVersion, "v3.24.0", 1),
		"another opa":         strings.Replace(versionOut, opa.GatorOPA, "v1.18.0", 1),
		"an older gator":      strings.Replace(versionOut, "v"+opa.GatorVersion, "v3.22.9", 1),
		"another major gator": strings.Replace(versionOut, "v"+opa.GatorVersion, "v4.0.0", 1),
	} {
		t.Run(name, func(t *testing.T) {
			f := useFakeGator(t)
			t.Setenv("FAKE_VERSION_OUT", out)
			env, artifacts := fakeEnv(t)
			result := evaluate(env, artifacts)
			requireOutcomeCode(t, result, CodeVersion)
			if f.ran() {
				t.Fatal("gator ran a test with another release")
			}
		})
	}
}

func TestEvaluateInheritsTheRunnerEnvironment(t *testing.T) {
	f := useFakeGator(t)
	env, artifacts := fakeEnv(t)
	t.Setenv("FAKE_OUT", outputPath(t, "pass"))
	t.Setenv("GATOR_FOO", "kept")
	t.Setenv("KEEP_ME", "yes")
	if out := evaluate(env, artifacts); out.Err != nil {
		t.Fatalf("outcome %+v", out)
	}
	logged, err := os.ReadFile(f.log)
	if err != nil || !strings.Contains(string(logged), "env=kept yes") {
		t.Fatalf("log %q err %v", logged, err)
	}
}
