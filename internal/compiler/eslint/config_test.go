package eslint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// goldenConfig is one compiled eslint.config.mjs from a compiler fixture, with the target and mode its entry carries.
type goldenConfig struct {
	name   string
	config []byte
	target string
	mode   string
}

func goldenConfigs(t *testing.T) []goldenConfig {
	t.Helper()
	manifests, err := filepath.Glob(filepath.Join("testdata", "*", "golden", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out []goldenConfig
	for _, path := range manifests {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var manifest struct {
			Entries []compiler.Entry `json:"entries"`
		}
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(path)
		for _, entry := range manifest.Entries {
			config, err := os.ReadFile(filepath.Join(dir, strconv.Itoa(entry.CheckIndex), artifactName))
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, goldenConfig{
				name:   filepath.Base(filepath.Dir(dir)) + "/" + strconv.Itoa(entry.CheckIndex),
				config: config,
				target: entry.Target,
				mode:   entry.Mode,
			})
		}
	}
	if len(out) < 8 {
		t.Fatalf("only %d golden configs", len(out))
	}
	return out
}

func requireBuildManifest(t *testing.T, err error) {
	t.Helper()
	if compiler.CodeOf(err) != compiler.CodeBuildManifest {
		t.Fatalf("code %q (%v), want %s", compiler.CodeOf(err), err, compiler.CodeBuildManifest)
	}
}

func TestReadCanonicalAcceptsEveryGoldenConfig(t *testing.T) {
	for _, item := range goldenConfigs(t) {
		t.Run(item.name, func(t *testing.T) {
			ids, err := readCanonical(item.config, item.target, item.mode)
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) == 0 {
				t.Fatal("no rule ids")
			}
		})
	}
}

func TestReadCanonicalReturnsTheRuleIDs(t *testing.T) {
	var found bool
	for _, item := range goldenConfigs(t) {
		if item.name != "app-violating/0" {
			continue
		}
		found = true
		ids, err := readCanonical(item.config, item.target, item.mode)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for id := range ids {
			got = append(got, id)
		}
		sort.Strings(got)
		if strings.Join(got, ",") != "@typescript-eslint/no-floating-promises" {
			t.Fatalf("ids %v", got)
		}
	}
	if !found {
		t.Fatal("fixture app-violating/0 is missing")
	}
}

func TestReadCanonicalKeepsRuleOptions(t *testing.T) {
	for _, item := range goldenConfigs(t) {
		if item.name != "options/0" {
			continue
		}
		if _, err := readCanonical(item.config, item.target, item.mode); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("fixture options/0 is missing")
}

// A config that ipso did not write must not reach ESLint, which runs it as JavaScript.
func TestReadCanonicalRefusesAnyOtherConfig(t *testing.T) {
	var ts, js goldenConfig
	for _, item := range goldenConfigs(t) {
		switch item.name {
		case "app-violating/0":
			ts = item
		case "app-violating/1":
			js = item
		}
	}
	if ts.config == nil || js.config == nil {
		t.Fatal("app-violating fixtures are missing")
	}
	tsText, jsText := string(ts.config), string(js.config)

	cases := map[string]struct {
		config string
		target string
		mode   string
	}{
		"an import before the config":     {"import fs from \"node:fs\";\n" + jsText, "javascript", "block"},
		"a statement after the config":    {jsText + "process.exit(0);\n", "javascript", "block"},
		"a comment":                       {strings.Replace(jsText, "export default", "// note\nexport default", 1), "javascript", "block"},
		"inline config allowed":           {strings.Replace(jsText, "noInlineConfig: true", "noInlineConfig: false", 1), "javascript", "block"},
		"inline config line removed":      {strings.Replace(jsText, "      noInlineConfig: true,\n", "", 1), "javascript", "block"},
		"a second config object":          {strings.Replace(jsText, "];\n", "  { rules: {} },\n];\n", 1), "javascript", "block"},
		"a changed severity":              {strings.Replace(jsText, `"eqeqeq": "error"`, `"eqeqeq": "warn"`, 1), "javascript", "block"},
		"the severity of another mode":    {jsText, "javascript", "warn"},
		"a rule that is turned off":       {strings.Replace(jsText, `"eqeqeq": "error"`, `"eqeqeq": "off"`, 1), "javascript", "block"},
		"a deprecated rule":               {strings.Replace(jsText, `"eqeqeq"`, `"indent"`, 1), "javascript", "block"},
		"an unknown rule":                 {strings.Replace(jsText, `"eqeqeq"`, `"no-such-rule"`, 1), "javascript", "block"},
		"a typescript rule on javascript": {strings.Replace(jsText, `"eqeqeq"`, `"@typescript-eslint/no-floating-promises"`, 1), "javascript", "block"},
		"the typescript config as js":     {tsText, "javascript", "block"},
		"the javascript config as ts":     {jsText, "typescript", "block"},
		"another target":                  {jsText, "python", "block"},
		"an empty target":                 {jsText, "", "block"},
		"another mode":                    {jsText, "javascript", "audit"},
		"a pattern that leaves the repo":  {strings.Replace(jsText, `"web/**/*.js"`, `"../x/**/*.js"`, 1), "javascript", "block"},
		"a negated pattern":               {strings.Replace(jsText, `"web/**/*.js"`, `"!web/**/*.js"`, 1), "javascript", "block"},
		"a spread of other files":         {strings.Replace(jsText, `files: ["web/**/*.js"]`, `files: ["web/**/*.js"], ...await import("node:fs")`, 1), "javascript", "block"},
		"files with code":                 {strings.Replace(jsText, `["web/**/*.js"]`, `[(await import("node:fs"), "a.js")]`, 1), "javascript", "block"},
		"a repeated files line":           {strings.Replace(jsText, "    files:", "    files: [\"x.js\"],\n    files:", 1), "javascript", "block"},
		"a rules line with code":          {strings.Replace(jsText, `"eqeqeq": "error"`, `"eqeqeq": "error", ...globalThis.x`, 1), "javascript", "block"},
		"a prototype key":                 {strings.Replace(jsText, `"eqeqeq": "error"`, `"eqeqeq": "error", "__proto__": "error"`, 1), "javascript", "block"},
		"a repeated rule":                 {strings.Replace(jsText, `"eqeqeq": "error"`, `"eqeqeq": "error", "eqeqeq": "error"`, 1), "javascript", "block"},
		"a typescript parser swapped":     {strings.Replace(tsText, "tseslint.parser", "globalThis.parser", 1), "typescript", "block"},
		"a missing plugin":                {strings.Replace(tsText, "      \"@typescript-eslint\": tseslint.plugin,\n", "", 1), "typescript", "block"},
		"windows line endings":            {strings.ReplaceAll(jsText, "\n", "\r\n"), "javascript", "block"},
		"a trailing newline removed":      {strings.TrimSuffix(jsText, "\n"), "javascript", "block"},
		"an extra blank line":             {jsText + "\n", "javascript", "block"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if tc.config == jsText && tc.target == "javascript" && tc.mode == "block" {
				t.Fatal("the case does not change anything")
			}
			_, err := readCanonical([]byte(tc.config), tc.target, tc.mode)
			requireBuildManifest(t, err)
		})
	}
}

func TestReadCanonicalRefusesWhatIsNotAConfig(t *testing.T) {
	for name, config := range map[string]string{
		"empty":                         "",
		"a newline":                     "\n",
		"only the export":               "export default [];\n",
		"files and no rules":            "export default [\n  {\n    files: [\"a.js\"],\n  },\n];\n",
		"rules and no files":            "export default [\n  {\n    rules: {\n      \"eqeqeq\": \"error\"\n    },\n  },\n];\n",
		"files without a comma":         "export default [\n  {\n    files: [\"a.js\"]\n    rules: {\n      \"eqeqeq\": \"error\"\n    },\n  },\n];\n",
		"files that are not JSON":       "export default [\n  {\n    files: [a.js],\n    rules: {\n      \"eqeqeq\": \"error\"\n    },\n  },\n];\n",
		"a rules block that never ends": "export default [\n  {\n    files: [\"a.js\"],\n    rules: {\n      \"eqeqeq\": \"error\"\n",
		"a rules line without indent":   "export default [\n  {\n    files: [\"a.js\"],\n    rules: {\n\"eqeqeq\": \"error\"\n    },\n  },\n];\n",
		"a binary file":                 "\x00\x01\x02",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := readCanonical([]byte(config), "javascript", "block")
			requireBuildManifest(t, err)
		})
	}
}
