//go:build river

package jobs

import (
	"context"
	"errors"
)

// RiverEnqueuer is the Postgres-backed JobEnqueuer wrapping a
// github.com/riverqueue/river client.
//
// This file is gated behind the `river` build tag. The real dependency
// is NOT imported in the default build; when you flip the tag on you
// are expected to also:
//
//  1. `go get github.com/riverqueue/river github.com/riverqueue/river/riverdriver/riverpgxv5`
//  2. Wire a pgx pool to the cloud control-plane RDS instance.
//  3. Replace the scaffolding below with real river.Client/WorkFunc
//     bindings. The stub is just enough to keep the interface stable
//     so consumers compile today.
//
// Keeping this scaffold in-tree means when KAI-234's Postgres half
// lands we change one file, not the whole package.
type RiverEnqueuer struct {
	// TODO(KAI-234): hold *river.Client[pgx.Tx] here once the real
	// dependency is added.
}

// NewRiverEnqueuer returns a RiverEnqueuer configured against the
// control-plane Postgres. In the current scaffold it returns an error
// until the river dependency is vendored.
func NewRiverEnqueuer() *RiverEnqueuer { return &RiverEnqueuer{} }

var errRiverNotWired = errors.New("jobs: River backend scaffold — vendor github.com/riverqueue/river and wire pgx pool")

// Enqueue implements JobEnqueuer.
func (*RiverEnqueuer) Enqueue(_ context.Context, _ Kind, _ TenantScoped, _ EnqueueOptions) (*Job, error) {
	return nil, errRiverNotWired
}

// Register implements JobRunner.
func (*RiverEnqueuer) Register(_ Worker) error { return errRiverNotWired }

// RunOnce implements JobRunner.
func (*RiverEnqueuer) RunOnce(_ context.Context) error { return errRiverNotWired }

// Shutdown implements JobRunner.
func (*RiverEnqueuer) Shutdown(_ context.Context) error { return nil }
