package semgrep

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/ipsofacto-io/ipso/internal/compiler/compilertest"
)

// Fixtures end in .fixture so CodeQL and the org Semgrep scan do not treat the deliberate violations as source.
func TestTemplates(t *testing.T) {
	ids := templateIDs()
	if strings.Join(ids, "\n") != strings.Join(wantTemplateIDs, "\n") {
		t.Fatalf("templates\n%s\nwant\n%s", strings.Join(ids, "\n"), strings.Join(wantTemplateIDs, "\n"))
	}
	rows := make([]docRow, 0, len(ids))
	required := map[string]struct{}{}
	for _, id := range ids {
		src, ok := lookup(id)
		if !ok {
			t.Fatalf("missing template %s", id)
		}
		if _, err := compileConfig(src, "block"); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		row, err := readDocRow(id, src)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		rows = append(rows, row)
		ruleID := strings.ReplaceAll(id, "/", ".")
		dir, name, _ := strings.Cut(id, "/")
		for _, language := range row.languages {
			ext, known := languageExt[language]
			if !known {
				t.Fatalf("%s: no fixture extension for %s", id, language)
			}
			rel := filepath.ToSlash(filepath.Join("testdata", "fixtures", dir, name+"."+ext+".fixture"))
			required[rel] = struct{}{}
			body, err := os.ReadFile(rel)
			if err != nil {
				t.Fatalf("%s: %v", rel, err)
			}
			text := string(body)
			if !strings.Contains(text, "ruleid: "+ruleID) || !strings.Contains(text, "ok: "+ruleID) {
				t.Errorf("%s: want ruleid and ok for %s", rel, ruleID)
			}
		}
	}
	checkOrphans(t, required)
	checkDoc(t, rows)
}

func readDocRow(id string, src []byte) (docRow, error) {
	var file struct {
		Rules []struct {
			Languages []string `yaml:"languages"`
			Metadata  struct {
				Title string `yaml:"title"`
			} `yaml:"metadata"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal(src, &file); err != nil {
		return docRow{}, err
	}
	if len(file.Rules) != 1 || file.Rules[0].Metadata.Title == "" || len(file.Rules[0].Languages) == 0 {
		return docRow{}, fmt.Errorf("template %s has no title or languages", id)
	}
	return docRow{id: id, title: file.Rules[0].Metadata.Title, languages: file.Rules[0].Languages}, nil
}

func checkOrphans(t *testing.T, required map[string]struct{}) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join("testdata", "fixtures"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := filepath.ToSlash(path)
		if _, ok := required[rel]; !ok {
			t.Errorf("orphan fixture %s", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type docRow struct {
	id        string
	title     string
	languages []string
}

func checkDoc(t *testing.T, rows []docRow) {
	t.Helper()
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	got := renderDoc(rows)
	path := filepath.Join("..", "..", "..", "docs", "semgrep-templates.md")
	if compilertest.Updating() {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read docs/semgrep-templates.md: %v (go test ./internal/compiler/semgrep -update)", err)
	}
	if string(body) != got {
		t.Fatalf("docs/semgrep-templates.md drifted\n%s", got)
	}
}

func renderDoc(rows []docRow) string {
	var b strings.Builder
	b.WriteString("# Semgrep templates\n\n")
	b.WriteString("A check with `engine: semgrep` and `ref: ipso:<id>` compiles one of these templates. Severity follows the check mode. Each template is one Semgrep rule.\n\n")
	b.WriteString("| id | title | languages |\n")
	b.WriteString("| --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", row.id, row.title, strings.Join(row.languages, ", "))
	}
	return b.String()
}

var languageExt = map[string]string{
	"go":         "go",
	"java":       "java",
	"javascript": "js",
	"python":     "py",
	"typescript": "ts",
}

var wantTemplateIDs = []string{
	"go/http-client-timeout",
	"go/sql-no-string-query",
	"go/use-slog",
	"java/no-system-out",
	"java/sql-no-string-query",
	"javascript/no-eval",
	"javascript/no-inner-html",
	"python/no-pickle",
	"python/no-shell-true",
	"python/requests-timeout",
}
