package gatekeeper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/manifests"
	"github.com/ipsofacto-io/ipso/internal/compiler/opa"
	"github.com/ipsofacto-io/ipso/internal/compiler/toolrun"
)

const (
	versionTimeout = 30 * time.Second
	checkTimeout   = 10 * time.Minute
	commandWait    = 5 * time.Second
	maxStdoutBytes = 256 << 20
	stderrLimit    = 1024
	probeLimit     = 4096
)

// versionLine is the first line of `gator --version`:
// gator version v3.23.1 (Feature State: beta), opa/v1.17.1, frameworks/v0.0.0-20260616163050-e1eaa1bf6d62
var versionLine = regexp.MustCompile(`^gator version v([0-9.]+) \([^()]*\), opa/v([0-9.]+), frameworks/[A-Za-z0-9._+-]+$`)

// Env is one evaluation run: the gator binary from PATH and the copied manifests.
// Setup creates it and Close removes it. Checks run one at a time, each in its own process.
type Env struct {
	dir        string
	resources  string
	version    string
	opaVersion string
	// timeout, outputLimit, and wait override the production bounds. Zero means the default.
	timeout     time.Duration
	outputLimit int
	wait        time.Duration
}

// Version is the gator release the binary reports.
// It is not the pinned release when the binary on PATH is another one, and then every check is an error.
func (e *Env) Version() string { return e.version }

// Setup prepares one evaluation run.
// paths are the manifest files and directories to check. They are repository-relative, slash-separated,
// and already normalized. Setup checks them, finds gator on PATH, reads its version, copies the
// manifests, and checks the documents. A named path that is missing, leaves the repository, or holds
// a symlink, a tree that holds no manifest or no object, and a document that is a Gatekeeper object or a
// list, fail the run before any check starts.
func Setup(ctx context.Context, root string, paths []string) (*Env, error) {
	named := manifests.Distinct(paths)
	if err := toolrun.WithinRoot(root, named); err != nil {
		return nil, &compiler.Error{Code: CodePath, Err: err}
	}
	repo, err := os.OpenRoot(root)
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = repo.Close() }()
	dir, err := os.MkdirTemp("", "ipso-gatekeeper-")
	if err != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
	env := &Env{dir: dir, resources: filepath.Join(dir, "resources")}
	if err := env.probe(ctx); err != nil {
		env.Close()
		return nil, err
	}
	sources, err := stageManifests(ctx, repo, named, env.resources)
	if err != nil {
		env.Close()
		return nil, err
	}
	if err := checkDocuments(ctx, env.resources, sources); err != nil {
		env.Close()
		return nil, err
	}
	return env, nil
}

// Close removes the temporary directory.
func (e *Env) Close() {
	_ = os.RemoveAll(e.dir)
}

// probe runs `gator --version` once. The command name is a literal, so Go resolves it with LookPath and
// refuses a binary found relative to the current directory. There is no variable, no flag, and no
// lookup inside the repository. The versions are compared with the pin by Evaluate, so a check that
// disagrees fails on its own.
func (e *Env) probe(ctx context.Context) error {
	if _, err := exec.LookPath("gator"); err != nil {
		return &compiler.Error{Code: CodeMissing, Err: errors.New("gator is not on PATH")}
	}
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gator", "--version")
	cmd.Dir = e.dir
	out, err := toolrun.Run(ctx, cmd, toolrun.Limits{Stdout: probeLimit, Stderr: stderrLimit, Wait: commandWait})
	if err != nil {
		return &compiler.Error{Code: CodeMissing, Err: fmt.Errorf("gator --version: %s", toolrun.Excerpt(toolrun.Stderr(err), "gator failed"))}
	}
	first, _, _ := strings.Cut(string(out.Stdout), "\n")
	match := versionLine.FindStringSubmatch(strings.TrimSpace(first))
	if match == nil {
		return &compiler.Error{Code: CodeVersion, Err: errors.New("the version output is not the shape ipso reads")}
	}
	for _, found := range match[1:] {
		if _, _, _, ok := toolrun.ParseSemver(found); !ok {
			return &compiler.Error{Code: CodeVersion, Err: errors.New("the version output is not the shape ipso reads")}
		}
	}
	e.version, e.opaVersion = match[1], "v"+match[2]
	return nil
}

// Evaluate runs one compiled check and applies its waivers.
// The check runs the canonical template and constraint recovered from the verified artifacts, never
// a file of the repository, on the copy of the manifests.
func (e *Env) Evaluate(ctx context.Context, entry compiler.Entry, artifacts map[string][]byte, now time.Time) compiler.Outcome {
	if e.version != opa.GatorVersion || e.opaVersion != opa.GatorOPA {
		return compiler.Outcome{Err: &compiler.Error{Code: CodeVersion, Err: fmt.Errorf("gator %s with opa %s, ipso runs gator %s with opa %s", e.version, e.opaVersion, opa.GatorVersion, opa.GatorOPA)}}
	}
	template, constraint, err := artifactPair(artifacts)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	id, err := opa.ReadGatekeeper(template, constraint, entry.Mode)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	stdout, exit, err := e.run(ctx, template, constraint)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	findings, err := ParseResults(stdout, exit, id)
	if err != nil {
		return compiler.Outcome{Err: err}
	}
	var violations []compiler.Violation
	for _, finding := range findings {
		violations = append(violations, finding.violation())
	}
	return compiler.Cover(entry, compiler.Outcome{Violations: violations}, now)
}

func artifactPair(artifacts map[string][]byte) (template, constraint []byte, err error) {
	template, ok := artifacts[artifactTemplate]
	if !ok {
		return nil, nil, &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("missing %s", artifactTemplate)}
	}
	constraint, ok = artifacts[artifactConstraint]
	if !ok {
		return nil, nil, &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("missing %s", artifactConstraint)}
	}
	return template, constraint, nil
}

// run starts gator on the copied manifests, then the template and the constraint, in that order.
// A name that the manifests and ipso both define therefore belongs to ipso. exit is its status: 1 when
// a result has the action deny, and 0 otherwise. Exit 1 with no result is a load or compile error,
// and any other status is an error here. The working directory is the check's own temporary directory,
// which holds only the two files.
func (e *Env) run(ctx context.Context, template, constraint []byte) (stdout []byte, exit int, err error) {
	dir, err := os.MkdirTemp(e.dir, "check-")
	if err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	templatePath := filepath.Join(dir, artifactTemplate)
	constraintPath := filepath.Join(dir, artifactConstraint)
	if err := os.WriteFile(templatePath, template, 0o600); err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	if err := os.WriteFile(constraintPath, constraint, 0o600); err != nil {
		return nil, 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	ctx, cancel := context.WithTimeout(ctx, e.checkLimit())
	defer cancel()
	cmd := exec.CommandContext(ctx, "gator", "test",
		"-f", e.resources,
		"-f", templatePath,
		"-f", constraintPath,
		"-o", "json",
		"--enable-k8s-native-validation=false",
	)
	cmd.Dir = dir
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
	return nil, 0, &compiler.Error{Code: CodeRun, Err: fmt.Errorf("gator exited with status %d: %s", code, toolrun.Excerpt(exitErr.Stderr, "no message"))}
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
		return &compiler.Error{Code: CodeRun, Err: fmt.Errorf("%s", toolrun.Excerpt(toolrun.Stderr(err), "gator failed"))}
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
