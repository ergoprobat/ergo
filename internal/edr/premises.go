package edr

import "gopkg.in/yaml.v3"

// premiseProblems reports a duplicate premise id or an equals value over 1024 bytes.
// The schema cannot express either rule. A missing premises key is not a problem.
func premiseProblems(front *yaml.Node) []string {
	premises := mappingValue(front, "premises")
	if premises == nil || premises.Kind != yaml.SequenceNode {
		return nil
	}
	var problems []string
	seen := map[string]struct{}{}
	duplicate := false
	tooLarge := false
	notJSON := false
	for _, item := range premises.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if idNode := mappingValue(item, "id"); idNode != nil && idNode.Kind == yaml.ScalarNode {
			if _, ok := seen[idNode.Value]; ok {
				duplicate = true
			}
			seen[idNode.Value] = struct{}{}
		}
		probe := mappingValue(item, "probe")
		if probe == nil || probe.Kind != yaml.MappingNode {
			continue
		}
		equals := mappingValue(probe, "equals")
		if equals == nil {
			continue
		}
		encoded, err := encodeJSON(equals)
		if err != nil {
			notJSON = true
			continue
		}
		if len(encoded) > equalsLimit {
			tooLarge = true
		}
	}
	if duplicate {
		problems = append(problems, "premises: duplicate premise id")
	}
	if notJSON {
		problems = append(problems, "premises: equals is not JSON")
	}
	if tooLarge {
		problems = append(problems, "premises: equals exceeds 1024 bytes")
	}
	return problems
}
