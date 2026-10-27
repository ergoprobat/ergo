package depcruise

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// rejectedOptions are dependency-cruiser options this compiler refuses.
// extends is rejected separately because it sits on the configuration root.
//
// webpackConfig, babelConfig, cache, externalModuleResolutionStrategy, and parser
// load code or select another compiler. detectJSDocImports and
// detectImportsInAmbientModules force parser tsc. preserveSymlinks lets a later
// cruise follow a symlink that the path checks never opened. knownViolations,
// baseline, and ignoreKnown grandfather violations outside ipso. focus, reaches,
// highlight, collapse, affected, prefix, suffix, progress, and reporterOptions
// only shape a report. doNotFollow, exclude, and includeOnly are regular
// expressions, not repository paths, so compile does not resolve them.
var rejectedOptions = []string{
	"affected",
	"babelConfig",
	"baseline",
	"cache",
	"collapse",
	"detectImportsInAmbientModules",
	"detectJSDocImports",
	"externalModuleResolutionStrategy",
	"focus",
	"highlight",
	"ignoreKnown",
	"knownViolations",
	"parser",
	"prefix",
	"preserveSymlinks",
	"progress",
	"reaches",
	"reporterOptions",
	"suffix",
	"webpackConfig",
}

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

func configurationSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		schemaCompiler := jsonschema.NewCompiler()
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(configurationSchemaJSON))
		if err != nil {
			schemaErr = fmt.Errorf("depcruise schema: %w", err)
			return
		}
		root, ok := document.(map[string]any)
		if !ok {
			schemaErr = errors.New("depcruise schema: not an object")
			return
		}
		id, _ := root["$id"].(string)
		if id == "" {
			schemaErr = errors.New("depcruise schema: missing $id")
			return
		}
		if err := schemaCompiler.AddResource(id, document); err != nil {
			schemaErr = fmt.Errorf("depcruise schema: %w", err)
			return
		}
		schema, err = schemaCompiler.Compile(id)
		if err != nil {
			schemaErr = fmt.Errorf("depcruise schema: %w", err)
		}
	})
	return schema, schemaErr
}

func decodeObject(src []byte) (map[string]any, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(src))
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &compiler.Error{Code: CodeJSON, Err: errors.New("empty file")}
		}
		return nil, &compiler.Error{Code: CodeJSON, Err: err}
	}
	doc, ok := document.(map[string]any)
	if !ok {
		return nil, &compiler.Error{Code: CodeJSON, Err: errors.New("not a JSON object")}
	}
	return doc, nil
}

func validateSchema(doc map[string]any) error {
	compiled, err := configurationSchema()
	if err != nil {
		return schemaLoadError(err)
	}
	if err := compiled.Validate(doc); err != nil {
		return schemaFailure(err)
	}
	return nil
}

func schemaLoadError(err error) error {
	if err == nil {
		return nil
	}
	var coded *compiler.Error
	if errors.As(err, &coded) {
		return err
	}
	return &compiler.Error{Code: CodeSchema, Err: err}
}

func schemaFailure(err error) error {
	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		return &compiler.Error{Code: CodeSchema, Err: err}
	}
	for _, detail := range invalid.DetailedOutput().Errors {
		if detail.Error == nil {
			continue
		}
		location := detail.InstanceLocation
		if location == "" {
			location = "/"
		}
		return &compiler.Error{Code: CodeSchema, Err: fmt.Errorf("%s: %s", location, detail.Error)}
	}
	return &compiler.Error{Code: CodeSchema, Err: errors.New(invalid.Error())}
}

func rejectOptions(doc map[string]any) error {
	if _, ok := doc["extends"]; ok {
		return &compiler.Error{Code: CodeOption, Err: errors.New("extends")}
	}
	raw, ok := doc["options"]
	if !ok {
		return nil
	}
	options, ok := raw.(map[string]any)
	if !ok {
		return &compiler.Error{Code: CodeSchema, Err: errors.New("options is not an object")}
	}
	for _, key := range rejectedOptions {
		if _, ok := options[key]; ok {
			return &compiler.Error{Code: CodeOption, Err: fmt.Errorf("options.%s", key)}
		}
	}
	return nil
}

func checkPaths(doc map[string]any, repo fs.FS) error {
	raw, ok := doc["options"]
	if !ok {
		return nil
	}
	options, ok := raw.(map[string]any)
	if !ok {
		return &compiler.Error{Code: CodeSchema, Err: errors.New("options is not an object")}
	}
	if base, ok := options["baseDir"]; ok {
		name, ok := base.(string)
		if !ok {
			return &compiler.Error{Code: CodeSchema, Err: errors.New("options.baseDir is not a string")}
		}
		if err := requireRepoPath(repo, "options.baseDir", name, true); err != nil {
			return err
		}
	}
	rawConfig, ok := options["tsConfig"]
	if !ok {
		return nil
	}
	tsConfig, ok := rawConfig.(map[string]any)
	if !ok {
		return &compiler.Error{Code: CodeSchema, Err: errors.New("options.tsConfig is not an object")}
	}
	rawName, ok := tsConfig["fileName"]
	if !ok {
		return nil
	}
	name, ok := rawName.(string)
	if !ok {
		return &compiler.Error{Code: CodeSchema, Err: errors.New("options.tsConfig.fileName is not a string")}
	}
	return requireRepoPath(repo, "options.tsConfig.fileName", name, false)
}

func requireRepoPath(repo fs.FS, label, name string, wantDir bool) error {
	if !fs.ValidPath(name) {
		return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s %q leaves the repository", label, name)}
	}
	if repo == nil {
		return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s %q: no repository", label, name)}
	}
	info, err := fs.Stat(repo, name)
	if err != nil {
		return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s %q: %w", label, name, err)}
	}
	if wantDir && !info.IsDir() {
		return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s %q is not a directory", label, name)}
	}
	if !wantDir && info.IsDir() {
		return &compiler.Error{Code: CodePath, Err: fmt.Errorf("%s %q is a directory", label, name)}
	}
	return nil
}

func checkRules(doc map[string]any) error {
	seen := map[string]struct{}{}
	total := 0
	for _, key := range []string{"forbidden", "allowed", "required"} {
		items, err := objectArray(doc, key)
		if err != nil {
			return err
		}
		total += len(items)
		if key == "allowed" {
			continue
		}
		for index, item := range items {
			rule, ok := item.(map[string]any)
			if !ok {
				return &compiler.Error{Code: CodeSchema, Err: fmt.Errorf("%s[%d] is not an object", key, index)}
			}
			name, _ := rule["name"].(string)
			if name == "" {
				return &compiler.Error{Code: CodeRuleName, Err: fmt.Errorf("%s[%d] has no name", key, index)}
			}
			if _, ok := seen[name]; ok {
				return &compiler.Error{Code: CodeRuleName, Err: fmt.Errorf("duplicate name %q", name)}
			}
			seen[name] = struct{}{}
		}
	}
	if total == 0 {
		return &compiler.Error{Code: CodeNoRules, Err: errors.New("no rules")}
	}
	return nil
}

func rewriteSeverity(doc map[string]any, mode string) error {
	severity, err := severityFor(mode)
	if err != nil {
		return err
	}
	for _, key := range []string{"forbidden", "required"} {
		items, err := objectArray(doc, key)
		if err != nil {
			return err
		}
		for index, item := range items {
			rule, ok := item.(map[string]any)
			if !ok {
				return &compiler.Error{Code: CodeSchema, Err: fmt.Errorf("%s[%d] is not an object", key, index)}
			}
			if current, _ := rule["severity"].(string); current == "ignore" {
				continue
			}
			rule["severity"] = severity
		}
	}
	if current, ok := doc["allowedSeverity"].(string); ok {
		if current != "ignore" {
			doc["allowedSeverity"] = severity
		}
		return nil
	}
	allowed, err := objectArray(doc, "allowed")
	if err != nil {
		return err
	}
	if len(allowed) > 0 {
		doc["allowedSeverity"] = severity
	}
	return nil
}

func objectArray(doc map[string]any, key string) ([]any, error) {
	raw, ok := doc[key]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, &compiler.Error{Code: CodeSchema, Err: fmt.Errorf("%s is not an array", key)}
	}
	return items, nil
}
