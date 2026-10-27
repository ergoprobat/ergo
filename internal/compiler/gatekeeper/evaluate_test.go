package gatekeeper

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/opa"
)

// The tests below run the pinned gator CLI. They need IPSO_GATEKEEPER=1 and gator on PATH,
// installed with ../opa/testdata/tool/install.sh.

func requireEngine(t *testing.T) {
	t.Helper()
	if os.Getenv("IPSO_GATEKEEPER") == "" {
		t.Skip("IPSO_GATEKEEPER is not set")
	}
	if _, err := exec.LookPath("gator"); err != nil {
		t.Fatal("gator is not on PATH")
	}
}

func realEnv(t *testing.T, root string, named ...string) *Env {
	t.Helper()
	requireEngine(t)
	env, err := Setup(context.Background(), root, named)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env
}

func evaluateReal(t *testing.T, env *Env, fixture string) compiler.Outcome {
	t.Helper()
	return env.Evaluate(context.Background(), entryFor(fixtureModes[fixture]), golden(t, fixture), evalNow)
}

func TestRealGatorVersionIsThePin(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/a.yaml": deployment("a", true)})
	env := realEnv(t, root, "k8s")
	if env.Version() != opa.GatorVersion || env.opaVersion != opa.GatorOPA {
		t.Fatalf("gator %s opa %s", env.Version(), env.opaVersion)
	}
}

func TestRealGatorCleanAndViolating(t *testing.T) {
	clean := t.TempDir()
	writeTree(t, clean, map[string]string{"k8s/good.yaml": deployment("good", true)})
	if out := evaluateReal(t, realEnv(t, clean, "k8s"), "gatekeeper-clean"); out.Err != nil || len(out.Violations) != 0 || out.Waived {
		t.Fatalf("clean: %+v", out)
	}

	bad := t.TempDir()
	writeTree(t, bad, map[string]string{
		"k8s/bad.yaml":  deployment("bad", false),
		"k8s/good.yaml": deployment("good", true),
		"k8s/team.yaml": "apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: api\n  namespace: team-a\nspec:\n  replicas: 1\n",
	})
	out := evaluateReal(t, realEnv(t, bad, "k8s"), "gatekeeper-clean")
	if out.Err != nil || len(out.Violations) != 2 {
		t.Fatalf("violating: %+v", out)
	}
	seen := map[string]string{}
	for _, v := range out.Violations {
		seen[v.Address] = v.Message
	}
	if seen["Deployment/bad"] != "missing owner label" || seen["team-a/Deployment/api"] != "missing owner label" {
		t.Fatalf("violations %v", seen)
	}
}

func TestRealGatorWarnAndAdviseReportTheViolationWithoutAnExitStatus(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/bad.yaml": deployment("bad", false)})
	for _, fixture := range []string{"gk-mode-warn", "gk-mode-advise"} {
		out := evaluateReal(t, realEnv(t, root, "k8s"), fixture)
		if out.Err != nil || len(out.Violations) != 1 || out.Waived {
			t.Fatalf("%s: %+v", fixture, out)
		}
	}
}

func TestRealGatorWaiverCoversAViolationAndNeverAnError(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/bad.yaml": deployment("bad", false)})
	env := realEnv(t, root, "k8s")
	entry := entryFor("block")
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	out := env.Evaluate(context.Background(), entry, golden(t, "gatekeeper-clean"), evalNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 1 {
		t.Fatalf("waived: %+v", out)
	}
	artifacts := golden(t, "gatekeeper-clean")
	artifacts[artifactTemplate] = []byte(strings.Replace(string(artifacts[artifactTemplate]), "not input.review.object.metadata.labels.owner", "data.inventory.cluster", 1))
	out = env.Evaluate(context.Background(), entry, artifacts, evalNow)
	requireCode(t, out.Err, compiler.CodeBuildManifest)
	if out.Waived || len(out.Violations) != 0 {
		t.Fatalf("a template compile rejects: %+v", out)
	}
}

// A constraint with no match.kinds reviews every object gator reads, including ipso's own template and
// constraint. Those are not findings, and the exit status that counts them must not break the run.
func TestRealGatorDropsItsOwnObjects(t *testing.T) {
	artifacts := golden(t, "gatekeeper-clean")
	constraint := string(artifacts[artifactConstraint])
	open := constraint[:strings.Index(constraint, "  match:")] + "  match: {}\n"
	artifacts[artifactConstraint] = []byte(open)

	bad := t.TempDir()
	writeTree(t, bad, map[string]string{"k8s/bad.yaml": deployment("bad", false)})
	out := realEnv(t, bad, "k8s").Evaluate(context.Background(), entryFor("block"), artifacts, evalNow)
	if out.Err != nil || len(out.Violations) != 1 || out.Violations[0].Address != "Deployment/bad" {
		t.Fatalf("violating: %+v", out)
	}

	clean := t.TempDir()
	writeTree(t, clean, map[string]string{"k8s/good.yaml": deployment("good", true)})
	out = realEnv(t, clean, "k8s").Evaluate(context.Background(), entryFor("block"), artifacts, evalNow)
	if out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("clean: %+v", out)
	}
}

func TestRealGatorRefusesWhatCouldChangeThePolicy(t *testing.T) {
	requireEngine(t)
	const constraint = "apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: RequireOwner\nmetadata:\n  name: other\nspec:\n  enforcementAction: dryrun\n  match: {}\n"
	for name, body := range map[string]string{
		"a constraint of the same kind":     constraint,
		"a constraint with the same name":   strings.Replace(constraint, "name: other", "name: owner", 1),
		"a template":                        "apiVersion: templates.gatekeeper.sh/v1\nkind: ConstraintTemplate\nmetadata:\n  name: requireowner\n",
		"a mutator":                         "apiVersion: mutations.gatekeeper.sh/v1\nkind: AssignMetadata\nmetadata:\n  name: owner\n",
		"a constraint behind a merge key":   "base: &a\n  apiVersion: constraints.gatekeeper.sh/v1beta1\n  kind: RequireOwner\n<<: *a\nmetadata:\n  name: other\n",
		"a repeated kind":                   "apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: Pod\nkind: RequireOwner\nmetadata:\n  name: other\n",
		"a list that hides a violation":     "apiVersion: v1\nkind: List\nitems:\n  - " + strings.ReplaceAll(strings.TrimSpace(deployment("hidden", false)), "\n", "\n    ") + "\n",
		"a list that hides behind an alias": "hidden: &a\n  - " + strings.ReplaceAll(strings.TrimSpace(deployment("hidden", false)), "\n", "\n    ") + "\napiVersion: v1\nkind: List\nitems: *a\n",
		"a typed list that hides one":       "apiVersion: apps/v1\nkind: DeploymentList\nitems:\n  - " + strings.ReplaceAll(strings.TrimSpace(deployment("hidden", false)), "\n", "\n    ") + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"k8s/ok.yaml": deployment("ok", true), "k8s/extra.yaml": body})
			_, err := Setup(context.Background(), root, []string{"k8s"})
			requireCode(t, err, CodeResource)
		})
	}
}

// These are the documents the check lets through, so gator is what stops them.
func TestRealGatorErrorsOnDocumentsItCannotReview(t *testing.T) {
	requireEngine(t)
	for name, body := range map[string]string{
		"a mapping with no kind":    "replicas: 2\n",
		"a kind with no apiVersion": "kind: ConfigMap\nmetadata:\n  name: x\n",
		"a helm template":           "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Values.name }}\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTree(t, root, map[string]string{"k8s/ok.yaml": deployment("ok", true), "k8s/extra.yaml": body})
			out := evaluateReal(t, realEnv(t, root, "k8s"), "gatekeeper-clean")
			requireOutcomeCode(t, out, CodeRun)
		})
	}
}

func TestRealGatorOnlyReadsYamlFilesAndSkipsHiddenOnes(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"k8s/good.yaml":       deployment("good", true),
		"k8s/package.json":    `{"name": "x"}`,
		"k8s/.hidden/bad.yml": deployment("hidden", false),
		"k8s/bad.yaml.orig":   deployment("backup", false),
	})
	out := evaluateReal(t, realEnv(t, root, "k8s"), "gatekeeper-clean")
	if out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("outcome %+v", out)
	}
}

func TestRealGatorLeavesTheRepositoryUntouched(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/bad.yaml": deployment("bad", false), "k8s/good.yaml": deployment("good", true)})
	before := listing(t, root)
	env := realEnv(t, root, "k8s")
	if out := evaluateReal(t, env, "gatekeeper-clean"); out.Err != nil || len(out.Violations) != 1 {
		t.Fatalf("outcome %+v", out)
	}
	if after := listing(t, root); after != before {
		t.Fatalf("the repository changed:\nbefore\n%s\nafter\n%s", before, after)
	}
}
