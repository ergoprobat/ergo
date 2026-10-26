package opa

import "gopkg.in/yaml.v3"

var schemaKeys = map[string]struct{}{
	"additionalProperties": {},
	"description":          {},
	"enum":                 {},
	"format":               {},
	"items":                {},
	"maxItems":             {},
	"maxLength":            {},
	"maximum":              {},
	"minItems":             {},
	"minLength":            {},
	"minimum":              {},
	"nullable":             {},
	"pattern":              {},
	"properties":           {},
	"required":             {},
	"type":                 {},
}

func checkSchema(node *yaml.Node) error {
	fields, err := gkMapping(node, schemaKeys, CodeGatekeeperTemplate)
	if err != nil {
		return err
	}
	if value, ok := fields["type"]; ok {
		if err := gkOneOf(value, CodeGatekeeperTemplate, "object", "array", "string", "boolean", "integer", "number"); err != nil {
			return err
		}
	}
	for _, key := range []string{"description", "format", "pattern"} {
		if value, ok := fields[key]; ok && !gkString(value) {
			return gatekeeperErr(CodeGatekeeperTemplate, "%s is not a string", key)
		}
	}
	for _, key := range []string{"minimum", "maximum", "minLength", "maxLength", "minItems", "maxItems"} {
		if value, ok := fields[key]; ok && !gkNumber(value) {
			return gatekeeperErr(CodeGatekeeperTemplate, "%s is not a number", key)
		}
	}
	if value, ok := fields["nullable"]; ok && !gkBool(value) {
		return gatekeeperErr(CodeGatekeeperTemplate, "nullable is not a boolean")
	}
	if value, ok := fields["required"]; ok {
		if err := gkStringList(value, CodeGatekeeperTemplate, true); err != nil {
			return err
		}
	}
	if value, ok := fields["enum"]; ok {
		if err := checkEnum(value); err != nil {
			return err
		}
	}
	if value, ok := fields["properties"]; ok {
		props, err := gkMapping(value, nil, CodeGatekeeperTemplate)
		if err != nil {
			return err
		}
		for _, prop := range props {
			if err := checkSchema(prop); err != nil {
				return err
			}
		}
	}
	if value, ok := fields["items"]; ok {
		if err := checkSchema(value); err != nil {
			return err
		}
	}
	if value, ok := fields["additionalProperties"]; ok {
		if gkBool(value) {
			return nil
		}
		if err := checkSchema(value); err != nil {
			return err
		}
	}
	return nil
}

func checkEnum(node *yaml.Node) error {
	items, err := gkSequence(node, CodeGatekeeperTemplate)
	if err != nil || len(items) == 0 {
		return gatekeeperErr(CodeGatekeeperTemplate, "enum is empty")
	}
	for _, item := range items {
		if !gkString(item) && !gkBool(item) && !gkNumber(item) {
			return gatekeeperErr(CodeGatekeeperTemplate, "enum value is not a scalar")
		}
	}
	return nil
}

func gkChild(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// gkMapping reads a mapping. allowed nil means every string key is accepted,
// which properties needs because the keys are schema names.
func gkMapping(node *yaml.Node, allowed map[string]struct{}, code string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, gatekeeperErr(code, "expected a mapping")
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if !gkString(key) || key.Value == "" {
			return nil, gatekeeperErr(code, "mapping key is not a string")
		}
		if allowed != nil {
			if _, ok := allowed[key.Value]; !ok {
				return nil, gatekeeperErr(CodeGatekeeperKey, "key %q", key.Value)
			}
		}
		fields[key.Value] = node.Content[i+1]
	}
	return fields, nil
}

func gkRequired(fields map[string]*yaml.Node, key, code string) (string, error) {
	node, ok := fields[key]
	if !ok || !gkString(node) || node.Value == "" {
		return "", gatekeeperErr(code, "%s is required", key)
	}
	return node.Value, nil
}

func gkRequiredList(fields map[string]*yaml.Node, key string) ([]*yaml.Node, error) {
	node, ok := fields[key]
	if !ok {
		return nil, gatekeeperErr(CodeGatekeeperConstraint, "%s is required", key)
	}
	items, err := gkSequence(node, CodeGatekeeperConstraint)
	if err != nil || len(items) == 0 {
		return nil, gatekeeperErr(CodeGatekeeperConstraint, "%s is required", key)
	}
	for _, item := range items {
		if !gkString(item) || item.Value == "" {
			return nil, gatekeeperErr(CodeGatekeeperConstraint, "%s is not a list of strings", key)
		}
	}
	return items, nil
}

func gkSequence(node *yaml.Node, code string) ([]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, gatekeeperErr(code, "expected a list")
	}
	return node.Content, nil
}

func gkStringList(node *yaml.Node, code string, allowEmpty bool) error {
	items, err := gkSequence(node, code)
	if err != nil {
		return err
	}
	if !allowEmpty && len(items) == 0 {
		return gatekeeperErr(code, "list is empty")
	}
	for _, item := range items {
		if !gkString(item) {
			return gatekeeperErr(code, "expected a string")
		}
	}
	return nil
}

func gkEnumList(node *yaml.Node, code string, allowed ...string) error {
	items, err := gkSequence(node, code)
	if err != nil || len(items) == 0 {
		return gatekeeperErr(code, "expected a list")
	}
	for _, item := range items {
		if err := gkOneOf(item, code, allowed...); err != nil {
			return err
		}
	}
	return nil
}

func gkOneOf(node *yaml.Node, code string, allowed ...string) error {
	if !gkString(node) {
		return gatekeeperErr(code, "expected a string")
	}
	for _, item := range allowed {
		if node.Value == item {
			return nil
		}
	}
	return gatekeeperErr(code, "value %q", node.Value)
}

func gkStringMap(node *yaml.Node, code string) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return gatekeeperErr(code, "expected a mapping")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if !gkString(key) || key.Value == "" || !gkString(value) {
			return gatekeeperErr(code, "expected string keys and values")
		}
	}
	return nil
}

func gkString(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.ShortTag() == "!!str"
}

func gkBool(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.ShortTag() == "!!bool"
}

func gkNumber(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.ScalarNode {
		return false
	}
	switch node.ShortTag() {
	case "!!int", "!!float":
		return true
	default:
		return false
	}
}
