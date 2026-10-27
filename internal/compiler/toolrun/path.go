package toolrun

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// pathSegment is one slash-separated piece of a scan path.
// A hyphen is allowed inside a segment. A path that starts with a hyphen is rejected separately,
// because the engine would read it as a flag.
var pathSegment = regexp.MustCompile(`^[A-Za-z0-9._@+-]+$`)

// EscapeError means a named path resolves through a symlink to a location outside root.
type EscapeError struct {
	Path string
}

func (e *EscapeError) Error() string {
	return e.Path + " leaves the repository"
}

// NormalizePath checks one path and returns its cleaned form.
// A path that starts with a hyphen is rejected so it cannot be read as a flag.
func NormalizePath(value string) (string, error) {
	if value == "" || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return "", fmt.Errorf("path %q", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == ".." || !pathSegment.MatchString(segment) {
			return "", fmt.Errorf("path %q", value)
		}
	}
	cleaned := path.Clean(value)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "-") {
		return "", fmt.Errorf("path %q", value)
	}
	return cleaned, nil
}

// NormalizePaths checks every path. The order is kept.
func NormalizePaths(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("no paths")
	}
	out := make([]string, len(values))
	for i, value := range values {
		cleaned, err := NormalizePath(value)
		if err != nil {
			return nil, err
		}
		out[i] = cleaned
	}
	return out, nil
}

// WithinRoot reports whether every path stays inside root after symlink resolution.
// A missing path is left for the engine. "." is the repository root and is not walked.
// A symlink that leaves root is an *EscapeError.
func WithinRoot(root string, paths []string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rootResolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	for _, rel := range paths {
		if rel == "." {
			continue
		}
		current := rootResolved
		for _, segment := range strings.Split(rel, "/") {
			next := filepath.Join(current, segment)
			info, err := os.Lstat(next)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				current = next
				continue
			}
			resolved, err := filepath.EvalSymlinks(next)
			if err != nil {
				return err
			}
			if !insideRoot(rootResolved, resolved) {
				return &EscapeError{Path: rel}
			}
			current = resolved
		}
	}
	return nil
}

func insideRoot(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
