package eslint

// CoreRules returns a copy of the ESLint rules compile accepts.
func CoreRules() map[string]struct{} { return copyRules(coreRules) }

// TypeScriptRules returns a copy of the typescript-eslint rules compile accepts.
func TypeScriptRules() map[string]struct{} { return copyRules(typescriptRules) }

func copyRules(rules map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(rules))
	for id := range rules {
		out[id] = struct{}{}
	}
	return out
}
