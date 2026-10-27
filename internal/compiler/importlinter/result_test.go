package importlinter

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// The files under testdata/output are the standard output of the shim on import-linter 2.15 with grimp 3.18.

func readOutput(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "output", name+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func canonOf(contracts ...Contract) Canonical {
	return Canonical{Roots: []string{"shop"}, Contracts: contracts}
}

var (
	forbidden    = Contract{ID: "forbidden", Name: "Forbidden", Type: "forbidden"}
	layers       = Contract{ID: "layers", Name: "Layers", Type: "layers"}
	independence = Contract{ID: "independence", Name: "Independence", Type: "independence"}
	protected    = Contract{ID: "protected", Name: "Protected", Type: "protected"}
	siblings     = Contract{ID: "siblings", Name: "Siblings", Type: "acyclic_siblings"}
	everyType    = canonOf(forbidden, layers, independence, protected, siblings)
)

func countsByContract(findings []Finding) map[string]int {
	counts := map[string]int{}
	for _, item := range findings {
		counts[item.Contract]++
	}
	return counts
}

func requireOutputCode(t *testing.T, raw string, exit int, canon Canonical, code string) error {
	t.Helper()
	findings, err := ParseOutput([]byte(raw), exit, canon)
	var coded *compiler.Error
	if !errors.As(err, &coded) || coded.Code != code || len(findings) != 0 {
		t.Fatalf("exit %d: findings %v err %v, want %s for\n%s", exit, findings, err, code, raw)
	}
	return err
}

func TestParseOutputKept(t *testing.T) {
	for name, canon := range map[string]Canonical{
		"clean":   canonOf(forbidden, layers),
		"ignored": canonOf(forbidden),
		"emoji":   canonOf(Contract{ID: "forbidden", Name: "Keep :warning: and :brick: out", Type: "forbidden"}),
	} {
		findings, err := ParseOutput([]byte(readOutput(t, name)), 0, canon)
		if err != nil || len(findings) != 0 {
			t.Fatalf("%s: findings %v err %v", name, findings, err)
		}
	}
}

func TestParseOutputCountsOneViolationPerRenderedUnit(t *testing.T) {
	cases := []struct {
		name  string
		canon Canonical
		want  map[string]int
	}{
		// A forbidden chain that goes through another module is still one violation.
		{"violating", everyType, map[string]int{"forbidden": 2, "layers": 1, "independence": 2, "protected": 1, "siblings": 1}},
		// Five modules that are not layers count one each, next to the one layer violation.
		{"exhaustive", canonOf(layers), map[string]int{"layers": 6}},
		// The dependencies listed under a cycle are the way out of it and are not counted.
		{"acyclic", canonOf(siblings), map[string]int{"siblings": 1}},
		{"layers-multi", canonOf(layers, independence, protected), map[string]int{"layers": 6, "independence": 5, "protected": 4}},
		{"external", canonOf(forbidden), map[string]int{"forbidden": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings, err := ParseOutput([]byte(readOutput(t, tc.name)), 1, tc.canon)
			if err != nil {
				t.Fatal(err)
			}
			got := countsByContract(findings)
			for id, want := range tc.want {
				if got[id] != want {
					t.Fatalf("counts %v, want %v", got, tc.want)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("counts %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseOutputMessages(t *testing.T) {
	findings, err := ParseOutput([]byte(readOutput(t, "violating")), 1, everyType)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, item := range findings {
		got = append(got, item.message())
	}
	want := []string{
		"forbidden shop.domain.models -> shop.infra.db (l.1)",
		"forbidden shop.domain.models -> shop.services.helper (l.2)",
		"layers shop.domain.models -> shop.api.views (l.3)",
		"independence shop.shipping.label -> shop.billing.core (l.1)",
		"independence shop.billing.core -> shop.shipping.label (l.1)",
		"protected shop.infra.db -> shop.domain.models (l.1)",
		"siblings No cycles are allowed in shop.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("messages\n%s", strings.Join(got, "\n"))
	}
}

func TestFindingMessageEscapesControlCharacters(t *testing.T) {
	got := Finding{Contract: "forbidden", Line: "a\x07b\u0085c\n"}.message()
	if strings.ContainsAny(got, "\x07\u0085\n") {
		t.Fatalf("message %q", got)
	}
}

// Rich prints a name that ends in backslashes differently in the heading of a broken contract:
// Ends[/bold], Ends\, and Ends\[/bold] for one, two, and three of them.
func TestParseOutputNameEndingInBackslashes(t *testing.T) {
	for n := 1; n <= 3; n++ {
		name := "Ends" + strings.Repeat(`\`, n)
		canon := canonOf(Contract{ID: "forbidden", Name: name, Type: "forbidden"})
		findings, err := ParseOutput([]byte(readOutput(t, "backslash-"+string(rune('0'+n)))), 1, canon)
		if err != nil || len(findings) != 2 {
			t.Fatalf("%d backslashes: findings %v err %v", n, findings, err)
		}
	}
	// Another name is still refused, with or without the closing tag.
	for _, name := range []string{`Other\`, `Ends`, `Endsx\`} {
		canon := canonOf(Contract{ID: "forbidden", Name: name, Type: "forbidden"})
		if _, err := ParseOutput([]byte(readOutput(t, "backslash-1")), 1, canon); err == nil {
			t.Fatalf("name %q accepted", name)
		}
	}
}

func TestHeadingIsIsExactForNamesWithoutTrailingBackslashes(t *testing.T) {
	for line, want := range map[string]bool{"Layers": true, "Layers[/bold]": false, `Layers\`: false, "layers": false, "Layers ": false} {
		if got := headingIs(line, "Layers"); got != want {
			t.Fatalf("%q: %v", line, got)
		}
	}
}

func TestParseOutputEngineErrorsAreRunErrors(t *testing.T) {
	for name, want := range map[string]string{
		"error-module":    "Module 'shop.nope' does not exist.",
		"error-unmatched": "No matches for ignored import shop.util.tools -> shop.infra.db.",
		"error-container": "Invalid container 'elsewhere'",
	} {
		err := requireOutputCode(t, readOutput(t, name), 1, canonOf(forbidden), CodeRun)
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestParseOutputFailsClosed(t *testing.T) {
	violating := readOutput(t, "violating")
	clean := readOutput(t, "clean")
	two := canonOf(forbidden, layers)
	cases := []struct {
		name  string
		raw   string
		exit  int
		canon Canonical
		code  string
	}{
		{"terminal escape", strings.Replace(clean, "KEPT", "\x1b[32mKEPT\x1b[0m", 1), 0, two, CodeOutput},
		{"empty output", "", 0, two, CodeOutput},
		{"empty output and a failure", "", 1, two, CodeRun},
		{"exit status 2", clean, 2, two, CodeRun},
		{"broken contracts with exit 0", violating, 0, everyType, CodeOutput},
		{"kept contracts with exit 1", clean, 1, two, CodeOutput},
		{"warning suffix", strings.Replace(clean, "Forbidden KEPT", "Forbidden KEPT (1 warning)", 1), 0, two, CodeOutput},
		{"warning with ignored imports", strings.Replace(readOutput(t, "ignored"), "(2 ignored imports)", "(2 ignored imports, 1 warning)", 1), 0, canonOf(forbidden), CodeOutput},
		{"summary says another count", strings.Replace(clean, "Contracts: 2 kept, 0 broken.", "Contracts: 1 kept, 1 broken.", 1), 0, two, CodeOutput},
		{"another contract name", clean, 0, canonOf(forbidden, Contract{ID: "layers", Name: "Other", Type: "layers"}), CodeOutput},
		{"a contract is missing from the report", clean, 0, canonOf(forbidden, layers, independence), CodeOutput},
		{"a contract the build does not have", clean, 0, canonOf(forbidden), CodeOutput},
		{"lines after a clean summary", clean + "\nWarnings\n--------\n", 0, two, CodeOutput},
		{"a line before the report", "Warning: skipping module with too many dots in the name: x\n" + clean, 0, two, CodeOutput},
		{"missing file count", strings.Replace(clean, "Analyzed 16 files, 10 dependencies.", "Analyzed some files.", 1), 0, two, CodeOutput},
		{"file count underline", strings.Replace(clean, strings.Repeat("-", 35), strings.Repeat("-", 34), 1), 0, two, CodeOutput},
		{"no files analysed", strings.Replace(strings.Replace(clean, "Analyzed 16 files, 10 dependencies.", "Analyzed 0 files, 0 dependencies.", 1), strings.Repeat("-", 35), strings.Repeat("-", 33), 1), 0, two, CodeEmpty},
		{"no broken contracts heading", violating[:strings.Index(violating, "----------------\nBroken contracts")], 1, everyType, CodeOutput},
		{"a broken section is missing", strings.Replace(violating, "Protected\n---------\n", "", 1), 1, everyType, CodeOutput},
		{"a section underline is short", strings.Replace(violating, "Independence\n------------", "Independence\n-----------", 1), 1, everyType, CodeOutput},
		{"a broken contract lists nothing", strings.Replace(violating, "- shop.domain.models -> shop.api.views (l.3)\n", "", 1), 1, everyType, CodeOutput},
		{"a broken forbidden contract lists nothing", dropLines(violating, "-   "), 1, everyType, CodeOutput},
		{"a broken acyclic contract lists nothing", strings.Replace(violating, "No cycles are allowed in shop.", "Cycles", 1), 1, everyType, CodeOutput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { _ = requireOutputCode(t, tc.raw, tc.exit, tc.canon, tc.code) })
	}
}

func dropLines(raw, prefix string) string {
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		if !strings.HasPrefix(line, prefix) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

func TestParseOutputToleratesBlankLinesAndWindowsLineEnds(t *testing.T) {
	violating := readOutput(t, "violating")
	want, err := ParseOutput([]byte(violating), 1, everyType)
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range map[string]string{
		"crlf":            strings.ReplaceAll(violating, "\n", "\r\n"),
		"leading blanks":  "\n\n" + strings.TrimLeft(violating, "\n"),
		"trailing blanks": violating + "\n\n\n",
	} {
		got, err := ParseOutput([]byte(raw), 1, everyType)
		if err != nil || len(got) != len(want) {
			t.Fatalf("%s: %d findings, want %d: %v", name, len(got), len(want), err)
		}
	}
}
