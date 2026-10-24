package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/builtin"
)

func TestInitCheckExplain(t *testing.T) {
	valid := readFixture(t, "valid.md")
	root := t.TempDir()
	withDir(t, root)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init code %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), ".ergo/config.yaml") {
		t.Fatalf("init output %q", stdout.String())
	}
	stdout.Reset()
	if code := Run([]string{"init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("second init code %d", code)
	}
	if !strings.Contains(stdout.String(), "already exists") {
		t.Fatalf("second init output %q", stdout.String())
	}

	dest := filepath.Join(root, "docs", "decisions", "01-tls.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, valid, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("check code %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "ok") {
		t.Fatalf("check output %q", stdout.String())
	}

	stdout.Reset()
	if code := Run([]string{"explain", "tls-on-public-ingress"}, &stdout, &stderr); code != 0 {
		t.Fatalf("explain code %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "title: Public ingress must terminate TLS") || !strings.Contains(stdout.String(), "terminates TLS") {
		t.Fatalf("explain output %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	adr := filepath.Join(root, "docs", "adr", "0002-use-postgres.md")
	if err := os.MkdirAll(filepath.Dir(adr), 0o755); err != nil {
		t.Fatal(err)
	}
	nygard := "# Use Postgres\n\nDate: 2024-01-02\n\n## Status\n\nAccepted\n\n## Context\n\nThe catalog is relational.\n\n## Decision\n\nUse Postgres.\n\n## Consequences\n\nOne engine to operate.\n"
	if err := os.WriteFile(adr, []byte(nygard), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"import"}, &stdout, &stderr); code != 0 {
		t.Fatalf("import code %d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "docs/adr/0002-use-postgres.md: imported inferred") {
		t.Fatalf("import output %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "docs/decisions/01-tls.md: skipped") {
		t.Fatalf("import did not skip the EDR file: %q", stdout.String())
	}
}

func TestCheckInvalid(t *testing.T) {
	invalid := readFixture(t, "invalid.md")
	root := t.TempDir()
	withDir(t, root)
	dest := filepath.Join(root, "docs", "decisions", "bad.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, invalid, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"check"}, &stdout, &stderr); code != 1 {
		t.Fatalf("code %d output %s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "missing heading ## Verification") {
		t.Fatalf("output %q", stdout.String())
	}
}

func TestHelpVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d", code)
	}
	if !strings.Contains(stdout.String(), "ergo dev") || !strings.Contains(stdout.String(), "ergo compile") {
		t.Fatalf("output %q", stdout.String())
	}
}

func TestCheckNone(t *testing.T) {
	root := t.TempDir()
	withDir(t, root)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"check"}, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d stderr %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "no EDR files found") {
		t.Fatalf("output %q", stdout.String())
	}
}

func TestCompile(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	reg := compiler.NewRegistry(copyCompiler{engine: "opa", version: "test"})

	t.Run("reports", func(t *testing.T) {
		root := t.TempDir()
		withDir(t, root)
		writeRepoFile(t, root, "policies/rule.txt", "rule-body\n")
		files := map[string]string{
			"docs/decisions/accepted.md": compileRecord(idA, "accepted-one", "accepted", "warning", "2026-01-01", "", `  - engine: opa
    ref: policies/rule.txt
  - engine: semgrep
    ref: rules/x.yaml
  - engine: llm-advisory
    mode: advise
`),
			"docs/decisions/ended.md":    compileRecord(idB, "ended-one", "accepted", "blocking", "2026-01-01", "2026-02-01", ""),
			"docs/decisions/open.md":     compileRecord(idC, "open-one", "needs-review", "blocking", "2026-02-01", "", ""),
			"docs/decisions/proposed.md": compileRecord("01M5C5QEHHNXJ3D7X2ZCMGXQTA", "proposed-one", "proposed", "blocking", "2026-01-01", "", ""),
		}
		for path, body := range files {
			writeRepoFile(t, root, path, body)
		}
		stdout, stderr, code := compileAt(t, now, reg)
		if code != 0 {
			t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
		}
		want := []string{
			"docs/decisions/accepted.md: checks[0] (opa): compiled 1 artifact",
			"docs/decisions/accepted.md: checks[1] (semgrep): no compiler",
			"docs/decisions/accepted.md: checks[2] (llm-advisory): advisory, not compiled",
			"docs/decisions/ended.md: skipped, not in force",
			"docs/decisions/proposed.md: skipped, not in force",
		}
		last := -1
		for _, line := range want {
			at := strings.Index(stdout, line)
			if at < 0 || at < last {
				t.Fatalf("missing or out of order %q in %s", line, stdout)
			}
			last = at
		}
		if strings.Contains(stdout, "open.md") {
			t.Fatalf("empty in-force record produced a line: %s", stdout)
		}
		manifest := readBuild(t, root, "manifest.json")
		if !strings.Contains(manifest, `"mode": "warn"`) || !strings.Contains(manifest, `"compiler_version": "test"`) {
			t.Fatalf("manifest %s", manifest)
		}
		if readBuild(t, root, ".gitignore") != "*\n" {
			t.Fatalf("gitignore %q", readBuild(t, root, ".gitignore"))
		}
		first := readBuild(t, root, "manifest.json")
		writeRepoFile(t, root, ".ergo/build/stale.txt", "old")
		stdout, stderr, code = compileAt(t, now, reg)
		if code != 0 {
			t.Fatalf("rerun code %d\n%s\n%s", code, stdout, stderr)
		}
		second := readBuild(t, root, "manifest.json")
		if first != second {
			t.Fatalf("manifest changed\n%s\n%s", first, second)
		}
		if _, err := os.Stat(filepath.Join(root, ".ergo", "build", "stale.txt")); !os.IsNotExist(err) {
			t.Fatal("stale artifact survived the rerun")
		}
	})

	t.Run("invalid keeps the previous build", func(t *testing.T) {
		invalid := string(readFixture(t, "invalid.md"))
		root := t.TempDir()
		withDir(t, root)
		writeRepoFile(t, root, ".ergo/build/stale.txt", "keep")
		writeRepoFile(t, root, "docs/decisions/bad.md", invalid)
		stdout, _, code := compileAt(t, now, reg)
		if code != 1 {
			t.Fatalf("code %d output %s", code, stdout)
		}
		if readBuild(t, root, "stale.txt") != "keep" {
			t.Fatal("invalid record replaced the build")
		}
	})

	t.Run("duplicate id", func(t *testing.T) {
		root := t.TempDir()
		withDir(t, root)
		writeRepoFile(t, root, ".ergo/build/stale.txt", "keep")
		writeRepoFile(t, root, "docs/decisions/a.md", compileRecord(idA, "one", "accepted", "blocking", "2026-01-01", "", ""))
		writeRepoFile(t, root, "docs/decisions/b.md", compileRecord(idA, "two", "accepted", "blocking", "2026-01-01", "", ""))
		stdout, _, code := compileAt(t, now, reg)
		if code != 1 || !strings.Contains(stdout, "duplicate-id") {
			t.Fatalf("code %d output %s", code, stdout)
		}
		if readBuild(t, root, "stale.txt") != "keep" {
			t.Fatal("duplicate id replaced the build")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		withDir(t, root)
		elsewhere := filepath.Join(root, "elsewhere")
		if err := os.MkdirAll(elsewhere, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(elsewhere, "keep.txt"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".ergo"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(elsewhere, filepath.Join(root, ".ergo", "build")); err != nil {
			t.Fatal(err)
		}
		writeRepoFile(t, root, "policies/rule.txt", "rule-body\n")
		writeRepoFile(t, root, "docs/decisions/accepted.md", compileRecord(idA, "accepted-one", "accepted", "blocking", "2026-01-01", "", "  - engine: opa\n    ref: policies/rule.txt\n"))
		stdout, stderr, code := compileAt(t, now, reg)
		if code != 1 || !strings.Contains(stderr, "symlink") {
			t.Fatalf("code %d stdout %s stderr %s", code, stdout, stderr)
		}
		if strings.Contains(stdout, "compiled") {
			t.Fatalf("symlink failure still reported success: %s", stdout)
		}
		kept, err := os.ReadFile(filepath.Join(elsewhere, "keep.txt"))
		if err != nil || string(kept) != "keep" {
			t.Fatalf("target %q err %v", kept, err)
		}
		if _, err := os.Stat(filepath.Join(elsewhere, "manifest.json")); !os.IsNotExist(err) {
			t.Fatal("compile followed the symlink")
		}
	})

	t.Run("no files", func(t *testing.T) {
		root := t.TempDir()
		withDir(t, root)
		stdout, stderr, code := compileAt(t, now, reg)
		if code != 0 {
			t.Fatalf("code %d stderr %s", code, stderr)
		}
		if !strings.Contains(stdout, "no EDR files found") {
			t.Fatalf("output %q", stdout)
		}
		if readBuild(t, root, "manifest.json") != "{\n  \"entries\": []\n}\n" {
			t.Fatalf("manifest %q", readBuild(t, root, "manifest.json"))
		}
	})

	t.Run("usage", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run([]string{"compile", "extra"}, &stdout, &stderr, deps{now: func() time.Time { return now }, registry: reg})
		if code != 2 || !strings.Contains(stderr.String(), "usage: ergo compile") {
			t.Fatalf("code %d stderr %s", code, stderr.String())
		}
	})
}

func TestCompileBuiltinTemplate(t *testing.T) {
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	withDir(t, root)
	writeRepoFile(t, root, "docs/decisions/s3.md", compileRecord(idA, "s3-kms", "accepted", "blocking", "2026-01-01", "", "  - engine: opa\n    target: terraform-plan\n    ref: ergo:aws/s3-kms-cmk\n"))
	stdout, stderr, code := compileAt(t, now, builtin.Registry())
	if code != 0 {
		t.Fatalf("code %d\n%s\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "docs/decisions/s3.md: checks[0] (opa): compiled 2 artifacts") {
		t.Fatalf("stdout %s", stdout)
	}
	if !strings.Contains(readBuild(t, root, idA+"/0/policy.rego"), "package ergo.templates.aws.s3_kms_cmk") {
		t.Fatal("policy.rego is not the S3 template")
	}
	if !strings.Contains(readBuild(t, root, idA+"/0/terraform.rego"), "package ergo.lib.terraform") {
		t.Fatal("terraform.rego is not the helper module")
	}
}

func compileAt(t *testing.T, now time.Time, reg compiler.Registry) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run([]string{"compile"}, &stdout, &stderr, deps{now: func() time.Time { return now }, registry: reg})
	return stdout.String(), stderr.String(), code
}

func writeRepoFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readBuild(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".ergo", "build", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type copyCompiler struct {
	engine  string
	version string
}

func (c copyCompiler) Engine() string  { return c.engine }
func (c copyCompiler) Version() string { return c.version }
func (c copyCompiler) Compile(in compiler.Input) ([]compiler.Artifact, error) {
	data, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{{Name: in.Check.Ref, Data: data}}, nil
}

const (
	idA = "01M5C5QEHHNXJ3D7X2ZCMGXQT7"
	idB = "01M5C5QEHHNXJ3D7X2ZCMGXQT8"
	idC = "01M5C5QEHHNXJ3D7X2ZCMGXQT9"
)

func compileRecord(id, slug, status, criticality, from, to, checks string) string {
	if to == "" {
		to = "null"
	}
	checkBlock := "checks: []\n"
	if checks != "" {
		checkBlock = "checks:\n" + checks
		if !strings.HasSuffix(checkBlock, "\n") {
			checkBlock += "\n"
		}
	}
	return fmt.Sprintf(`---
edr: 1
id: %s
slug: %s
title: %s
status: %s
owner: team:platform
scope:
  - org: acme
valid_from: %s
valid_to: %s
review_by: 2026-07-15
supersedes: []
criticality: %s
%swaivers: []
sources:
  - type: pr
    url: https://github.com/acme/platform/pull/10
tags: [fixture]
---

# %s

## Context

The record is a compile fixture.

## Decision

Compile the checks that are in force.

## Alternatives considered

- Leave the checks empty.

## Consequences

ergo compile writes one manifest entry per compiled check.

## Verification

The command output names each check.
`, id, slug, slug, status, from, to, criticality, checkBlock, slug)
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "edr", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func withDir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})
}
