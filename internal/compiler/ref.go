package compiler

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// ReadRef reads the file named by the check ref.
// ref is relative to the repository root. A path that fails fs.ValidPath is outside
// the repository. os.Root also rejects every symlink, including one whose target
// stays inside the tree, and reports that as "path escapes from parent".
func ReadRef(in Input) ([]byte, error) {
	ref := in.Check.Ref
	if ref == "" {
		return nil, &Error{Code: CodeRefRequired, Err: errors.New("check has no ref")}
	}
	if !fs.ValidPath(ref) {
		return nil, &Error{Code: CodeRefOutside, Err: fmt.Errorf("ref %q leaves the repository", ref)}
	}
	if in.Repo == nil {
		return nil, &Error{Code: CodeRefMissing, Err: errors.New("no repository")}
	}
	info, err := fs.Stat(in.Repo, ref)
	if err != nil {
		return nil, classifyRef(err)
	}
	if info.Size() > MaxRefBytes {
		return nil, &Error{Code: CodeRefTooLarge, Err: fmt.Errorf("ref %q is %d bytes", ref, info.Size())}
	}
	data, err := fs.ReadFile(in.Repo, ref)
	if err != nil {
		return nil, classifyRef(err)
	}
	if len(data) > MaxRefBytes {
		return nil, &Error{Code: CodeRefTooLarge, Err: fmt.Errorf("ref %q is %d bytes", ref, len(data))}
	}
	return data, nil
}

func classifyRef(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return &Error{Code: CodeRefMissing, Err: err}
	}
	if errors.Is(err, fs.ErrInvalid) || strings.Contains(err.Error(), "escapes from parent") {
		return &Error{Code: CodeRefOutside, Err: err}
	}
	return &Error{Code: CodeRefMissing, Err: err}
}
