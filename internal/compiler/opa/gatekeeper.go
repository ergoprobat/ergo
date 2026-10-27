package opa

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/strictyaml"
	"gopkg.in/yaml.v3"
)

const (
	// CodeGatekeeperYAML means ref is not a YAML file, or the file is not two plain documents.
	CodeGatekeeperYAML = "gatekeeper-yaml"
	// CodeGatekeeperTemplate means the ConstraintTemplate has the wrong shape.
	CodeGatekeeperTemplate = "gatekeeper-template"
	// CodeGatekeeperConstraint means the Constraint has the wrong shape.
	CodeGatekeeperConstraint = "gatekeeper-constraint"
	// CodeGatekeeperKey means the file sets a key this compiler rejects.
	CodeGatekeeperKey = "gatekeeper-key"
	// CodeRegoNoViolation means the template has no multi-value violation rule.
	CodeRegoNoViolation = "rego-no-violation"

	// GatorVersion is the gator release the conformance suite runs.
	GatorVersion = "3.23.1"
	// GatorOPA is the OPA version embedded in that gator release.
	// Rego is type-checked against this builtin set.
	GatorOPA = "v1.17.1"

	templateVersion    = "templates.gatekeeper.sh/v1"
	constraintVersion  = "constraints.gatekeeper.sh/v1beta1"
	templateKind       = "ConstraintTemplate"
	admissionTarget    = "admission.k8s.gatekeeper.sh"
	regoEngine         = "Rego"
	regoVersion        = "v1"
	templateArtifact   = "constraint-template.yaml"
	constraintArtifact = "constraint.yaml"
)

var (
	constraintKindName = regexp.MustCompile(`^[A-Z][A-Za-z0-9]*$`)
	gatekeeperDNS      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
)

var gatekeeperRootKeys = map[string]struct{}{
	"apiVersion": {},
	"kind":       {},
	"metadata":   {},
	"spec":       {},
}

var gatekeeperMetaKeys = map[string]struct{}{
	"annotations": {},
	"labels":      {},
	"name":        {},
}

func compileGatekeeper(in compiler.Input) ([]compiler.Artifact, error) {
	mode, err := in.Check.EffectiveMode(in.Record.Criticality)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(path.Ext(in.Check.Ref))
	if ext != ".yaml" && ext != ".yml" {
		return nil, gatekeeperErr(CodeGatekeeperYAML, "ref %q is not a YAML file", in.Check.Ref)
	}
	src, err := compiler.ReadRef(in)
	if err != nil {
		return nil, err
	}
	docs, err := strictyaml.Documents(src, CodeGatekeeperYAML, 2)
	if err != nil {
		return nil, err
	}
	regoSrc, libs, kind, err := checkTemplate(docs[0])
	if err != nil {
		return nil, err
	}
	if err := checkConstraint(docs[1], kind); err != nil {
		return nil, err
	}
	if err := vetGatekeeper(regoSrc, libs); err != nil {
		return nil, err
	}
	if err := setEnforcement(gkChild(docs[1], "spec"), mode); err != nil {
		return nil, err
	}
	template, err := strictyaml.Canonical(docs[0])
	if err != nil {
		return nil, err
	}
	constraint, err := strictyaml.Canonical(docs[1])
	if err != nil {
		return nil, err
	}
	return []compiler.Artifact{
		{Name: templateArtifact, Data: template},
		{Name: constraintArtifact, Data: constraint},
	}, nil
}

func checkTemplate(root *yaml.Node) (regoSrc string, libs []string, kind string, err error) {
	fields, err := gkMapping(root, gatekeeperRootKeys, CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, "", err
	}
	version, err := gkRequired(fields, "apiVersion", CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, "", err
	}
	docKind, err := gkRequired(fields, "kind", CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, "", err
	}
	if version != templateVersion || docKind != templateKind {
		return "", nil, "", gatekeeperErr(CodeGatekeeperTemplate, "kind %s %s", version, docKind)
	}
	meta, ok := fields["metadata"]
	if !ok {
		return "", nil, "", gatekeeperErr(CodeGatekeeperTemplate, "metadata is required")
	}
	name, err := checkGatekeeperMeta(meta, CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, "", err
	}
	spec, ok := fields["spec"]
	if !ok {
		return "", nil, "", gatekeeperErr(CodeGatekeeperTemplate, "spec is required")
	}
	kind, regoSrc, libs, err = checkTemplateSpec(spec)
	if err != nil {
		return "", nil, "", err
	}
	if name != strings.ToLower(kind) {
		return "", nil, "", gatekeeperErr(CodeGatekeeperTemplate, "metadata.name %q", name)
	}
	return regoSrc, libs, kind, nil
}

func checkTemplateSpec(node *yaml.Node) (kind, regoSrc string, libs []string, err error) {
	fields, err := gkMapping(node, map[string]struct{}{"crd": {}, "targets": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", "", nil, err
	}
	crd, ok := fields["crd"]
	if !ok {
		return "", "", nil, gatekeeperErr(CodeGatekeeperTemplate, "crd is required")
	}
	kind, err = checkCRD(crd)
	if err != nil {
		return "", "", nil, err
	}
	targets, ok := fields["targets"]
	if !ok {
		return "", "", nil, gatekeeperErr(CodeGatekeeperTemplate, "targets is required")
	}
	regoSrc, libs, err = checkTargets(targets)
	if err != nil {
		return "", "", nil, err
	}
	return kind, regoSrc, libs, nil
}

func checkCRD(node *yaml.Node) (string, error) {
	fields, err := gkMapping(node, map[string]struct{}{"spec": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", err
	}
	spec, ok := fields["spec"]
	if !ok {
		return "", gatekeeperErr(CodeGatekeeperTemplate, "crd.spec is required")
	}
	body, err := gkMapping(spec, map[string]struct{}{"names": {}, "validation": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", err
	}
	names, ok := body["names"]
	if !ok {
		return "", gatekeeperErr(CodeGatekeeperTemplate, "names is required")
	}
	nameFields, err := gkMapping(names, map[string]struct{}{"kind": {}, "shortNames": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", err
	}
	kind, err := gkRequired(nameFields, "kind", CodeGatekeeperTemplate)
	if err != nil {
		return "", err
	}
	if !constraintKindName.MatchString(kind) {
		return "", gatekeeperErr(CodeGatekeeperTemplate, "kind %q", kind)
	}
	if short, ok := nameFields["shortNames"]; ok {
		if err := gkStringList(short, CodeGatekeeperTemplate, true); err != nil {
			return "", err
		}
	}
	validation, ok := body["validation"]
	if !ok {
		return "", gatekeeperErr(CodeGatekeeperTemplate, "validation is required")
	}
	valid, err := gkMapping(validation, map[string]struct{}{"openAPIV3Schema": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", err
	}
	schema, ok := valid["openAPIV3Schema"]
	if !ok {
		return "", gatekeeperErr(CodeGatekeeperTemplate, "openAPIV3Schema is required")
	}
	if err := checkSchema(schema); err != nil {
		return "", err
	}
	return kind, nil
}

func checkTargets(node *yaml.Node) (string, []string, error) {
	items, err := gkSequence(node, CodeGatekeeperTemplate)
	if err != nil || len(items) != 1 {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "expected one target")
	}
	fields, err := gkMapping(items[0], map[string]struct{}{
		"code":       {},
		"operations": {},
		"target":     {},
	}, CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	target, err := gkRequired(fields, "target", CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	if target != admissionTarget {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "target %q", target)
	}
	if ops, ok := fields["operations"]; ok {
		if err := gkEnumList(ops, CodeGatekeeperTemplate, "CREATE", "UPDATE", "DELETE", "CONNECT", "*"); err != nil {
			return "", nil, err
		}
	}
	code, ok := fields["code"]
	if !ok {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "code is required")
	}
	return checkCode(code)
}

func checkCode(node *yaml.Node) (string, []string, error) {
	items, err := gkSequence(node, CodeGatekeeperTemplate)
	if err != nil || len(items) != 1 {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "expected one Rego engine")
	}
	fields, err := gkMapping(items[0], map[string]struct{}{"engine": {}, "source": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	engine, err := gkRequired(fields, "engine", CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	if engine != regoEngine {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "engine %q", engine)
	}
	source, ok := fields["source"]
	if !ok {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "source is required")
	}
	body, err := gkMapping(source, map[string]struct{}{"libs": {}, "rego": {}, "version": {}}, CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	version, err := gkRequired(body, "version", CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	if version != regoVersion {
		return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "version %q", version)
	}
	regoSrc, err := gkRequired(body, "rego", CodeGatekeeperTemplate)
	if err != nil {
		return "", nil, err
	}
	var libs []string
	if value, ok := body["libs"]; ok {
		items, err := gkSequence(value, CodeGatekeeperTemplate)
		if err != nil || len(items) == 0 {
			return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "libs is empty")
		}
		for _, item := range items {
			if !gkString(item) || item.Value == "" {
				return "", nil, gatekeeperErr(CodeGatekeeperTemplate, "lib is not a string")
			}
			libs = append(libs, item.Value)
		}
	}
	return regoSrc, libs, nil
}

func checkConstraint(root *yaml.Node, kind string) error {
	fields, err := gkMapping(root, gatekeeperRootKeys, CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	version, err := gkRequired(fields, "apiVersion", CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	docKind, err := gkRequired(fields, "kind", CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	if version != constraintVersion || docKind != kind {
		return gatekeeperErr(CodeGatekeeperConstraint, "kind %s %s", version, docKind)
	}
	meta, ok := fields["metadata"]
	if !ok {
		return gatekeeperErr(CodeGatekeeperConstraint, "metadata is required")
	}
	if _, err := checkGatekeeperMeta(meta, CodeGatekeeperConstraint); err != nil {
		return err
	}
	spec, ok := fields["spec"]
	if !ok {
		return gatekeeperErr(CodeGatekeeperConstraint, "spec is required")
	}
	return checkConstraintSpec(spec)
}

func checkConstraintSpec(node *yaml.Node) error {
	fields, err := gkMapping(node, map[string]struct{}{
		"enforcementAction": {},
		"match":             {},
		"parameters":        {},
	}, CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	if value, ok := fields["enforcementAction"]; ok {
		if err := gkOneOf(value, CodeGatekeeperConstraint, "deny", "dryrun", "warn"); err != nil {
			return err
		}
	}
	match, ok := fields["match"]
	if !ok {
		return gatekeeperErr(CodeGatekeeperConstraint, "match is required")
	}
	if err := checkMatch(match); err != nil {
		return err
	}
	if value, ok := fields["parameters"]; ok && (value.Kind != yaml.MappingNode) {
		return gatekeeperErr(CodeGatekeeperConstraint, "parameters is not a mapping")
	}
	return nil
}

func checkMatch(node *yaml.Node) error {
	fields, err := gkMapping(node, map[string]struct{}{
		"excludedNamespaces": {},
		"kinds":              {},
		"labelSelector":      {},
		"name":               {},
		"namespaceSelector":  {},
		"namespaces":         {},
		"scope":              {},
		"source":             {},
	}, CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	if value, ok := fields["kinds"]; ok {
		if err := checkKinds(value); err != nil {
			return err
		}
	}
	for _, key := range []string{"namespaces", "excludedNamespaces"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if err := gkStringList(value, CodeGatekeeperConstraint, true); err != nil {
			return err
		}
	}
	for _, key := range []string{"labelSelector", "namespaceSelector"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if err := checkLabelSelector(value); err != nil {
			return err
		}
	}
	if value, ok := fields["scope"]; ok {
		if err := gkOneOf(value, CodeGatekeeperConstraint, "*", "Cluster", "Namespaced"); err != nil {
			return err
		}
	}
	for _, key := range []string{"name", "source"} {
		if value, ok := fields[key]; ok && !gkString(value) {
			return gatekeeperErr(CodeGatekeeperConstraint, "%s is not a string", key)
		}
	}
	return nil
}

func checkKinds(node *yaml.Node) error {
	items, err := gkSequence(node, CodeGatekeeperConstraint)
	if err != nil || len(items) == 0 {
		return gatekeeperErr(CodeGatekeeperConstraint, "kinds is required")
	}
	for _, item := range items {
		fields, err := gkMapping(item, map[string]struct{}{"apiGroups": {}, "kinds": {}}, CodeGatekeeperConstraint)
		if err != nil {
			return err
		}
		if _, err := gkRequiredList(fields, "kinds"); err != nil {
			return err
		}
		if value, ok := fields["apiGroups"]; ok {
			if err := gkStringList(value, CodeGatekeeperConstraint, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkLabelSelector(node *yaml.Node) error {
	fields, err := gkMapping(node, map[string]struct{}{"matchExpressions": {}, "matchLabels": {}}, CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	if value, ok := fields["matchLabels"]; ok {
		if err := gkStringMap(value, CodeGatekeeperConstraint); err != nil {
			return err
		}
	}
	value, ok := fields["matchExpressions"]
	if !ok {
		return nil
	}
	items, err := gkSequence(value, CodeGatekeeperConstraint)
	if err != nil {
		return err
	}
	for _, item := range items {
		req, err := gkMapping(item, map[string]struct{}{"key": {}, "operator": {}, "values": {}}, CodeGatekeeperConstraint)
		if err != nil {
			return err
		}
		if _, err := gkRequired(req, "key", CodeGatekeeperConstraint); err != nil {
			return err
		}
		if err := gkOneOf(req["operator"], CodeGatekeeperConstraint, "In", "NotIn", "Exists", "DoesNotExist"); err != nil {
			return err
		}
		if values, ok := req["values"]; ok {
			if err := gkStringList(values, CodeGatekeeperConstraint, true); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkGatekeeperMeta(node *yaml.Node, code string) (string, error) {
	fields, err := gkMapping(node, gatekeeperMetaKeys, code)
	if err != nil {
		return "", err
	}
	name, err := gkRequired(fields, "name", code)
	if err != nil {
		return "", err
	}
	if len(name) > 253 || !gatekeeperDNS.MatchString(name) {
		return "", gatekeeperErr(code, "metadata.name %q", name)
	}
	for _, key := range []string{"labels", "annotations"} {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if err := gkStringMap(value, code); err != nil {
			return "", err
		}
	}
	return name, nil
}

func setEnforcement(spec *yaml.Node, mode string) error {
	action, err := enforcementFor(mode)
	if err != nil {
		return err
	}
	content := spec.Content
	kept := make([]*yaml.Node, 0, len(content))
	for i := 0; i < len(content); i += 2 {
		if content[i].Value == "enforcementAction" {
			continue
		}
		kept = append(kept, content[i], content[i+1])
	}
	spec.Content = append(kept,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "enforcementAction"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: action},
	)
	return nil
}

func enforcementFor(mode string) (string, error) {
	switch mode {
	case "block":
		return "deny", nil
	case "warn":
		return "warn", nil
	case "advise":
		return "dryrun", nil
	default:
		return "", gatekeeperErr(CodeGatekeeperConstraint, "mode %q", mode)
	}
}

func gatekeeperErr(code, format string, args ...any) error {
	return &compiler.Error{Code: code, Err: fmt.Errorf(format, args...)}
}
