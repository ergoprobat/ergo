package edr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const equalsLimit = 1024

// mappingNode returns the frontmatter mapping. A document node unwraps once.
func mappingNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) != 1 {
			return nil
		}
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	return node
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Kind == yaml.ScalarNode && node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// encodeJSON writes compact UTF-8 JSON for a YAML value, with object keys in node order.
// Strings keep non-ASCII characters, matching json.dumps(..., ensure_ascii=False).
func encodeJSON(node *yaml.Node) ([]byte, error) {
	var buf []byte
	buf, err := appendJSON(buf, node)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func appendJSON(dst []byte, node *yaml.Node) ([]byte, error) {
	if node == nil {
		return nil, fmt.Errorf("empty")
	}
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			return nil, fmt.Errorf("alias")
		}
		return appendJSON(dst, node.Alias)
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return nil, fmt.Errorf("document")
		}
		return appendJSON(dst, node.Content[0])
	case yaml.SequenceNode:
		dst = append(dst, '[')
		for i, item := range node.Content {
			if i > 0 {
				dst = append(dst, ',')
			}
			var err error
			dst, err = appendJSON(dst, item)
			if err != nil {
				return nil, err
			}
		}
		return append(dst, ']'), nil
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return nil, fmt.Errorf("mapping")
		}
		dst = append(dst, '{')
		for i := 0; i < len(node.Content); i += 2 {
			if i > 0 {
				dst = append(dst, ',')
			}
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
				return nil, fmt.Errorf("key")
			}
			dst = appendJSONString(dst, key.Value)
			dst = append(dst, ':')
			var err error
			dst, err = appendJSON(dst, node.Content[i+1])
			if err != nil {
				return nil, err
			}
		}
		return append(dst, '}'), nil
	case yaml.ScalarNode:
		return appendScalar(dst, node)
	default:
		return nil, fmt.Errorf("kind")
	}
}

func appendScalar(dst []byte, node *yaml.Node) ([]byte, error) {
	switch node.ShortTag() {
	case "!!null":
		return append(dst, "null"...), nil
	case "!!bool":
		var value bool
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		if value {
			return append(dst, "true"...), nil
		}
		return append(dst, "false"...), nil
	case "!!int":
		var value int64
		if err := node.Decode(&value); err == nil {
			return strconv.AppendInt(dst, value, 10), nil
		}
		if !jsonInt(node.Value) {
			return nil, fmt.Errorf("int")
		}
		return append(dst, node.Value...), nil
	case "!!float":
		var value float64
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("float")
		}
		text := strconv.FormatFloat(value, 'f', -1, 64)
		if !strings.ContainsAny(text, ".eE") {
			text += ".0"
		}
		return append(dst, text...), nil
	case "!!timestamp":
		var value time.Time
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		return appendJSONString(dst, value.Format("2006-01-02")), nil
	case "!!str":
		return appendJSONString(dst, node.Value), nil
	default:
		return nil, fmt.Errorf("scalar")
	}
}

func jsonInt(value string) bool {
	if value == "" {
		return false
	}
	if value[0] == '-' {
		value = value[1:]
		if value == "" {
			return false
		}
	}
	if value == "0" {
		return true
	}
	if value[0] == '0' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func appendJSONString(dst []byte, value string) []byte {
	dst = append(dst, '"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			dst = append(dst, '\\', byte(r))
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			if r < 0x20 {
				dst = append(dst, fmt.Sprintf(`\u%04x`, r)...)
				continue
			}
			var buf [utf8.UTFMax]byte
			n := utf8.EncodeRune(buf[:], r)
			dst = append(dst, buf[:n]...)
		}
	}
	return append(dst, '"')
}
