package archunit

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ergoprobat/ergo/internal/compiler"
)

const billingClass = "ergo.d01m5wvyech306k6ddjxr2vapcs.c0.ArchitectureTest"

var billingTest = Canonical{
	Class:    billingClass,
	Packages: []string{"com.acme.billing"},
	Rules:    []string{"domain-not-to-adapters", "layers"},
}

func junitFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "junit", name+".xml"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// junitSuite builds a report from testcase elements.
func junitSuite(cases ...string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<testsuite name=\"JUnit Jupiter\">\n" + strings.Join(cases, "\n") + "\n</testsuite>\n")
}

func junitCase(method, inner string) string {
	return fmt.Sprintf(`<testcase name="%s()" classname="%s">%s</testcase>`, method, billingClass, inner)
}

func junitFailure(typ, message string) string {
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(message)); err != nil {
		panic(err)
	}
	return fmt.Sprintf(`<failure message="%s" type="%s"/>`, escaped.String(), typ)
}

const (
	passCase     = ""
	domainMethod = "rule_domain_not_to_adapters"
)

func violatedMessage(rule string, lines ...string) string {
	return fmt.Sprintf("Architecture Violation [Priority: MEDIUM] - Rule '%s' was violated (%d times):\n%s", rule, len(lines), strings.Join(lines, "\n"))
}

func TestParseReportPass(t *testing.T) {
	findings, err := ParseReport(junitFixture(t, "pass"), 0, billingTest)
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings %v err %v", findings, err)
	}
}

func TestParseReportViolations(t *testing.T) {
	findings, err := ParseReport(junitFixture(t, "violated"), 1, billingTest)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, item := range findings {
		counts[item.Rule]++
		if !strings.Contains(item.Line, "com.acme.billing.") {
			t.Fatalf("line %q", item.Line)
		}
	}
	if counts["domain-not-to-adapters"] != 2 || counts["layers"] != 3 || len(findings) != 5 {
		t.Fatalf("counts %v", counts)
	}
	if findings[0].Rule != "domain-not-to-adapters" {
		t.Fatalf("findings follow the order of the rules in the test, got %q first", findings[0].Rule)
	}
}

func TestParseReportReasonCodes(t *testing.T) {
	passed := junitCase("rule_layers", passCase)
	cases := map[string]struct {
		report []byte
		exit   int
		code   string
	}{
		"a rule that checked nothing": {junitFixture(t, "empty-tree"), 1, CodeEmpty},
		"a rule that raised":          {junitFixture(t, "error"), 1, CodeRun},
		"a skipped rule":              {junitSuite(passed, junitCase(domainMethod, "<skipped/>")), 0, CodeOutput},
		"a violation with exit 0":     {junitFixture(t, "violated"), 0, CodeOutput},
		"a pass with exit 1":          {junitFixture(t, "pass"), 1, CodeOutput},
		"a missing rule":              {junitSuite(passed), 0, CodeOutput},
		"a rule listed twice":         {junitSuite(passed, passed, junitCase(domainMethod, passCase)), 0, CodeOutput},
		"an extra test":               {junitSuite(passed, junitCase(domainMethod, passCase), junitCase("rule_other", passCase)), 0, CodeOutput},
		"another class":               {[]byte(strings.ReplaceAll(string(junitFixture(t, "pass")), billingClass, "ergo.dother.c0.ArchitectureTest")), 0, CodeOutput},
		"no report":                   {nil, 0, CodeOutput},
		"not xml":                     {[]byte("not xml"), 0, CodeOutput},
		"another root":                {[]byte(`<testsuites/>`), 0, CodeOutput},
		"an unknown entity":           {[]byte(`<!DOCTYPE s [<!ENTITY e SYSTEM "file:///etc/passwd">]><testsuite><testcase name="rule_layers()" classname="` + billingClass + `"><failure message="&e;" type="java.lang.AssertionError"/></testcase></testsuite>`), 1, CodeOutput},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseReport(c.report, c.exit, billingTest)
			if compiler.CodeOf(err) != c.code {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
		})
	}
}

func TestParseReportFailureShapes(t *testing.T) {
	ok := junitCase("rule_layers", passCase)
	oneLine := "Method <a.A.m()> calls method <b.B.n()> in (A.java:3)"
	cases := map[string]struct {
		failure string
		code    string
		count   int
	}{
		"one violation":       {junitFailure(assertionError, violatedMessage("domain-not-to-adapters", oneLine)), "", 1},
		"three violations":    {junitFailure(assertionError, violatedMessage("domain-not-to-adapters", oneLine, oneLine, oneLine)), "", 3},
		"windows line breaks": {junitFailure(assertionError, strings.ReplaceAll(violatedMessage("domain-not-to-adapters", oneLine, oneLine), "\n", "\r\n")), "", 2},
		"another rule's name": {junitFailure(assertionError, violatedMessage("layers", oneLine)), CodeOutput, 0},
		"count too high":      {junitFailure(assertionError, strings.Replace(violatedMessage("domain-not-to-adapters", oneLine), "(1 times)", "(2 times)", 1)), CodeOutput, 0},
		"count too low":       {junitFailure(assertionError, strings.Replace(violatedMessage("domain-not-to-adapters", oneLine, oneLine), "(2 times)", "(1 times)", 1)), CodeOutput, 0},
		"zero violations":     {junitFailure(assertionError, "Architecture Violation [Priority: MEDIUM] - Rule 'domain-not-to-adapters' was violated (0 times):\n"), CodeOutput, 0},
		"no lines":            {junitFailure(assertionError, "Architecture Violation [Priority: MEDIUM] - Rule 'domain-not-to-adapters' was violated (1 times):"), CodeOutput, 0},
		"a blank line":        {junitFailure(assertionError, violatedMessage("domain-not-to-adapters", oneLine)+"\n"), CodeOutput, 0},
		"other text":          {junitFailure(assertionError, "boom"), CodeOutput, 0},
		"not an assertion":    {junitFailure("java.lang.IllegalStateException", violatedMessage("domain-not-to-adapters", oneLine)), CodeOutput, 0},
		"two failures":        {junitFailure(assertionError, violatedMessage("domain-not-to-adapters", oneLine)) + junitFailure(assertionError, violatedMessage("domain-not-to-adapters", oneLine)), CodeOutput, 0},
		"empty rule":          {junitFailure(assertionError, "Rule 'domain-not-to-adapters' failed to check any classes. This means either"), CodeEmpty, 0},
		"empty names another": {junitFailure(assertionError, "Rule 'layers' failed to check any classes. This means either"), CodeOutput, 0},
		"a name with a quote": {junitFailure(assertionError, "Architecture Violation [Priority: MEDIUM] - Rule 'a' - Rule 'domain-not-to-adapters' was violated (1 times):\nx"), CodeOutput, 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			report := junitSuite(ok, junitCase(domainMethod, c.failure))
			exit := 1
			if c.code == "" && c.count == 0 {
				exit = 0
			}
			findings, err := ParseReport(report, exit, billingTest)
			if compiler.CodeOf(err) != c.code {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
			if len(findings) != c.count {
				t.Fatalf("findings %v, want %d", findings, c.count)
			}
		})
	}
}

var layersOnlyTest = Canonical{
	Class:    billingClass,
	Packages: []string{"com.acme.billing"},
	Rules:    []string{"layers"},
	Layers:   map[string]int{"layers": 3},
}

func layersFailure(lines ...string) string {
	return junitFailure(assertionError, violatedMessage("layers", lines...))
}

// ArchUnit reports a layer that holds no class as a violation. When every layer of a rule is empty
// the rule checked nothing, which is an error. A partly empty rule keeps its lines as violations.
func TestParseReportLayersThatAreAllEmpty(t *testing.T) {
	const web, domain, persistence = "Layer 'web' is empty", "Layer 'domain' is empty", "Layer 'persistence' is empty"
	const real = "Method <a.A.m()> calls method <b.B.n()> in (A.java:3)"
	cases := map[string]struct {
		lines []string
		code  string
		count int
	}{
		"every layer is empty":           {[]string{domain, persistence, web}, CodeEmpty, 0},
		"two of three layers are empty":  {[]string{domain, web}, "", 2},
		"an empty layer and a violation": {[]string{domain, persistence, real}, "", 3},
		"one layer is empty":             {[]string{persistence}, "", 1},
		"empty lines of another shape":   {[]string{"Layer 'x' is empty.", domain, web}, "", 3},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			report := junitSuite(junitCase("rule_layers", layersFailure(c.lines...)))
			findings, err := ParseReport(report, 1, layersOnlyTest)
			if compiler.CodeOf(err) != c.code {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
			if len(findings) != c.count {
				t.Fatalf("findings %v, want %d", findings, c.count)
			}
		})
	}
}

// A forbidden rule has no layers, so a line that looks like an empty layer is only a violation line.
func TestParseReportEmptyLayerLineOfAForbiddenRule(t *testing.T) {
	test := Canonical{Class: billingClass, Rules: []string{"domain-not-to-adapters"}}
	report := junitSuite(junitCase(domainMethod, junitFailure(assertionError, violatedMessage("domain-not-to-adapters", "Layer 'web' is empty"))))
	findings, err := ParseReport(report, 1, test)
	if err != nil || len(findings) != 1 {
		t.Fatalf("findings %v err %v", findings, err)
	}
}

// An error names the rule that caused it. When one rule has violations and another checked nothing,
// the check is an error and the violations are not reported, because part of the check did not run.
func TestParseReportErrorsNameTheRule(t *testing.T) {
	violatedAndEmpty := junitSuite(
		junitCase("rule_layers", layersFailure("Layer 'domain' is empty", "Layer 'persistence' is empty", "Layer 'web' is empty")),
		junitCase(domainMethod, junitFailure(assertionError, violatedMessage("domain-not-to-adapters", "Method <a.A.m()> calls method <b.B.n()> in (A.java:3)"))),
	)
	cases := map[string]struct {
		report []byte
		code   string
		rule   string
	}{
		"empty selection":             {junitFixture(t, "empty-tree"), CodeEmpty, "domain-not-to-adapters"},
		"exception":                   {junitFixture(t, "error"), CodeRun, "domain-not-to-adapters"},
		"skipped":                     {junitSuite(junitCase("rule_layers", passCase), junitCase(domainMethod, "<skipped/>")), CodeOutput, "domain-not-to-adapters"},
		"one violated, one all empty": {violatedAndEmpty, CodeEmpty, "layers"},
	}
	billing := billingTest
	billing.Layers = map[string]int{"layers": 3}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			findings, err := ParseReport(c.report, 1, billing)
			if compiler.CodeOf(err) != c.code || !strings.Contains(err.Error(), c.rule) {
				t.Fatalf("code %q err %v", compiler.CodeOf(err), err)
			}
			if len(findings) != 0 {
				t.Fatalf("an error carries no findings, got %v", findings)
			}
		})
	}
}

func TestFindingMessageEscapesControls(t *testing.T) {
	got := Finding{Rule: "layers", Line: "a\x1b[31m\nb"}.message()
	if strings.ContainsAny(got, "\x1b\n") {
		t.Fatalf("message %q", got)
	}
	if !strings.HasPrefix(got, "layers ") {
		t.Fatalf("message %q", got)
	}
}

func TestFindingsAreCoveredByAWaiver(t *testing.T) {
	entry := compiler.Entry{
		DecisionID:      "01M5WVYECH306K6DDJXR2VAPCS",
		CheckIndex:      0,
		CompilerVersion: version,
		Mode:            "block",
		Waivers:         []compiler.ManifestWaiver{{Approver: "Ada", ValidFrom: "2026-01-01", ValidTo: "2026-03-01"}},
	}
	findings, err := ParseReport(junitFixture(t, "violated"), 1, billingTest)
	if err != nil {
		t.Fatal(err)
	}
	var violations []compiler.Violation
	for _, item := range findings {
		violations = append(violations, compiler.Violation{Message: item.message()})
	}
	now := time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC)
	if out := compiler.Cover(entry, compiler.Outcome{Violations: violations}, now); !out.Waived || len(out.Violations) != 5 {
		t.Fatalf("outcome %+v", out)
	}
	if out := compiler.Cover(entry, compiler.Outcome{Violations: violations}, now.AddDate(0, 3, 0)); out.Waived {
		t.Fatalf("expired waiver covered: %+v", out)
	}
}
