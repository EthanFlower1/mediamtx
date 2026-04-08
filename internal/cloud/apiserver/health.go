// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/permissions"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// ReadinessProbes bundles the checks /readyz runs. The server wires real
// probes in New(); tests inject stubs to exercise the 503 path.
type ReadinessProbes struct {
	DB       func(ctx context.Context) error
	Identity func(ctx context.Context) error
	Policy   func(ctx context.Context) error
}

// livenessHandler is a pure liveness probe: the process is up, so return OK.
// /healthz MUST NOT take dependencies because k8s will kill the pod on
// transient DB hiccups if it does.
func livenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
}

// readinessHandler returns 200 when every probe passes and 503 otherwise.
// The body is a JSON map of probe name → "ok"|error message for easy
// diagnosis in the alert runbook.
func readinessHandler(probes ReadinessProbes) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		result := make(map[string]string, 3)
		healthy := true

		run := func(name string, fn func(context.Context) error) {
			if fn == nil {
				result[name] = "skipped"
				return
			}
			if err := fn(ctx); err != nil {
				result[name] = err.Error()
				healthy = false
				return
			}
			result[name] = "ok"
		}
		run("db", probes.DB)
		run("identity", probes.Identity)
		run("policy", probes.Policy)

		w.Header().Set("Content-Type", "application/json")
		if !healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}

// defaultReadinessProbes wires production probes against the real deps.
func defaultReadinessProbes(database *db.DB, idp auth.IdentityProvider, enf *permissions.Enforcer) ReadinessProbes {
	return ReadinessProbes{
		DB: func(ctx context.Context) error {
			if database == nil {
				return nil
			}
			return database.PingContext(ctx)
		},
		Identity: func(ctx context.Context) error {
			if idp == nil {
				return nil
			}
			// Cheap round-trip: VerifyToken("") must fail with
			// ErrTokenInvalid quickly. Any OTHER error (network,
			// timeout) indicates an unhealthy IdP.
			_, err := idp.VerifyToken(ctx, "")
			if err == nil {
				// Implementation bug: an empty token verified.
				return nil
			}
			if err == auth.ErrTokenInvalid {
				return nil
			}
			return err
		},
		Policy: func(_ context.Context) error {
			if enf == nil {
				return nil
			}
			// Touching Store() forces the enforcer to expose its
			// backing store; a nil store means the enforcer was never
			// initialised.
			if enf.Store() == nil {
				return errNilPolicyStore
			}
			return nil
		},
	}
}

// errNilPolicyStore is a package-level sentinel for the readiness probe.
var errNilPolicyStore = errSentinel("policy store is nil")

type errSentinel string

func (e errSentinel) Error() string { return string(e) }
