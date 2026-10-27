package opa

import (
	"bytes"
	"fmt"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

const (
	// maxParseDepth bounds nested Rego terms. A policy does not need the parser default of 100000.
	maxParseDepth = 128
	// maxExprs bounds the type checker. Compile time follows the expression count.
	maxExprs = 10000
)

// deniedBuiltins are host and network functions. A compiled policy must not gain them.
var deniedBuiltins = map[string]struct{}{
	"http.send":          {},
	"net.lookup_ip_addr": {},
	"opa.runtime":        {},
}

// terraformLib is the data document of the shared helper module.
var terraformLib = ast.Ref{
	ast.DefaultRootDocument,
	ast.StringTerm("ipso"),
	ast.StringTerm("lib"),
	ast.StringTerm("terraform"),
}

func vet(name string, src []byte, allowLib bool, lib []byte) error {
	mod, err := parseModule(name, src)
	if err != nil {
		return err
	}
	if err := checkImports(mod, allowLib); err != nil {
		return err
	}
	modules := map[string]*ast.Module{name: mod}
	if allowLib {
		libMod, err := parseModule(libName, lib)
		if err != nil {
			return err
		}
		if err := checkImports(libMod, false); err != nil {
			return err
		}
		modules[libName] = libMod
	}
	if err := compileModules(modules); err != nil {
		return err
	}
	if !hasMultiDeny(mod) {
		return &compiler.Error{Code: CodeRegoNoDeny, Err: fmt.Errorf("module %s", mod.Package.Path)}
	}
	return nil
}

func parseModule(name string, src []byte) (*ast.Module, error) {
	if len(src) > compiler.MaxRefBytes {
		return nil, &compiler.Error{Code: compiler.CodeRefTooLarge, Err: fmt.Errorf("%s is %d bytes", name, len(src))}
	}
	parser := ast.NewParser().
		WithFilename(name).
		WithReader(bytes.NewReader(src)).
		WithRegoVersion(ast.RegoV1).
		WithProcessAnnotation(true).
		WithMaxRecursionDepth(maxParseDepth).
		WithCapabilities(compileCaps())
	if _, _, errs := parser.Parse(); len(errs) > 0 {
		return nil, &compiler.Error{Code: CodeRegoParse, Err: errs}
	}
	mod, err := ast.ParseModuleWithOpts(name, string(src), ast.ParserOptions{
		RegoVersion:       ast.RegoV1,
		ProcessAnnotation: true,
		Capabilities:      compileCaps(),
	})
	if err != nil {
		return nil, &compiler.Error{Code: CodeRegoParse, Err: err}
	}
	return mod, nil
}

func compileModules(modules map[string]*ast.Module) error {
	if exprs(modules) > maxExprs {
		return &compiler.Error{Code: CodeRegoCompile, Err: fmt.Errorf("more than %d expressions", maxExprs)}
	}
	comp := ast.NewCompiler().
		WithStrict(true).
		WithDefaultRegoVersion(ast.RegoV1).
		WithCapabilities(compileCaps())
	comp.Compile(modules)
	if comp.Failed() {
		return &compiler.Error{Code: CodeRegoCompile, Err: comp.Errors}
	}
	return nil
}

func exprs(modules map[string]*ast.Module) int {
	n := 0
	for _, mod := range modules {
		ast.WalkExprs(mod, func(*ast.Expr) bool {
			n++
			return false
		})
	}
	return n
}

func compileCaps() *ast.Capabilities {
	caps := ast.CapabilitiesForThisVersion(ast.CapabilitiesRegoVersion(ast.RegoV1))
	kept := make([]*ast.Builtin, 0, len(caps.Builtins))
	for _, builtin := range caps.Builtins {
		if _, deny := deniedBuiltins[builtin.Name]; deny {
			continue
		}
		kept = append(kept, builtin)
	}
	caps.Builtins = kept
	// An empty allow list means no host, including schema fetches.
	caps.AllowNet = []string{}
	return caps
}

func checkImports(mod *ast.Module, allowLib bool) error {
	pkg := mod.Package.Path
	for _, imp := range mod.Imports {
		path := importRef(imp)
		if path == nil {
			return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("import %s", imp)}
		}
		if importAllowed(path, pkg, allowLib) {
			continue
		}
		return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("import %s", path)}
	}
	return nil
}

func importRef(imp *ast.Import) ast.Ref {
	if imp == nil || imp.Path == nil {
		return nil
	}
	switch value := imp.Path.Value.(type) {
	case ast.Ref:
		return value
	case ast.Var:
		return ast.Ref{ast.NewTerm(value)}
	default:
		return nil
	}
}

func importAllowed(path, pkg ast.Ref, allowLib bool) bool {
	if path.Equal(ast.RegoV1CompatibleRef) || path.HasPrefix(ast.FutureKeywordsRef) {
		return true
	}
	if len(path) == 1 && path[0].Value.Compare(ast.InputRootDocument.Value) == 0 {
		return true
	}
	if path.HasPrefix(pkg) {
		return true
	}
	return allowLib && path.HasPrefix(terraformLib)
}

func hasMultiDeny(mod *ast.Module) bool {
	for _, rule := range mod.Rules {
		if rule == nil || rule.Head == nil || rule.Head.Name != ast.Var("deny") {
			continue
		}
		if rule.Head.Value == nil && rule.Head.Key == nil {
			continue
		}
		if rule.Head.RuleKind() == ast.MultiValue {
			return true
		}
	}
	return false
}
