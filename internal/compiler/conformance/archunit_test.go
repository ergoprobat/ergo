//go:build conformance

package conformance

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/archunit"
	"github.com/ipsofacto-io/ipso/internal/edr"
)

const (
	archunitJar = "../archunit/testdata/tool/lib/archunit-1.5.1.jar"
	slf4jJar    = "../archunit/testdata/tool/lib/slf4j-api-2.0.19.jar"
	junitJar    = "../archunit/testdata/tool/lib/junit-platform-console-standalone-6.1.3.jar"
)

func TestArchUnit(t *testing.T) {
	for _, path := range []string{archunitJar, slf4jJar, junitJar} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	requireVersion(t, []byte(implementationVersion(t, archunitJar)), archunit.New().Version())

	clean := compileArchUnit(t, archunitClean)
	bad := compileArchUnit(t, archunitBad)
	if !bytes.Equal(clean, bad) {
		t.Fatal("clean and violating tests compiled to different artifacts")
	}
	failed, exit := runArchUnit(t, clean, archunitClean)
	if exit != 0 || len(failed) != 0 {
		t.Fatalf("clean exit %d failures %#v", exit, failed)
	}
	failed, exit = runArchUnit(t, clean, archunitBad)
	want := []string{"rule_domain_not_to_adapters", "rule_layers"}
	if exit == 0 || strings.Join(failed, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violating exit %d failures %#v", exit, failed)
	}
}

func implementationVersion(t *testing.T, path string) string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	var manifest io.ReadCloser
	for _, file := range reader.File {
		if file.Name == "META-INF/MANIFEST.MF" {
			manifest, err = file.Open()
			if err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	if manifest == nil {
		t.Fatal("jar has no manifest")
	}
	t.Cleanup(func() { _ = manifest.Close() })
	scanner := bufio.NewScanner(manifest)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Implementation-Version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Implementation-Version:"))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("manifest has no Implementation-Version")
	return ""
}

func compileArchUnit(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "record.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := edr.Parse(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if problems := edr.Validate(doc); len(problems) != 0 {
		t.Fatal(strings.Join(problems, "; "))
	}
	rec, err := edr.RecordFrom(doc)
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(filepath.Join(dir, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	var in compiler.Input
	found := false
	for index, check := range rec.Checks {
		if check.Engine != "archunit" {
			continue
		}
		in = compiler.Input{Record: rec, Check: check, Index: index, Repo: root.FS()}
		found = true
		break
	}
	if !found {
		t.Fatal("no archunit check")
	}
	artifacts, err := archunit.New().Compile(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Name != "ArchitectureTest.java" {
		t.Fatalf("artifacts %#v", artifacts)
	}
	return artifacts[0].Data
}

func runArchUnit(t *testing.T, source []byte, subject string) (failed []string, exit int) {
	t.Helper()
	repo := copyFixtureTree(t, filepath.Join(subject, "repo"))
	testFile := filepath.Join(t.TempDir(), "ArchitectureTest.java")
	if err := os.WriteFile(testFile, source, 0o644); err != nil {
		t.Fatal(err)
	}
	classes := t.TempDir()
	args := []string{"--release", "21", "-encoding", "UTF-8", "-d", classes, "-cp", archClasspath(classes)}
	args = append(args, javaSources(t, repo)...)
	args = append(args, testFile)
	stdout, stderr, code := javacOutput(t, 60*time.Second, "", args...)
	if code != 0 {
		t.Fatalf("javac exit %d\n%s\n%s", code, stdout, stderr)
	}
	reports := t.TempDir()
	_, stderr, code = javaOutput(t, 60*time.Second, "",
		"-jar", junitJar,
		"execute",
		"--disable-banner",
		"--disable-ansi-colors",
		"--class-path", archClasspath(classes),
		"--select-class", testClass(t, source),
		"--reports-dir", reports,
	)
	failed = junitFailures(t, reports)
	if len(failed) == 0 && code != 0 {
		t.Fatalf("junit exit %d\n%s", code, stderr)
	}
	return failed, code
}

func archClasspath(classes string) string {
	return strings.Join([]string{classes, archunitJar, slf4jJar, junitJar}, string(os.PathListSeparator))
}

func javaSources(t *testing.T, dir string) []string {
	t.Helper()
	var sources []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".java") {
			sources = append(sources, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(sources)
	return sources
}

func testClass(t *testing.T, source []byte) string {
	t.Helper()
	for _, line := range strings.Split(string(source), "\n") {
		if strings.HasPrefix(line, "package ") && strings.HasSuffix(line, ";") {
			return strings.TrimSuffix(strings.TrimPrefix(line, "package "), ";") + ".ArchitectureTest"
		}
	}
	t.Fatal("generated test has no package")
	return ""
}

func junitFailures(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	reports := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xml") {
			continue
		}
		reports++
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		dec := xml.NewDecoder(bytes.NewReader(body))
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			start, ok := tok.(xml.StartElement)
			if !ok || start.Name.Local != "testcase" {
				continue
			}
			var item struct {
				Name     string     `xml:"name,attr"`
				Failures []struct{} `xml:"failure"`
				Errors   []struct{} `xml:"error"`
			}
			if err := dec.DecodeElement(&item, &start); err != nil {
				t.Fatal(err)
			}
			if len(item.Failures)+len(item.Errors) == 0 {
				continue
			}
			name := strings.TrimSuffix(item.Name, "()")
			seen[name] = struct{}{}
		}
	}
	if reports == 0 {
		t.Fatal("junit wrote no report")
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
