package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.Contains(stdout.String(), "ergo dev") {
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
