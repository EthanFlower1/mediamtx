package itsm

import "context"

// Provider is the interface that all ITSM alerting backends must implement.
type Provider interface {
	// Type returns the provider type identifier.
	Type() ProviderType

	// SendAlert delivers an alert to the upstream ITSM platform.
	SendAlert(ctx context.Context, alert Alert) (AlertResult, error)

	// ResolveAlert resolves/closes an existing alert identified by its dedup key.
	ResolveAlert(ctx context.Context, dedupKey string) (AlertResult, error)

	// TestConnection sends a test/ping alert to verify the integration is
	// correctly configured.
	TestConnection(ctx context.Context) error
}
