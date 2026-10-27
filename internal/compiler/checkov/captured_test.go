package checkov

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func TestCapturedReports(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	binding := map[string][]byte{"binding.json": []byte("{\n  \"check_id\": \"CKV_AWS_24\"\n}\n")}
	entry := compiler.Entry{}

	pass := readCaptured(t, "pass.json")
	if outcome := EvalCheck(entry, binding, pass, now); outcome.Err != nil || len(outcome.Violations) != 0 {
		t.Fatalf("pass %+v", outcome)
	}
	failed := readCaptured(t, "fail.json")
	outcome := EvalCheck(entry, binding, failed, now)
	if outcome.Err != nil || len(outcome.Violations) != 1 {
		t.Fatalf("fail %+v", outcome)
	}
	skipped := readCaptured(t, "skip.json")
	outcome = EvalCheck(entry, binding, skipped, now)
	if outcome.Err != nil || len(outcome.Violations) != 1 {
		t.Fatalf("skip %+v", outcome)
	}
	empty := readCaptured(t, "empty.json")
	outcome = EvalCheck(entry, binding, empty, now)
	if outcome.Err != nil || len(outcome.Violations) != 1 {
		t.Fatalf("empty %+v", outcome)
	}
	parsed := readCaptured(t, "parse-error.json")
	if compiler.CodeOf(EvalCheck(entry, binding, parsed, now).Err) != CodeFormat {
		t.Fatal("parse error was not an evaluation error")
	}
	multi := readCaptured(t, "multi.json")
	if outcome := EvalCheck(entry, binding, multi, now); outcome.Err != nil || len(outcome.Violations) != 0 {
		t.Fatalf("multi pass %+v", outcome)
	}
	docker := map[string][]byte{"binding.json": []byte("{\n  \"check_id\": \"CKV_DOCKER_2\"\n}\n")}
	outcome = EvalCheck(entry, docker, multi, now)
	if outcome.Err != nil || len(outcome.Violations) != 1 {
		t.Fatalf("multi fail %+v", outcome)
	}
}

func readCaptured(t *testing.T, name string) Results {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", "captured", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	results, err := ReadResults(file)
	if err != nil {
		t.Fatal(err)
	}
	if results.Version != version {
		t.Fatalf("%s version %s", name, results.Version)
	}
	return results
}
