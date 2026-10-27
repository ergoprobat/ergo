package kyverno

import (
	"context"
	"errors"
	"os"

	"github.com/ipsofacto-io/ipso/internal/compiler"
	"github.com/ipsofacto-io/ipso/internal/compiler/manifests"
)

// stageManifests copies the manifests of the named paths into dest, the one flat directory the Kyverno
// CLI is given, and returns how many files it copied. manifests holds the copy rules.
func stageManifests(ctx context.Context, repo *os.Root, named []string, dest string) (int, error) {
	sources, err := manifests.Stage(ctx, repo, named, dest, manifests.MaxBytes)
	if err == nil {
		return len(sources), nil
	}
	var pathErr *manifests.PathError
	switch {
	case errors.As(err, &pathErr):
		return 0, &compiler.Error{Code: CodePath, Err: err}
	case errors.Is(err, manifests.ErrEmpty):
		return 0, &compiler.Error{Code: CodeEmpty, Err: err}
	default:
		return 0, &compiler.Error{Code: CodeRun, Err: err}
	}
}
