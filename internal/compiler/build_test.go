package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ergoprobat/ergo/internal/edr"
)

func TestWriteBuildReplacesAndRefusesSymlink(t *testing.T) {
	root := t.TempDir()
	compiled := []Compiled{{
		Entry: Entry{
			DecisionID:      "01M5C5QEHHNXJ3D7X2ZCMGXQT7",
			Slug:            "tls",
			CheckIndex:      0,
			Engine:          "opa",
			Mode:            "block",
			Compiler:        "opa",
			CompilerVersion: "test",
			Artifacts:       []ManifestArtifact{{Name: "policies/rule.txt", SHA256: "ab"}},
		},
		Artifacts: []Artifact{{Name: "policies/rule.txt", Data: []byte("yes")}},
	}}
	ids := []DecisionRef{{Path: "docs/decisions/a.md", ID: compiled[0].Entry.DecisionID}}
	if err := WriteBuild(root, ids, compiled); err != nil {
		t.Fatal(err)
	}
	build := filepath.Join(root, ".ergo", "build")
	ignore, err := os.ReadFile(filepath.Join(build, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(ignore) != "*\n" {
		t.Fatalf("gitignore %q", ignore)
	}
	body, err := os.ReadFile(filepath.Join(build, compiled[0].Entry.DecisionID, "0", "policies", "rule.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "yes" {
		t.Fatalf("artifact %q", body)
	}
	stale := filepath.Join(build, "stale.txt")
	if err := os.WriteFile(stale, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteBuild(root, ids, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file remained: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(build, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manifest) != "{\n  \"entries\": []\n}\n" {
		t.Fatalf("empty manifest %q", manifest)
	}

	elsewhere := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(elsewhere, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(build); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, build); err != nil {
		t.Fatal(err)
	}
	if err := WriteBuild(root, nil, nil); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink err %v", err)
	}
	kept, err := os.ReadFile(keep)
	if err != nil {
		t.Fatal(err)
	}
	if string(kept) != "keep" {
		t.Fatalf("symlink target changed: %q", kept)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("write followed the symlink")
	}
}

func TestWriteBuildDuplicateID(t *testing.T) {
	root := t.TempDir()
	id := "01M5C5QEHHNXJ3D7X2ZCMGXQT7"
	err := WriteBuild(root, []DecisionRef{{Path: "a.md", ID: id}, {Path: "b.md", ID: id}}, nil)
	if CodeOf(err) != CodeDuplicateID {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".ergo", "build")); !os.IsNotExist(statErr) {
		t.Fatal("duplicate id wrote a build")
	}
}

func TestCompileRecordReports(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "policies"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "policies", "rule.txt"), []byte("yes"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	rec := edr.Record{
		ID: "01M5C5QEHHNXJ3D7X2ZCMGXQT7", Slug: "tls", Criticality: "blocking",
		Checks: []edr.Check{
			{Engine: "opa", Ref: "policies/rule.txt"},
			{Engine: "semgrep", Ref: "rules/x.yaml"},
			{Engine: "llm-advisory"},
		},
	}
	reg := NewRegistry(readRefCompiler{engine: "opa", version: "test"})
	compiled, reports, err := CompileRecord(reg, rec, repo.FS())
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled) != 1 || compiled[0].Entry.Mode != "block" || string(compiled[0].Artifacts[0].Data) != "yes" {
		t.Fatalf("compiled %+v", compiled)
	}
	if len(reports) != 3 || reports[0].Kind != ReportCompiled || reports[1].Kind != ReportNoCompiler || reports[2].Kind != ReportAdvisory {
		t.Fatalf("reports %+v", reports)
	}
	rec.Checks = []edr.Check{{Engine: "opa", Ref: "policies/missing.txt"}}
	_, _, err = CompileRecord(reg, rec, repo.FS())
	if CodeOf(err) != CodeRefMissing {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}

type readRefCompiler struct {
	engine  string
	version string
}

func (c readRefCompiler) Engine() string  { return c.engine }
func (c readRefCompiler) Version() string { return c.version }
func (c readRefCompiler) Compile(in Input) ([]Artifact, error) {
	data, err := ReadRef(in)
	if err != nil {
		return nil, err
	}
	return []Artifact{{Name: in.Check.Ref, Data: data}}, nil
}
