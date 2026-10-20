package edr

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var requiredHeadings = []string{
	"## Context",
	"## Decision",
	"## Alternatives considered",
	"## Consequences",
	"## Verification",
}

// Validate checks frontmatter against the embedded schema and the body headings.
func Validate(doc Document) []string {
	var problems []string
	problems = append(problems, schemaErrors(doc.Front)...)
	problems = append(problems, bodyErrors(doc.Body)...)
	return problems
}

func schemaErrors(front map[string]any) []string {
	schema, err := compileSchema()
	if err != nil {
		return []string{err.Error()}
	}
	err = schema.Validate(front)
	if err == nil {
		return nil
	}
	validation, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{err.Error()}
	}
	var problems []string
	for _, detail := range validation.DetailedOutput().Errors {
		if detail.Error == nil && detail.InstanceLocation == "" {
			continue
		}
		location := detail.InstanceLocation
		if location == "" {
			location = "/"
		}
		message := ""
		if detail.Error != nil {
			message = detail.Error.String()
		}
		if message == "" {
			message = "does not match the schema"
		}
		problems = append(problems, location+": "+message)
	}
	if len(problems) == 0 {
		problems = append(problems, validation.Error())
	}
	return problems
}

func compileSchema() (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("embedded schema: %w", err)
	}
	if err := compiler.AddResource(SchemaURL, document); err != nil {
		return nil, fmt.Errorf("embedded schema: %w", err)
	}
	schema, err := compiler.Compile(SchemaURL)
	if err != nil {
		return nil, fmt.Errorf("embedded schema: %w", err)
	}
	return schema, nil
}

func bodyErrors(body string) []string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var h1 []int
	for index, line := range lines {
		if strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "## ") {
			h1 = append(h1, index)
		}
	}
	if len(h1) != 1 {
		return []string{"expected exactly one level-1 heading"}
	}
	positions := make([]int, len(requiredHeadings))
	var missing []string
	for i, heading := range requiredHeadings {
		found := -1
		for index, line := range lines {
			if line == heading {
				found = index
				break
			}
		}
		positions[i] = found
		if found < 0 {
			missing = append(missing, "missing heading "+heading)
		}
	}
	if len(missing) > 0 {
		return missing
	}
	ordered := true
	previous := h1[0]
	for _, position := range positions {
		if position < previous {
			ordered = false
			break
		}
		previous = position
	}
	if !ordered {
		return []string{"headings are out of order"}
	}
	bounds := append(append([]int{}, positions...), len(lines))
	var problems []string
	for i, heading := range requiredHeadings {
		start := positions[i] + 1
		stop := bounds[i+1]
		hasText := false
		for _, line := range lines[start:stop] {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				hasText = true
				break
			}
		}
		if !hasText {
			problems = append(problems, heading+" has no paragraph or list")
		}
	}
	return problems
}

// DecisionText returns the body of the Decision section.
func DecisionText(body string) (string, bool) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	start := -1
	for index, line := range lines {
		if line == "## Decision" {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for index := start; index < len(lines); index++ {
		if strings.HasPrefix(lines[index], "## ") {
			end = index
			break
		}
	}
	text := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
	return text, true
}
