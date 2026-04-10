package credentialvault

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixedClock(t time.Time) Clock {
	return func() time.Time { return t }
}

func newTestService(t *testing.T) (*Service, *MemoryBackend, []AuditEvent) {
	t.Helper()
	backend := NewMemoryBackend()
	var events []AuditEvent
	svc, err := NewService(Config{
		Backend: backend,
		Clock:   fixedClock(time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)),
		AuditHook: func(_ context.Context, e AuditEvent) {
			events = append(events, e)
		},
	})
	require.NoError(t, err)
	return svc, backend, events
}

func TestNewService_RequiresBackend(t *testing.T) {
	_, err := NewService(Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "backend is required")
}

func TestStoreCredential(t *testing.T) {
	svc, _, _ := newTestService(t)

	cred, err := svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "tenant-1",
		Type:     AppleDistributionCert,
		Label:    "ACME iOS Dist 2026",
		Value:    []byte("-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----"),
	})
	require.NoError(t, err)
	require.Equal(t, "tenant-1", cred.TenantID)
	require.Equal(t, AppleDistributionCert, cred.Type)
	require.Equal(t, "ACME iOS Dist 2026", cred.Label)
	require.NotEmpty(t, cred.SecretID)
	require.Equal(t, "v1", cred.Version)
}

func TestStoreCredential_ValidationErrors(t *testing.T) {
	svc, _, _ := newTestService(t)

	tests := []struct {
		name string
		req  StoreRequest
		want string
	}{
		{"missing tenant", StoreRequest{Type: APNsKey, Value: []byte("x")}, "tenant_id is required"},
		{"invalid type", StoreRequest{TenantID: "t", Type: "bogus", Value: []byte("x")}, "invalid credential type"},
		{"missing value", StoreRequest{TenantID: "t", Type: APNsKey}, "value is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.StoreCredential(context.Background(), tt.req)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestGetCredential(t *testing.T) {
	svc, _, _ := newTestService(t)
	secret := []byte(`{"type":"service_account","project_id":"acme"}`)

	_, err := svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "tenant-1",
		Type:     GooglePlayServiceAccount,
		Label:    "Google Play SA",
		Value:    secret,
	})
	require.NoError(t, err)

	val, cred, err := svc.GetCredential(context.Background(), "tenant-1", GooglePlayServiceAccount)
	require.NoError(t, err)
	require.Equal(t, secret, val)
	require.Equal(t, "tenant-1", cred.TenantID)
	require.Equal(t, GooglePlayServiceAccount, cred.Type)
}

func TestGetCredential_NotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, _, err := svc.GetCredential(context.Background(), "tenant-1", APNsKey)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

func TestRotateCredential(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "tenant-1",
		Type:     APNsKey,
		Label:    "APNs Auth Key",
		Value:    []byte("old-key"),
	})
	require.NoError(t, err)

	newKey := []byte("new-key-2026")
	cred, err := svc.RotateCredential(context.Background(), RotateRequest{
		TenantID: "tenant-1",
		Type:     APNsKey,
		NewValue: newKey,
	})
	require.NoError(t, err)
	require.Equal(t, "v2", cred.Version)

	// Verify the rotated value
	val, _, err := svc.GetCredential(context.Background(), "tenant-1", APNsKey)
	require.NoError(t, err)
	require.Equal(t, newKey, val)
}

func TestDeleteCredential(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "tenant-1",
		Type:     FCMServiceAccount,
		Label:    "FCM SA",
		Value:    []byte("fcm-json"),
	})
	require.NoError(t, err)

	err = svc.DeleteCredential(context.Background(), "tenant-1", FCMServiceAccount)
	require.NoError(t, err)

	_, _, err = svc.GetCredential(context.Background(), "tenant-1", FCMServiceAccount)
	require.Error(t, err)
}

func TestListCredentials(t *testing.T) {
	svc, _, _ := newTestService(t)

	for _, ct := range []CredentialType{AppleDistributionCert, APNsKey, GooglePlayServiceAccount} {
		_, err := svc.StoreCredential(context.Background(), StoreRequest{
			TenantID: "tenant-1",
			Type:     ct,
			Label:    string(ct),
			Value:    []byte("secret"),
		})
		require.NoError(t, err)
	}

	// Store a credential for a different tenant — must not appear
	_, err := svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "tenant-2",
		Type:     FCMServiceAccount,
		Label:    "other",
		Value:    []byte("x"),
	})
	require.NoError(t, err)

	types, err := svc.ListCredentials(context.Background(), "tenant-1")
	require.NoError(t, err)
	require.Len(t, types, 3)
	require.ElementsMatch(t, []CredentialType{AppleDistributionCert, APNsKey, GooglePlayServiceAccount}, types)
}

func TestTenantIsolation(t *testing.T) {
	svc, _, _ := newTestService(t)

	_, err := svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "tenant-a",
		Type:     APNsKey,
		Label:    "A's key",
		Value:    []byte("tenant-a-secret"),
	})
	require.NoError(t, err)

	// Tenant B cannot read Tenant A's credential
	_, _, err = svc.GetCredential(context.Background(), "tenant-b", APNsKey)
	require.Error(t, err)
}

func TestAuditHookFired(t *testing.T) {
	svc, _, _ := newTestService(t)
	// The events slice is captured by reference in the closure — but since
	// newTestService returns the initial (empty) slice header, we need to
	// re-collect via the audit hook. Rebuild with a shared pointer.
	var events []AuditEvent
	svc.auditHook = func(_ context.Context, e AuditEvent) {
		events = append(events, e)
	}

	_, _ = svc.StoreCredential(context.Background(), StoreRequest{
		TenantID: "t1", Type: APNsKey, Label: "k", Value: []byte("v"),
	})
	_, _ = svc.RotateCredential(context.Background(), RotateRequest{
		TenantID: "t1", Type: APNsKey, NewValue: []byte("v2"),
	})
	_ = svc.DeleteCredential(context.Background(), "t1", APNsKey)

	require.Len(t, events, 3)
	require.Equal(t, "store", events[0].Action)
	require.Equal(t, "rotate", events[1].Action)
	require.Equal(t, "delete", events[2].Action)
}

func TestCredentialType_IsValid(t *testing.T) {
	require.True(t, AppleDistributionCert.IsValid())
	require.True(t, FCMServiceAccount.IsValid())
	require.False(t, CredentialType("invalid").IsValid())
}

func TestStoreDuplicate(t *testing.T) {
	svc, _, _ := newTestService(t)

	req := StoreRequest{TenantID: "t1", Type: APNsKey, Label: "k", Value: []byte("v")}
	_, err := svc.StoreCredential(context.Background(), req)
	require.NoError(t, err)

	_, err = svc.StoreCredential(context.Background(), req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}
