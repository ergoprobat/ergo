package eslint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/toolrun"
)

const (
	versionTimeout = 30 * time.Second
	checkTimeout   = 10 * time.Minute
	commandWait    = 5 * time.Second
	maxStdoutBytes = 256 << 20
	stderrLimit    = 1024
	probeLimit     = 4096
	packageLimit   = 1 << 20

	envPrefix       = "ESLINT_"
	timingVar       = "TIMING"
	scratchPrefix   = ".ipso-eslint-"
	suppressionName = "suppressions.json"
	// noSuppressions is a suppressions file with no entries. ESLint parses the file as JSON, so a zero-byte file fails.
	noSuppressions = "{}\n"
)

// Env is one evaluation run: the eslint binary from PATH, the directory that holds its packages, and
// the paths to lint. Setup creates it and Close removes it. Checks run one at a time, each in its own process.
type Env struct {
	// dir is the working directory of the version probe. It is empty and outside the repository.
	dir string
	// scratch is a directory inside the node_modules directory of eslint. Each check writes its config
	// there, so that the config imports typescript-eslint from the same install that runs it.
	scratch string
	// modules is the node_modules directory that holds .bin/eslint.
	modules string
	// root is the repository root with symlinks resolved. ESLint reports file paths against it.
	root    string
	paths   []string
	version string
	// pins is nil when typescript-eslint and typescript are the pinned releases.
	pins error
	// timeout, outputLimit, and wait override the production bounds. Zero means the default.
	timeout     time.Duration
	outputLimit int
	wait        time.Duration
}

// Version is the release of the eslint binary the run found, without the leading v.
// It is not the pinned release when the binary on PATH is another one, and then every check is an error.
func (e *Env) Version() string { return e.version }

// Setup prepares one evaluation run.
// paths are the files and directories to lint. They are repository-relative, slash-separated, and already
// normalized. Setup checks them, finds eslint on PATH, checks where its packages are, and reads its
// version. A named path that is missing or leaves the repository through a symlink, an eslint whose
// packages sit inside the repository, and a probe that fails stop the run before any check starts.
func Setup(ctx context.Context, root string, paths []string) (*Env, error) {
	realRoot, err := resolve(root)
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	if err := toolrun.WithinRoot(realRoot, paths); err != nil {
		return nil, &compiler.Error{Code: CodePath, Err: err}
	}
	if err := checkExists(realRoot, paths); err != nil {
		return nil, err
	}
	modules, err := locate(realRoot)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "ipso-eslint-")
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	env := &Env{dir: dir, modules: modules, root: realRoot, paths: append([]string(nil), paths...)}
	if err := env.probe(ctx); err != nil {
		env.Close()
		return nil, err
	}
	env.scratch, err = os.MkdirTemp(modules, scratchPrefix)
	if err != nil {
		env.Close()
		return nil, &compiler.Error{Code: CodeRun, Err: fmt.Errorf("the packages of eslint are not writable: %w", err)}
	}
	env.pins = typeScriptPins(modules)
	return env, nil
}

// Close removes the temporary directories.
func (e *Env) Close() {
	_ = os.RemoveAll(e.dir)
	if e.scratch != "" {
		_ = os.RemoveAll(e.scratch)
	}
}

func resolve(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func checkExists(root string, paths []string) error {
	repo, err := os.OpenRoot(root)
	if err != nil {
		return &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = repo.Close() }()
	for _, rel := range paths {
		if rel == "." {
			continue
		}
		if _, err := repo.Stat(rel); err != nil {
			return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s is missing or cannot be read", rel)}
		}
	}
	return nil
}

// locate resolves eslint on PATH and returns the node_modules directory that holds its binary link.
//
// The command name is a literal, so Go resolves it with LookPath and refuses a binary found relative
// to the current directory. There is no variable, no flag, and no lookup inside the repository. The
// binary must be <dir>/node_modules/.bin/eslint next to <dir>/node_modules/eslint, and neither that
// directory nor the real path of the binary may be inside the repository: eslint, typescript-eslint,
// and TypeScript decide the verdict, so they must not come from the tree under review.
func locate(root string) (string, error) {
	found, err := exec.LookPath("eslint")
	if err != nil {
		return "", missing(errors.New("eslint is not on PATH"))
	}
	bin, err := filepath.Abs(found)
	if err != nil {
		return "", missing(err)
	}
	binDir := filepath.Dir(bin)
	modules := filepath.Dir(binDir)
	name := strings.TrimSuffix(filepath.Base(bin), filepath.Ext(bin))
	if !strings.EqualFold(name, "eslint") || filepath.Base(binDir) != ".bin" || filepath.Base(modules) != "node_modules" {
		return "", missing(errors.New("eslint on PATH is not <dir>/node_modules/.bin/eslint"))
	}
	info, err := os.Stat(filepath.Join(modules, "eslint", "package.json"))
	if err != nil || !info.Mode().IsRegular() {
		return "", missing(errors.New("eslint is not installed next to its binary link"))
	}
	for _, p := range []string{modules, bin} {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return "", missing(err)
		}
		if _, inside := within(root, resolved); inside {
			return "", missing(errors.New("eslint is installed inside the repository"))
		}
	}
	return modules, nil
}

func missing(err error) error {
	return &compiler.Error{Code: CodeMissing, Err: err}
}

// probe runs `eslint --version` once, in an empty directory outside the repository. The version is
// compared with the pin by Evaluate, so a check that disagrees fails on its own.
func (e *Env) probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "eslint", "--version")
	cmd.Dir = e.dir
	cmd.Env = childEnv()
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: probeLimit, Stderr: stderrLimit, Wait: commandWait})
	if err != nil {
		return missing(fmt.Errorf("eslint --version: %s", toolrun.Excerpt(toolrun.Stderr(err), "eslint failed")))
	}
	found, ok := strings.CutPrefix(strings.TrimSpace(string(out.Stdout)), "v")
	if _, _, _, valid := toolrun.ParseSemver(found); !ok || !valid {
		return &compiler.Error{Code: CodeVersion, Err: errors.New("the version output is not the shape ipso reads")}
	}
	e.version = found
	return nil
}

// typeScriptPins checks the releases of typescript-eslint and typescript next to eslint. Only a typescript
// check needs them. A package that is not installed is missing, and any other release is a version error.
func typeScriptPins(modules string) error {
	dir, err := os.OpenRoot(modules)
	if err != nil {
		return missing(err)
	}
	defer func() { _ = dir.Close() }()
	for _, pin := range []struct{ name, want string }{
		{"typescript-eslint", typescriptESLintVersion},
		{"typescript", typescriptVersion},
	} {
		found, err := packageVersion(dir, pin.name)
		if err != nil {
			return missing(fmt.Errorf("%s is not installed next to eslint", pin.name))
		}
		if found != pin.want {
			return &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("%s %s, ipso runs %s", pin.name, found, pin.want)}
		}
	}
	return nil
}

func packageVersion(modules *os.Root, name string) (string, error) {
	file, err := modules.Open(name + "/package.json")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, packageLimit))
	if err != nil {
		return "", err
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", err
	}
	return manifest.Version, nil
}

// Evaluate runs one compiled check and applies its waivers.
// The check runs the canonical config recovered from the verified artifact, never a file of the
// repository, over the named paths.
func (e *Env) Evaluate(ctx context.Context, entry compiler.Entry, artifacts map[string][]byte, now time.Time) compiler.Outcome {
	if entry.CompilerVersion != version || e.version != version {
		return compiler.Outcome{Err: &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("eslint %s and a check compiled for %s, ipso runs %s", e.version, entry.CompilerVersion, version)}}
	}
	config, ok := artifacts[artifactName]
	if !ok {
		return compiler.Outcome{Err: &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("missing %s", artifactName)}}
	}
	rules, err := readCanonical(config, entry.Target, entry.Mode)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	if entry.Target == targetTypeScript && e.pins != nil {
		return compiler.Outcome{Err: e.pins}
	}
	stdout, exit, err := e.run(ctx, config)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	findings, err := ParseResults(stdout, exit, rules, entry.Mode, e.root)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	var violations []compiler.Violation
	for _, finding := range findings {
		violations = append(violations, finding.violation())
	}
	return compiler.Cover(entry, compiler.Outcome{Violations: violations}, now)
}

// run starts eslint with a copy of the config and a suppressions file that holds {}, in the repository root.
// exit is its status: 0 when no message has severity error, and 1 when one has. Exit 1 with no results
// is an error, and so is any other status, such as 2 for a configuration or load error.
//
// --no-inline-config makes ESLint ignore every directive comment, so no message about a directive reaches
// the reader. --no-error-on-unmatched-pattern lets checks with different file patterns share the same paths,
// and a check that lints nothing is eslint-empty. ipso never passes --fix, --cache, --no-ignore, --rule,
// --flag, --max-warnings, --quiet, --suppress-*, --output-file, --concurrency, or --inspect-config.
func (e *Env) run(ctx context.Context, config []byte) (stdout []byte, exit int, err error) {
	dir, err := os.MkdirTemp(e.scratch, "check-")
	if err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	configPath := filepath.Join(dir, artifactName)
	suppressionsPath := filepath.Join(dir, suppressionName)
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	if err := os.WriteFile(suppressionsPath, []byte(noSuppressions), 0o600); err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, e.checkLimit())
	defer cancel()
	args := []string{
		"--config", configPath,
		"--format", "json",
		"--no-inline-config",
		"--no-warn-ignored",
		"--no-error-on-unmatched-pattern",
		"--suppressions-location", suppressionsPath,
	}
	cmd := exec.CommandContext(ctx, "eslint", append(args, e.paths...)...)
	cmd.Dir = e.root
	cmd.Env = childEnv()
	out, err := toolrun.Run(ctx, cmd, e.limits())
	if err == nil {
		return out.Stdout, 0, nil
	}
	var exitErr *toolrun.ExitError
	if errors.As(err, &exitErr) {
		var status *exec.ExitError
		if errors.As(exitErr, &status) {
			return statusOf(exitErr, status.ExitCode())
		}
	}
	return nil, 0, runErr(err, e.outputMax())
}

func statusOf(exitErr *toolrun.ExitError, code int) ([]byte, int, error) {
	if code == 1 && len(exitErr.Stdout) > 0 {
		return exitErr.Stdout, 1, nil
	}
	return nil, 0, &compiler.Error{Code: CodeRun, Err: fmt.Errorf("eslint exited with status %d: %s", code, toolrun.Excerpt(exitErr.Stderr, "no message"))}
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
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", toolrun.Excerpt(toolrun.Stderr(err), "eslint failed"))}
	}
}

// childEnv is the runner environment without ESLINT_* variables and TIMING. ESLINT_FLAGS turns on feature
// flags that change how ESLint reads its config and files, and TIMING makes it print a table after the
// results. Every other variable is inherited, including NODE_OPTIONS, so the step that runs ipso should
// not export secrets.
func childEnv() []string {
	environ := os.Environ()
	out := make([]string, 0, len(environ))
	for _, item := range environ {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, envPrefix) || upper == timingVar {
			continue
		}
		out = append(out, item)
	}
	return out
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
