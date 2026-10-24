package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/config"
	"github.com/ergoprobat/ergo/internal/edr"
)

// deps is the clock and compiler registry for one invocation.
// Tests inject both. The binary uses the local clock and Builtin.
type deps struct {
	now      func() time.Time
	registry compiler.Registry
}

// Run executes one CLI invocation. args excludes the program name.
func Run(args []string, stdout, stderr io.Writer) int {
	return run(args, stdout, stderr, deps{now: time.Now, registry: compiler.Builtin()})
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
		if len(args) != 1 {
			fprintln(stderr, "usage: ergo check")
			return 2
		}
		return checkCmd(root, stdout, stderr)
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

func checkCmd(root string, stdout, stderr io.Writer) int {
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
