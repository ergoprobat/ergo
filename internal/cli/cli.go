package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/builtin"
	"github.com/ergoprobat/ergo/internal/compiler/checkov"
	"github.com/ergoprobat/ergo/internal/compiler/cibinding"
	"github.com/ergoprobat/ergo/internal/compiler/opa"
	"github.com/ergoprobat/ergo/internal/config"
	"github.com/ergoprobat/ergo/internal/edr"
)

// deps is the clock and compiler registry for one invocation.
// Tests inject both. The binary uses the local clock and the built-in registry.
type deps struct {
	now      func() time.Time
	registry compiler.Registry
}

// Run executes one CLI invocation. args excludes the program name.
func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr, deps{now: time.Now, registry: builtin.Registry()})
}

func run(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fprintln(stderr, usage)
		return 2
	}
	root, err := os.Getwd()
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	switch args[0] {
	case "init":
		if len(args) != 1 {
			fprintln(stderr, "usage: ergo init")
			return 2
		}
		return initCmd(root, stdout, stderr)
	case "check":
		return checkCmd(root, args[1:], stdout, stderr, d)
	case "explain":
		if len(args) != 2 || args[1] == "" {
			fprintln(stderr, "usage: ergo explain <slug>")
			return 2
		}
		return explainCmd(root, args[1], stdout, stderr)
	case "import":
		if len(args) > 1 && args[1] == "" {
			fprintln(stderr, "usage: ergo import [path...]")
			return 2
		}
		return importCmd(root, args[1:], stdout, stderr)
	case "compile":
		if len(args) != 1 {
			fprintln(stderr, "usage: ergo compile")
			return 2
		}
		return compileCmd(root, stdout, stderr, d)
	case "-h", "--help", "help", "version", "--version", "-v":
		fprintln(stdout, helpText())
		return 0
	default:
		fprintf(stderr, "unknown command %q\n%s\n", args[0], usage)
		return 2
	}
}

// Version, Commit, and Date are set with -X at release time.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func helpText() string {
	return fmt.Sprintf("ergo %s (%s, %s)\n\n%s", Version, Commit, Date, usage)
}

const usage = `ergo reads and compiles engineering decision records in a repository.

usage:
  ergo init
  ergo check
  ergo check plan <plan.json|-> [--env name] [--report file]
  ergo check ci <evidence.json|-> --sha <sha> [--env name] [--report file]
  ergo check checkov <results.json|-> [--env name] [--report file]
  ergo explain <slug>
  ergo import [path...]
  ergo compile`

func initCmd(root string, stdout, stderr io.Writer) int {
	created, err := config.Init(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	path := config.Path(root)
	if created {
		fprintf(stdout, "wrote %s\n", displayPath(root, path))
		return 0
	}
	fprintf(stdout, "%s already exists\n", displayPath(root, path))
	return 0
}

func checkCmd(root string, args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		return checkFiles(root, stdout, stderr)
	}
	switch args[0] {
	case "plan":
		return planCmd(root, args[1:], stdout, stderr, d)
	case "ci":
		return ciCmd(root, args[1:], stdout, stderr, d)
	case "checkov":
		return checkovCmd(root, args[1:], stdout, stderr, d)
	default:
		fprintln(stderr, checkUsage)
		return 2
	}
}

func checkFiles(root string, stdout, stderr io.Writer) int {
	cfg, _, err := config.Load(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	files, err := edr.Match(root, cfg.Decisions)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	if len(files) == 0 {
		fprintln(stdout, "no EDR files found")
		return 0
	}
	failed := false
	for _, path := range files {
		shown := displayPath(root, path)
		doc, err := edr.ReadFile(path)
		if err != nil {
			fprintf(stdout, "%s: %s\n", shown, err)
			failed = true
			continue
		}
		problems := edr.Validate(doc)
		if len(problems) == 0 {
			fprintf(stdout, "%s: ok\n", shown)
			continue
		}
		failed = true
		for _, problem := range problems {
			fprintf(stdout, "%s: %s\n", shown, problem)
		}
	}
	if failed {
		return 1
	}
	return 0
}

const (
	planUsage    = "usage: ergo check plan <plan.json|-> [--env name] [--report file]"
	ciUsage      = "usage: ergo check ci <evidence.json|-> --sha <sha> [--env name] [--report file]"
	checkovUsage = "usage: ergo check checkov <results.json|-> [--env name] [--report file]"
	checkUsage   = planUsage + "\n" + ciUsage + "\n" + checkovUsage
)

func planCmd(root string, args []string, stdout, stderr io.Writer, d deps) int {
	opts, err := parsePlanArgs(args)
	if err != nil {
		fprintln(stderr, err)
		return 2
	}
	if d.now == nil {
		d.now = time.Now
	}
	var reader io.Reader
	if opts.path == "-" {
		reader = os.Stdin
	} else {
		file, err := os.Open(opts.path)
		if err != nil {
			fprintln(stderr, err)
			return 1
		}
		defer func() { _ = file.Close() }()
		reader = file
	}
	plan, err := opa.ReadPlan(reader)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	build, err := compiler.LoadBuild(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	now := d.now()
	report := compiler.PlanReport{
		ReportVersion: compiler.ReportVersion,
		ErgoVersion:   Version,
		EvaluatedAt:   now.UTC(),
		Env:           opts.env,
		PlanSHA256:    plan.SHA256(),
		BuildSHA256:   build.SHA256(),
	}
	verdicts, failed, selected, err := evaluateChecks(stdout, build, func(entry compiler.Entry) bool {
		return entry.Selected(opts.env, now)
	}, func(entry compiler.Entry, artifacts map[string][]byte) compiler.Outcome {
		return opa.EvalCheck(entry, artifacts, plan, now)
	})
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	report.Verdicts = verdicts
	if selected == 0 {
		fprintln(stdout, "no checks selected")
	}
	if opts.report != "" {
		raw, err := compiler.MarshalReport(report)
		if err != nil {
			fprintln(stderr, err)
			return 1
		}
		if err := writeReport(opts.report, raw); err != nil {
			fprintln(stderr, err)
			return 1
		}
	}
	if failed {
		return 1
	}
	return 0
}

func ciCmd(root string, args []string, stdout, stderr io.Writer, d deps) int {
	opts, err := parseBoundArgs(args, ciUsage, true)
	if err != nil {
		fprintln(stderr, err)
		return 2
	}
	if !cibinding.ValidCommitSHA(opts.sha) {
		fprintln(stderr, ciUsage)
		return 2
	}
	if d.now == nil {
		d.now = time.Now
	}
	reader, err := openInput(opts.path)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	defer func() { _ = reader.Close() }()
	evidence, err := cibinding.ReadEvidence(reader, opts.sha)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	build, err := compiler.LoadBuild(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	now := d.now()
	verdicts, failed, selected, err := evaluateChecks(stdout, build, func(entry compiler.Entry) bool {
		return entry.Engine == "ci-binding" && entry.Applies(opts.env, now)
	}, func(entry compiler.Entry, artifacts map[string][]byte) compiler.Outcome {
		return cibinding.EvalCheck(entry, artifacts, evidence, now)
	})
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	if selected == 0 {
		fprintln(stdout, "no checks selected")
	}
	report := compiler.CIReport{
		ReportVersion:  compiler.ReportVersion,
		ErgoVersion:    Version,
		EvaluatedAt:    now.UTC(),
		Env:            opts.env,
		CommitSHA:      opts.sha,
		EvidenceSHA256: evidence.SHA256(),
		BuildSHA256:    build.SHA256(),
		Verdicts:       verdicts,
	}
	if opts.report != "" {
		raw, err := compiler.MarshalCIReport(report)
		if err != nil {
			fprintln(stderr, err)
			return 1
		}
		if err := writeReport(opts.report, raw); err != nil {
			fprintln(stderr, err)
			return 1
		}
	}
	if failed {
		return 1
	}
	return 0
}

func checkovCmd(root string, args []string, stdout, stderr io.Writer, d deps) int {
	opts, err := parseBoundArgs(args, checkovUsage, false)
	if err != nil {
		fprintln(stderr, err)
		return 2
	}
	if d.now == nil {
		d.now = time.Now
	}
	reader, err := openInput(opts.path)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	defer func() { _ = reader.Close() }()
	results, err := checkov.ReadResults(reader)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	build, err := compiler.LoadBuild(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	now := d.now()
	verdicts, failed, selected, err := evaluateChecks(stdout, build, func(entry compiler.Entry) bool {
		return entry.Engine == "checkov" && entry.Applies(opts.env, now)
	}, func(entry compiler.Entry, artifacts map[string][]byte) compiler.Outcome {
		return checkov.EvalCheck(entry, artifacts, results, now)
	})
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	if selected == 0 {
		fprintln(stdout, "no checks selected")
	}
	report := compiler.CheckovReport{
		ReportVersion:  compiler.ReportVersion,
		ErgoVersion:    Version,
		EvaluatedAt:    now.UTC(),
		Env:            opts.env,
		EvidenceSHA256: results.SHA256(),
		BuildSHA256:    build.SHA256(),
		Verdicts:       verdicts,
	}
	if opts.report != "" {
		raw, err := compiler.MarshalCheckovReport(report)
		if err != nil {
			fprintln(stderr, err)
			return 1
		}
		if err := writeReport(opts.report, raw); err != nil {
			fprintln(stderr, err)
			return 1
		}
	}
	if failed {
		return 1
	}
	return 0
}

func openInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

type planOptions struct {
	path   string
	env    string
	report string
}

func parsePlanArgs(args []string) (planOptions, error) {
	var opts planOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--env":
			if opts.env != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return planOptions{}, fmt.Errorf("%s", planUsage)
			}
			i++
			opts.env = args[i]
		case "--report":
			if opts.report != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return planOptions{}, fmt.Errorf("%s", planUsage)
			}
			i++
			opts.report = args[i]
		default:
			if strings.HasPrefix(args[i], "-") || opts.path != "" {
				return planOptions{}, fmt.Errorf("%s", planUsage)
			}
			opts.path = args[i]
		}
	}
	if opts.path == "" {
		return planOptions{}, fmt.Errorf("%s", planUsage)
	}
	return opts, nil
}

type boundOptions struct {
	path   string
	env    string
	report string
	sha    string
}

func parseBoundArgs(args []string, usage string, sha bool) (boundOptions, error) {
	var opts boundOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--env":
			if opts.env != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return boundOptions{}, fmt.Errorf("%s", usage)
			}
			i++
			opts.env = args[i]
		case "--report":
			if opts.report != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return boundOptions{}, fmt.Errorf("%s", usage)
			}
			i++
			opts.report = args[i]
		case "--sha":
			if !sha || opts.sha != "" || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return boundOptions{}, fmt.Errorf("%s", usage)
			}
			i++
			opts.sha = args[i]
		default:
			if strings.HasPrefix(args[i], "-") || opts.path != "" {
				return boundOptions{}, fmt.Errorf("%s", usage)
			}
			opts.path = args[i]
		}
	}
	if opts.path == "" || (sha && opts.sha == "") {
		return boundOptions{}, fmt.Errorf("%s", usage)
	}
	return opts, nil
}

// writeReport replaces path with raw. A symlink is refused. The file mode is 0600.
func writeReport(path string, raw []byte) error {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", path)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	parent, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if parent.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", dir)
	}
	temp, err := os.CreateTemp(dir, ".report-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// evaluateChecks runs match against the build and prints one line per selected check.
// hard is true when a block-mode check fails or any check errors.
func evaluateChecks(stdout io.Writer, build compiler.Build, match func(compiler.Entry) bool, eval func(compiler.Entry, map[string][]byte) compiler.Outcome) (verdicts []compiler.Verdict, hard bool, selected int, err error) {
	for _, entry := range build.Entries {
		if !match(entry) {
			continue
		}
		selected++
		artifacts, artErr := build.Artifacts(entry)
		if artErr != nil {
			return nil, true, selected, artErr
		}
		verdict, failed := verdictOf(stdout, entry, eval(entry, artifacts))
		if failed {
			hard = true
		}
		verdicts = append(verdicts, verdict)
	}
	return verdicts, hard, selected, nil
}

func verdictOf(stdout io.Writer, entry compiler.Entry, outcome compiler.Outcome) (compiler.Verdict, bool) {
	verdict := compiler.Verdict{
		DecisionID: entry.DecisionID,
		Slug:       entry.Slug,
		CheckIndex: entry.CheckIndex,
		Mode:       entry.Mode,
		Result:     "pass",
	}
	switch {
	case outcome.Err != nil:
		verdict.Result = "error"
		fprintf(stdout, "%s: checks[%d] (%s): error: %s\n", entry.Slug, entry.CheckIndex, entry.Mode, outcome.Err)
		return verdict, true
	case len(outcome.Violations) > 0 && outcome.Waived:
		verdict.Result = "waived"
		verdict.Violations = len(outcome.Violations)
		fprintf(stdout, "%s: checks[%d] (%s): waived\n", entry.Slug, entry.CheckIndex, entry.Mode)
		return verdict, false
	case len(outcome.Violations) > 0:
		verdict.Result = "fail"
		verdict.Violations = len(outcome.Violations)
		for _, violation := range outcome.Violations {
			fprintf(stdout, "%s: checks[%d] (%s): %s\n", entry.Slug, entry.CheckIndex, entry.Mode, violationLine(violation))
		}
		return verdict, entry.Mode == "block"
	default:
		fprintf(stdout, "%s: checks[%d] (%s): pass\n", entry.Slug, entry.CheckIndex, entry.Mode)
		return verdict, false
	}
}

func violationLine(violation compiler.Violation) string {
	if violation.Address == "" {
		return violation.Message
	}
	return violation.Address + ": " + violation.Message
}

func explainCmd(root, slug string, stdout, stderr io.Writer) int {
	cfg, _, err := config.Load(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	files, err := edr.Match(root, cfg.Decisions)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	var found []string
	var chosen edr.Document
	for _, path := range files {
		doc, err := edr.ReadFile(path)
		if err != nil {
			continue
		}
		if doc.Slug() != slug {
			continue
		}
		found = append(found, path)
		chosen = doc
	}
	if len(found) == 0 {
		fprintf(stderr, "no EDR file with slug %q\n", slug)
		return 1
	}
	if len(found) > 1 {
		fprintf(stderr, "more than one EDR file with slug %q\n", slug)
		return 1
	}
	text, ok := edr.DecisionText(chosen.Body)
	if !ok {
		fprintf(stderr, "%s: missing heading ## Decision\n", displayPath(root, found[0]))
		return 1
	}
	fprintf(stdout, "title: %s\nstatus: %s\n\n%s\n", chosen.Title(), chosen.Status(), text)
	return 0
}

func compileCmd(root string, stdout, stderr io.Writer, d deps) int {
	if d.now == nil {
		d.now = time.Now
	}
	cfg, _, err := config.Load(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	files, err := edr.Match(root, cfg.Decisions)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	if len(files) == 0 {
		fprintln(stdout, "no EDR files found")
		if err := compiler.WriteBuild(root, nil, nil); err != nil {
			fprintln(stderr, err)
			return 1
		}
		return 0
	}
	type loaded struct {
		shown string
		rec   edr.Record
	}
	var docs []loaded
	failed := false
	for _, path := range files {
		shown := displayPath(root, path)
		doc, err := edr.ReadFile(path)
		if err != nil {
			fprintf(stdout, "%s: %s\n", shown, err)
			failed = true
			continue
		}
		problems := edr.Validate(doc)
		if len(problems) > 0 {
			failed = true
			for _, problem := range problems {
				fprintf(stdout, "%s: %s\n", shown, problem)
			}
			continue
		}
		rec, err := edr.RecordFrom(doc)
		if err != nil {
			fprintf(stdout, "%s: %s\n", shown, err)
			failed = true
			continue
		}
		docs = append(docs, loaded{shown: shown, rec: rec})
	}
	if failed {
		return 1
	}

	repo, err := os.OpenRoot(root)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	defer func() { _ = repo.Close() }()

	now := d.now()
	var ids []compiler.DecisionRef
	var compiled []compiler.Compiled
	var lines []string
	seen := map[string]string{}
	for _, item := range docs {
		if !item.rec.InForce(now) {
			lines = append(lines, fmt.Sprintf("%s: skipped, not in force", item.shown))
			continue
		}
		if prev, ok := seen[item.rec.ID]; ok {
			fprintf(stdout, "%s: duplicate-id: %s\n", item.shown, prev)
			return 1
		}
		seen[item.rec.ID] = item.shown
		ids = append(ids, compiler.DecisionRef{Path: item.shown, ID: item.rec.ID})
		part, reports, err := compiler.CompileRecord(d.registry, item.rec, repo.FS())
		if err != nil {
			fprintf(stdout, "%s: %s\n", item.shown, err)
			return 1
		}
		compiled = append(compiled, part...)
		for _, report := range reports {
			lines = append(lines, compileLine(item.shown, report))
		}
	}
	if err := compiler.WriteBuild(root, ids, compiled); err != nil {
		fprintln(stderr, err)
		return 1
	}
	for _, line := range lines {
		fprintln(stdout, line)
	}
	return 0
}

func compileLine(shown string, report compiler.Report) string {
	switch report.Kind {
	case compiler.ReportAdvisory:
		return fmt.Sprintf("%s: checks[%d] (%s): advisory, not compiled", shown, report.Index, report.Engine)
	case compiler.ReportNoCompiler:
		return fmt.Sprintf("%s: checks[%d] (%s): no compiler", shown, report.Index, report.Engine)
	default:
		word := "artifacts"
		if report.Count == 1 {
			word = "artifact"
		}
		return fmt.Sprintf("%s: checks[%d] (%s): compiled %d %s", shown, report.Index, report.Engine, report.Count, word)
	}
}

func importCmd(root string, args []string, stdout, stderr io.Writer) int {
	targets, err := importTargets(root, args)
	if err != nil {
		fprintln(stderr, err)
		return 1
	}
	if len(targets) == 0 {
		fprintln(stdout, "no ADR files found")
		return 0
	}
	failed := false
	for _, path := range targets {
		shown := displayPath(root, path)
		raw, err := os.ReadFile(path)
		if err != nil {
			fprintf(stdout, "%s: %s\n", shown, err)
			failed = true
			continue
		}
		if doc, err := edr.Parse(string(raw)); err == nil && len(edr.Validate(doc)) == 0 {
			fprintf(stdout, "%s: skipped\n", shown)
			continue
		}
		imported, err := edr.Import(filepath.Base(path), string(raw), time.Now())
		if err != nil {
			fprintf(stdout, "%s: %s\n", shown, err)
			failed = true
			continue
		}
		if err := os.WriteFile(path, []byte(imported), 0o644); err != nil {
			fprintf(stdout, "%s: %s\n", shown, err)
			failed = true
			continue
		}
		if strings.Contains(imported, "\ninferred: true\n") {
			fprintf(stdout, "%s: imported inferred\n", shown)
			continue
		}
		fprintf(stdout, "%s: imported\n", shown)
	}
	if failed {
		return 1
	}
	return 0
}

func importTargets(root string, args []string) ([]string, error) {
	if len(args) == 0 {
		var found []string
		for _, dir := range []string{"docs/adr", "docs/decisions"} {
			path := filepath.Join(root, filepath.FromSlash(dir))
			info, err := os.Stat(path)
			if err != nil || !info.IsDir() {
				continue
			}
			files, err := markdownFiles(path)
			if err != nil {
				return nil, err
			}
			found = append(found, files...)
		}
		return found, nil
	}
	var found []string
	for _, arg := range args {
		if arg == "" {
			return nil, fmt.Errorf("usage: ergo import [path...]")
		}
		path := arg
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			found = append(found, path)
			continue
		}
		files, err := markdownFiles(path)
		if err != nil {
			return nil, err
		}
		found = append(found, files...)
	}
	return found, nil
}

func markdownFiles(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			found = append(found, path)
		}
		return nil
	})
	return found, err
}

func fprintf(w io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		return
	}
}

func fprintln(w io.Writer, args ...any) {
	if _, err := fmt.Fprintln(w, args...); err != nil {
		return
	}
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
