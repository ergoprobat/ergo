package eslint

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// The lines of eslint.config.mjs that carry data. generate writes them in this layout.
const (
	filesPrefix   = "    files: "
	ignoresPrefix = "    ignores: "
	rulesStart    = "    rules: {"
	rulesEnd      = "    },"
	indent        = "    "
)

// readCanonical checks that config is the file Compile writes for target and mode, and returns the
// rule ids it enables.
//
// ESLint runs the config as JavaScript, so nothing but a file ipso could have written may reach it.
// The data is read back from the config, compiled again with the same rules as Compile, and the
// result must equal the verified bytes. The digest check alone accepts a manifest and an artifact
// that were edited together. Any difference is build-manifest.
func readCanonical(config []byte, target, mode string) (map[string]struct{}, error) {
	if target != targetJavaScript && target != targetTypeScript {
		return nil, notCanonical(fmt.Errorf("target %q", target))
	}
	document, err := recoverDocument(config)
	if err != nil {
		return nil, notCanonical(err)
	}
	again, err := compileRules(document, target, mode)
	if err != nil {
		return nil, notCanonical(err)
	}
	if !bytes.Equal(again, config) {
		return nil, notCanonical(errors.New("it differs from the config ipso writes for this target and mode"))
	}
	return ruleIDs(document)
}

func notCanonical(err error) error {
	return &compiler.Error{Code: compiler.CodeBuildManifest, Err: fmt.Errorf("%s is not the config ipso compiles: %w", artifactName, err)}
}

// recoverDocument reads the files, ignores, and rules values out of a generated config and returns
// them as the JSON rule file that produced it. It reads by layout and checks nothing else: the
// caller compiles the result again and compares the bytes.
func recoverDocument(config []byte) ([]byte, error) {
	lines := strings.Split(string(config), "\n")
	var files, ignores string
	var haveFiles, haveIgnores bool
	rulesAt := -1
	for i, line := range lines {
		switch {
		case !haveFiles && strings.HasPrefix(line, filesPrefix):
			value, err := trailingComma(strings.TrimPrefix(line, filesPrefix))
			if err != nil {
				return nil, err
			}
			files, haveFiles = value, true
		case !haveIgnores && strings.HasPrefix(line, ignoresPrefix):
			value, err := trailingComma(strings.TrimPrefix(line, ignoresPrefix))
			if err != nil {
				return nil, err
			}
			ignores, haveIgnores = value, true
		case rulesAt < 0 && line == rulesStart:
			rulesAt = i
		}
	}
	if !haveFiles {
		return nil, errors.New("no files line")
	}
	if rulesAt < 0 {
		return nil, errors.New("no rules block")
	}
	rules, err := rulesBlock(lines[rulesAt:])
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(`{"files":`)
	b.WriteString(files)
	if haveIgnores {
		b.WriteString(`,"ignores":`)
		b.WriteString(ignores)
	}
	b.WriteString(`,"rules":`)
	b.WriteString(rules)
	b.WriteString("}")
	return []byte(b.String()), nil
}

func trailingComma(value string) (string, error) {
	text, ok := strings.CutSuffix(value, ",")
	if !ok {
		return "", errors.New("a data line does not end with a comma")
	}
	return text, nil
}

// rulesBlock joins the lines of the rules object. The first line is `    rules: {` and the object ends at
// the first line that is exactly `    },`. A nested object closes at a deeper indent, so that line is unique.
func rulesBlock(lines []string) (string, error) {
	var b strings.Builder
	b.WriteString("{")
	for _, line := range lines[1:] {
		if line == rulesEnd {
			b.WriteString("\n}")
			return b.String(), nil
		}
		text, ok := strings.CutPrefix(line, indent)
		if !ok {
			return "", errors.New("a line of the rules block is not indented")
		}
		b.WriteString("\n")
		b.WriteString(text)
	}
	return "", errors.New("the rules block does not end")
}

func ruleIDs(document []byte) (map[string]struct{}, error) {
	value, err := decodeJSON(document)
	if err != nil {
		return nil, err
	}
	doc, ok := value.(map[string]any)
	if !ok {
		return nil, notCanonical(errors.New("not an object"))
	}
	rules, ok := doc["rules"].(map[string]any)
	if !ok {
		return nil, notCanonical(errors.New("no rules"))
	}
	ids := make(map[string]struct{}, len(rules))
	for id := range rules {
		ids[id] = struct{}{}
	}
	return ids, nil
}
