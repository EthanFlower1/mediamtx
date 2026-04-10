package gateway

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

// fakeVerifier returns a canned StreamClaims (or error) regardless of
// input. Tests set Result/Err directly.
type fakeVerifier struct {
	Result *streamclaims.StreamClaims
	Err    error
	Calls  int
}

func (f *fakeVerifier) Verify(_ string) (*streamclaims.StreamClaims, error) {
	f.Calls++
	if f.Err != nil {
		return nil, f.Err
	}
	return f.Result, nil
}

// fakeResolver is an in-memory RecorderResolver for tests.
type fakeResolver struct {
	byCamera map[string]*RecorderEndpoint
	all      []RecorderEndpoint
	listErr  error
}

func newFakeResolver(all []RecorderEndpoint) *fakeResolver {
	m := make(map[string]*RecorderEndpoint)
	for i := range all {
		ep := all[i]
		// Index by RecorderID since the test claims set CameraID == PathName.
		m[ep.PathName] = &ep
	}
	return &fakeResolver{byCamera: m, all: all}
}

func (f *fakeResolver) Resolve(_ context.Context, claims *streamclaims.StreamClaims) (*RecorderEndpoint, error) {
	ep, ok := f.byCamera[claims.CameraID]
	if !ok {
		return nil, ErrCameraNotFound
	}
	return ep, nil
}

func (f *fakeResolver) ListRecorders(_ context.Context) ([]RecorderEndpoint, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.all, nil
}

// fakeNonce records consumed nonces and returns ErrReplay on duplicates.
type fakeNonce struct {
	mu   sync.Mutex
	seen map[string]bool
}

func newFakeNonce() *fakeNonce { return &fakeNonce{seen: map[string]bool{}} }

func (f *fakeNonce) CheckAndConsume(_ context.Context, nonce string, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen[nonce] {
		return fmt.Errorf("replay: %w", ErrReplay)
	}
	f.seen[nonce] = true
	return nil
}

// errResolver returns the same error from both methods. Used to verify
// the 502 path.
type errResolver struct{ err error }

func (e errResolver) Resolve(context.Context, *streamclaims.StreamClaims) (*RecorderEndpoint, error) {
	return nil, e.err
}
func (e errResolver) ListRecorders(context.Context) ([]RecorderEndpoint, error) {
	return nil, e.err
}

var errBoom = errors.New("boom")
