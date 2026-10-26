package kyverno

import (
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/ast"
	"gopkg.in/yaml.v3"
)

// deniedCEL are Kyverno context variables. http and image reach the network or a
// registry. resource, globalContext, and generator read cluster state, which an
// offline verdict cannot reproduce.
// image("ref") is a function call, not this variable, so it is not an identifier
// here. image.GetMetadata is a use of the variable and is rejected.
var deniedCEL = map[string]struct{}{
	"generator":     {},
	"globalContext": {},
	"http":          {},
	"image":         {},
	"resource":      {},
}

const (
	celDepth = 64
	celSize  = 100000
)

var (
	celOnce sync.Once
	celEnv  *cel.Env
	celErr  error
)

func celParser() (*cel.Env, error) {
	celOnce.Do(func() {
		celEnv, celErr = cel.NewEnv(
			cel.ParserRecursionLimit(celDepth),
			cel.ParserExpressionSizeLimit(celSize),
		)
	})
	return celEnv, celErr
}

func checkCEL(root *yaml.Node) error {
	for _, expr := range expressions(root) {
		if err := checkExpression(expr); err != nil {
			return err
		}
	}
	return nil
}

func expressions(root *yaml.Node) []string {
	spec := child(root, "spec")
	var out []string
	out = append(out, field(spec, "validations", "expression")...)
	out = append(out, field(spec, "validations", "messageExpression")...)
	out = append(out, field(spec, "matchConditions", "expression")...)
	out = append(out, field(spec, "variables", "expression")...)
	out = append(out, field(spec, "auditAnnotations", "valueExpression")...)
	return out
}

func field(spec *yaml.Node, listName, key string) []string {
	list := child(spec, listName)
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, item := range list.Content {
		value := child(item, key)
		if value != nil && isString(value) && value.Value != "" {
			out = append(out, value.Value)
		}
	}
	return out
}

func child(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func checkExpression(expr string) error {
	env, err := celParser()
	if err != nil {
		return fail(CodeCEL, "parser: %v", err)
	}
	parsed, iss := env.Parse(expr)
	if iss != nil && iss.Err() != nil {
		return fail(CodeCEL, "%s", iss.Err())
	}
	var denied string
	ast.PreOrderVisit(parsed.NativeRep().Expr(), ast.NewExprVisitor(func(e ast.Expr) {
		if denied != "" || e.Kind() != ast.IdentKind {
			return
		}
		name := e.AsIdent()
		if _, ok := deniedCEL[name]; ok {
			denied = name
		}
	}))
	if denied != "" {
		return fail(CodeCEL, "context %s", denied)
	}
	return nil
}
