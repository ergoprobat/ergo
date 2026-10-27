package archunit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestPinnedJarsMatchLockfile(t *testing.T) {
	raw, err := os.ReadFile("testdata/tool/jars.lock")
	if err != nil {
		t.Fatal(err)
	}
	locked := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		if len(fields) != 3 {
			t.Fatalf("lock line %q", line)
		}
		locked[fields[0]] = fields[1]
	}
	if len(locked) != len(pinnedJars) {
		t.Fatalf("lockfile has %d jars, code pins %d", len(locked), len(pinnedJars))
	}
	for _, jar := range pinnedJars {
		if locked[jar.name] != jar.sha256 {
			t.Fatalf("%s: lockfile %q, code %q", jar.name, locked[jar.name], jar.sha256)
		}
	}
	if archunitJarName != "archunit-"+version+".jar" {
		t.Fatalf("jar %s is not version %s", archunitJarName, version)
	}
}

func TestStageJars(t *testing.T) {
	source := fakeJars(t)
	dest := filepath.Join(t.TempDir(), "lib")
	if err := stageJars(source, dest); err != nil {
		t.Fatal(err)
	}
	for _, jar := range pinnedJars {
		info, err := os.Stat(filepath.Join(dest, jar.name))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", jar.name, info, err)
		}
	}
}

func TestStageJarsRefusesWhatIsNotPinned(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"changed bytes": func(t *testing.T, dir string) {
			writeFiles(t, dir, pinnedJars[0].name)
		},
		"missing": func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, pinnedJars[1].name)); err != nil {
				t.Fatal(err)
			}
		},
		"directory": func(t *testing.T, dir string) {
			path := filepath.Join(dir, pinnedJars[2].name)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"over the cap": func(t *testing.T, dir string) {
			path := filepath.Join(dir, pinnedJars[0].name)
			if err := os.Truncate(path, maxJarBytes+1); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			source := fakeJars(t)
			change(t, source)
			err := stageJars(source, filepath.Join(t.TempDir(), "lib"))
			if compiler.CodeOf(err) != CodeJar {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}

func TestStageJarsDoesNotReadAgainAfterTheCheck(t *testing.T) {
	source := fakeJars(t)
	dest := filepath.Join(t.TempDir(), "lib")
	if err := stageJars(source, dest); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, source, pinnedJars[0].name)
	got := readText(t, filepath.Join(dest, pinnedJars[0].name))
	if got != "fake "+pinnedJars[0].name {
		t.Fatalf("staged copy changed: %q", got)
	}
}

func TestDocumentedJarHashesMatchThePins(t *testing.T) {
	doc := readText(t, filepath.Join("..", "..", "..", "docs", "archunit-rules.md"))
	for _, jar := range pinnedJars {
		if !strings.Contains(doc, "`"+jar.name+"` | `"+jar.sha256+"`") {
			t.Errorf("docs/archunit-rules.md does not list %s with its pinned sha256", jar.name)
		}
	}
}
