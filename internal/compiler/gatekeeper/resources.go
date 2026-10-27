package gatekeeper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/manifests"
)

// gatekeeperDomain is in the apiVersion of every Gatekeeper object: templates, constraints, expansion
// templates, mutators, and config. Gator reads those from the same files as the objects it reviews.
const gatekeeperDomain = "gatekeeper.sh"

// stageManifests copies the manifests of the named paths into dest, the one flat directory gator is
// given, and returns the repository path each copy came from. manifests holds the copy rules.
func stageManifests(ctx context.Context, repo *os.Root, named []string, dest string) ([]string, error) {
	sources, err := manifests.Stage(ctx, repo, named, dest, manifests.MaxBytes)
	if err == nil {
		return sources, nil
	}
	var pathErr *manifests.PathError
	switch {
	case errors.As(err, &pathErr):
		return nil, &compiler.Error{Code: CodePath, Err: err}
	case errors.Is(err, manifests.ErrEmpty):
		return nil, &compiler.Error{Code: CodeEmpty, Err: err}
	default:
		return nil, &compiler.Error{Code: CodeRun, Err: err}
	}
}

// checkDocuments reads every document of the copied manifests, which sources names in copy order, and
// refuses what could change the policy gator runs or hide an object from it.
//
// Gator reads templates and constraints from the same files as the objects it reviews, so a repository
// that held its own would add or replace a policy. It does not review the items of a list, so a list
// would pass unchecked. And it reads YAML more loosely than ipso does: a repeated key or a merge key
// can make a document a Gatekeeper object that a plain reading would not show. The bytes are not
// changed, so gator still reads what the repository wrote. Wherever the two readers can differ, ipso
// refuses the document. A tree with no object at all is an error, because it would pass with nothing checked.
func checkDocuments(ctx context.Context, dir string, sources []string) error {
	objects := 0
	for i, source := range sources {
		found, err := checkFile(ctx, filepath.Join(dir, fmt.Sprintf("%06d.yaml", i+1)), source)
		if err != nil {
			return err
		}
		objects += found
	}
	if objects == 0 {
		return &compiler.Error{Code: CodeEmpty, Err: errors.New("the named paths hold no Kubernetes object")}
	}
	return nil
}

// checkFile checks the documents of one copied file and returns how many are objects.
func checkFile(ctx context.Context, copyPath, source string) (int, error) {
	file, err := os.Open(copyPath)
	if err != nil {
		return 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = file.Close() }()
	dec := yaml.NewDecoder(bufio.NewReader(file))
	objects := 0
	for n := 1; ; n++ {
		if err := ctx.Err(); err != nil {
			return 0, &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
		}
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return objects, nil
		}
		if err != nil {
			return 0, resourceError(source, n, "cannot be read as YAML")
		}
		isObject, reason := classify(&doc)
		if reason != "" {
			return 0, resourceError(source, n, reason)
		}
		if isObject {
			objects++
		}
	}
}

func resourceError(source string, n int, reason string) error {
	return &compiler.Error{Code: CodeResource, Err: fmt.Errorf("%s document %d %s", compiler.EscapeControls(source), n, reason)}
}

// classify reports whether a document is an object, and the reason ipso refuses it when it does.
// A document with no content or an empty mapping holds no object: gator skips it too.
func classify(doc *yaml.Node) (isObject bool, refusal string) {
	if len(doc.Content) == 0 {
		return false, ""
	}
	root := doc.Content[0]
	switch {
	case root.Kind == yaml.ScalarNode && root.ShortTag() == "!!null":
		return false, ""
	case root.Kind != yaml.MappingNode:
		return false, "is not a mapping"
	case len(root.Content) == 0:
		return false, ""
	}
	var apiVersion, kind string
	hasItems := false
	seen := map[string]struct{}{}
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if !isPlainString(key) {
			return false, "has a key that is not a plain string"
		}
		if _, ok := seen[key.Value]; ok {
			return false, "repeats a key"
		}
		seen[key.Value] = struct{}{}
		switch key.Value {
		case "apiVersion":
			text, ok := stringValue(value)
			if !ok {
				return false, "has an apiVersion that is not a plain string"
			}
			apiVersion = text
		case "kind":
			text, ok := stringValue(value)
			if !ok {
				return false, "has a kind that is not a plain string"
			}
			kind = text
		case "items":
			// An alias to a sequence defined elsewhere in the document hides objects as well as a literal one.
			hasItems = value.Kind != yaml.ScalarNode || value.ShortTag() != "!!null"
		}
	}
	switch {
	case strings.Contains(strings.ToLower(apiVersion), gatekeeperDomain):
		return false, "is a Gatekeeper object, which the check supplies itself"
	case hasItems && strings.HasSuffix(strings.ToLower(kind), "list"):
		return false, "is a list, and gator does not review the objects inside a list"
	}
	return true, ""
}

// isPlainString reports whether node is a string scalar with no explicit tag and no alias.
// A quoted string is plain here: what matters is that the text is the text, not a tag, an alias,
// or a merge key that resolves to something else.
func isPlainString(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode && node.Style&yaml.TaggedStyle == 0 && node.ShortTag() == "!!str"
}

func stringValue(node *yaml.Node) (string, bool) {
	if !isPlainString(node) {
		return "", false
	}
	return node.Value, true
}
