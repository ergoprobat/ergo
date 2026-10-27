package semgrep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestEvaluateFixtures(t *testing.T) {
	if os.Getenv("IPSO_SEMGREP") == "" {
		t.Skip("IPSO_SEMGREP is not set")
	}
	ctx := context.Background()
	tool, err := FindTool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tool.Version != version {
		t.Fatalf("semgrep %s is not %s", tool.Version, version)
	}

	rule := []byte("rules:\n  - id: no-eval\n    message: Do not eval\n    languages:\n      - python\n    pattern: eval(...)\n")
	config, err := compileConfig(rule, "block")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("clean", func(t *testing.T) {
		root := copyScanTree(t, "testdata/scan-clean")
		raw, err := tool.Scan(ctx, root, config, []string{"."})
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseResult(raw, tool.Version)
		if err != nil || len(got.Findings) != 0 {
			t.Fatalf("clean %+v err %v", got, err)
		}
	})

	t.Run("violating", func(t *testing.T) {
		root := copyScanTree(t, "testdata/scan-violating")
		raw, err := tool.Scan(ctx, root, config, []string{"."})
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseResult(raw, tool.Version)
		if err != nil {
			t.Fatal(err)
		}
		lines := map[int]string{}
		for _, finding := range got.Findings {
			if finding.Rule != "no-eval" {
				t.Fatalf("rule %s", finding.Rule)
			}
			lines[finding.Line] = finding.Path
		}
		if len(lines) != 2 || lines[1] == "" || lines[2] == "" {
			t.Fatalf("findings %+v", got.Findings)
		}
		outcome := EvalCheck(compiler.Entry{CompilerVersion: version, Mode: "block"}, map[string][]byte{ArtifactName: config}, tool.Version, raw, time.Now())
		if outcome.Err != nil || len(outcome.Violations) != 2 {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("options", func(t *testing.T) {
		src, err := os.ReadFile("testdata/every-option.yaml")
		if err != nil {
			t.Fatal(err)
		}
		for key := range allowedOptionKeys {
			if !strings.Contains(string(src), "\n      "+key+":") {
				t.Errorf("every-option rule omits %s", key)
			}
		}
		compiled, err := compileConfig(src, "block")
		if err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "hit.txt"), []byte("sample(1)\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		raw, err := tool.Scan(ctx, root, compiled, []string{"hit.txt"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := ParseResult(raw, tool.Version)
		if err != nil {
			t.Fatalf("options %+v err %v\n%s", got, err, raw)
		}
	})
}

func copyScanTree(t *testing.T, src string) string {
	t.Helper()
	dest := t.TempDir()
	err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".fixture") {
			rel = filepath.Join(filepath.Dir(rel), strings.TrimSuffix(entry.Name(), ".fixture"))
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dest
}
