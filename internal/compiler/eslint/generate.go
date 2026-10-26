package eslint

import (
	"encoding/json"
	"strings"
)

func generate(files, ignores []string, rules map[string]any, target string) ([]byte, error) {
	var b strings.Builder
	if target == targetTypeScript {
		b.WriteString("import tseslint from \"typescript-eslint\";\n\n")
	}
	b.WriteString("export default [\n  {\n")
	if err := writeValue(&b, "files", files); err != nil {
		return nil, err
	}
	if ignores != nil {
		if err := writeValue(&b, "ignores", ignores); err != nil {
			return nil, err
		}
	}
	if target == targetTypeScript {
		b.WriteString("    languageOptions: {\n")
		b.WriteString("      parser: tseslint.parser,\n")
		b.WriteString("      parserOptions: {\n")
		b.WriteString("        projectService: true,\n")
		b.WriteString("        tsconfigRootDir: process.cwd(),\n")
		b.WriteString("      },\n")
		b.WriteString("    },\n")
		b.WriteString("    plugins: {\n")
		b.WriteString("      \"@typescript-eslint\": tseslint.plugin,\n")
		b.WriteString("    },\n")
	}
	b.WriteString("    linterOptions: {\n")
	b.WriteString("      noInlineConfig: true,\n")
	b.WriteString("    },\n")
	if err := writeRules(&b, rules); err != nil {
		return nil, err
	}
	b.WriteString("  },\n];\n")
	return []byte(b.String()), nil
}

func writeValue(b *strings.Builder, name string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	b.WriteString("    ")
	b.WriteString(name)
	b.WriteString(": ")
	b.Write(raw)
	b.WriteString(",\n")
	return nil
}

func writeRules(b *strings.Builder, rules map[string]any) error {
	raw, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	b.WriteString("    rules: ")
	b.WriteString(lines[0])
	b.WriteByte('\n')
	last := len(lines) - 1
	for _, line := range lines[1:last] {
		b.WriteString("    ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("    ")
	b.WriteString(lines[last])
	b.WriteString(",\n")
	return nil
}
