package importlinter

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

// maxSourceBytes bounds the Python source copied for one run. A variable so a test can lower it.
var maxSourceBytes int64 = 1 << 30

const (
	initFile     = "__init__.py"
	sourceSuffix = ".py"
)

// sourceTree stages the root packages of the checks. Each root package is copied once, below dir,
// and every check shares that copy read-only.
//
// Only the files grimp reads as modules are copied. A directory of the copy is never on sys.path, so
// nothing in it can be imported by name, and no other file leaves the repository. A symlink anywhere
// in a package is refused: grimp walks with followlinks=True, and the target would go unread.
type sourceTree struct {
	repo   *os.Root
	named  []string
	dir    string
	total  int64
	staged map[string]bool
}

// stage copies the root package name from the one named directory that holds it.
// A failed copy is removed, so a later check that names the same root starts again.
func (t *sourceTree) stage(ctx context.Context, name string) error {
	if t.staged[name] {
		return nil
	}
	pkg, err := t.locate(name)
	if err != nil {
		return err
	}
	dest := filepath.Join(t.dir, name)
	before := t.total
	if err := t.copyPackage(ctx, pkg, dest); err != nil {
		_ = os.RemoveAll(dest)
		t.total = before
		return err
	}
	t.staged[name] = true
	return nil
}

// locate returns the repository path of the root package name. Exactly one named directory must hold
// name/__init__.py as a regular file.
func (t *sourceTree) locate(name string) (string, error) {
	var found []string
	for _, dir := range t.named {
		pkg := path.Join(dir, name)
		ok, err := t.isPackage(pkg)
		if err != nil {
			return "", err
		}
		if ok {
			found = append(found, pkg)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return "", &compiler.Error{Code: CodeEmpty, Err: fmt.Errorf("no %s/%s in the named directories", name, initFile)}
	default:
		return "", pathError("root package %s is in more than one named directory", name)
	}
}

func (t *sourceTree) isPackage(pkg string) (bool, error) {
	dirInfo, err := t.repo.Lstat(pkg)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, pathError("%s cannot be read", pkg)
	case dirInfo.Mode()&fs.ModeSymlink != 0:
		return false, pathError("%s is a symlink", pkg)
	case !dirInfo.IsDir():
		return false, nil
	}
	initPath := path.Join(pkg, initFile)
	initInfo, err := t.repo.Lstat(initPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, pathError("%s cannot be read", initPath)
	case initInfo.Mode()&fs.ModeSymlink != 0:
		return false, pathError("%s is a symlink", initPath)
	case !initInfo.Mode().IsRegular():
		return false, pathError("%s is not a regular file", initPath)
	}
	return true, nil
}

func (t *sourceTree) copyPackage(ctx context.Context, pkg, dest string) error {
	err := fs.WalkDir(t.repo.FS(), pkg, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			return pathError("%s is a symlink", p)
		case entry.IsDir():
			return nil
		case !isModuleFile(entry.Name()):
			return nil
		case !entry.Type().IsRegular():
			return pathError("%s is not a regular file", p)
		}
		rel := strings.TrimPrefix(p, pkg+"/")
		return t.copyFile(p, filepath.Join(dest, filepath.FromSlash(rel)))
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
	return pathError("%s cannot be read", pkg)
}

// isModuleFile reports whether grimp 3.18 reads the file as a module: not hidden, ending in .py,
// and with no other dot in its name. Skipping the rest changes nothing for the analysis. It also
// keeps grimp from printing a warning about a file it would skip.
func isModuleFile(name string) bool {
	return !strings.HasPrefix(name, ".") && strings.HasSuffix(name, sourceSuffix) && strings.Count(name, ".") == 1
}

func (t *sourceTree) copyFile(source, target string) error {
	in, err := t.repo.Open(source)
	if err != nil {
		return pathError("%s cannot be read", source)
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return pathError("%s is not a regular file", source)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return &compiler.Error{Code: CodeRun, Err: err}
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return &compiler.Error{Code: CodeRun, Err: err}
	}
	room := maxSourceBytes - t.total
	n, copyErr := io.Copy(out, io.LimitReader(in, room+1))
	closeErr := out.Close()
	t.total += n
	switch {
	case copyErr != nil:
		return pathError("%s cannot be read", source)
	case n > room:
		return pathError("Python source is larger than %d bytes", maxSourceBytes)
	case closeErr != nil:
		return &compiler.Error{Code: CodeRun, Err: closeErr}
	}
	return nil
}

func pathError(format string, args ...any) error {
	return &compiler.Error{Code: CodePath, Err: fmt.Errorf(format, args...)}
}
