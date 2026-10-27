// Package strictyaml decodes plain YAML and encodes it in a stable form.
// Anchors, aliases, merge keys, explicit tags, and duplicate keys are rejected.
package strictyaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

const (
	// MaxDepth is the deepest mapping or list a document may contain.
	// yaml.v3 has already built the tree, so the byte cap bounds the work.
	// The depth cap is what keeps that walk off the call stack.
	MaxDepth = 64
	// MaxNodes stops a wide tree from turning one compile into a long walk.
	MaxNodes = 100000
)

// Documents decodes src as exactly count plain YAML documents.
// The returned nodes are the document roots. code is the reason on failure.
func Documents(src []byte, code string, count int) ([]*yaml.Node, error) {
	if count < 1 {
		return nil, fmt.Errorf("document count %d", count)
	}
	dec := yaml.NewDecoder(bytes.NewReader(src))
	roots := make([]*yaml.Node, 0, count)
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, &compiler.Error{Code: code, Err: err}
		}
		if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
			return nil, &compiler.Error{Code: code, Err: errors.New("not one document")}
		}
		roots = append(roots, doc.Content[0])
		if len(roots) > count {
			if count == 1 {
				return nil, &compiler.Error{Code: code, Err: errors.New("more than one document")}
			}
			return nil, &compiler.Error{Code: code, Err: fmt.Errorf("more than %d documents", count)}
		}
	}
	if len(roots) == 0 {
		return nil, &compiler.Error{Code: code, Err: errors.New("empty file")}
	}
	if len(roots) != count {
		return nil, &compiler.Error{Code: code, Err: fmt.Errorf("%d documents", len(roots))}
	}
	nodes := 0
	for _, root := range roots {
		if err := walk(root, 0, &nodes, code); err != nil {
			return nil, err
		}
	}
	return roots, nil
}

func walk(node *yaml.Node, depth int, nodes *int, code string) error {
	if node == nil {
		return &compiler.Error{Code: code, Err: errors.New("empty document")}
	}
	*nodes++
	if depth > MaxDepth || *nodes > MaxNodes {
		return &compiler.Error{Code: code, Err: errors.New("document is too large")}
	}
	if node.Kind == yaml.AliasNode {
		return &compiler.Error{Code: code, Err: errors.New("alias")}
	}
	if node.Anchor != "" {
		return &compiler.Error{Code: code, Err: fmt.Errorf("anchor %q", node.Anchor)}
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return &compiler.Error{Code: code, Err: fmt.Errorf("tag %s", node.Tag)}
	}
	if node.Kind != yaml.MappingNode {
		for _, child := range node.Content {
			if err := walk(child, depth+1, nodes, code); err != nil {
				return err
			}
		}
		return nil
	}
	if len(node.Content)%2 != 0 {
		return &compiler.Error{Code: code, Err: errors.New("broken mapping")}
	}
	seen := map[string]struct{}{}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if err := walk(key, depth+1, nodes, code); err != nil {
			return err
		}
		if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" {
			return &compiler.Error{Code: code, Err: errors.New("mapping key is not a string")}
		}
		if key.Value == "<<" {
			return &compiler.Error{Code: code, Err: errors.New("merge key")}
		}
		if _, ok := seen[key.Value]; ok {
			return &compiler.Error{Code: code, Err: fmt.Errorf("duplicate key %q", key.Value)}
		}
		seen[key.Value] = struct{}{}
		if err := walk(node.Content[i+1], depth+1, nodes, code); err != nil {
			return err
		}
	}
	return nil
}

// Canonical encodes root with mapping keys sorted, comments dropped, and a trailing newline.
func Canonical(root *yaml.Node) ([]byte, error) {
	if err := canonicalize(root, 0); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func canonicalize(node *yaml.Node, depth int) error {
	if depth > MaxDepth {
		return errors.New("document is too large")
	}
	node.HeadComment = ""
	node.LineComment = ""
	node.FootComment = ""
	node.Style = 0
	if node.Kind != yaml.MappingNode {
		for _, child := range node.Content {
			if err := canonicalize(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	type pair struct {
		key   *yaml.Node
		value *yaml.Node
	}
	pairs := make([]pair, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		if err := canonicalize(node.Content[i], depth+1); err != nil {
			return err
		}
		if err := canonicalize(node.Content[i+1], depth+1); err != nil {
			return err
		}
		pairs = append(pairs, pair{node.Content[i], node.Content[i+1]})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].key.Value < pairs[j].key.Value
	})
	node.Content = node.Content[:0]
	for _, item := range pairs {
		node.Content = append(node.Content, item.key, item.value)
	}
	return nil
}
