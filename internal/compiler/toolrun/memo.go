package toolrun

import (
	"context"
	"crypto/sha256"
	"errors"
)

type memoResult struct {
	raw []byte
	err error
}

// Memo runs one command per distinct config. Later calls with the same bytes reuse that result.
type Memo struct {
	// Run executes the command for one config. Do sets it before the first call when the caller builds it lazily.
	Run  func(ctx context.Context, config []byte) ([]byte, error)
	hits map[[sha256.Size]byte]memoResult
}

// Do returns the result for config, reusing an earlier result of the same bytes.
func (m *Memo) Do(ctx context.Context, config []byte) ([]byte, error) {
	if m.hits == nil {
		m.hits = map[[sha256.Size]byte]memoResult{}
	}
	key := sha256.Sum256(config)
	if hit, ok := m.hits[key]; ok {
		return append([]byte(nil), hit.raw...), hit.err
	}
	if m.Run == nil {
		return nil, errors.New("memo run is unset")
	}
	raw, err := m.Run(ctx, config)
	m.hits[key] = memoResult{raw: append([]byte(nil), raw...), err: err}
	return raw, err
}
