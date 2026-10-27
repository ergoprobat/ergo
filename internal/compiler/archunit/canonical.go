package archunit

import (
	"bytes"
	"errors"
	"regexp"
	"strings"

	"github.com/ergoprobat/ergo/internal/compiler"
)

// Canonical is what evaluation needs to know about a generated ArchitectureTest.java.
type Canonical struct {
	// Class is the fully qualified name of the test class.
	Class string
	// Packages are the packages the test imports.
	Packages []string
	// Rules are the rule names, in file order.
	Rules []string
	// Layers is the number of layers of each layers rule. A forbidden rule has no entry.
	Layers map[string]int
}

var (
	importLine      = regexp.MustCompile(`^\s+\.importPackages\((.*)\);$`)
	methodLine      = regexp.MustCompile(`^    void rule_[a-z0-9_]+\(\) \{$`)
	sourceLine      = regexp.MustCompile(`^\s+\.that\(\)\.resideInAnyPackage\((.*)\)$`)
	forbiddenLine   = regexp.MustCompile(`^\s+\.should\(\)\.dependOnClassesThat\(\)\.resideInAnyPackage\((.*)\)$`)
	layerLine       = regexp.MustCompile(`^\s+\.layer\("([^"]*)"\)\.definedBy\((.*)\)$`)
	asLine          = regexp.MustCompile(`^\s+\.as\("([^"]*)"\)$`)
	quotedString    = regexp.MustCompile(`"([^"]*)"`)
	errNotCanonical = errors.New("ArchitectureTest.java is not the test ergo compiles")
)

// ReadCanonical checks that src is exactly what compile writes for this decision and check index.
// It recovers the packages and rules from the generator's fixed layout, validates them with the
// rule-file schema, regenerates the file, and requires the same bytes. A file that carries
// anything else, such as a repository's own Java, is a build-manifest error and is never compiled.
func ReadCanonical(decisionID string, index int, src []byte) (Canonical, error) {
	if !ulidPattern.MatchString(decisionID) || index < 0 {
		return Canonical{}, notCanonical()
	}
	doc, ok := recoverRules(string(src))
	if !ok {
		return Canonical{}, notCanonical()
	}
	if err := validateSchema(doc); err != nil {
		return Canonical{}, notCanonical()
	}
	packages, rules, err := rulesFrom(doc)
	if err != nil {
		return Canonical{}, notCanonical()
	}
	regenerated, err := generate(decisionID, index, packages, rules)
	if err != nil || !bytes.Equal(regenerated, src) {
		return Canonical{}, notCanonical()
	}
	names := make([]string, len(rules))
	layers := map[string]int{}
	for i, rule := range rules {
		names[i] = rule.name
		if rule.kind == "layers" {
			layers[rule.name] = len(rule.layers)
		}
	}
	return Canonical{Class: packageName(decisionID, index) + ".ArchitectureTest", Packages: packages, Rules: names, Layers: layers}, nil
}

func notCanonical() error {
	return &compiler.Error{Code: compiler.CodeBuildManifest, Err: errNotCanonical}
}

// recoverRules reads the rule file back out of generated Java. It is lenient on purpose:
// the byte comparison in ReadCanonical rejects anything the generator would not write.
func recoverRules(src string) (map[string]any, bool) {
	var packages []any
	imports := 0
	var rules []any
	var current map[string]any
	for _, line := range strings.Split(src, "\n") {
		if match := importLine.FindStringSubmatch(line); match != nil {
			packages = quotedList(match[1])
			imports++
			continue
		}
		if methodLine.MatchString(line) {
			current = map[string]any{}
			rules = append(rules, current)
			continue
		}
		if current != nil {
			readRuleLine(current, line)
		}
	}
	if imports != 1 {
		return nil, false
	}
	return map[string]any{"packages": packages, "rules": rules}, true
}

func readRuleLine(rule map[string]any, line string) {
	if match := layerLine.FindStringSubmatch(line); match != nil {
		layers, _ := rule["layers"].([]any)
		rule["layers"] = append(layers, map[string]any{"name": match[1], "packages": quotedList(match[2])})
		return
	}
	if match := sourceLine.FindStringSubmatch(line); match != nil {
		rule["source_packages"] = quotedList(match[1])
		return
	}
	if match := forbiddenLine.FindStringSubmatch(line); match != nil {
		rule["forbidden_packages"] = quotedList(match[1])
		return
	}
	if match := asLine.FindStringSubmatch(line); match != nil {
		rule["name"] = match[1]
		return
	}
	switch strings.TrimSpace(line) {
	case "noClasses()":
		rule["type"] = "forbidden"
	case "layeredArchitecture()":
		rule["type"] = "layers"
	}
}

func quotedList(list string) []any {
	matches := quotedString.FindAllStringSubmatch(list, -1)
	out := make([]any, len(matches))
	for i, match := range matches {
		out[i] = match[1]
	}
	return out
}
