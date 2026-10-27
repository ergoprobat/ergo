package compiler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSelected(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	end := "2026-12-31"
	base := Entry{Engine: "opa", Target: "terraform-plan", ValidFrom: "2026-01-15"}
	if !base.Selected("", now) {
		t.Fatal("an in-force plan check should be selected")
	}
	prod := base
	prod.Env = []string{"dev", "prod"}
	if !prod.Selected("prod", now) || prod.Selected("stage", now) {
		t.Fatal("env filter")
	}
	if !prod.Selected("", now) {
		t.Fatal("no env should select every check")
	}
	expired := base
	expired.ValidTo = &end
	expired.ValidFrom = "2027-01-01"
	if expired.Selected("", now) {
		t.Fatal("a future decision should not be selected")
	}
	other := base
	other.Engine = "semgrep"
	if other.Selected("", now) {
		t.Fatal("another engine should not be selected")
	}
	repo := base
	repo.Target = ""
	if repo.Selected("", now) {
		t.Fatal("a repository policy should not be selected")
	}
}

func TestLoadBuildDigest(t *testing.T) {
	root := t.TempDir()
	id := "01M5C5QEHHNXJ3D7X2ZCMGXQT7"
	dir := filepath.Join(root, ".ipso", "build", id, "0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("package demo\n")
	if err := os.WriteFile(filepath.Join(dir, "policy.rego"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := Entry{
		DecisionID: id,
		CheckIndex: 0,
		Engine:     "opa",
		Target:     "terraform-plan",
		ValidFrom:  "2026-01-15",
		Artifacts:  []ManifestArtifact{{Name: "policy.rego", SHA256: "ec11a60fd4b8d78e363830f844d3de58fcf218682562ed7d49731620407e6737"}},
	}
	raw, err := MarshalManifest([]Entry{entry})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ipso", "build", "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	build, err := LoadBuild(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(build.Entries) != 1 || build.SHA256() == "" {
		t.Fatalf("build %+v", build.Entries)
	}
	got, err := build.Artifacts(build.Entries[0])
	if err != nil || string(got["policy.rego"]) != string(body) {
		t.Fatalf("artifacts %v %v", got, err)
	}

	kept, err := build.Artifacts(build.Entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "policy.rego"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := build.Artifacts(build.Entries[0])
	if err != nil || string(again["policy.rego"]) != string(kept["policy.rego"]) {
		t.Fatal("evaluation would read the file a second time")
	}
	_, err = LoadBuild(root)
	if CodeOf(err) != CodeBuildDigest {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}

func TestLoadBuildRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	secret := filepath.Join(root, "secret.rego")
	if err := os.WriteFile(secret, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".ipso", "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalManifest([]Entry{{
		DecisionID: "..",
		ValidFrom:  "2026-01-15",
		Artifacts:  []ManifestArtifact{{Name: "secret.rego", SHA256: "0"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ipso", "build", "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadBuild(root)
	if CodeOf(err) != CodeArtifactName {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}

func TestLoadBuildRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ipso"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(root, ".ipso", "build")); err != nil {
		t.Fatal(err)
	}
	_, err := LoadBuild(root)
	if err == nil || CodeOf(err) != "" {
		t.Fatalf("symlink was accepted: %v", err)
	}
}

func TestLoadBuildRejectsArtifactSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "policy.rego")
	body := []byte("package demo\n")
	if err := os.WriteFile(outside, body, 0o644); err != nil {
		t.Fatal(err)
	}
	id := "01M5C5QEHHNXJ3D7X2ZCMGXQT7"
	dir := filepath.Join(root, ".ipso", "build", id, "0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "policy.rego")); err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalManifest([]Entry{{
		DecisionID: id,
		CheckIndex: 0,
		ValidFrom:  "2026-01-15",
		Artifacts:  []ManifestArtifact{{Name: "policy.rego", SHA256: "ec11a60fd4b8d78e363830f844d3de58fcf218682562ed7d49731620407e6737"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ipso", "build", "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadBuild(root)
	if CodeOf(err) != CodeArtifactName {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}

func TestLoadBuildRejectsBadDate(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".ipso", "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := MarshalManifest([]Entry{{
		DecisionID: "01M5C5QEHHNXJ3D7X2ZCMGXQT7",
		Engine:     "opa",
		Target:     "terraform-plan",
		ValidFrom:  "tomorrow",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".ipso", "build", "manifest.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadBuild(root)
	if CodeOf(err) != CodeBuildManifest {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}

func TestLoadBuildMissing(t *testing.T) {
	_, err := LoadBuild(t.TempDir())
	if CodeOf(err) != CodeBuildMissing {
		t.Fatalf("code %q err %v", CodeOf(err), err)
	}
}
