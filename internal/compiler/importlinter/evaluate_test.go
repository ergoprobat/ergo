package importlinter

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// The tests below run the pinned import-linter and grimp. They need IPSO_IMPORTLINTER=1 and a python
// on PATH that carries both, installed from testdata/tool/requirements.txt.

func requireEngine(t *testing.T) {
	t.Helper()
	if os.Getenv("IPSO_IMPORTLINTER") == "" {
		t.Skip("IPSO_IMPORTLINTER is not set")
	}
	if _, err := exec.LookPath("python"); err != nil {
		t.Fatal("python is not on PATH")
	}
}

// fixtureRepo copies the Python sources of one fixture subject into a new repository.
// A .py.fixture file loses its suffix, so the org scanners do not read the subject as source.
func fixtureRepo(t *testing.T, subject string) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", subject, "repo")
	err := filepath.WalkDir(src, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(p, ".py.fixture") {
			return walkErr
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		target := filepath.Join(root, strings.TrimSuffix(rel, ".fixture"))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
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

func entryFor(mode string) compiler.Entry {
	return compiler.Entry{
		DecisionID:      "01M5WVYECH20GSNT3MW1VM5CC5",
		Slug:            "shop",
		Engine:          "importlinter",
		Mode:            mode,
		CompilerVersion: version,
		ValidFrom:       "2026-01-01",
	}
}

var evalNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

func evaluateGolden(t *testing.T, env *Env, subject string) compiler.Outcome {
	t.Helper()
	return env.Evaluate(context.Background(), entryFor("block"), map[string][]byte{artifactName: []byte(readGolden(t, subject))}, evalNow)
}

// evaluateConfig compiles an .importlinter text the way ipso compile does and evaluates it.
func evaluateConfig(t *testing.T, env *Env, config string) compiler.Outcome {
	t.Helper()
	artifact, err := compileConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	return env.Evaluate(context.Background(), entryFor("block"), map[string][]byte{artifactName: artifact}, evalNow)
}

func requireOutcomeCode(t *testing.T, out compiler.Outcome, code string) {
	t.Helper()
	requireCode(t, out.Err, code)
	if len(out.Violations) != 0 || out.Waived {
		t.Fatalf("outcome %+v", out)
	}
}

func violationsByContract(out compiler.Outcome) map[string]int {
	counts := map[string]int{}
	for _, violation := range out.Violations {
		id, _, _ := strings.Cut(violation.Message, " ")
		counts[id]++
	}
	return counts
}

func listing(t *testing.T, dir string) string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, p)
		files = append(files, rel)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return strings.Join(files, "\n")
}

func TestEvaluateCleanAndViolatingShop(t *testing.T) {
	requireEngine(t)
	clean := realEnv(t, fixtureRepo(t, "shop-clean"), ".")
	if out := evaluateGolden(t, clean, "shop-clean"); out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("clean: %+v", out)
	}

	violating := realEnv(t, fixtureRepo(t, "shop-violating"), ".")
	out := evaluateGolden(t, violating, "shop-violating")
	if out.Err != nil || out.Waived {
		t.Fatalf("violating: %+v", out)
	}
	counts := violationsByContract(out)
	if len(out.Violations) != 3 || counts["layers"] != 1 || counts["independence"] != 1 || counts["forbidden"] != 1 {
		t.Fatalf("violations %+v", out.Violations)
	}
	messages := make([]string, len(out.Violations))
	for i, violation := range out.Violations {
		messages[i] = violation.Message
	}
	for _, want := range []string{
		"layers shop.domain -> shop.api (l.1)",
		"independence shop.billing -> shop.shipping (l.1)",
		"forbidden shop.domain -> shop.infra (l.2)",
	} {
		if !strings.Contains(strings.Join(messages, "\n"), want) {
			t.Fatalf("missing %q in\n%s", want, strings.Join(messages, "\n"))
		}
	}
}

func TestEvaluateWaiverCoversFindingsAndNotErrors(t *testing.T) {
	requireEngine(t)
	env := realEnv(t, fixtureRepo(t, "shop-violating"), ".")
	entry := entryFor("block")
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada Lovelace", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	artifacts := map[string][]byte{artifactName: []byte(readGolden(t, "shop-violating"))}
	out := env.Evaluate(context.Background(), entry, artifacts, evalNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 3 {
		t.Fatalf("waived: %+v", out)
	}

	artifacts[artifactName] = []byte("[importlinter]\nroot_packages =\n    shop.api\n")
	out = env.Evaluate(context.Background(), entry, artifacts, evalNow)
	requireCode(t, out.Err, compiler.CodeBuildManifest)
	if out.Waived {
		t.Fatal("an error was waived")
	}
}

// Repository code is parsed and never imported: neither the root package, nor a module of it, nor a
// module next to it in the named directory runs.
func TestEvaluateNeverImportsRepositoryCode(t *testing.T) {
	requireEngine(t)
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	touch := "open(" + quoted(marker) + ", 'w').close()\n"
	writeTree(t, root, map[string]string{
		"shop/__init__.py":         touch,
		"shop/api/__init__.py":     touch,
		"shop/api/views.py":        touch + "import shop.domain\n",
		"shop/domain/__init__.py":  touch,
		"typing_extensions.py":     touch,
		"sitecustomize.py":         touch,
		"usercustomize.py":         touch,
		"click/__init__.py":        touch,
		"rich/__init__.py":         touch,
		"importlinter/__init__.py": touch,
		"conftest.py":              touch,
	})
	env := realEnv(t, root, ".")
	out := evaluateConfig(t, env, "[importlinter]\nroot_packages =\n    shop\n\n[importlinter:contract:layers]\nname = Layers\ntype = layers\nlayers =\n    shop.api\n    shop.domain\n")
	if out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("repository code ran")
	}
}

func quoted(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `\'`) + "'"
}

// A root named like a module the engine imports must not run repository code, and it must not be
// analysed as the installed module either.
func TestEvaluateRootNamedLikeAnEngineModule(t *testing.T) {
	requireEngine(t)
	for _, name := range []string{"typing_extensions", "click", "rich", "grimp", "importlinter", "os", "sys", "json"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			marker := filepath.Join(t.TempDir(), "ran")
			touch := "open(" + quoted(marker) + ", 'w').close()\n"
			writeTree(t, root, map[string]string{name + "/__init__.py": touch, name + "/a.py": touch, name + "/b.py": touch})
			env := realEnv(t, root, ".")
			out := evaluateConfig(t, env, "[importlinter]\nroot_packages =\n    "+name+"\n\n[importlinter:contract:forbidden]\nname = Forbidden\ntype = forbidden\nsource_modules =\n    "+name+".a\nforbidden_modules =\n    "+name+".b\n")
			requireOutcomeCode(t, out, CodeRun)
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("repository code ran")
			}
		})
	}
}

// import-linter 2.15 keeps a contract whose wildcard matches no module, and a layers contract in which
// every layer is optional and missing. ipso keeps that result and documents it. A release that changes it
// fails here, in the same change as the pin.
func TestEvaluateKeepsTheEnginesResultForAContractThatMatchesNothing(t *testing.T) {
	requireEngine(t)
	env := realEnv(t, fixtureRepo(t, "shop-clean"), ".")
	out := evaluateConfig(t, env, `[importlinter]
root_packages =
    shop

[importlinter:contract:wild]
name = Wildcard
type = forbidden
source_modules =
    shop.nothing.*
forbidden_modules =
    shop.infra

[importlinter:contract:optional]
name = Optional
type = layers
layers =
    (shop.ghost)
    (shop.phantom)
`)
	if out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("outcome %+v", out)
	}
}

// -I makes the interpreter ignore PYTHON* variables, so a step that exported them cannot add a module
// path or a startup script to the evaluation.
func TestEvaluateIgnoresPythonEnvironmentVariables(t *testing.T) {
	requireEngine(t)
	marker := filepath.Join(t.TempDir(), "ran")
	touch := "open(" + quoted(marker) + ", 'w').close()\n"
	hooks := t.TempDir()
	writeTree(t, hooks, map[string]string{"typing_extensions.py": touch, "sitecustomize.py": touch, "startup.py": touch})
	t.Setenv("PYTHONPATH", hooks)
	t.Setenv("PYTHONSTARTUP", filepath.Join(hooks, "startup.py"))
	t.Setenv("PYTHONINSPECT", "1")
	t.Setenv("PYTHONUSERBASE", hooks)
	env := realEnv(t, fixtureRepo(t, "shop-clean"), ".")
	if out := evaluateGolden(t, env, "shop-clean"); out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a PYTHON* variable added code to the run")
	}
}

func TestEvaluateLeavesTheRepositoryAsItWas(t *testing.T) {
	requireEngine(t)
	root := fixtureRepo(t, "shop-violating")
	before := listing(t, root)
	env := realEnv(t, root, ".")
	_ = evaluateGolden(t, env, "shop-violating")
	if after := listing(t, root); after != before {
		t.Fatalf("the repository changed\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestEvaluateRootProblems(t *testing.T) {
	requireEngine(t)
	root := fixtureRepo(t, "shop-clean")
	env := realEnv(t, root, ".")
	config := func(name string) string {
		return "[importlinter]\nroot_packages =\n    " + name + "\n\n[importlinter:contract:forbidden]\nname = Forbidden\ntype = forbidden\nsource_modules =\n    " + name + ".a\nforbidden_modules =\n    " + name + ".b\n"
	}
	requireOutcomeCode(t, evaluateConfig(t, env, config("nowhere")), CodeEmpty)
	// A module the root does not have is the engine's own error.
	out := evaluateConfig(t, env, config("shop"))
	requireOutcomeCode(t, out, CodeRun)
	if !strings.Contains(out.Err.Error(), "does not exist") {
		t.Fatalf("err %v", out.Err)
	}
}

func TestEvaluateRefusesASymlinkBelowARoot(t *testing.T) {
	requireEngine(t)
	root := fixtureRepo(t, "shop-clean")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "shop", "link")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	env := realEnv(t, root, ".")
	requireOutcomeCode(t, evaluateGolden(t, env, "shop-clean"), CodePath)
}

func TestEvaluateAnotherReleaseIsRefused(t *testing.T) {
	requireEngine(t)
	env := realEnv(t, fixtureRepo(t, "shop-clean"), ".")
	entry := entryFor("block")
	entry.CompilerVersion = "2.14"
	out := env.Evaluate(context.Background(), entry, map[string][]byte{artifactName: []byte(readGolden(t, "shop-clean"))}, evalNow)
	requireOutcomeCode(t, out, CodeVersion)
}

func TestSetupNeedsADirectoryInsideTheRepository(t *testing.T) {
	requireEngine(t)
	root := fixtureRepo(t, "shop-clean")
	if _, err := Setup(context.Background(), root, []string{"nope"}); err == nil {
		t.Fatal("a missing directory was accepted")
	} else {
		requireCode(t, err, CodePath)
	}
	if _, err := Setup(context.Background(), root, []string{"shop/__init__.py"}); err == nil {
		t.Fatal("a file was accepted")
	} else {
		requireCode(t, err, CodePath)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if _, err := Setup(context.Background(), root, []string{"escape"}); err == nil {
		t.Fatal("a symlink to outside was accepted")
	} else {
		requireCode(t, err, CodePath)
	}
}
