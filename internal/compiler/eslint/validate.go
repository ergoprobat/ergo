package eslint

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	schemaOnce sync.Once
	ruleSchema *jsonschema.Schema
	schemaErr  error
)

func compileRules(src []byte, target, mode string) ([]byte, error) {
	severity, err := severityFor(mode)
	if err != nil {
		return nil, err
	}
	value, err := decodeJSON(src)
	if err != nil {
		return nil, err
	}
	doc, ok := value.(map[string]any)
	if !ok {
		return nil, fail(CodeSchema, "not a JSON object")
	}
	if err := validateSchema(doc); err != nil {
		return nil, err
	}
	files, ignores, rules, err := checkDocument(doc, target)
	if err != nil {
		return nil, err
	}
	if err := rewriteSeverity(rules, severity); err != nil {
		return nil, err
	}
	return generate(files, ignores, rules, target)
}

func rulesSchemaCompiled() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(rulesSchemaJSON))
		if err != nil {
			schemaErr = fmt.Errorf("eslint schema: %w", err)
			return
		}
		root, ok := document.(map[string]any)
		if !ok {
			schemaErr = errors.New("eslint schema: not an object")
			return
		}
		id, _ := root["$id"].(string)
		if id == "" {
			schemaErr = errors.New("eslint schema: missing $id")
			return
		}
		if err := compiler.AddResource(id, document); err != nil {
			schemaErr = fmt.Errorf("eslint schema: %w", err)
			return
		}
		ruleSchema, err = compiler.Compile(id)
		if err != nil {
			schemaErr = fmt.Errorf("eslint schema: %w", err)
		}
	})
	return ruleSchema, schemaErr
}

func validateSchema(doc map[string]any) error {
	compiled, err := rulesSchemaCompiled()
	if err != nil {
		return err
	}
	if err := compiled.Validate(doc); err != nil {
		return fail(CodeSchema, "%v", err)
	}
	return nil
}

func checkDocument(doc map[string]any, target string) (files, ignores []string, rules map[string]any, err error) {
	files, err = stringList(doc["files"])
	if err != nil {
		return nil, nil, nil, fail(CodeSchema, "files: %v", err)
	}
	if err := checkPatterns(files); err != nil {
		return nil, nil, nil, err
	}
	if raw, ok := doc["ignores"]; ok {
		ignores, err = stringList(raw)
		if err != nil {
			return nil, nil, nil, fail(CodeSchema, "ignores: %v", err)
		}
		if err := checkPatterns(ignores); err != nil {
			return nil, nil, nil, err
		}
	}
	rules, ok := doc["rules"].(map[string]any)
	if !ok {
		return nil, nil, nil, fail(CodeSchema, "rules is not an object")
	}
	for id := range rules {
		if !ruleAllowed(id, target) {
			return nil, nil, nil, fail(CodeRule, "rule %q", id)
		}
	}
	return files, ignores, rules, nil
}

func ruleAllowed(id, target string) bool {
	if _, ok := coreRules[id]; ok {
		return true
	}
	if target == targetTypeScript {
		_, ok := typescriptRules[id]
		return ok
	}
	return false
}

func stringList(value any) ([]string, error) {
	items, ok := value.([]any)
	if !ok {
		return nil, errors.New("not a list")
	}
	out := make([]string, len(items))
	for i, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("not a string")
		}
		out[i] = text
	}
	return out, nil
}

func checkPatterns(patterns []string) error {
	for _, pattern := range patterns {
		if path.IsAbs(pattern) || strings.HasPrefix(pattern, "!") || strings.Contains(pattern, "..") {
			return fail(CodePath, "pattern %q", pattern)
		}
	}
	return nil
}

func rewriteSeverity(rules map[string]any, severity string) error {
	for id, value := range rules {
		switch item := value.(type) {
		case string:
			if item == "off" {
				return fail(CodeRule, "rule %q is off", id)
			}
			rules[id] = severity
		case []any:
			if len(item) == 0 {
				return fail(CodeRule, "rule %q", id)
			}
			level, ok := item[0].(string)
			if !ok || level == "off" {
				return fail(CodeRule, "rule %q is off", id)
			}
			item[0] = severity
		default:
			return fail(CodeSchema, "rule %q", id)
		}
	}
	return nil
}
