package compiler

import "strings"

const (
	// CodeTemplateUnknown means ref names an ergo: template that is not built in.
	CodeTemplateUnknown = "template-unknown"

	// TemplatePrefix marks a ref that selects a built-in template.
	TemplatePrefix = "ergo:"
)

// ValidTemplateID reports whether id is one provider token, a slash, and one name token.
// A token is lowercase letters, digits, and hyphens. The provider lib is valid here.
// The OPA compiler rejects that provider itself, because that directory holds the helper module.
func ValidTemplateID(id string) bool {
	provider, name, ok := strings.Cut(id, "/")
	if !ok || strings.Contains(name, "/") {
		return false
	}
	return TemplateToken(provider) && TemplateToken(name)
}

// TemplateToken reports whether value is one non-empty template path segment.
func TemplateToken(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}
