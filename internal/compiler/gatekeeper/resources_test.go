package gatekeeper

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// checkTree writes each document text as a copied file and checks them. The source of file N is N.yaml
// in the repository, as the messages show it.
func checkTree(t *testing.T, files ...string) error {
	t.Helper()
	dir := t.TempDir()
	var sources []string
	for i, body := range files {
		name := fmt.Sprintf("%06d.yaml", i+1)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, "src/"+name)
	}
	return checkDocuments(context.Background(), dir, sources)
}

func TestCheckDocumentsAcceptsObjects(t *testing.T) {
	cases := map[string][]string{
		"a deployment":                {deployment("api", true)},
		"several documents":           {deployment("a", true) + "---\n" + deployment("b", false) + "---\napiVersion: v1\nkind: Namespace\nmetadata:\n  name: team\n"},
		"several files":               {deployment("a", true), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n"},
		"empty documents around one":  {"---\n---\n# nothing\n---\n" + deployment("a", true) + "---\n{}\n---\n~\n"},
		"a quoted apiVersion":         {"apiVersion: \"apps/v1\"\nkind: 'Deployment'\nmetadata:\n  name: a\n"},
		"a kustomization":             {"apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n  - a.yaml\n"},
		"a name that says gatekeeper": {"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: gatekeeper.sh\n  namespace: gatekeeper-system\n"},
		"a list kind with no items":   {"apiVersion: v1\nkind: DeploymentList\nmetadata:\n  name: x\n"},
		"items on an object":          {"apiVersion: v1\nkind: ConfigMap\nitems:\n  - a\n"},
		"a list kind with null items": {"apiVersion: v1\nkind: List\nitems:\n"},
		"a list kind with ~ items":    {"apiVersion: v1\nkind: DeploymentList\nitems: ~\n"},
		"anchors and merges below":    {"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: a\ndata:\n  base: &b\n    x: y\n  more:\n    <<: *b\n    z: w\n"},
		"a mapping with no kind":      {"replicas: 2\n"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			if err := checkTree(t, files...); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCheckDocumentsRefusesGatekeeperObjects(t *testing.T) {
	for name, apiVersion := range map[string]string{
		"a template":          "templates.gatekeeper.sh/v1",
		"a v1beta1 template":  "templates.gatekeeper.sh/v1beta1",
		"a constraint":        "constraints.gatekeeper.sh/v1beta1",
		"an expansion":        "expansion.gatekeeper.sh/v1alpha1",
		"a mutator":           "mutations.gatekeeper.sh/v1",
		"a config":            "config.gatekeeper.sh/v1alpha1",
		"a sync set":          "syncset.gatekeeper.sh/v1alpha1",
		"upper case":          "Constraints.Gatekeeper.SH/v1beta1",
		"a trailing space":    "\"constraints.gatekeeper.sh/v1beta1 \"",
		"an escaped dot":      "\"constraints\\x2egatekeeper.sh/v1beta1\"",
		"a folded scalar":     ">-\n  constraints.gatekeeper.sh/v1beta1",
		"a literal scalar":    "|-\n  constraints.gatekeeper.sh/v1beta1",
		"a made up subdomain": "anything.gatekeeper.sh/v1",
	} {
		t.Run(name, func(t *testing.T) {
			body := "apiVersion: " + apiVersion + "\nkind: Thing\nmetadata:\n  name: x\n"
			requireCode(t, checkTree(t, deployment("a", true), body), CodeResource)
		})
	}
}

func TestCheckDocumentsRefusesWhatGatorReadsDifferently(t *testing.T) {
	const constraint = "apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: RequireOwner\nmetadata:\n  name: x\n"
	cases := map[string]string{
		"a repeated apiVersion":     "apiVersion: v1\n" + constraint,
		"a repeated kind":           "apiVersion: v1\nkind: ConfigMap\nkind: Pod\nmetadata:\n  name: x\n",
		"a repeated key anywhere":   "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: x\nmetadata:\n  name: y\n",
		"a merge key":               "base: &a\n  apiVersion: constraints.gatekeeper.sh/v1beta1\n  kind: RequireOwner\n<<: *a\nmetadata:\n  name: x\n",
		"an inline merge":           "<<: {apiVersion: constraints.gatekeeper.sh/v1beta1, kind: RequireOwner}\nmetadata:\n  name: x\n",
		"an alias apiVersion":       "x: &a v1\napiVersion: *a\nkind: ConfigMap\n",
		"an alias kind":             "x: &a ConfigMap\napiVersion: v1\nkind: *a\n",
		"a tagged apiVersion":       "apiVersion: !!str apps/v1\nkind: Deployment\n",
		"a custom tag":              "apiVersion: !custom apps/v1\nkind: Deployment\n",
		"a numeric apiVersion":      "apiVersion: 1\nkind: ConfigMap\n",
		"a null apiVersion":         "apiVersion:\nkind: ConfigMap\n",
		"a list kind":               "apiVersion: v1\nkind: [ConfigMap]\n",
		"a boolean kind":            "apiVersion: v1\nkind: true\n",
		"a numeric key":             "1: x\napiVersion: v1\nkind: ConfigMap\n",
		"a null key":                "~: x\napiVersion: v1\nkind: ConfigMap\n",
		"a complex key":             "? [a]\n: b\napiVersion: v1\nkind: ConfigMap\n",
		"a tagged key":              "!!str apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: RequireOwner\n",
		"a List":                    "apiVersion: v1\nkind: List\nitems:\n  - " + strings.ReplaceAll(strings.TrimSpace(deployment("a", false)), "\n", "\n    ") + "\n",
		"a typed list":              "apiVersion: apps/v1\nkind: DeploymentList\nitems: []\n",
		"a list behind an alias":    "hidden: &a\n  - apiVersion: apps/v1\n    kind: Deployment\n    metadata:\n      name: x\napiVersion: v1\nkind: List\nitems: *a\n",
		"a list with mapping items": "apiVersion: v1\nkind: List\nitems:\n  a: b\n",
		"a list with scalar items":  "apiVersion: v1\nkind: List\nitems: x\n",
		"a lower case list":         "apiVersion: v1\nkind: list\nitems:\n  - a\n",
		"a scalar root":             "hello\n",
		"a sequence root":           "- a\n- b\n",
		"a string root":             "\"apiVersion: v1\"\n",
		"a document end marker":     "a: 1\n...\n" + constraint,
		"a separator in a flow":     "a: {\n---\n" + constraint + "}\n",
		"a separator in a quote":    "a: \"x\n---\n" + constraint + "\"\n",
		"unclosed flow":             "a: [\n",
		"a tab":                     "a:\n\tb: 1\n",
		"binary":                    "\x00\x01\x02",
		"an invalid document start": "apiVersion: v1\n--- kind: ConfigMap\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requireCode(t, checkTree(t, deployment("a", true)+"---\n"+body), CodeResource)
		})
	}
}

// A plain object after a refused one is never reached, and a refused file is named with its document.
func TestCheckDocumentsNamesTheSourceAndTheDocument(t *testing.T) {
	err := checkTree(t, deployment("a", true), deployment("b", true)+"---\napiVersion: templates.gatekeeper.sh/v1\nkind: ConstraintTemplate\n")
	requireCode(t, err, CodeResource)
	if want := "src/000002.yaml document 2 is a Gatekeeper object"; !strings.Contains(err.Error(), want) {
		t.Fatalf("err %v, want %q", err, want)
	}
}

func TestCheckDocumentsEscapesTheSourceName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000001.yaml"), []byte("apiVersion: constraints.gatekeeper.sh/v1beta1\nkind: X\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := checkDocuments(context.Background(), dir, []string{"k8s/a\n\x1b[31mb.yaml"})
	requireCode(t, err, CodeResource)
	if strings.ContainsAny(err.Error(), "\n\x1b") {
		t.Fatalf("message %q", err)
	}
}

func TestCheckDocumentsNeedsAnObject(t *testing.T) {
	for name, files := range map[string][]string{
		"empty files":      {"", "\n\n"},
		"only comments":    {"# nothing here\n"},
		"separators only":  {"---\n---\n---\n"},
		"empty mappings":   {"{}\n---\n{}\n"},
		"nulls":            {"~\n---\nnull\n"},
		"comments and ---": {"---\n# a\n---\n# b\n"},
	} {
		t.Run(name, func(t *testing.T) {
			requireCode(t, checkTree(t, files...), CodeEmpty)
		})
	}
	if err := checkTree(t, "", deployment("a", true)); err != nil {
		t.Fatalf("an empty file beside an object: %v", err)
	}
}

func TestCheckDocumentsStopsWhenTheContextEnds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "000001.yaml"), []byte(deployment("a", true)), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requireCode(t, checkDocuments(ctx, dir, []string{"a.yaml"}), CodeRun)
}

// The copy rules are tested in internal/compiler/manifests. These tests pin the reason code each kind
// of failure gets for Gatekeeper.
func TestStageMapsEachFailureToItsCode(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"k8s/pod.yaml": "pod", "docs/readme.md": "docs"})
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	stage := func(ctx context.Context, named string) ([]string, error) {
		return stageManifests(ctx, repo, []string{named}, filepath.Join(t.TempDir(), "copy"))
	}
	sources, err := stage(context.Background(), "k8s")
	if err != nil || len(sources) != 1 || sources[0] != "k8s/pod.yaml" {
		t.Fatalf("sources %v err %v", sources, err)
	}
	_, err = stage(context.Background(), "nope")
	requireCode(t, err, CodePath)
	_, err = stage(context.Background(), "docs")
	requireCode(t, err, CodeEmpty)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = stage(ctx, "k8s")
	requireCode(t, err, CodeRun)
}
