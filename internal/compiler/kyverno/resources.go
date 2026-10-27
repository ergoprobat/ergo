package kyverno

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

// maxResourceBytes bounds the manifests copied for one run. A variable so a test can lower it.
var maxResourceBytes int64 = 256 << 20

// manifestCopier copies the manifests below the named paths into one flat directory, a file for each
// source file, numbered in the order they are found.
//
// The Kyverno CLI reads a directory without recursing and keeps only names that end in lower case
// .yaml or .yml, and one --resource flag for each file would hit the command line limit of Windows.
// The copy also keeps repository symlinks and later edits out of the run. A symlink anywhere below a
// named directory is refused: the CLI would not follow it, so the manifests behind it would go unchecked.
type manifestCopier struct {
	repo  *os.Root
	dest  string
	count int
	total int64
	seen  map[string]struct{}
}

// stageManifests copies the manifests of every named path into dest and returns how many files it copied.
// A named path is a manifest file or a directory. Below a directory, entries that start with a dot are
// skipped, and so is everything that does not end in .yaml or .yml, in any case.
func stageManifests(ctx context.Context, repo *os.Root, named []string, dest string) (int, error) {
	if err := os.Mkdir(dest, 0o700); err != nil {
		return 0, &compiler.Error{Code: CodeRun, Err: err}
	}
	c := &manifestCopier{repo: repo, dest: dest, seen: map[string]struct{}{}}
	for _, name := range named {
		if err := c.collect(ctx, name); err != nil {
			return 0, err
		}
	}
	if c.count == 0 {
		return 0, &compiler.Error{Code: CodeEmpty, Err: errors.New("the named paths hold no .yaml or .yml file")}
	}
	return c.count, nil
}

func (c *manifestCopier) collect(ctx context.Context, named string) error {
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
	var coded *compiler.Error
	if errors.As(err, &coded) {
		return err
	}
	if ctx.Err() != nil {
		return &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
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
func (c *manifestCopier) copyFile(source string) error {
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
	target := filepath.Join(c.dest, fmt.Sprintf("%06d.yaml", c.count+1))
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return &compiler.Error{Code: CodeRun, Err: err}
	}
	room := maxResourceBytes - c.total
	n, copyErr := io.Copy(out, io.LimitReader(in, room+1))
	closeErr := out.Close()
	c.total += n
	switch {
	case copyErr != nil:
		return pathError("%s cannot be read", source)
	case n > room:
		return pathError("manifests are larger than %d bytes", maxResourceBytes)
	case closeErr != nil:
		return &compiler.Error{Code: CodeRun, Err: closeErr}
	}
	c.count++
	return nil
}

func pathError(format string, args ...any) error {
	return &compiler.Error{Code: CodePath, Err: fmt.Errorf(format, args...)}
}
