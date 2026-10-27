package archunit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler"
)

// The tests below run the pinned jars. They need ERGO_ARCHUNIT=1, a JDK 17 or newer on PATH,
// and the jars from testdata/tool/install.sh.

func realJars(t *testing.T) string {
	t.Helper()
	if os.Getenv("ERGO_ARCHUNIT") == "" {
		t.Skip("ERGO_ARCHUNIT is not set")
	}
	dir, err := filepath.Abs(filepath.Join("testdata", "tool", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// compileSubject compiles the Java fixtures of one subject into <root>/target/classes.
func compileSubject(t *testing.T, subject string) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", subject, "repo", "src", "main", "java")
	var sources []string
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".java.fixture") {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(root, "src", strings.TrimSuffix(rel, ".fixture"))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sources = append(sources, target)
		return os.WriteFile(target, body, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	classes := filepath.Join(root, "target", "classes")
	if err := os.MkdirAll(classes, 0o755); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"--release", "17", "-proc:none", "-d", classes}, sources...)
	if out, err := exec.Command("javac", args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	return root
}

func realEnv(t *testing.T, root string) *Env {
	t.Helper()
	env, err := Setup(context.Background(), root, realJars(t), []string{"target/classes"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env
}

func violationCounts(out compiler.Outcome) map[string]int {
	counts := map[string]int{}
	for _, violation := range out.Violations {
		rule, _, _ := strings.Cut(violation.Message, " ")
		counts[rule]++
	}
	return counts
}

func TestEvaluateRealCleanAndViolating(t *testing.T) {
	realJars(t)
	artifacts := billingArtifacts(t)
	ctx := context.Background()

	clean := realEnv(t, compileSubject(t, "billing-clean"))
	if out := clean.Evaluate(ctx, billingEntry(), artifacts, testNow); out.Err != nil || len(out.Violations) != 0 {
		t.Fatalf("clean outcome %+v", out)
	}

	bad := realEnv(t, compileSubject(t, "billing-violating"))
	out := bad.Evaluate(ctx, billingEntry(), artifacts, testNow)
	counts := violationCounts(out)
	if out.Err != nil || counts["domain-not-to-adapters"] != 2 || counts["layers"] != 3 || len(out.Violations) != 5 {
		t.Fatalf("violating outcome %+v counts %v", out, counts)
	}
}

// Files that sit next to the classes must not change the verdict. ArchUnit and JUnit read
// them from the class path, and a native run would honor them.
func TestEvaluateRealIgnoresResourceFiles(t *testing.T) {
	realJars(t)
	hostile := map[string]string{
		"archunit_ignore_patterns.txt": ".*\n",
		"archunit.properties":          "archRule.failOnEmptyShould=false\n",
		"junit-platform.properties":    "junit.jupiter.conditions.deactivate=*\n",
		"META-INF/services/org.junit.platform.launcher.TestExecutionListener": "evil.Listener\n",
	}
	root := compileSubject(t, "billing-violating")
	addFiles(t, filepath.Join(root, "target", "classes"), hostile)
	env := realEnv(t, root)
	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	if out.Err != nil || len(out.Violations) != 5 {
		t.Fatalf("staged outcome %+v", out)
	}
}

// The two premises of the test above: left on the class path, one hostile file hides every violation
// and another stops the JVM before it writes a report.
func TestResourceFilesChangeANativeRun(t *testing.T) {
	realJars(t)
	cases := map[string]struct {
		files map[string]string
		check func(t *testing.T, out compiler.Outcome)
	}{
		"an ignore file": {
			map[string]string{"archunit_ignore_patterns.txt": ".*\n"},
			func(t *testing.T, out compiler.Outcome) {
				if out.Err != nil || len(out.Violations) != 0 {
					t.Fatalf("outcome %+v", out)
				}
			},
		},
		"a listener service file": {
			map[string]string{"META-INF/services/org.junit.platform.launcher.TestExecutionListener": "evil.Listener\n"},
			func(t *testing.T, out compiler.Outcome) {
				if compiler.CodeOf(out.Err) != CodeOutput {
					t.Fatalf("outcome %+v", out)
				}
			},
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			root := compileSubject(t, "billing-violating")
			classes := filepath.Join(root, "target", "classes")
			addFiles(t, classes, c.files)
			env := realEnv(t, root)
			native := *env
			native.classes = classTree{roots: []string{classes}, dirs: env.classes.dirs}
			c.check(t, native.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow))
		})
	}
}

func addFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// ArchUnit's DO_NOT_INCLUDE_TESTS option skips a location that looks like test output. ergo keeps the named
// path below the copy, so naming such a directory checks nothing, exactly as a native run would.
// That is an error for a forbidden rule and for a layers rule alike, never a set of violations.
func TestEvaluateRealSkipsANamedTestOutputDirectory(t *testing.T) {
	jars := realJars(t)
	layersOnly, err := generate(billingDecision, 0, []string{"com.acme.billing"}, []archRule{{
		name: "only-layers",
		kind: "layers",
		layers: []archLayer{
			{name: "web", packages: []string{"com.acme.billing.web.."}},
			{name: "domain", packages: []string{"com.acme.billing.domain.."}},
			{name: "persistence", packages: []string{"com.acme.billing.persistence.."}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	artifacts := map[string]map[string][]byte{
		"a forbidden rule and a layers rule": billingArtifacts(t),
		"a layers rule only":                 {artifactName: layersOnly},
	}
	for _, named := range []string{"target/test-classes", "build/classes/java/test"} {
		for kind, artifact := range artifacts {
			t.Run(named+" with "+kind, func(t *testing.T) {
				root := compileSubject(t, "billing-violating")
				dest := filepath.Join(root, filepath.FromSlash(named))
				if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(filepath.Join(root, "target", "classes"), dest); err != nil {
					t.Fatal(err)
				}
				env, err := Setup(context.Background(), root, jars, []string{named})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(env.Close)
				out := env.Evaluate(context.Background(), billingEntry(), artifact, testNow)
				if compiler.CodeOf(out.Err) != CodeEmpty || len(out.Violations) != 0 {
					t.Fatalf("outcome %+v", out)
				}
			})
		}
	}
}

// ArchUnit reads a class file as bytecode. A static initializer in the scanned classes must not run.
func TestEvaluateRealDoesNotInitializeClasses(t *testing.T) {
	realJars(t)
	root := compileSubject(t, "billing-violating")
	created := filepath.Join(t.TempDir(), "initialized")
	source := filepath.Join(root, "src", "com", "acme", "billing", "domain", "Boom.java")
	body := `package com.acme.billing.domain;

public class Boom {
    static {
        try {
            java.nio.file.Files.createFile(java.nio.file.Path.of("` + filepath.ToSlash(created) + `"));
        } catch (java.io.IOException e) {
            throw new IllegalStateException(e);
        }
    }

    public static void main(String[] args) {
    }
}
`
	if err := os.WriteFile(source, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	classes := filepath.Join(root, "target", "classes")
	if out, err := exec.Command("javac", "--release", "17", "-proc:none", "-d", classes, source).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}

	// The premise: loading the class as a program creates the file.
	if out, err := exec.Command("java", "-cp", classes, "com.acme.billing.domain.Boom").CombinedOutput(); err != nil {
		t.Fatalf("java: %v\n%s", err, out)
	}
	if _, err := os.Stat(created); err != nil {
		t.Fatalf("the static initializer did not run when the class was loaded: %v", err)
	}
	if err := os.Remove(created); err != nil {
		t.Fatal(err)
	}

	env := realEnv(t, root)
	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	if out.Err != nil || len(out.Violations) != 5 {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := os.Stat(created); err == nil {
		t.Fatal("ArchUnit ran the static initializer of a scanned class")
	}
}

func TestEvaluateRealRuleThatChecksNothing(t *testing.T) {
	realJars(t)
	env := realEnv(t, compileSubject(t, "billing-violating"))
	source, err := generate(billingDecision, 0, []string{"com.acme.billing"}, []archRule{
		{name: "nothing-here", kind: "forbidden", source: []string{"com.acme.nothing.."}, forbidden: []string{"com.acme.web.."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entry := billingEntry()
	entry.Waivers = nil
	out := env.Evaluate(context.Background(), entry, map[string][]byte{artifactName: source}, testNow)
	if compiler.CodeOf(out.Err) != CodeEmpty {
		t.Fatalf("outcome %+v", out)
	}
}

func TestEvaluateRealUnreadableClasses(t *testing.T) {
	realJars(t)
	root := t.TempDir()
	classes := filepath.Join(root, "target", "classes", "com", "acme", "billing")
	if err := os.MkdirAll(classes, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(classes, "Broken.class"), []byte("not a class file"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := realEnv(t, root)
	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	t.Logf("outcome %+v", out)
	if out.Err == nil && len(out.Violations) == 0 {
		t.Fatal("a tree with no readable class passed")
	}
}
