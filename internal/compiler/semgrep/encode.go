package semgrep

import "gopkg.in/yaml.v3"

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
