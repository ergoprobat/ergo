package eslint

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

func decodeJSON(src []byte) (any, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, fail(CodeJSON, "empty file")
	}
	dec := json.NewDecoder(bytes.NewReader(src))
	dec.UseNumber()
	value, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fail(CodeJSON, "trailing data")
		}
		return nil, fail(CodeJSON, "%v", err)
	}
	return value, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, fail(CodeJSON, "%v", err)
	}
	switch token {
	case json.Delim('{'):
		return decodeObject(dec)
	case json.Delim('['):
		return decodeArray(dec)
	default:
		return token, nil
	}
}

func decodeObject(dec *json.Decoder) (map[string]any, error) {
	obj := map[string]any{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, fail(CodeJSON, "%v", err)
		}
		key, ok := token.(string)
		if !ok {
			return nil, fail(CodeJSON, "object key is not a string")
		}
		if key == "__proto__" {
			return nil, fail(CodeJSON, "key %q", key)
		}
		if _, exists := obj[key]; exists {
			return nil, fail(CodeJSON, "duplicate key %q", key)
		}
		value, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		obj[key] = value
	}
	token, err := dec.Token()
	if err != nil {
		return nil, fail(CodeJSON, "%v", err)
	}
	if token != json.Delim('}') {
		return nil, fail(CodeJSON, "object did not end")
	}
	return obj, nil
}

func decodeArray(dec *json.Decoder) ([]any, error) {
	var items []any
	for dec.More() {
		value, err := decodeValue(dec)
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	token, err := dec.Token()
	if err != nil {
		return nil, fail(CodeJSON, "%v", err)
	}
	if token != json.Delim(']') {
		return nil, fail(CodeJSON, "array did not end")
	}
	if items == nil {
		items = []any{}
	}
	return items, nil
}
