package semgrep

import (
	"bytes"
	"errors"
	"sort"

	"gopkg.in/yaml.v3"
)

func setSeverity(rule *yaml.Node, severity string) {
	for i := 0; i < len(rule.Content); i += 2 {
		if rule.Content[i].Value != "severity" {
			continue
		}
		rule.Content[i+1].Kind = yaml.ScalarNode
		rule.Content[i+1].Tag = "!!str"
		rule.Content[i+1].Value = severity
		rule.Content[i+1].Style = 0
		rule.Content[i+1].Content = nil
		return
	}
	rule.Content = append(rule.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "severity"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: severity},
	)
}

func canonicalYAML(root *yaml.Node) ([]byte, error) {
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
	if depth > maxYAMLDepth {
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
