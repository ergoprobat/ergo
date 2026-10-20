package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ergoprobat/ergo/internal/config"
	"github.com/ergoprobat/ergo/internal/edr"
)

// Run executes one CLI invocation. args excludes the program name.
func Run(args []string, stdout, stderr io.Writer) int {
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
		fprintln(stderr, "The MADR and Nygard importer is T-021.")
		return 2
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

const usage = `ergo validates engineering decision records in a repository.

usage:
  ergo init
  ergo check
  ergo explain <slug>
  ergo import`

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
