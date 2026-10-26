package archunit

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/rules.schema.json
var rulesSchemaJSON []byte

var ulidPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

var (
	schemaOnce  sync.Once
	rulesSchema *jsonschema.Schema
	schemaErr   error
)

type archRule struct {
	name      string
	kind      string
	source    []string
	forbidden []string
	layers    []archLayer
}

type archLayer struct {
	name     string
	packages []string
}

func compileRules(src []byte, decisionID string, index int) ([]byte, error) {
	if !ulidPattern.MatchString(decisionID) {
		return nil, fail(CodeJSON, "decision id %q", decisionID)
	}
	doc, err := decodeObject(src)
	if err != nil {
		return nil, err
	}
	if err := validateSchema(doc); err != nil {
		return nil, err
	}
	packages, rules, err := rulesFrom(doc)
	if err != nil {
		return nil, err
	}
	return generate(decisionID, index, packages, rules)
}

func decodeObject(src []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, fail(CodeJSON, "empty file")
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	val, err := decodeValue(dec)
	if err != nil {
		return nil, fail(CodeJSON, "%s", err.Error())
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, fail(CodeJSON, "trailing data")
	}
	doc, ok := val.(map[string]any)
	if !ok {
		return nil, fail(CodeJSON, "not a JSON object")
	}
	return doc, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		return decodeObjectTokens(dec)
	case '[':
		return decodeArray(dec)
	default:
		return nil, errors.New("unexpected delimiter")
	}
}

func decodeObjectTokens(dec *json.Decoder) (map[string]any, error) {
	obj := map[string]any{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("object key")
		}
		if _, exists := obj[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		val, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		obj[key] = val
	}
	end, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if end != json.Delim('}') {
		return nil, errors.New("unclosed object")
	}
	return obj, nil
}

func decodeArray(dec *json.Decoder) ([]any, error) {
	var items []any
	for dec.More() {
		val, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		items = append(items, val)
	}
	end, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if end != json.Delim(']') {
		return nil, errors.New("unclosed array")
	}
	return items, nil
}

func ruleSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		schemaCompiler := jsonschema.NewCompiler()
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(rulesSchemaJSON))
		if err != nil {
			schemaErr = err
			return
		}
		root, ok := document.(map[string]any)
		if !ok {
			schemaErr = errors.New("schema is not an object")
			return
		}
		id, _ := root["$id"].(string)
		if id == "" {
			schemaErr = errors.New("schema has no $id")
			return
		}
		if err := schemaCompiler.AddResource(id, document); err != nil {
			schemaErr = err
			return
		}
		rulesSchema, err = schemaCompiler.Compile(id)
		if err != nil {
			schemaErr = err
		}
	})
	return rulesSchema, schemaErr
}

func validateSchema(doc map[string]any) error {
	compiled, err := ruleSchema()
	if err != nil {
		return fail(CodeSchema, "load schema: %s", err.Error())
	}
	if err := compiled.Validate(doc); err != nil {
		return fail(CodeSchema, "%s", err.Error())
	}
	return nil
}

func rulesFrom(doc map[string]any) ([]string, []archRule, error) {
	packages, err := stringList(doc["packages"])
	if err != nil {
		return nil, nil, fail(CodeSchema, "packages")
	}
	rawRules, ok := doc["rules"].([]any)
	if !ok {
		return nil, nil, fail(CodeSchema, "rules")
	}
	rules := make([]archRule, 0, len(rawRules))
	seen := map[string]struct{}{}
	for _, raw := range rawRules {
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, fail(CodeSchema, "rule")
		}
		name, _ := obj["name"].(string)
		if _, ok := seen[name]; ok {
			return nil, nil, fail(CodeRuleName, "duplicate name %q", name)
		}
		seen[name] = struct{}{}
		kind, _ := obj["type"].(string)
		switch kind {
		case "forbidden":
			source, err := stringList(obj["source_packages"])
			if err != nil {
				return nil, nil, fail(CodeSchema, "source_packages")
			}
			forbidden, err := stringList(obj["forbidden_packages"])
			if err != nil {
				return nil, nil, fail(CodeSchema, "forbidden_packages")
			}
			rules = append(rules, archRule{name: name, kind: kind, source: source, forbidden: forbidden})
		case "layers":
			layers, err := layersFrom(obj["layers"])
			if err != nil {
				return nil, nil, err
			}
			rules = append(rules, archRule{name: name, kind: kind, layers: layers})
		default:
			return nil, nil, fail(CodeSchema, "type %q", kind)
		}
	}
	return packages, rules, nil
}

func layersFrom(raw any) ([]archLayer, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fail(CodeSchema, "layers")
	}
	layers := make([]archLayer, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fail(CodeSchema, "layer")
		}
		name, _ := obj["name"].(string)
		if _, ok := seen[name]; ok {
			return nil, fail(CodeLayerName, "duplicate name %q", name)
		}
		seen[name] = struct{}{}
		packages, err := stringList(obj["packages"])
		if err != nil {
			return nil, fail(CodeSchema, "layer packages")
		}
		layers = append(layers, archLayer{name: name, packages: packages})
	}
	return layers, nil
}

func stringList(raw any) ([]string, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, errors.New("not a list")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, errors.New("not a string")
		}
		out = append(out, text)
	}
	return out, nil
}
