package archunit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/toolrun"
)

const (
	versionTimeout = 30 * time.Second
	checkTimeout   = 10 * time.Minute
	commandWait    = 5 * time.Second
	maxStdoutBytes = 256 << 20
	stderrLimit    = 1024
	versionLimit   = 1024
	// minJava is the oldest JDK that runs JUnit Platform 6.
	minJava = 17
	// javaRelease is the class file level of the generated test.
	javaRelease = "17"
	reportName  = "TEST-junit-jupiter.xml"
)

// Env is one evaluation run: the verified jars, the copied class files, and a JDK from PATH.
// Setup creates it and Close removes it. Checks run one at a time in separate JVMs.
type Env struct {
	dir     string
	classes classTree
	// timeout, outputLimit, and wait override the production bounds. Zero means the default.
	timeout     time.Duration
	outputLimit int
	wait        time.Duration
}

// Version is the ArchUnit release the verified jars carry.
func (*Env) Version() string { return version }

// Setup prepares one evaluation run.
// It verifies the pinned jars found in jarsDir, finds java and javac on PATH, and copies the class
// files below paths. paths are repository-relative, slash-separated, and already normalized.
func Setup(ctx context.Context, root, jarsDir string, paths []string) (*Env, error) {
	dir, err := os.MkdirTemp("", "ergo-archunit-")
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	env := &Env{dir: dir}
	if err := env.prepare(ctx, root, jarsDir, paths); err != nil {
		env.Close()
		return nil, err
	}
	return env, nil
}

func (e *Env) prepare(ctx context.Context, root, jarsDir string, paths []string) error {
	if err := stageJars(jarsDir, e.libDir()); err != nil {
		return err
	}
	if err := checkJDK(ctx, e.dir); err != nil {
		return err
	}
	tree, err := stageClasses(ctx, root, paths, filepath.Join(e.dir, "classes"))
	if err != nil {
		return err
	}
	e.classes = tree
	return nil
}

// Close removes the temporary directory.
func (e *Env) Close() {
	_ = os.RemoveAll(e.dir)
}

func (e *Env) libDir() string { return filepath.Join(e.dir, "lib") }

func (e *Env) jar(name string) string { return filepath.Join(e.libDir(), name) }

// jdkTool is one tool the JDK check runs. Each command name is a literal at its call site, so Go resolves it
// with LookPath and nothing from the repository or the environment can pick the binary.
type jdkTool struct {
	name    string
	command func(ctx context.Context) *exec.Cmd
}

// checkJDK reads the version of java and javac. Both must be 17 or newer.
// The tools run in dir, never in the repository.
func checkJDK(ctx context.Context, dir string) error {
	tools := []jdkTool{
		{"java", func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "java", "--version") }},
		{"javac", func(ctx context.Context) *exec.Cmd { return exec.CommandContext(ctx, "javac", "--version") }},
	}
	for _, tool := range tools {
		if _, err := exec.LookPath(tool.name); err != nil {
			return &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("%s is not on PATH", tool.name)}
		}
		out, err := runVersion(ctx, dir, tool)
		if err != nil {
			return err
		}
		major, ok := jdkMajor(out)
		if !ok || major < minJava {
			return &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("%s needs JDK %d or newer", tool.name, minJava)}
		}
	}
	return nil
}

func runVersion(ctx context.Context, dir string, tool jdkTool) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := tool.command(ctx)
	cmd.Dir = dir
	cmd.Env = jvmEnv()
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: versionLimit, Stderr: stderrLimit, Wait: commandWait})
	if err != nil {
		return "", &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("%s --version: %s", tool.name, toolrun.Excerpt(toolrun.Stderr(err), "failed"))}
	}
	return string(out.Stdout), nil
}

// jdkMajor reads the major version from the first line of java --version or javac --version,
// such as "openjdk 21.0.4 2024-07-16 LTS" or "javac 21.0.4". The quoted form
// `openjdk version "21.0.4"` is what java -version prints, which ergo does not run, so it is unreadable here.
func jdkMajor(output string) (int, bool) {
	line, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	digits := fields[1]
	if end := strings.IndexAny(digits, ".+-"); end >= 0 {
		digits = digits[:end]
	}
	major, err := strconv.Atoi(digits)
	if err != nil || major < 1 {
		return 0, false
	}
	return major, true
}

// jvmEnv is the runner environment without the variables that change what a JVM or javac loads.
// A step that ran earlier can write these through the job environment: a -javaagent, an
// archunit.* property, or a class path.
func jvmEnv() []string {
	drop := map[string]struct{}{
		"JAVA_TOOL_OPTIONS": {},
		"JDK_JAVA_OPTIONS":  {},
		"_JAVA_OPTIONS":     {},
		"JDK_JAVAC_OPTIONS": {},
		"CLASSPATH":         {},
	}
	out := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if _, ok := drop[strings.ToUpper(key)]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

// run compiles the canonical test and executes it with the JUnit console.
// exit is the console status. 0 means every rule passed and 1 means a rule failed.
func (e *Env) run(ctx context.Context, test Canonical, source []byte) (report []byte, exit int, err error) {
	dir, err := os.MkdirTemp(e.dir, "check-")
	if err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	sourcePath := filepath.Join(dir, "ArchitectureTest.java")
	testDir := filepath.Join(dir, "test")
	reportDir := filepath.Join(dir, "reports")
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	for _, sub := range []string{testDir, reportDir} {
		if err := os.Mkdir(sub, 0o700); err != nil {
			return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, e.checkLimit())
	defer cancel()

	javac := exec.CommandContext(ctx, "javac",
		"--release", javaRelease,
		"-proc:none",
		"-encoding", "UTF-8",
		"-d", testDir,
		"-cp", strings.Join([]string{e.jar(archunitJarName), e.jar(consoleJarName)}, string(os.PathListSeparator)),
		sourcePath,
	)
	javac.Dir = dir
	javac.Env = jvmEnv()
	if _, err := toolrun.Run(ctx, javac, e.limits()); err != nil {
		return nil, 0, e.javacErr(err)
	}

	classpath := []string{testDir, e.jar(archunitJarName), e.jar(slf4jJarName)}
	classpath = append(classpath, e.classes.roots...)
	java := exec.CommandContext(ctx, "java",
		"-jar", e.jar(consoleJarName),
		"execute",
		"--disable-banner",
		"--disable-ansi-colors",
		"--details=none",
		"--fail-if-no-tests",
		"--include-engine=junit-jupiter",
		"--class-path", strings.Join(classpath, string(os.PathListSeparator)),
		"--select-class", test.Class,
		"--reports-dir", reportDir,
	)
	java.Dir = dir
	java.Env = jvmEnv()
	exit, err = e.runConsole(ctx, java)
	if err != nil {
		return nil, 0, err
	}
	raw, err := readReport(filepath.Join(reportDir, reportName), e.outputMax())
	if err != nil {
		return nil, 0, err
	}
	return raw, exit, nil
}

// runConsole runs the JUnit console. Status 1 is a failed rule and is not an error here: the report says which.
func (e *Env) runConsole(ctx context.Context, cmd *exec.Cmd) (int, error) {
	_, err := toolrun.Run(ctx, cmd, e.limits())
	if err == nil {
		return 0, nil
	}
	var exitErr *toolrun.ExitError
	if errors.As(err, &exitErr) {
		var status *exec.ExitError
		if errors.As(exitErr, &status) && status.ExitCode() == 1 {
			return 1, nil
		}
	}
	return 0, runErr(err, e.outputMax())
}

func (e *Env) javacErr(err error) error {
	var exitErr *toolrun.ExitError
	if errors.As(err, &exitErr) {
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("javac failed: %s", toolrun.Excerpt(exitErr.Stderr, "no output"))}
	}
	return runErr(err, e.outputMax())
}

func runErr(err error, limit int) error {
	var timeout *toolrun.TimeoutError
	var over *toolrun.OutputError
	switch {
	case errors.As(err, &timeout):
		return &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
	case errors.As(err, &over):
		return &compiler.Error{Code: CodeOutput, Err: fmt.Errorf("output is larger than %d bytes", limit)}
	default:
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", toolrun.Excerpt(toolrun.Stderr(err), "java failed"))}
	}
}

func (e *Env) limits() toolrun.Limits {
	return toolrun.Limits{Stdout: e.outputMax(), Stderr: stderrLimit, Wait: e.waitDelay()}
}

func (e *Env) checkLimit() time.Duration {
	if e.timeout > 0 {
		return e.timeout
	}
	return checkTimeout
}

func (e *Env) waitDelay() time.Duration {
	if e.wait > 0 {
		return e.wait
	}
	return commandWait
}

func (e *Env) outputMax() int {
	if e.outputLimit > 0 {
		return e.outputLimit
	}
	return maxStdoutBytes
}
