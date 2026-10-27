package semgrep

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/strictyaml"
)

// allowedRuleKeys are the Semgrep rule keys compile keeps.
// Pattern syntax inside those keys is left to Semgrep.
// fix and fix-regex rewrite the tree. min-version and max-version can turn a
// rule off under the pinned engine. mode join reads other rule files, and mode
// extract splits a file into another language. Any other key is rejected too,
// including validators, which call external services.
var allowedRuleKeys = map[string]struct{}{
	"id":                  {},
	"languages":           {},
	"message":             {},
	"metadata":            {},
	"mode":                {},
	"options":             {},
	"paths":               {},
	"pattern":             {},
	"pattern-either":      {},
	"pattern-propagators": {},
	"pattern-regex":       {},
	"pattern-sanitizers":  {},
	"pattern-sinks":       {},
	"pattern-sources":     {},
	"patterns":            {},
	"severity":            {},
}

// allowedOptionKeys are the Semgrep rule options compile keeps.
// The names are the fields of Rule_options.atd at Semgrep 1.182.0, except
// interfile (Pro analysis, while evaluation runs the OSS engine) and the
// deprecated taint_match_on and commutative_compop. Values are left to Semgrep.
// A value Semgrep rejects fails the scan, and that fails the check.
var allowedOptionKeys = map[string]struct{}{
	"ac_matching":                              {},
	"arrow_is_function":                        {},
	"attr_expr":                                {},
	"commutative_boolop":                       {},
	"constant_propagation":                     {},
	"cpp_parsing_pref":                         {},
	"decorators_order_matters":                 {},
	"flddef_assign":                            {},
	"generic_braces":                           {},
	"generic_caseless":                         {},
	"generic_comment_style":                    {},
	"generic_ellipsis_max_span":                {},
	"generic_engine":                           {},
	"generic_extra_braces":                     {},
	"generic_extra_word_characters":            {},
	"generic_multiline":                        {},
	"go_deeper_expr":                           {},
	"go_deeper_stmt":                           {},
	"implicit_deep_exprstmt":                   {},
	"implicit_ellipsis":                        {},
	"implicit_return":                          {},
	"let_is_var":                               {},
	"strict_mvar_name_binding":                 {},
	"symbolic_propagation":                     {},
	"symmetric_eq":                             {},
	"taint_assume_safe_booleans":               {},
	"taint_assume_safe_comparisons":            {},
	"taint_assume_safe_functions":              {},
	"taint_assume_safe_indexes":                {},
	"taint_assume_safe_numbers":                {},
	"taint_focus_on":                           {},
	"taint_interproc_lambdas":                  {},
	"taint_only_propagate_through_assignments": {},
	"taint_unify_mvars":                        {},
	"unify_ids_strictly":                       {},
	"vardef_assign":                            {},
	"xml_attrs_implicit_ellipsis":              {},
	"xml_children_ordered":                     {},
	"xml_singleton_loose_matching":             {},
}

var rejectedRuleKeys = map[string]string{
	"fix":         "autofix",
	"fix-regex":   "autofix",
	"max-version": "version gate",
	"min-version": "version gate",
}

var searchOperators = []string{"pattern", "pattern-either", "pattern-regex", "patterns"}

var taintLists = []string{"pattern-propagators", "pattern-sanitizers", "pattern-sinks", "pattern-sources"}

var ruleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var patternStringKeys = map[string]struct{}{
	"pattern":            {},
	"pattern-inside":     {},
	"pattern-not":        {},
	"pattern-not-inside": {},
	"pattern-not-regex":  {},
	"pattern-regex":      {},
}

func compileConfig(src []byte, mode string) ([]byte, error) {
	severity, err := severityFor(mode)
	if err != nil {
		return nil, err
	}
	docs, err := strictyaml.Documents(src, CodeYAML, 1)
	if err != nil {
		return nil, err
	}
	root := docs[0]
	rules, err := ruleList(root)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, rule := range rules.Content {
		if err := checkRule(rule, seen); err != nil {
			return nil, err
		}
		setSeverity(rule, severity)
	}
	out, err := strictyaml.Canonical(root)
	if err != nil {
		return nil, fmt.Errorf("encode semgrep rules: %w", err)
	}
	return out, nil
}

func ruleList(root *yaml.Node) (*yaml.Node, error) {
	if root.Kind != yaml.MappingNode {
		return nil, &compiler.Error{Code: CodeRules, Err: errors.New("root is not a mapping")}
	}
	fields := fieldMap(root)
	rules, ok := fields["rules"]
	if !ok {
		return nil, &compiler.Error{Code: CodeRules, Err: errors.New("missing rules")}
	}
	if len(fields) != 1 {
		return nil, &compiler.Error{Code: CodeRules, Err: errors.New("unexpected root key")}
	}
	if rules.Kind != yaml.SequenceNode || len(rules.Content) == 0 {
		return nil, &compiler.Error{Code: CodeRules, Err: errors.New("rules is empty")}
	}
	return rules, nil
}

func checkRule(rule *yaml.Node, seen map[string]struct{}) error {
	if rule.Kind != yaml.MappingNode {
		return &compiler.Error{Code: CodeRule, Err: errors.New("rule is not a mapping")}
	}
	fields := fieldMap(rule)
	id := ""
	if node, ok := fields["id"]; ok && isString(node) {
		id = node.Value
	}
	if err := checkKeys(fields, id); err != nil {
		return err
	}
	if err := checkID(fields, id, seen); err != nil {
		return err
	}
	if err := checkMessage(fields, id); err != nil {
		return err
	}
	if err := checkLanguages(fields, id); err != nil {
		return err
	}
	mode, err := checkMode(fields, id)
	if err != nil {
		return err
	}
	if err := checkOperators(fields, id, mode); err != nil {
		return err
	}
	if err := checkPaths(fields, id); err != nil {
		return err
	}
	if err := checkMappingField(fields, "metadata", id); err != nil {
		return err
	}
	if err := checkOptions(fields, id); err != nil {
		return err
	}
	if node, ok := fields["severity"]; ok && !isString(node) {
		return ruleErr(id, errors.New("severity is not a string"))
	}
	return nil
}

func checkKeys(fields map[string]*yaml.Node, id string) error {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowedRuleKeys[key]; ok {
			continue
		}
		if why, ok := rejectedRuleKeys[key]; ok {
			return keyErr(id, fmt.Errorf("%s %q", why, key))
		}
		return keyErr(id, fmt.Errorf("unknown key %q", key))
	}
	return nil
}

func checkID(fields map[string]*yaml.Node, id string, seen map[string]struct{}) error {
	node, ok := fields["id"]
	if !ok || !isString(node) || id == "" {
		return &compiler.Error{Code: CodeRuleID, Err: errors.New("rule has no id")}
	}
	if !ruleIDPattern.MatchString(id) {
		return &compiler.Error{Code: CodeRuleID, Err: fmt.Errorf("rule id %q", id)}
	}
	if _, dup := seen[id]; dup {
		return &compiler.Error{Code: CodeRuleID, Err: fmt.Errorf("duplicate rule id %q", id)}
	}
	seen[id] = struct{}{}
	return nil
}

func checkMessage(fields map[string]*yaml.Node, id string) error {
	node, ok := fields["message"]
	if !ok || !isString(node) || strings.TrimSpace(node.Value) == "" {
		return ruleErr(id, errors.New("message is empty"))
	}
	return nil
}

func checkLanguages(fields map[string]*yaml.Node, id string) error {
	node, ok := fields["languages"]
	if !ok {
		return ruleErr(id, errors.New("languages is empty"))
	}
	if err := stringItems(node, false); err != nil {
		return ruleErr(id, fmt.Errorf("languages: %w", err))
	}
	return nil
}

func checkMode(fields map[string]*yaml.Node, id string) (string, error) {
	node, ok := fields["mode"]
	if !ok {
		return "search", nil
	}
	if !isString(node) {
		return "", ruleErr(id, errors.New("mode is not a string"))
	}
	switch node.Value {
	case "search", "taint":
		return node.Value, nil
	default:
		return "", keyErr(id, fmt.Errorf("mode %q", node.Value))
	}
}

func checkOperators(fields map[string]*yaml.Node, id, mode string) error {
	var present []string
	for _, name := range searchOperators {
		node, ok := fields[name]
		if !ok {
			continue
		}
		if err := operatorShape(name, node); err != nil {
			return ruleErr(id, err)
		}
		present = append(present, name)
	}
	if mode == "taint" {
		if len(present) != 0 {
			return ruleErr(id, fmt.Errorf("taint rule also sets %s", strings.Join(present, ", ")))
		}
		for _, name := range []string{"pattern-sinks", "pattern-sources"} {
			if err := sequenceField(fields, name); err != nil {
				return ruleErr(id, err)
			}
			if err := checkPatternTree(fields[name]); err != nil {
				return ruleErr(id, err)
			}
		}
		for _, name := range []string{"pattern-propagators", "pattern-sanitizers"} {
			node, ok := fields[name]
			if !ok {
				continue
			}
			if node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
				return ruleErr(id, fmt.Errorf("%s is not a list", name))
			}
			if err := checkPatternTree(node); err != nil {
				return ruleErr(id, err)
			}
		}
		return nil
	}
	for _, name := range taintLists {
		if _, ok := fields[name]; ok {
			return ruleErr(id, fmt.Errorf("search rule sets %s", name))
		}
	}
	if len(present) != 1 {
		return ruleErr(id, fmt.Errorf("%d search operators", len(present)))
	}
	return nil
}

func operatorShape(name string, node *yaml.Node) error {
	switch name {
	case "pattern", "pattern-regex":
		if !isString(node) || strings.TrimSpace(node.Value) == "" {
			return fmt.Errorf("%s is not a string", name)
		}
	case "pattern-either", "patterns":
		if node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
			return fmt.Errorf("%s is not a list", name)
		}
		for _, item := range node.Content {
			if item.Kind != yaml.MappingNode || len(item.Content) == 0 {
				return fmt.Errorf("%s item is empty", name)
			}
		}
	}
	return checkPatternTree(node)
}

func checkPatternTree(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1]
			if _, ok := patternStringKeys[key]; ok {
				if !isString(value) || strings.TrimSpace(value.Value) == "" {
					return fmt.Errorf("%s is not a string", key)
				}
			}
			if key == "pattern-either" || key == "patterns" {
				if value.Kind != yaml.SequenceNode || len(value.Content) == 0 {
					return fmt.Errorf("%s is not a list", key)
				}
				for _, item := range value.Content {
					if item.Kind != yaml.MappingNode || len(item.Content) == 0 {
						return fmt.Errorf("%s item is empty", key)
					}
				}
			}
			if err := checkPatternTree(value); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if err := checkPatternTree(item); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkPaths(fields map[string]*yaml.Node, id string) error {
	node, ok := fields["paths"]
	if !ok {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return ruleErr(id, errors.New("paths is not a mapping"))
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if key != "include" && key != "exclude" {
			return keyErr(id, fmt.Errorf("unknown paths key %q", key))
		}
		if err := stringItems(node.Content[i+1], true); err != nil {
			return ruleErr(id, fmt.Errorf("paths.%s: %w", key, err))
		}
	}
	return nil
}

func checkOptions(fields map[string]*yaml.Node, id string) error {
	node, ok := fields["options"]
	if !ok {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return ruleErr(id, errors.New("options is not a mapping"))
	}
	keys := make([]string, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, ok := allowedOptionKeys[key]; ok {
			continue
		}
		return keyErr(id, fmt.Errorf("options.%s", key))
	}
	return nil
}

func checkMappingField(fields map[string]*yaml.Node, name, id string) error {
	node, ok := fields[name]
	if !ok {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return ruleErr(id, fmt.Errorf("%s is not a mapping", name))
	}
	return nil
}

func sequenceField(fields map[string]*yaml.Node, name string) error {
	node, ok := fields[name]
	if !ok || node.Kind != yaml.SequenceNode || len(node.Content) == 0 {
		return fmt.Errorf("%s is not a list", name)
	}
	return nil
}

func stringItems(node *yaml.Node, allowEmpty bool) error {
	if node.Kind != yaml.SequenceNode || (!allowEmpty && len(node.Content) == 0) {
		return errors.New("expected a list of strings")
	}
	for _, item := range node.Content {
		if !isString(item) || item.Value == "" {
			return errors.New("expected a list of strings")
		}
	}
	return nil
}

func fieldMap(node *yaml.Node) map[string]*yaml.Node {
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		fields[node.Content[i].Value] = node.Content[i+1]
	}
	return fields
}

func isString(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.ShortTag() == "!!str"
}

func ruleErr(id string, err error) error {
	if id == "" {
		return &compiler.Error{Code: CodeRule, Err: err}
	}
	return &compiler.Error{Code: CodeRule, Err: fmt.Errorf("rule %q: %w", id, err)}
}

func keyErr(id string, err error) error {
	if id == "" {
		return &compiler.Error{Code: CodeKey, Err: err}
	}
	return &compiler.Error{Code: CodeKey, Err: fmt.Errorf("rule %q: %w", id, err)}
}
