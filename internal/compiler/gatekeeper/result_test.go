package gatekeeper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

func output(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "output", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func outputPath(t *testing.T, name string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "output", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// mutate decodes a captured array, lets edit change its first result, and encodes it again.
func mutate(t *testing.T, name string, edit func(item map[string]any)) []byte {
	t.Helper()
	var items []map[string]any
	if err := json.Unmarshal(output(t, name), &items); err != nil {
		t.Fatal(err)
	}
	edit(items[0])
	raw, err := json.MarshalIndent(items, "", "    ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func labels(findings []Finding) []string {
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.Object + ": " + f.Message
	}
	sort.Strings(out)
	return out
}

func TestParseResultsReadsRealCaptures(t *testing.T) {
	cases := []struct {
		name   string
		exit   int
		action string
		want   []string
	}{
		{"pass", 0, "deny", nil},
		{"deny", 1, "deny", []string{"Deployment/web: missing owner label"}},
		{"multi", 1, "deny", []string{"Deployment/web: missing owner label", "team-a/Deployment/api: missing owner label"}},
		{"warn", 0, "warn", []string{"Deployment/web: missing owner label"}},
		{"dryrun", 0, "dryrun", []string{"Deployment/web: missing owner label"}},
		// The constraint has no match.kinds, so gator also reviewed ipso's own template and constraint.
		{"self", 1, "deny", []string{"Deployment/web: missing owner label"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseResults(output(t, tc.name), tc.exit, ownerID(tc.action))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(labels(got), "|") != strings.Join(tc.want, "|") {
				t.Fatalf("findings %v, want %v", labels(got), tc.want)
			}
		})
	}
}

// When only ipso's own objects fail the constraint, there is nothing to report, and the exit status
// still counts the results that were dropped.
func TestParseResultsDropsOwnObjectsButKeepsTheirExitStatus(t *testing.T) {
	var items []map[string]any
	if err := json.Unmarshal(output(t, "self"), &items); err != nil {
		t.Fatal(err)
	}
	var own []map[string]any
	for _, item := range items {
		if item["violatingObject"].(map[string]any)["kind"] != "Deployment" {
			own = append(own, item)
		}
	}
	if len(own) != 2 {
		t.Fatalf("the capture holds %d own results", len(own))
	}
	raw, err := json.Marshal(own)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseResults(raw, 1, ownerID("deny"))
	if err != nil || len(got) != 0 {
		t.Fatalf("findings %v err %v", got, err)
	}
	_, err = ParseResults(raw, 0, ownerID("deny"))
	requireCode(t, err, CodeOutput)
}

func TestParseResultsKeepsWhatEachExitStatusMeans(t *testing.T) {
	deny := ownerID("deny")
	cases := []struct {
		name string
		raw  []byte
		exit int
		code string
	}{
		{"status 2", output(t, "pass"), 2, CodeRun},
		{"killed", nil, 137, CodeRun},
		{"a load error", nil, 1, CodeRun},
		{"nothing at all and exit 0", nil, 0, CodeOutput},
		{"no result and exit 1", output(t, "pass"), 1, CodeOutput},
		{"a deny result and exit 0", output(t, "deny"), 0, CodeOutput},
		{"a deny result with status 2", output(t, "deny"), 2, CodeRun},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseResults(tc.raw, tc.exit, deny)
			requireCode(t, err, tc.code)
		})
	}
	_, err := ParseResults(output(t, "warn"), 1, ownerID("warn"))
	requireCode(t, err, CodeOutput)
	_, err = ParseResults(output(t, "dryrun"), 1, ownerID("dryrun"))
	requireCode(t, err, CodeOutput)
}

func TestParseResultsRefusesOutputItDoesNotKnow(t *testing.T) {
	good := string(output(t, "deny"))
	cases := map[string]string{
		"a banner":          "INFO starting\n" + good,
		"trailing text":     good + "\ndone",
		"two arrays":        good + good,
		"an escape byte":    "\x1b[0m" + good,
		"an empty array":    "[]",
		"a bare object":     "{}",
		"a string":          "\"null\"",
		"null and text":     "null\nnull",
		"truncated":         good[:len(good)/2],
		"results of a null": "[null]",
		"a number":          "1",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseResults([]byte(raw), 1, ownerID("deny"))
			requireCode(t, err, CodeOutput)
		})
	}
}

func TestParseResultsAcceptsTrailingWhitespace(t *testing.T) {
	if got, err := ParseResults([]byte("null\n"), 0, ownerID("deny")); err != nil || got != nil {
		t.Fatalf("null: %v %v", got, err)
	}
	if got, err := ParseResults([]byte(string(output(t, "deny"))+"\n\n"), 1, ownerID("deny")); err != nil || len(got) != 1 {
		t.Fatalf("array: %v %v", got, err)
	}
}

func TestParseResultsRefusesAResultThatIsNotThisConstraint(t *testing.T) {
	cases := map[string]func(item map[string]any){
		"another target": func(i map[string]any) { i["target"] = "mutation.gatekeeper.sh" },
		"no target":      func(i map[string]any) { delete(i, "target") },
		"another constraint": func(i map[string]any) {
			i["constraint"].(map[string]any)["metadata"].(map[string]any)["name"] = "other"
		},
		"another kind": func(i map[string]any) { i["constraint"].(map[string]any)["kind"] = "Other" },
		"another version": func(i map[string]any) {
			i["constraint"].(map[string]any)["apiVersion"] = "constraints.gatekeeper.sh/v1alpha1"
		},
		"no constraint":        func(i map[string]any) { delete(i, "constraint") },
		"another action":       func(i map[string]any) { i["enforcementAction"] = "warn" },
		"no action":            func(i map[string]any) { delete(i, "enforcementAction") },
		"scoped actions":       func(i map[string]any) { i["scopedActions"] = []string{"deny"} },
		"empty scoped actions": func(i map[string]any) { i["scopedActions"] = nil },
		"a trace":              func(i map[string]any) { i["trace"] = "Trace: ..." },
		"no object":            func(i map[string]any) { delete(i, "violatingObject") },
		"an object with no kind": func(i map[string]any) {
			delete(i["violatingObject"].(map[string]any), "kind")
		},
		"an object with no apiVersion": func(i map[string]any) {
			delete(i["violatingObject"].(map[string]any), "apiVersion")
		},
		"a gatekeeper object that ipso did not supply": func(i map[string]any) {
			i["violatingObject"] = map[string]any{
				"apiVersion": "constraints.gatekeeper.sh/v1beta1",
				"kind":       "RequireOwner",
				"metadata":   map[string]any{"name": "planted"},
			}
		},
		"a template that ipso did not supply": func(i map[string]any) {
			i["violatingObject"] = map[string]any{
				"apiVersion": "templates.gatekeeper.sh/v1",
				"kind":       "ConstraintTemplate",
				"metadata":   map[string]any{"name": "planted"},
			}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseResults(mutate(t, "deny", edit), 1, ownerID("deny"))
			requireCode(t, err, CodeOutput)
		})
	}
}

func TestParseResultsAcceptsANullTraceAndNoScopedActions(t *testing.T) {
	raw := mutate(t, "deny", func(i map[string]any) { i["trace"] = nil })
	if got, err := ParseResults(raw, 1, ownerID("deny")); err != nil || len(got) != 1 {
		t.Fatalf("findings %v err %v", got, err)
	}
}

func TestParseResultsFallsBackWhenTheMessageOrNameIsMissing(t *testing.T) {
	raw := mutate(t, "deny", func(i map[string]any) {
		delete(i, "msg")
		delete(i["violatingObject"].(map[string]any)["metadata"].(map[string]any), "name")
	})
	got, err := ParseResults(raw, 1, ownerID("deny"))
	if err != nil || len(got) != 1 {
		t.Fatalf("findings %v err %v", got, err)
	}
	if got[0].Object != "Deployment/(unnamed)" || got[0].Message != "constraint violated" {
		t.Fatalf("finding %+v", got[0])
	}
}

func TestFindingEscapesControlCharacters(t *testing.T) {
	raw := mutate(t, "deny", func(i map[string]any) {
		i["msg"] = "bad\nINJECTED: pass\x1b[31m"
		i["violatingObject"].(map[string]any)["metadata"].(map[string]any)["name"] = "web\nfake"
	})
	got, err := ParseResults(raw, 1, ownerID("deny"))
	if err != nil || len(got) != 1 {
		t.Fatalf("findings %v err %v", got, err)
	}
	violation := got[0].violation()
	if strings.ContainsAny(violation.Address+violation.Message, "\n\x1b") {
		t.Fatalf("violation %+v", violation)
	}
	if !strings.Contains(violation.Message, `\u000a`) || !strings.Contains(violation.Address, `\u000a`) {
		t.Fatalf("violation %+v", violation)
	}
}

func TestParseResultsErrorsCarryTheReasonCode(t *testing.T) {
	_, err := ParseResults(output(t, "deny"), 0, ownerID("deny"))
	if compiler.CodeOf(err) != CodeOutput || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("err %v", err)
	}
}
