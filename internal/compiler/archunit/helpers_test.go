package archunit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeJars writes three stand-in jars and pins their hashes for the test.
// Tests that use it must not run in parallel: the pins are package state.
func fakeJars(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	saved := pinnedJars
	pins := make([]pinnedJar, len(saved))
	for i, jar := range saved {
		body := []byte("fake " + jar.name)
		sum := sha256.Sum256(body)
		pins[i] = pinnedJar{name: jar.name, sha256: hex.EncodeToString(sum[:])}
		if err := os.WriteFile(filepath.Join(dir, jar.name), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pinnedJars = pins
	t.Cleanup(func() { pinnedJars = saved })
	return dir
}

func skipFakeJDK(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake java is a shell script")
	}
}

// installFakeJDK puts a fake java and a fake javac first on PATH, alone.
// Each script records what it saw into the files named by FAKE_* variables.
func installFakeJDK(t *testing.T) {
	t.Helper()
	skipFakeJDK(t)
	dir := t.TempDir()
	java := `#!/bin/sh
if [ "$1" = "--version" ]; then
  if [ -n "$FAKE_JAVA_VERSION_PWD" ]; then
    pwd > "$FAKE_JAVA_VERSION_PWD"
  fi
  printf '%s\n' "$FAKE_JAVA_VERSION"
  exit 0
fi
if [ -n "$FAKE_JAVA_ARGS" ]; then
  printf '%s\n' "$@" > "$FAKE_JAVA_ARGS"
fi
if [ -n "$FAKE_JAVA_ENV" ]; then
  {
    printf 'tool=%s\n' "$JAVA_TOOL_OPTIONS"
    printf 'jdk=%s\n' "$JDK_JAVA_OPTIONS"
    printf '_java=%s\n' "$_JAVA_OPTIONS"
    printf 'javac=%s\n' "$JDK_JAVAC_OPTIONS"
    printf 'classpath=%s\n' "$CLASSPATH"
    printf 'keep=%s\n' "$IPSO_TEST_KEEP"
  } > "$FAKE_JAVA_ENV"
fi
if [ -n "$FAKE_JAVA_PWD" ]; then
  pwd > "$FAKE_JAVA_PWD"
fi
prev=""
reports=""
for arg in "$@"; do
  if [ "$prev" = "--reports-dir" ]; then
    reports=$arg
  fi
  prev=$arg
done
if [ -n "$FAKE_JAVA_SLEEP" ]; then
  /bin/sleep "$FAKE_JAVA_SLEEP"
fi
if [ -n "$FAKE_JAVA_REPORT" ] && [ -n "$reports" ]; then
  /bin/cp "$FAKE_JAVA_REPORT" "$reports/TEST-junit-jupiter.xml"
fi
printf '%s' "$FAKE_JAVA_STDOUT"
printf '%s' "$FAKE_JAVA_STDERR" >&2
exit "$FAKE_JAVA_EXIT"
`
	javac := `#!/bin/sh
if [ "$1" = "--version" ]; then
  printf 'javac %s\n' "$FAKE_JAVAC_VERSION"
  exit 0
fi
if [ -n "$FAKE_JAVAC_ARGS" ]; then
  printf '%s\n' "$@" > "$FAKE_JAVAC_ARGS"
  last=""
  for arg in "$@"; do
    last=$arg
  done
  /bin/cp "$last" "$FAKE_JAVAC_ARGS.source"
fi
if [ "$FAKE_JAVAC_EXIT" != "0" ]; then
  printf '%s' "$FAKE_JAVAC_STDERR" >&2
  exit "$FAKE_JAVAC_EXIT"
fi
exit 0
`
	for name, body := range map[string]string{"java": java, "javac": javac} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_JAVA_VERSION", "openjdk 21.0.4 2024-07-16 LTS")
	t.Setenv("FAKE_JAVAC_VERSION", "21.0.4")
	for _, name := range []string{
		"FAKE_JAVA_ARGS", "FAKE_JAVA_ENV", "FAKE_JAVA_PWD", "FAKE_JAVA_VERSION_PWD", "FAKE_JAVA_SLEEP", "FAKE_JAVA_REPORT",
		"FAKE_JAVA_STDOUT", "FAKE_JAVA_STDERR", "FAKE_JAVAC_ARGS", "FAKE_JAVAC_STDERR",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("FAKE_JAVA_EXIT", "0")
	t.Setenv("FAKE_JAVAC_EXIT", "0")
}

func readText(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// writeFiles writes a small file for each slash-separated name below dir.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x "+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
