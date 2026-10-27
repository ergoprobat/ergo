package opa

import (
	"bytes"
	"fmt"

	"github.com/open-policy-agent/opa/v1/ast"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// gatorDenied are host and network builtins. gator does not disable http.send,
// so a template that called it would make a network call during evaluation.
var gatorDenied = map[string]struct{}{
	"http.send":          {},
	"net.lookup_ip_addr": {},
	"opa.runtime":        {},
}

func vetGatekeeper(src string, libs []string) error {
	caps, err := gatorCaps()
	if err != nil {
		return err
	}
	main, err := parseGatekeeper("template.rego", src, caps)
	if err != nil {
		return err
	}
	if err := checkGatekeeperModule(main, false); err != nil {
		return err
	}
	modules := map[string]*ast.Module{"template.rego": main}
	for i, lib := range libs {
		name := fmt.Sprintf("lib%d.rego", i)
		mod, err := parseGatekeeper(name, lib, caps)
		if err != nil {
			return err
		}
		if err := checkGatekeeperModule(mod, true); err != nil {
			return err
		}
		modules[name] = mod
	}
	if exprs(modules) > maxExprs {
		return &compiler.Error{Code: CodeRegoCompile, Err: fmt.Errorf("more than %d expressions", maxExprs)}
	}
	comp := ast.NewCompiler().
		WithStrict(true).
		WithDefaultRegoVersion(ast.RegoV1).
		WithCapabilities(caps)
	comp.Compile(modules)
	if comp.Failed() {
		return &compiler.Error{Code: CodeRegoCompile, Err: comp.Errors}
	}
	if !hasMultiViolation(main) {
		return &compiler.Error{Code: CodeRegoNoViolation, Err: fmt.Errorf("module %s", main.Package.Path)}
	}
	return nil
}

func gatorCaps() (*ast.Capabilities, error) {
	caps, err := ast.LoadCapabilitiesVersion(GatorOPA)
	if err != nil {
		return nil, fmt.Errorf("load OPA capabilities %s: %w", GatorOPA, err)
	}
	kept := make([]*ast.Builtin, 0, len(caps.Builtins))
	for _, builtin := range caps.Builtins {
		if _, deny := gatorDenied[builtin.Name]; deny {
			continue
		}
		kept = append(kept, builtin)
	}
	caps.Builtins = kept
	caps.AllowNet = []string{}
	return caps, nil
}

func parseGatekeeper(name, src string, caps *ast.Capabilities) (*ast.Module, error) {
	if len(src) > compiler.MaxRefBytes {
		return nil, &compiler.Error{Code: compiler.CodeRefTooLarge, Err: fmt.Errorf("%s is %d bytes", name, len(src))}
	}
	// ParserOptions cannot carry the depth cap, so the depth check is a separate parse.
	parser := ast.NewParser().
		WithFilename(name).
		WithReader(bytes.NewReader([]byte(src))).
		WithRegoVersion(ast.RegoV1).
		WithMaxRecursionDepth(maxParseDepth).
		WithCapabilities(caps)
	if _, _, errs := parser.Parse(); len(errs) > 0 {
		return nil, &compiler.Error{Code: CodeRegoParse, Err: errs}
	}
	mod, err := ast.ParseModuleWithOpts(name, src, ast.ParserOptions{
		RegoVersion:  ast.RegoV1,
		Capabilities: caps,
	})
	if err != nil {
		return nil, &compiler.Error{Code: CodeRegoParse, Err: err}
	}
	return mod, nil
}

func checkGatekeeperModule(mod *ast.Module, lib bool) error {
	if err := checkGatekeeperPackage(mod, lib); err != nil {
		return err
	}
	if err := checkGatekeeperImports(mod); err != nil {
		return err
	}
	var bad error
	ast.WalkRefs(mod, func(ref ast.Ref) bool {
		if err := checkGatekeeperRef(ref, mod.Package.Path); err != nil {
			bad = err
			return true
		}
		return false
	})
	return bad
}

func checkGatekeeperPackage(mod *ast.Module, lib bool) error {
	parts, ok := refParts(mod.Package.Path)
	if !ok || len(parts) < 2 || parts[0] != "data" {
		return &compiler.Error{Code: CodeRegoCompile, Err: fmt.Errorf("package %s", mod.Package.Path)}
	}
	if parts[1] == "inventory" {
		return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("package %s", mod.Package.Path)}
	}
	if lib {
		if len(parts) < 3 || parts[1] != "lib" {
			return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("lib package %s", mod.Package.Path)}
		}
		return nil
	}
	if len(parts) != 2 || parts[1] == "lib" {
		return &compiler.Error{Code: CodeRegoCompile, Err: fmt.Errorf("package %s", mod.Package.Path)}
	}
	return nil
}

func checkGatekeeperImports(mod *ast.Module) error {
	for _, imp := range mod.Imports {
		path := importRef(imp)
		if path == nil {
			return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("import %s", imp)}
		}
		if path.Equal(ast.RegoV1CompatibleRef) || path.HasPrefix(ast.RegoV1CompatibleRef) {
			return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("import %s", path)}
		}
		if path.HasPrefix(ast.FutureKeywordsRef) {
			continue
		}
		parts, ok := refParts(path)
		if ok && len(parts) >= 3 && parts[0] == "data" && parts[1] == "lib" {
			continue
		}
		return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("import %s", path)}
	}
	return nil
}

func checkGatekeeperRef(ref, pkg ast.Ref) error {
	parts, static := refParts(ref)
	if len(parts) == 0 || parts[0] != "data" {
		return nil
	}
	if !static || (len(parts) > 1 && parts[1] == "inventory") {
		return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("ref %s", ref)}
	}
	if ref.HasPrefix(pkg) {
		return nil
	}
	if len(parts) >= 2 && parts[1] == "lib" {
		return nil
	}
	return &compiler.Error{Code: CodeRegoImport, Err: fmt.Errorf("ref %s", ref)}
}

func refParts(ref ast.Ref) ([]string, bool) {
	parts := make([]string, 0, len(ref))
	for i, term := range ref {
		switch value := term.Value.(type) {
		case ast.String:
			parts = append(parts, string(value))
		case ast.Var:
			// The data document is a var. Any other var is a dynamic ref.
			if i == 0 && value == ast.DefaultRootDocument.Value {
				parts = append(parts, string(value))
				continue
			}
			return nil, false
		default:
			return nil, false
		}
	}
	return parts, true
}

func hasMultiViolation(mod *ast.Module) bool {
	for _, rule := range mod.Rules {
		if rule == nil || rule.Head == nil || rule.Head.Name != ast.Var("violation") {
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
