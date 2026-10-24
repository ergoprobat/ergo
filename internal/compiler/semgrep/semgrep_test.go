package semgrep

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/compilertest"
	"github.com/ergoprobat/ergo/internal/edr"
)

func TestCompile(t *testing.T) {
	compilertest.Run(t, New())
}

func TestUpperExtension(t *testing.T) {
	dir := t.TempDir()
	body := []byte("rules:\n  - id: sample\n    message: sample rule\n    languages:\n      - python\n    pattern: foo(...)\n")
	if err := os.WriteFile(filepath.Join(dir, "rule.YAML"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	artifacts, err := New().Compile(compiler.Input{
		Record: edr.Record{Criticality: "warning"},
		Check:  edr.Check{Engine: "semgrep", Ref: "rule.YAML", Mode: "warn"},
		Repo:   root.FS(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || !bytesContains(artifacts[0].Data, "severity: WARNING") {
		t.Fatalf("artifacts %#v", artifacts)
	}
}

func TestDeepDocument(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxYAMLDepth+5; i++ {
		b.WriteString(strings.Repeat("  ", i))
		b.WriteString("a:\n")
	}
	b.WriteString(strings.Repeat("  ", maxYAMLDepth+5))
	b.WriteString("b: 1\n")
	_, err := compileConfig([]byte(b.String()), "warn")
	if compiler.CodeOf(err) != CodeYAML {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func TestEmptyPatternClause(t *testing.T) {
	src := []byte("rules:\n  - id: sample\n    message: sample rule\n    languages:\n      - python\n    pattern-either:\n      - {}\n")
	_, err := compileConfig(src, "warn")
	if compiler.CodeOf(err) != CodeRule {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func TestSearchRejectsTaintKeys(t *testing.T) {
	src := []byte("rules:\n  - id: sample\n    message: sample rule\n    languages:\n      - python\n    pattern: foo(...)\n    pattern-sources:\n      - pattern: source(...)\n")
	_, err := compileConfig(src, "warn")
	if compiler.CodeOf(err) != CodeRule {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func TestEmptyNestedPattern(t *testing.T) {
	src := []byte(`rules:
  - id: sample
    message: sample rule
    languages:
      - python
    pattern-either:
      - pattern: ""
`)
	_, err := compileConfig(src, "warn")
	if compiler.CodeOf(err) != CodeRule {
		t.Fatalf("code %q (%v)", compiler.CodeOf(err), err)
	}
}

func bytesContains(data []byte, part string) bool {
	return strings.Contains(string(data), part)
}
