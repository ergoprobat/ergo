// Package manifests copies the Kubernetes manifests below named repository paths into one flat
// directory, so an engine reads a private copy and never the repository itself.
package manifests

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ipsofacto-io/ipso/internal/compiler"
)

// MaxBytes bounds the manifests copied for one run.
const MaxBytes int64 = 256 << 20

// ErrEmpty means the named paths hold no .yaml or .yml file.
var ErrEmpty = errors.New("the named paths hold no .yaml or .yml file")

// PathError means a named path, or something below it, cannot be copied: it is missing, is not a
// manifest file or a directory, holds a symlink or a file that is not regular, or the manifests are
// larger than the limit. Any other error from Stage is a failure to copy, not a problem with the paths.
type PathError struct {
	Reason string
}

func (e *PathError) Error() string { return e.Reason }

// pathError escapes control characters in its string arguments. A file name below a named directory
// is the repository's own text, and the message goes to the terminal.
func pathError(format string, args ...any) error {
	for i, arg := range args {
		if text, ok := arg.(string); ok {
			args[i] = compiler.EscapeControls(text)
		}
	}
	return &PathError{Reason: fmt.Sprintf(format, args...)}
}

// Distinct returns paths without repeats, in their first order.
func Distinct(paths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

// copier copies the manifests below the named paths into one flat directory, a file for each
// source file, numbered in the order they are found.
//
// An engine that reads a directory flat, or that takes one flag for each file, would miss manifests in
// subdirectories or hit the command line limit of Windows. The copy also keeps repository symlinks and
// later edits out of the run. A symlink anywhere below a named directory is refused: an engine would
// not follow it, so the manifests behind it would go unchecked.
type copier struct {
	repo    *os.Root
	dest    string
	limit   int64
	sources []string
	total   int64
	seen    map[string]struct{}
}

// Stage copies the manifests of every named path into dest, which it creates, and returns the
// repository path each copy came from: the file 000001.yaml is the first of them. A named path is a manifest file or a directory. Below a directory, entries that
// start with a dot are skipped, and so is everything that does not end in .yaml or .yml, in any case.
// Files are copied as 000001.yaml, 000002.yaml, and so on, in argument order and then lexical order.
// A named path that is itself a symlink is followed, because the caller chose it, so the caller checks
// that it stays inside the repository.
func Stage(ctx context.Context, repo *os.Root, named []string, dest string, limit int64) ([]string, error) {
	if err := os.Mkdir(dest, 0o700); err != nil {
		return nil, err
	}
	c := &copier{repo: repo, dest: dest, limit: limit, seen: map[string]struct{}{}}
	for _, name := range named {
		if err := c.collect(ctx, name); err != nil {
			return nil, err
		}
	}
	if len(c.sources) == 0 {
		return nil, ErrEmpty
	}
	return c.sources, nil
}

func (c *copier) collect(ctx context.Context, named string) error {
	info, err := c.repo.Stat(named)
	if err != nil {
		return pathError("%s cannot be read", named)
	}
	if !info.IsDir() {
		if !isManifest(path.Base(named)) {
			return pathError("%s is not a .yaml or .yml file", named)
		}
		if !info.Mode().IsRegular() {
			return pathError("%s is not a regular file", named)
		}
		return c.copyFile(named)
	}
	err = fs.WalkDir(c.repo.FS(), named, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p != named && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			return pathError("%s is a symlink", p)
		case entry.IsDir(), !isManifest(entry.Name()):
			return nil
		case !entry.Type().IsRegular():
			return pathError("%s is not a regular file", p)
		}
		return c.copyFile(p)
	})
	if err == nil {
		return nil
	}
	var coded *PathError
	if errors.As(err, &coded) {
		return err
	}
	if ctx.Err() != nil {
		return errors.New("timed out")
	}
	return pathError("%s cannot be read", named)
}

// isManifest reports whether name ends in .yaml or .yml, in any case.
func isManifest(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// copyFile copies one repository file. A file reached through two named paths is copied once, so a
// resource is not counted twice.
func (c *copier) copyFile(source string) error {
	if _, ok := c.seen[source]; ok {
		return nil
	}
	c.seen[source] = struct{}{}
	in, err := c.repo.Open(source)
	if err != nil {
		return pathError("%s cannot be read", source)
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return pathError("%s is not a regular file", source)
	}
	target := filepath.Join(c.dest, fmt.Sprintf("%06d.yaml", len(c.sources)+1))
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	room := c.limit - c.total
	n, copyErr := io.Copy(out, io.LimitReader(in, room+1))
	closeErr := out.Close()
	c.total += n
	switch {
	case copyErr != nil:
		return pathError("%s cannot be read", source)
	case n > room:
		return pathError("manifests are larger than %d bytes", c.limit)
	case closeErr != nil:
		return closeErr
	}
	c.sources = append(c.sources, source)
	return nil
}
