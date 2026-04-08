//go:build !river

package jobs

import (
	"context"
	"errors"
)

// RiverEnqueuer is the Postgres-backed JobEnqueuer using
// github.com/riverqueue/river. The real implementation lives behind
// the `river` build tag (see river_real.go) — in the default build we
// ship this stub so the package compiles without pulling the river
// dependency and its required Postgres driver.
//
// To enable the real backend:
//
//	go build -tags river ./...
//
// See README.md for the rationale and the swap-out procedure.
type RiverEnqueuer struct{}

// NewRiverEnqueuer returns a stub that errors on every call. Callers
// should fall back to MemoryEnqueuer in environments where Postgres is
// unavailable (CI sandboxes, local unit tests).
func NewRiverEnqueuer() *RiverEnqueuer { return &RiverEnqueuer{} }

// errRiverDisabled is returned by every method on the stub.
var errRiverDisabled = errors.New("jobs: River backend disabled; build with -tags river and configure Postgres")

// Enqueue implements JobEnqueuer.
func (*RiverEnqueuer) Enqueue(_ context.Context, _ Kind, _ TenantScoped, _ EnqueueOptions) (*Job, error) {
	return nil, errRiverDisabled
}

// Register implements JobRunner.
func (*RiverEnqueuer) Register(_ Worker) error { return errRiverDisabled }

// RunOnce implements JobRunner.
func (*RiverEnqueuer) RunOnce(_ context.Context) error { return errRiverDisabled }

// Shutdown implements JobRunner.
func (*RiverEnqueuer) Shutdown(_ context.Context) error { return nil }
