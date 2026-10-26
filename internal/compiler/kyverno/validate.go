package kyverno

import (
	"regexp"

	"github.com/ergoprobat/ergo/internal/compiler/strictyaml"
	"gopkg.in/yaml.v3"
)

const (
	policyVersion = "policies.kyverno.io/v1"
	policyKind    = "ValidatingPolicy"
)

var dnsName = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

var rootKeys = map[string]struct{}{
	"apiVersion": {},
	"kind":       {},
	"metadata":   {},
	"spec":       {},
}

var metadataKeys = map[string]struct{}{
	"annotations": {},
	"labels":      {},
	"name":        {},
}

var specKeys = map[string]struct{}{
	"auditAnnotations":  {},
	"autogen":           {},
	"failurePolicy":     {},
	"matchConditions":   {},
	"matchConstraints":  {},
	"validationActions": {},
	"validations":       {},
	"variables":         {},
}

var selectorKeys = map[string]struct{}{
	"matchExpressions": {},
	"matchLabels":      {},
}

var requirementKeys = map[string]struct{}{
	"key":      {},
	"operator": {},
	"values":   {},
}

var ruleKeys = map[string]struct{}{
	"apiGroups":     {},
	"apiVersions":   {},
	"operations":    {},
	"resourceNames": {},
	"resources":     {},
	"scope":         {},
}

var constraintKeys = map[string]struct{}{
	"excludeResourceRules": {},
	"matchPolicy":          {},
	"namespaceSelector":    {},
	"objectSelector":       {},
	"resourceRules":        {},
}

var validationKeys = map[string]struct{}{
	"expression":        {},
	"message":           {},
	"messageExpression": {},
	"reason":            {},
}

var autogenKeys = map[string]struct{}{
	"podControllers":            {},
	"validatingAdmissionPolicy": {},
}

func compilePolicy(src []byte, mode string) ([]byte, error) {
	docs, err := strictyaml.Documents(src, CodeYAML, 1)
	if err != nil {
		return nil, err
	}
	root := docs[0]
	if err := checkDocument(root); err != nil {
		return nil, err
	}
	if err := setActions(child(root, "spec"), mode); err != nil {
		return nil, err
	}
	if err := checkCEL(root); err != nil {
		return nil, err
	}
	return strictyaml.Canonical(root)
}

func checkDocument(root *yaml.Node) error {
	fields, err := mapping(root, rootKeys)
	if err != nil {
		return err
	}
	version, err := requiredString(fields, "apiVersion")
	if err != nil {
		return err
	}
	kind, err := requiredString(fields, "kind")
	if err != nil {
		return err
	}
	if version != policyVersion || kind != policyKind {
		return fail(CodeKind, "kind %s %s", version, kind)
	}
	meta, ok := fields["metadata"]
	if !ok {
		return fail(CodePolicy, "metadata is required")
	}
	if err := checkMetadata(meta); err != nil {
		return err
	}
	spec, ok := fields["spec"]
	if !ok {
		return fail(CodePolicy, "spec is required")
	}
	return checkSpec(spec)
}

func checkMetadata(node *yaml.Node) error {
	fields, err := mapping(node, metadataKeys)
	if err != nil {
		return err
	}
	name, err := requiredString(fields, "name")
	if err != nil {
		return err
	}
	if len(name) > 253 || !dnsName.MatchString(name) {
		return fail(CodePolicy, "metadata.name %q", name)
	}
	for _, key := range []string{"labels", "annotations"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if err := stringMap(value); err != nil {
			return err
		}
	}
	return nil
}

func checkSpec(node *yaml.Node) error {
	fields, err := mapping(node, specKeys)
	if err != nil {
		return err
	}
	constraints, ok := fields["matchConstraints"]
	if !ok {
		return fail(CodePolicy, "matchConstraints is required")
	}
	if err := checkConstraints(constraints); err != nil {
		return err
	}
	validations, ok := fields["validations"]
	if !ok {
		return fail(CodePolicy, "validations is required")
	}
	if err := checkValidations(validations); err != nil {
		return err
	}
	if value, ok := fields["validationActions"]; ok {
		if err := stringEnumList(value, "Deny", "Audit", "Warn"); err != nil {
			return err
		}
	}
	if value, ok := fields["failurePolicy"]; ok {
		if err := oneOf(value, "Fail", "Ignore"); err != nil {
			return err
		}
	}
	if value, ok := fields["matchConditions"]; ok {
		if err := checkNamedExpressions(value); err != nil {
			return err
		}
	}
	if value, ok := fields["variables"]; ok {
		if err := checkNamedExpressions(value); err != nil {
			return err
		}
	}
	if value, ok := fields["auditAnnotations"]; ok {
		if err := checkAuditAnnotations(value); err != nil {
			return err
		}
	}
	if value, ok := fields["autogen"]; ok {
		if err := checkAutogen(value); err != nil {
			return err
		}
	}
	return nil
}

func checkConstraints(node *yaml.Node) error {
	fields, err := mapping(node, constraintKeys)
	if err != nil {
		return err
	}
	rules, ok := fields["resourceRules"]
	if !ok {
		return fail(CodePolicy, "resourceRules is required")
	}
	if err := checkRules(rules); err != nil {
		return err
	}
	if value, ok := fields["excludeResourceRules"]; ok {
		if err := checkRules(value); err != nil {
			return err
		}
	}
	if value, ok := fields["matchPolicy"]; ok {
		if err := oneOf(value, "Exact", "Equivalent"); err != nil {
			return err
		}
	}
	for _, key := range []string{"namespaceSelector", "objectSelector"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if err := checkSelector(value); err != nil {
			return err
		}
	}
	return nil
}

func checkRules(node *yaml.Node) error {
	items, err := sequence(node)
	if err != nil || len(items) == 0 {
		return fail(CodePolicy, "resource rules are required")
	}
	for _, item := range items {
		fields, err := mapping(item, ruleKeys)
		if err != nil {
			return err
		}
		for _, key := range []string{"apiGroups", "apiVersions", "resources"} {
			value, ok := fields[key]
			if !ok {
				return fail(CodePolicy, "%s is required", key)
			}
			if err := stringList(value, false); err != nil {
				return err
			}
		}
		for _, key := range []string{"operations", "resourceNames"} {
			value, ok := fields[key]
			if !ok {
				continue
			}
			if err := stringList(value, true); err != nil {
				return err
			}
		}
		if value, ok := fields["operations"]; ok {
			if err := stringEnumList(value, "CREATE", "UPDATE", "DELETE", "CONNECT", "*"); err != nil {
				return err
			}
		}
		if value, ok := fields["scope"]; ok {
			if err := oneOf(value, "*", "Cluster", "Namespaced"); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkSelector(node *yaml.Node) error {
	fields, err := mapping(node, selectorKeys)
	if err != nil {
		return err
	}
	if value, ok := fields["matchLabels"]; ok {
		if err := stringMap(value); err != nil {
			return err
		}
	}
	value, ok := fields["matchExpressions"]
	if !ok {
		return nil
	}
	items, err := sequence(value)
	if err != nil {
		return err
	}
	for _, item := range items {
		req, err := mapping(item, requirementKeys)
		if err != nil {
			return err
		}
		if _, err := requiredString(req, "key"); err != nil {
			return err
		}
		op, err := requiredString(req, "operator")
		if err != nil {
			return err
		}
		switch op {
		case "In", "NotIn", "Exists", "DoesNotExist", "Gt", "Lt":
		default:
			return fail(CodePolicy, "operator %q", op)
		}
		if values, ok := req["values"]; ok {
			if err := stringList(values, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkValidations(node *yaml.Node) error {
	items, err := sequence(node)
	if err != nil || len(items) == 0 {
		return fail(CodePolicy, "validations is required")
	}
	for _, item := range items {
		fields, err := mapping(item, validationKeys)
		if err != nil {
			return err
		}
		if _, err := requiredString(fields, "expression"); err != nil {
			return err
		}
		for _, key := range []string{"message", "messageExpression", "reason"} {
			if value, ok := fields[key]; ok && !isString(value) {
				return fail(CodePolicy, "%s is not a string", key)
			}
		}
	}
	return nil
}

func checkNamedExpressions(node *yaml.Node) error {
	items, err := sequence(node)
	if err != nil || len(items) == 0 {
		return fail(CodePolicy, "expected a list")
	}
	allowed := map[string]struct{}{"expression": {}, "name": {}}
	for _, item := range items {
		fields, err := mapping(item, allowed)
		if err != nil {
			return err
		}
		if _, err := requiredString(fields, "name"); err != nil {
			return err
		}
		if _, err := requiredString(fields, "expression"); err != nil {
			return err
		}
	}
	return nil
}

func checkAuditAnnotations(node *yaml.Node) error {
	items, err := sequence(node)
	if err != nil || len(items) == 0 {
		return fail(CodePolicy, "auditAnnotations is empty")
	}
	allowed := map[string]struct{}{"key": {}, "valueExpression": {}}
	for _, item := range items {
		fields, err := mapping(item, allowed)
		if err != nil {
			return err
		}
		if _, err := requiredString(fields, "key"); err != nil {
			return err
		}
		if _, err := requiredString(fields, "valueExpression"); err != nil {
			return err
		}
	}
	return nil
}

func checkAutogen(node *yaml.Node) error {
	fields, err := mapping(node, autogenKeys)
	if err != nil {
		return err
	}
	if value, ok := fields["podControllers"]; ok {
		controllers, err := mapping(value, map[string]struct{}{"controllers": {}})
		if err != nil {
			return err
		}
		list, ok := controllers["controllers"]
		if !ok {
			return fail(CodePolicy, "controllers is required")
		}
		if err := stringList(list, false); err != nil {
			return err
		}
	}
	if value, ok := fields["validatingAdmissionPolicy"]; ok {
		enabled, err := mapping(value, map[string]struct{}{"enabled": {}})
		if err != nil {
			return err
		}
		flag, ok := enabled["enabled"]
		if !ok || !isBool(flag) {
			return fail(CodePolicy, "enabled is required")
		}
	}
	return nil
}

func setActions(spec *yaml.Node, mode string) error {
	action, err := actionFor(mode)
	if err != nil {
		return err
	}
	content := spec.Content
	kept := make([]*yaml.Node, 0, len(content))
	for i := 0; i < len(content); i += 2 {
		if content[i].Value == "validationActions" {
			continue
		}
		kept = append(kept, content[i], content[i+1])
	}
	spec.Content = append(kept,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "validationActions"},
		stringSequence(action),
	)
	return nil
}

func actionFor(mode string) (string, error) {
	switch mode {
	case "block":
		return "Deny", nil
	case "warn":
		return "Warn", nil
	case "advise":
		return "Audit", nil
	default:
		return "", fail(CodePolicy, "mode %q", mode)
	}
}

func mapping(node *yaml.Node, allowed map[string]struct{}) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fail(CodePolicy, "expected a mapping")
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if !isString(key) {
			return nil, fail(CodePolicy, "mapping key is not a string")
		}
		if _, ok := allowed[key.Value]; !ok {
			return nil, fail(CodeKey, "key %q", key.Value)
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, nil
}

func requiredString(fields map[string]*yaml.Node, key string) (string, error) {
	node, ok := fields[key]
	if !ok || !isString(node) || node.Value == "" {
		return "", fail(CodePolicy, "%s is required", key)
	}
	return node.Value, nil
}

func oneOf(node *yaml.Node, allowed ...string) error {
	if !isString(node) {
		return fail(CodePolicy, "expected a string")
	}
	for _, item := range allowed {
		if node.Value == item {
			return nil
		}
	}
	return fail(CodePolicy, "value %q", node.Value)
}

func sequence(node *yaml.Node) ([]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, fail(CodePolicy, "expected a list")
	}
	return node.Content, nil
}

func stringList(node *yaml.Node, allowEmpty bool) error {
	items, err := sequence(node)
	if err != nil {
		return err
	}
	if !allowEmpty && len(items) == 0 {
		return fail(CodePolicy, "list is empty")
	}
	for _, item := range items {
		if !isString(item) {
			return fail(CodePolicy, "expected a string")
		}
	}
	return nil
}

func stringEnumList(node *yaml.Node, allowed ...string) error {
	items, err := sequence(node)
	if err != nil || len(items) == 0 {
		return fail(CodePolicy, "expected a list")
	}
	for _, item := range items {
		if err := oneOf(item, allowed...); err != nil {
			return err
		}
	}
	return nil
}

func stringMap(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return fail(CodePolicy, "expected a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if !isString(key) || key.Value == "" || !isString(value) {
			return fail(CodePolicy, "expected string keys and values")
		}
	}
	return nil
}

func stringSequence(value string) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
		Content: []*yaml.Node{{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: value,
		}},
	}
}

func isString(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.ShortTag() == "!!str"
}

func isBool(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.ShortTag() == "!!bool"
}
