package depcruise

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestEvaluateFixtures(t *testing.T) {
	bin := os.Getenv("IPSO_DEPCRUISE")
	if bin == "" {
		t.Skip("IPSO_DEPCRUISE is not set")
	}
	binDir := filepath.Dir(bin)
	if _, err := os.Stat(filepath.Join(binDir, "depcruise")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx := context.Background()
	tool, err := FindTool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !VersionFits(tool.Version, version) {
		t.Fatalf("depcruise %s does not satisfy ^%s", tool.Version, version)
	}

	cleanCfg := compileCase(t, "testdata/monorepo-clean")
	badCfg := compileCase(t, "testdata/monorepo-violating")
	cleanOut, err := tool.Cruise(ctx, absRepo(t, "testdata/monorepo-clean/repo"), cleanCfg, []string{"packages"})
	if err != nil {
		t.Fatal(err)
	}
	clean, err := ParseResult(cleanOut, tool.Version)
	if err != nil || len(clean.Violations) != 0 {
		t.Fatalf("clean %+v err %v", clean, err)
	}

	badOut, err := tool.Cruise(ctx, absRepo(t, "testdata/monorepo-violating/repo"), badCfg, []string{"packages"})
	if err != nil {
		t.Fatal(err)
	}
	outcome := EvalCheck(compiler.Entry{CompilerVersion: version, Mode: "block"}, map[string][]byte{artifactName: badCfg}, tool.Version, badOut, time.Now())
	if outcome.Err != nil || len(outcome.Violations) != 2 {
		t.Fatalf("outcome %+v", outcome)
	}
	seen := map[string]bool{}
	for _, violation := range outcome.Violations {
		seen[violation.Message] = true
	}
	for _, want := range []string{
		"no-circular packages/ui/src/a.ts -> packages/ui/src/b.ts",
		"ui-not-to-db packages/ui/src/index.ts -> packages/db/src/index.ts",
	} {
		if !seen[want] {
			t.Fatalf("missing %s in %+v", want, outcome.Violations)
		}
	}
}

func absRepo(t *testing.T, rel string) string {
	t.Helper()
	abs, err := filepath.Abs(rel)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}
