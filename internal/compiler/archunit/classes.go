package archunit

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

	"github.com/ergoprobat/ergo/internal/compiler"
	"github.com/ergoprobat/ergo/internal/compiler/toolrun"
)

// maxClassBytes bounds the class files copied for one run. A variable so a test can lower it.
var maxClassBytes int64 = 1 << 30

const classSuffix = ".class"

// classTree is the copied class files. Every named path is one classpath root.
type classTree struct {
	roots []string
	// dirs holds, for each root, the slash-separated directories that hold a class file directly.
	dirs []map[string]struct{}
}

// stageClasses copies the *.class files below each named path into dest and nothing else.
//
// ArchUnit reads archunit.properties and archunit_ignore_patterns.txt from the classpath, and JUnit
// reads junit-platform.properties and service files from it. Those can hide a violation or add code to
// the test JVM, so no other file leaves the repository. The layout below each named path is kept,
// so ArchUnit's test-class patterns match the same locations they match in a native run.
// A symlink anywhere in the tree is refused: it would leave classes unread.
func stageClasses(ctx context.Context, root string, paths []string, dest string) (classTree, error) {
	if err := toolrun.WithinRoot(root, paths); err != nil {
		return classTree{}, &compiler.Error{Code: CodePath, Err: err}
	}
	repo, err := os.OpenRoot(root)
	if err != nil {
		return classTree{}, &compiler.Error{Code: CodeRun, Err: err}
	}
	defer func() { _ = repo.Close() }()
	var tree classTree
	var total int64
	seen := map[string]struct{}{}
	for _, named := range paths {
		if _, ok := seen[named]; ok {
			continue
		}
		seen[named] = struct{}{}
		rootDir := filepath.Join(dest, filepath.FromSlash(named))
		if err := os.MkdirAll(rootDir, 0o700); err != nil {
			return classTree{}, &compiler.Error{Code: CodeRun, Err: err}
		}
		dirs, err := copyRoot(ctx, repo.FS(), named, rootDir, &total)
		if err != nil {
			return classTree{}, err
		}
		tree.roots = append(tree.roots, rootDir)
		tree.dirs = append(tree.dirs, dirs)
	}
	return tree, nil
}

func copyRoot(ctx context.Context, repo fs.FS, named, rootDir string, total *int64) (map[string]struct{}, error) {
	info, err := fs.Stat(repo, named)
	if err != nil {
		return nil, pathError("%s cannot be read", named)
	}
	if !info.IsDir() {
		return nil, pathError("%s is not a directory", named)
	}
	dirs := map[string]struct{}{}
	err = fs.WalkDir(repo, named, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case entry.IsDir():
			return nil
		case entry.Type()&fs.ModeSymlink != 0:
			return pathError("%s is a symlink", p)
		case !strings.HasSuffix(entry.Name(), classSuffix):
			return nil
		case !entry.Type().IsRegular():
			return pathError("%s is not a regular file", p)
		}
		rel := relativeTo(named, p)
		if err := copyClass(repo, p, filepath.Join(rootDir, filepath.FromSlash(rel)), total); err != nil {
			return err
		}
		dirs[path.Dir(rel)] = struct{}{}
		return nil
	})
	if err == nil {
		return dirs, nil
	}
	var coded *compiler.Error
	if errors.As(err, &coded) {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, &compiler.Error{Code: CodeRun, Err: errors.New("timed out")}
	}
	return nil, pathError("%s cannot be read", named)
}

func copyClass(repo fs.FS, source, target string, total *int64) error {
	in, err := repo.Open(source)
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
	room := maxClassBytes - *total
	n, copyErr := io.Copy(out, io.LimitReader(in, room+1))
	closeErr := out.Close()
	*total += n
	switch {
	case copyErr != nil:
		return pathError("%s cannot be read", source)
	case n > room:
		return pathError("class files are larger than %d bytes", maxClassBytes)
	case closeErr != nil:
		return &compiler.Error{Code: CodeRun, Err: closeErr}
	}
	return nil
}

// relativeTo is p below the named directory, slash-separated.
func relativeTo(named, p string) string {
	if named == "." {
		return p
	}
	return strings.TrimPrefix(p, named+"/")
}

func pathError(format string, args ...any) error {
	return &compiler.Error{Code: CodePath, Err: fmt.Errorf(format, args...)}
}

// hasClassesUnder reports whether some copied class file sits in one of the packages or below it.
func (t classTree) hasClassesUnder(packages []string) bool {
	for _, pkg := range packages {
		prefix := strings.ReplaceAll(pkg, ".", "/")
		for _, dirs := range t.dirs {
			for dir := range dirs {
				if dir == prefix || strings.HasPrefix(dir, prefix+"/") {
					return true
				}
			}
		}
	}
	return false
}
