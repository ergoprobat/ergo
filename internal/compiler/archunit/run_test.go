package archunit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
)

var testNow = time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)

const billingDecision = "01M5WVYECH306K6DDJXR2VAPCS"

func billingEntry() compiler.Entry {
	return compiler.Entry{DecisionID: billingDecision, Slug: "billing", CheckIndex: 0, Engine: "archunit", Mode: "block", CompilerVersion: version}
}

func billingArtifacts(t *testing.T) map[string][]byte {
	t.Helper()
	_, source := goldenTest(t, "billing-violating")
	return map[string][]byte{artifactName: source}
}

// fakeEnv sets up an Env over a fake JDK, fake jars, and one class directory.
func fakeEnv(t *testing.T) (*Env, string) {
	t.Helper()
	installFakeJDK(t)
	jars := fakeJars(t)
	root := t.TempDir()
	writeFiles(t, root,
		"target/classes/com/acme/billing/domain/Order.class",
		"target/classes/archunit.properties",
	)
	env, err := Setup(context.Background(), root, jars, []string{"target/classes"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	return env, root
}

func fakeReport(t *testing.T, name string) {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "junit", name+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_JAVA_REPORT", path)
}

func TestJDKMajor(t *testing.T) {
	for output, want := range map[string]int{
		"openjdk 21.0.4 2024-07-16 LTS\nOpenJDK Runtime Environment": 21,
		"openjdk 17 2021-09-14":      17,
		"java 17.0.1 2021-10-19 LTS": 17,
		"openjdk 22-ea 2024-03-19":   22,
		"javac 21.0.4":               21,
		"javac 11.0.2\n":             11,
		"javac 25+36":                25,
		"":                           0,
		"openjdk":                    0,
		"openjdk x.y":                0,
		"openjdk 0.1":                0,
		"openjdk -1":                 0,
		// java -version prints this form. ergo runs java --version, so a tool that prints it is refused.
		`openjdk version "17.0.13" 2024-10-15`: 0,
	} {
		got, ok := jdkMajor(output)
		if (want == 0) == ok || got != want {
			t.Errorf("%q: got %d, %v; want %d", output, got, ok, want)
		}
	}
}

func TestSetupJDK(t *testing.T) {
	cases := map[string]struct {
		java, javac string
		code        string
	}{
		"current":          {"openjdk 21.0.4 2024-07-16 LTS", "21.0.4", ""},
		"the oldest":       {"openjdk 17 2021-09-14", "17.0.1", ""},
		"old java":         {"openjdk 11.0.2 2019-01-15", "21.0.4", CodeVersion},
		"old javac":        {"openjdk 21.0.4 2024-07-16", "11.0.2", CodeVersion},
		"unreadable java":  {"garbage", "21.0.4", CodeVersion},
		"unreadable javac": {"openjdk 21.0.4 2024-07-16", "", CodeVersion},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			installFakeJDK(t)
			t.Setenv("FAKE_JAVA_VERSION", c.java)
			t.Setenv("FAKE_JAVAC_VERSION", c.javac)
			root := t.TempDir()
			writeFiles(t, root, "classes/a/A.class")
			env, err := Setup(context.Background(), root, fakeJars(t), []string{"classes"})
			if compiler.CodeOf(err) != c.code {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
			if err == nil {
				env.Close()
			}
		})
	}
}

// The version probes run in the run's temporary directory, not in the repository.
func TestSetupProbesTheJDKOutsideTheRepository(t *testing.T) {
	installFakeJDK(t)
	probe := filepath.Join(t.TempDir(), "pwd")
	t.Setenv("FAKE_JAVA_VERSION_PWD", probe)
	root := t.TempDir()
	writeFiles(t, root, "classes/a/A.class")
	withWorkingDirectory(t, root)
	env, err := Setup(context.Background(), root, fakeJars(t), []string{"classes"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	if got := resolved(t, strings.TrimSpace(readText(t, probe))); got != resolved(t, env.dir) {
		t.Fatalf("java --version ran in %q, not in %q", got, env.dir)
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
}

func TestSetupMissingTools(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"no java":  func(t *testing.T, dir string) { remove(t, dir, "java") },
		"no javac": func(t *testing.T, dir string) { remove(t, dir, "javac") },
		"version fails": func(t *testing.T, dir string) {
			script := "#!/bin/sh\necho 'Unrecognized option: --version' >&2\nexit 3\n"
			if err := os.WriteFile(filepath.Join(dir, "java"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			installFakeJDK(t)
			change(t, os.Getenv("PATH"))
			root := t.TempDir()
			writeFiles(t, root, "classes/a/A.class")
			_, err := Setup(context.Background(), root, fakeJars(t), []string{"classes"})
			if compiler.CodeOf(err) != CodeMissing {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}

func remove(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func TestSetupCleansUp(t *testing.T) {
	installFakeJDK(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	root := t.TempDir()
	writeFiles(t, root, "classes/a/A.class")

	badJars := fakeJars(t)
	writeFiles(t, badJars, pinnedJars[0].name)
	if _, err := Setup(context.Background(), root, badJars, []string{"classes"}); compiler.CodeOf(err) != CodeJar {
		t.Fatalf("jar err %v", err)
	}
	if _, err := Setup(context.Background(), root, fakeJars(t), []string{"missing"}); compiler.CodeOf(err) != CodePath {
		t.Fatalf("path err %v", err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("a failed Setup left %d entries", len(left))
	}

	env, err := Setup(context.Background(), root, fakeJars(t), []string{"classes"})
	if err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 1 {
		t.Fatalf("a ready Env owns one directory, found %d", len(left))
	}
	env.Close()
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Fatalf("Close left %d entries", len(left))
	}
}

func TestEvaluateRunsTheCanonicalTest(t *testing.T) {
	env, root := fakeEnv(t)
	scratch := t.TempDir()
	javacArgs := filepath.Join(scratch, "javac")
	javaArgs := filepath.Join(scratch, "java")
	javaEnv := filepath.Join(scratch, "env")
	javaPwd := filepath.Join(scratch, "pwd")
	t.Setenv("FAKE_JAVAC_ARGS", javacArgs)
	t.Setenv("FAKE_JAVA_ARGS", javaArgs)
	t.Setenv("FAKE_JAVA_ENV", javaEnv)
	t.Setenv("FAKE_JAVA_PWD", javaPwd)
	t.Setenv("JAVA_TOOL_OPTIONS", "-javaagent:/tmp/agent.jar")
	t.Setenv("JDK_JAVA_OPTIONS", "-Darchunit.x=1")
	t.Setenv("_JAVA_OPTIONS", "-Xmx1k")
	t.Setenv("JDK_JAVAC_OPTIONS", "-processorpath /tmp")
	t.Setenv("CLASSPATH", "/tmp/evil")
	t.Setenv("ERGO_TEST_KEEP", "kept")
	fakeReport(t, "pass")

	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	if out.Err != nil || len(out.Violations) != 0 || out.Waived {
		t.Fatalf("outcome %+v", out)
	}

	javac := strings.Split(strings.TrimSpace(readText(t, javacArgs)), "\n")
	for _, want := range []string{"--release", "17", "-proc:none", "-encoding", "UTF-8", "-d", "-cp"} {
		if !contains(javac, want) {
			t.Errorf("javac args miss %q: %v", want, javac)
		}
	}
	if last := javac[len(javac)-1]; filepath.Base(last) != "ArchitectureTest.java" || strings.HasPrefix(last, root) {
		t.Errorf("javac compiles %q", last)
	}
	_, golden := goldenTest(t, "billing-violating")
	if got := readText(t, javacArgs+".source"); got != string(golden) {
		t.Errorf("javac compiled other bytes than the canonical test")
	}
	classpath := javac[indexOf(javac, "-cp")+1]
	if want := env.jar(archunitJarName) + string(os.PathListSeparator) + env.jar(consoleJarName); classpath != want {
		t.Errorf("javac classpath %q, want %q", classpath, want)
	}

	java := strings.Split(strings.TrimSpace(readText(t, javaArgs)), "\n")
	head := strings.Join(java[:3], " ")
	if head != "-jar "+env.jar(consoleJarName)+" execute" {
		t.Errorf("java starts %q", head)
	}
	for _, want := range []string{"--disable-banner", "--disable-ansi-colors", "--details=none", "--fail-if-no-tests", "--include-engine=junit-jupiter"} {
		if !contains(java, want) {
			t.Errorf("java args miss %q: %v", want, java)
		}
	}
	if got := java[indexOf(java, "--select-class")+1]; got != billingClass {
		t.Errorf("selects %q", got)
	}
	entries := strings.Split(java[indexOf(java, "--class-path")+1], string(os.PathListSeparator))
	if len(entries) != 4 ||
		filepath.Base(entries[0]) != "test" ||
		entries[1] != env.jar(archunitJarName) ||
		entries[2] != env.jar(slf4jJarName) ||
		entries[3] != filepath.Join(env.dir, "classes", "target", "classes") {
		t.Errorf("class path %v", entries)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry, root) {
			t.Errorf("class path entry %q is inside the repository", entry)
		}
	}

	seen := readText(t, javaEnv)
	for _, want := range []string{"tool=\n", "jdk=\n", "_java=\n", "javac=\n", "classpath=\n", "keep=kept\n"} {
		if !strings.Contains(seen, want) {
			t.Errorf("environment %q lacks %q", seen, want)
		}
	}
	if pwd := strings.TrimSpace(readText(t, javaPwd)); resolved(t, filepath.Dir(pwd)) != resolved(t, env.dir) || !strings.HasPrefix(filepath.Base(pwd), "check-") {
		t.Errorf("java runs in %q, not in a check directory of %q", pwd, env.dir)
	}

	entriesLeft, err := os.ReadDir(env.dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entriesLeft {
		if strings.HasPrefix(entry.Name(), "check-") {
			t.Errorf("a finished check left %s", entry.Name())
		}
	}
}

func TestEvaluateViolations(t *testing.T) {
	env, _ := fakeEnv(t)
	fakeReport(t, "violated")
	t.Setenv("FAKE_JAVA_EXIT", "1")
	t.Setenv("FAKE_JAVA_STDOUT", "Failures (2): ...")
	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	if out.Err != nil || len(out.Violations) != 5 || out.Waived {
		t.Fatalf("outcome %+v", out)
	}
	if !strings.HasPrefix(out.Violations[0].Message, "domain-not-to-adapters Method <com.acme.billing.domain.Order.leak()>") {
		t.Fatalf("first violation %q", out.Violations[0].Message)
	}

	entry := billingEntry()
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	out = env.Evaluate(context.Background(), entry, billingArtifacts(t), testNow)
	if out.Err != nil || !out.Waived || len(out.Violations) != 5 {
		t.Fatalf("waived outcome %+v", out)
	}
}

func TestEvaluateErrorsAreNeverWaived(t *testing.T) {
	env, _ := fakeEnv(t)
	fakeReport(t, "empty-tree")
	t.Setenv("FAKE_JAVA_EXIT", "1")
	entry := billingEntry()
	entry.Waivers = []compiler.ManifestWaiver{{Approver: "Ada", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}}
	out := env.Evaluate(context.Background(), entry, billingArtifacts(t), testNow)
	if compiler.CodeOf(out.Err) != CodeEmpty || out.Waived {
		t.Fatalf("outcome %+v", out)
	}
}

func TestEvaluateRunFailures(t *testing.T) {
	cases := map[string]struct {
		set  func(t *testing.T, env *Env)
		code string
		text string
	}{
		"console status 2": {func(t *testing.T, _ *Env) {
			t.Setenv("FAKE_JAVA_EXIT", "2")
			t.Setenv("FAKE_JAVA_STDERR", "boom")
		}, CodeRun, "boom"},
		"javac fails": {func(t *testing.T, _ *Env) {
			t.Setenv("FAKE_JAVAC_EXIT", "1")
			t.Setenv("FAKE_JAVAC_STDERR", "cannot find symbol")
		}, CodeRun, "javac failed: cannot find symbol"},
		"timeout": {func(t *testing.T, env *Env) {
			env.timeout = 500 * time.Millisecond
			env.wait = 200 * time.Millisecond
			t.Setenv("FAKE_JAVA_SLEEP", "30")
		}, CodeRun, "timed out"},
		"too much output": {func(t *testing.T, env *Env) {
			env.outputLimit = 16
			t.Setenv("FAKE_JAVA_STDOUT", strings.Repeat("x", 64))
		}, CodeOutput, "larger than 16 bytes"},
		"no report": {func(t *testing.T, _ *Env) {
			t.Setenv("FAKE_JAVA_REPORT", "")
		}, CodeOutput, "no report"},
		"report over the cap": {func(t *testing.T, env *Env) {
			env.outputLimit = 64
		}, CodeOutput, "larger than 64 bytes"},
		"status 1 and a passing report": {func(t *testing.T, _ *Env) {
			t.Setenv("FAKE_JAVA_EXIT", "1")
		}, CodeOutput, "disagree"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env, _ := fakeEnv(t)
			fakeReport(t, "pass")
			c.set(t, env)
			out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
			if compiler.CodeOf(out.Err) != c.code || !strings.Contains(out.Err.Error(), c.text) {
				t.Fatalf("outcome %+v", out)
			}
		})
	}
}

func TestEvaluateRefusesBeforeRunningJava(t *testing.T) {
	good := billingArtifacts(t)
	tampered := map[string][]byte{artifactName: append(append([]byte(nil), good[artifactName]...), "// extra\n"...)}
	cases := map[string]struct {
		entry     func() compiler.Entry
		artifacts map[string][]byte
		code      string
	}{
		"another compiler version": {func() compiler.Entry { e := billingEntry(); e.CompilerVersion = "1.5.0"; return e }, good, CodeVersion},
		"no artifact":              {billingEntry, map[string][]byte{}, compiler.CodeBuildManifest},
		"another artifact":         {billingEntry, map[string][]byte{"other": []byte("x")}, compiler.CodeBuildManifest},
		"edited artifact":          {billingEntry, tampered, compiler.CodeBuildManifest},
		"another check index":      {func() compiler.Entry { e := billingEntry(); e.CheckIndex = 3; return e }, good, compiler.CodeBuildManifest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			env, _ := fakeEnv(t)
			fakeReport(t, "pass")
			marker := filepath.Join(t.TempDir(), "java")
			t.Setenv("FAKE_JAVA_ARGS", marker)
			out := env.Evaluate(context.Background(), c.entry(), c.artifacts, testNow)
			if compiler.CodeOf(out.Err) != c.code {
				t.Fatalf("outcome %+v", out)
			}
			if _, err := os.Stat(marker); err == nil {
				t.Fatal("java ran")
			}
		})
	}
}

func TestEvaluateEmptyClassTree(t *testing.T) {
	installFakeJDK(t)
	root := t.TempDir()
	writeFiles(t, root, "target/classes/org/other/Other.class")
	env, err := Setup(context.Background(), root, fakeJars(t), []string{"target/classes"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	fakeReport(t, "pass")
	marker := filepath.Join(t.TempDir(), "java")
	t.Setenv("FAKE_JAVA_ARGS", marker)
	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	if compiler.CodeOf(out.Err) != CodeEmpty {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("java ran")
	}
}

// Naming the repository root keeps every class below its path from the root, so the packages are not found
// and the check fails closed instead of passing.
func TestEvaluateRepositoryRootAsClassDirectoryIsEmpty(t *testing.T) {
	installFakeJDK(t)
	root := t.TempDir()
	writeFiles(t, root, "target/classes/com/acme/billing/domain/Order.class")
	env, err := Setup(context.Background(), root, fakeJars(t), []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(env.Close)
	fakeReport(t, "pass")
	marker := filepath.Join(t.TempDir(), "java")
	t.Setenv("FAKE_JAVA_ARGS", marker)
	out := env.Evaluate(context.Background(), billingEntry(), billingArtifacts(t), testNow)
	if compiler.CodeOf(out.Err) != CodeEmpty {
		t.Fatalf("outcome %+v", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("java ran")
	}
}

func contains(list []string, want string) bool { return indexOf(list, want) >= 0 }

func indexOf(list []string, want string) int {
	for i, item := range list {
		if item == want {
			return i
		}
	}
	return -1
}

func resolved(t *testing.T, path string) string {
	t.Helper()
	out, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
