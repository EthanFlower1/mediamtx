package zitadel

import (
	"errors"
	"net/http"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/audit"
)

// Config is the construction-time configuration for the Zitadel adapter.
// All fields except HTTPClient are required; New returns an error on any
// missing required field to honor the fail-closed-at-construction policy.
type Config struct {
	// Domain is the Zitadel instance hostname, e.g. "kaivue-prod.zitadel.cloud".
	// No scheme; the adapter always uses https.
	Domain string

	// ServiceAccountKey is the path to the Zitadel service-account JSON key
	// file (JWT profile grant). The adapter reads it once during New and
	// uses it to mint short-lived admin tokens for bootstrap/management
	// calls. Never log the file's contents.
	ServiceAccountKey string

	// PlatformOrgID is the Zitadel org ID that owns the adapter's own
	// service account. It is the parent org under which integrator orgs
	// are created by BootstrapIntegrator.
	PlatformOrgID string

	// HTTPClient is the HTTP client used for every Zitadel REST call. The
	// field is exposed primarily so tests can inject a fake round-tripper
	// and assert request shapes against fixtures in testdata/. If nil,
	// New installs http.DefaultClient with a 10s timeout.
	HTTPClient *http.Client

	// AuditRecorder receives one audit.Entry per successful auth operation.
	// nil is permitted (audit becomes a no-op) but New will log a warning
	// to stderr so operators notice missing audit in production.
	AuditRecorder audit.Recorder

	// Now is an injectable clock for deterministic tests. nil means
	// time.Now.
	Now func() time.Time
}

// validate returns an error describing the first missing or invalid field.
func (c Config) validate() error {
	if c.Domain == "" {
		return errors.New("zitadel: Config.Domain is required")
	}
	if c.ServiceAccountKey == "" {
		return errors.New("zitadel: Config.ServiceAccountKey is required")
	}
	if c.PlatformOrgID == "" {
		return errors.New("zitadel: Config.PlatformOrgID is required")
	}
	return nil
}
