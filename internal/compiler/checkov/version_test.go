package checkov

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCapturedVersion(t *testing.T) {
	dir := filepath.Join("testdata", "captured")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no captured Checkov output")
	}
	seen := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, got := range versionsOf(t, raw) {
			seen++
			if got != version {
				t.Fatalf("%s checkov_version %s, compiler version %s", entry.Name(), got, version)
			}
		}
	}
	if seen == 0 {
		t.Fatal("captured files have no checkov_version")
	}
}

func versionsOf(t *testing.T, raw []byte) []string {
	t.Helper()
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	switch doc := decoded.(type) {
	case []any:
		var versions []string
		for _, item := range doc {
			versions = append(versions, versionsOf(t, mustJSON(t, item))...)
		}
		return versions
	case map[string]any:
		if text, ok := doc["checkov_version"].(string); ok && text != "" {
			return []string{text}
		}
		summary, _ := doc["summary"].(map[string]any)
		text, _ := summary["checkov_version"].(string)
		if text == "" {
			t.Fatalf("no checkov_version in %v", doc)
		}
		return []string{text}
	default:
		t.Fatalf("captured JSON %T", decoded)
		return nil
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
