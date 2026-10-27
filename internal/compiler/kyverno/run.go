package kyverno

import (
	"context"
	"errors"
	"fmt"
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

	versionPrefix = "Version: "
	envPrefix     = "KYVERNO_"
)

// Env is one evaluation run: the kyverno binary from PATH and the copied manifests.
// Setup creates it and Close removes it. Checks run one at a time, each in its own process.
type Env struct {
	dir       string
	resources string
	version   string
	// timeout, outputLimit, and wait override the production bounds. Zero means the default.
	timeout     time.Duration
	outputLimit int
	wait        time.Duration
}

// Version is the release of the kyverno binary the run found.
// It is not the pinned release when the binary on PATH is another one, and then every check is an error.
func (e *Env) Version() string { return e.version }

// Setup prepares one evaluation run.
// paths are the manifest files and directories to check. They are repository-relative, slash-separated,
// and already normalized. Setup checks them, finds kyverno on PATH, reads its version, and copies the
// manifests. A named path that is missing, leaves the repository, or holds a symlink, and a tree that
// holds no manifest, fail the run before any check starts.
func Setup(ctx context.Context, root string, paths []string) (*Env, error) {
	named := distinct(paths)
	if err := toolrun.WithinRoot(root, named); err != nil {
		return nil, &compiler.Error{Code: CodePath, Err: err}
	}
	repo, err := os.OpenRoot(root)
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = repo.Close() }()
	dir, err := os.MkdirTemp("", "ipso-kyverno-")
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	env := &Env{dir: dir, resources: filepath.Join(dir, "resources")}
	if err := env.probe(ctx); err != nil {
		env.Close()
		return nil, err
	}
	if _, err := stageManifests(ctx, repo, named, env.resources); err != nil {
		env.Close()
		return nil, err
	}
	return env, nil
}

// Close removes the temporary directory.
func (e *Env) Close() {
	_ = os.RemoveAll(e.dir)
}

func distinct(paths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

// probe runs `kyverno version` once. The command name is a literal, so Go resolves it with LookPath and
// refuses a binary found relative to the current directory. There is no variable, no flag, and no
// lookup inside the repository. The version is compared with the pin by Evaluate, so a check that
// disagrees fails on its own.
func (e *Env) probe(ctx context.Context) error {
	if _, err := exec.LookPath("kyverno"); err != nil {
		return &compiler.Error{Code: CodeMissing, Err: errors.New("kyverno is not on PATH")}
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "kyverno", "version")
	cmd.Dir = e.dir
	cmd.Env = childEnv()
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: probeLimit, Stderr: stderrLimit, Wait: commandWait})
	if err != nil {
		return &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("kyverno version: %s", toolrun.Excerpt(toolrun.Stderr(err), "kyverno failed"))}
	}
	first, _, _ := strings.Cut(string(out.Stdout), "\n")
	found, ok := strings.CutPrefix(strings.TrimSpace(first), versionPrefix)
	if _, _, _, valid := toolrun.ParseSemver(found); !ok || !valid {
		return &compiler.Error{Code: CodeVersion, Err: errors.New("the version output is not the shape ipso reads")}
	}
	e.version = found
	return nil
}

// Evaluate runs one compiled check and applies its waivers.
// The check runs the canonical policy recovered from the verified artifact, never a file of the
// repository, on the copy of the manifests.
func (e *Env) Evaluate(ctx context.Context, entry compiler.Entry, artifacts map[string][]byte, now time.Time) compiler.Outcome {
	if entry.CompilerVersion != version || e.version != version {
		return compiler.Outcome{Err: &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("kyverno %s and a check compiled for %s, ipso runs %s", e.version, entry.CompilerVersion, version)}}
	}
	policy, ok := artifacts[artifactName]
	if !ok {
		return compiler.Outcome{Err: &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("missing %s", artifactName)}}
	}
	name, err := ReadCanonical(policy, entry.Mode)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	stdout, exit, err := e.run(ctx, policy)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	findings, err := ParseReport(stdout, exit, name)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	var violations []compiler.Violation
	for _, finding := range findings {
		violations = append(violations, finding.violation())
	}
	return compiler.Cover(entry, compiler.Outcome{Violations: violations}, now)
}

// run starts kyverno on a copy of the policy. exit is its status: 0 when no resource failed or errored,
// and 1 when one did. Exit 1 with no report is a load error, and any other status is an error here.
// The working directory is the check's own temporary directory, which holds only the policy.
func (e *Env) run(ctx context.Context, policy []byte) (stdout []byte, exit int, err error) {
	dir, err := os.MkdirTemp(e.dir, "check-")
	if err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	policyPath := filepath.Join(dir, artifactName)
	if err := os.WriteFile(policyPath, policy, 0o600); err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, e.checkLimit())
	defer cancel()
	cmd := exec.CommandContext(ctx, "kyverno", "apply", policyPath,
		"--resource", e.resources,
		"--policy-report",
		"--output-format", "json",
		"--remove-color",
	)
	cmd.Dir = dir
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
	return nil, 0, &compiler.Error{Code: CodeRun, Err: fmt.Errorf("kyverno exited with status %d: %s", code, toolrun.Excerpt(exitErr.Stderr, "no message"))}
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
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", toolrun.Excerpt(toolrun.Stderr(err), "kyverno failed"))}
	}
}

// childEnv is the runner environment without KYVERNO_* variables, which configure the engine's own
// namespaces, services, and endpoints. Every other variable is inherited, so the step that runs ipso
// should not export secrets.
func childEnv() []string {
	environ := os.Environ()
	out := make([]string, 0, len(environ))
	for _, item := range environ {
		key, _, _ := strings.Cut(item, "=")
		if strings.HasPrefix(strings.ToUpper(key), envPrefix) {
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
