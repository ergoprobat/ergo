package compiler

// Registry is the set of compilers linked into one binary.
type Registry struct {
	compilers []Compiler
}

// NewRegistry returns a registry. The first compiler for an engine wins.
func NewRegistry(compilers ...Compiler) Registry {
	return Registry{compilers: append([]Compiler(nil), compilers...)}
}

// Lookup returns the compiler for an EDR engine value.
func (r Registry) Lookup(engine string) (Compiler, bool) {
	for _, candidate := range r.compilers {
		if candidate != nil && candidate.Engine() == engine {
			return candidate, true
		}
	}
	return nil, false
}
