package archunit

import (
	"fmt"
	"strings"
)

// generate writes one JUnit Jupiter test. The package name carries the decision
// id, so every check can share .ergo/build as a test source root. Only strings
// that matched the rule schema are embedded, so the file cannot carry free text.
func generate(decisionID string, index int, packages []string, rules []archRule) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "package ergo.d%s.c%d;\n\n", strings.ToLower(decisionID), index)
	forbidden, layers := false, false
	for _, rule := range rules {
		switch rule.kind {
		case "forbidden":
			forbidden = true
		case "layers":
			layers = true
		}
	}
	if forbidden {
		b.WriteString("import static com.tngtech.archunit.lang.syntax.ArchRuleDefinition.noClasses;\n")
	}
	if layers {
		b.WriteString("import static com.tngtech.archunit.library.Architectures.layeredArchitecture;\n")
	}
	b.WriteString("\n")
	b.WriteString("import com.tngtech.archunit.core.domain.JavaClasses;\n")
	b.WriteString("import com.tngtech.archunit.core.importer.ClassFileImporter;\n")
	b.WriteString("import com.tngtech.archunit.core.importer.ImportOption;\n")
	b.WriteString("import org.junit.jupiter.api.BeforeAll;\n")
	b.WriteString("import org.junit.jupiter.api.Test;\n\n")
	b.WriteString("class ArchitectureTest {\n")
	b.WriteString("    private static JavaClasses classes;\n\n")
	b.WriteString("    @BeforeAll\n")
	b.WriteString("    static void importClasses() {\n")
	b.WriteString("        classes = new ClassFileImporter()\n")
	b.WriteString("                .withImportOption(ImportOption.Predefined.DO_NOT_INCLUDE_TESTS)\n")
	b.WriteString("                .withImportOption(ImportOption.Predefined.DO_NOT_INCLUDE_JARS)\n")
	fmt.Fprintf(&b, "                .importPackages(%s);\n", javaStrings(packages))
	b.WriteString("    }\n")
	for _, rule := range rules {
		b.WriteByte('\n')
		if err := writeRule(&b, rule); err != nil {
			return nil, err
		}
	}
	b.WriteString("}\n")
	return []byte(b.String()), nil
}

func writeRule(b *strings.Builder, rule archRule) error {
	fmt.Fprintf(b, "    @Test\n    void %s() {\n", methodName(rule.name))
	switch rule.kind {
	case "forbidden":
		fmt.Fprintf(b, "        noClasses()\n")
		fmt.Fprintf(b, "                .that().resideInAnyPackage(%s)\n", javaStrings(rule.source))
		fmt.Fprintf(b, "                .should().dependOnClassesThat().resideInAnyPackage(%s)\n", javaStrings(rule.forbidden))
	case "layers":
		b.WriteString("        layeredArchitecture()\n")
		b.WriteString("                .consideringOnlyDependenciesInLayers()\n")
		for _, layer := range rule.layers {
			fmt.Fprintf(b, "                .layer(%s).definedBy(%s)\n", javaQuote(layer.name), javaStrings(layer.packages))
		}
		for i, layer := range rule.layers {
			if i == 0 {
				fmt.Fprintf(b, "                .whereLayer(%s).mayNotBeAccessedByAnyLayer()\n", javaQuote(layer.name))
				continue
			}
			above := make([]string, i)
			for j := range above {
				above[j] = rule.layers[j].name
			}
			fmt.Fprintf(b, "                .whereLayer(%s).mayOnlyBeAccessedByLayers(%s)\n", javaQuote(layer.name), javaStrings(above))
		}
	default:
		return fail(CodeSchema, "type %q", rule.kind)
	}
	fmt.Fprintf(b, "                .as(%s)\n", javaQuote(rule.name))
	b.WriteString("                .check(classes);\n")
	b.WriteString("    }\n")
	return nil
}

func methodName(name string) string {
	return "rule_" + strings.ReplaceAll(name, "-", "_")
}

func javaStrings(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = javaQuote(value)
	}
	return strings.Join(quoted, ", ")
}

func javaQuote(value string) string {
	return `"` + value + `"`
}
