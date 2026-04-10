package revocation

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// recordingAudit captures audit events for test assertions.
type recordingAudit struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (r *recordingAudit) RecordRevocation(_ context.Context, e AuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recordingAudit) Events() []AuditEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]AuditEvent, len(r.events))
	copy(out, r.events)
	return out
}

// recordingNotifier captures NotifyRevocation calls.
type recordingNotifier struct {
	mu     sync.Mutex
	called []string
}

func (n *recordingNotifier) NotifyRevocation(_ context.Context, recorderID string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.called = append(n.called, recorderID)
	return nil
}

func (n *recordingNotifier) Called() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.called))
	copy(out, n.called)
	return out
}

func TestHandlerRevokeWithExplicitJTIs(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)
	audit := &recordingAudit{}
	notifier := &recordingNotifier{}

	handler := RevokeTokensHandler(HandlerConfig{
		Store:    store,
		Notifier: notifier,
		Audit:    audit,
		Log:      slog.Default(),
	})

	expiresAt := time.Now().Add(2 * time.Hour)
	body, _ := json.Marshal(RevokeRequest{
		Reason:    "compromised device",
		TokenJTIs: []string{"tok-1", "tok-2"},
		ExpiresAt: &expiresAt,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/recorders/rec-X/revoke-tokens", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler(w, req, "rec-X", "tenant-1", "admin-alice")

	require.Equal(t, http.StatusOK, w.Code)

	var resp RevokeResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, int64(2), resp.RevokedCount)
	require.Equal(t, "rec-X", resp.RecorderID)

	// Tokens should be in the blocklist.
	ctx := context.Background()
	for _, jti := range []string{"tok-1", "tok-2"} {
		ok, err := store.IsRevoked(ctx, jti)
		require.NoError(t, err)
		require.True(t, ok, "expected %s to be revoked", jti)
	}

	// Audit event recorded.
	events := audit.Events()
	require.Len(t, events, 1)
	require.Equal(t, "revoke_recorder_tokens", events[0].Action)
	require.Equal(t, "rec-X", events[0].RecorderID)
	require.Equal(t, "admin-alice", events[0].RevokedBy)
	require.Equal(t, int64(2), events[0].TokenCount)

	// Notifier called.
	require.Equal(t, []string{"rec-X"}, notifier.Called())
}

func TestHandlerRevokeBlanketNoJTIs(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	handler := RevokeTokensHandler(HandlerConfig{
		Store: store,
		Log:   slog.Default(),
	})

	body, _ := json.Marshal(RevokeRequest{Reason: "decommission"})
	req := httptest.NewRequest(http.MethodPost, "/revoke", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler(w, req, "rec-Y", "tenant-2", "admin-bob")
	require.Equal(t, http.StatusOK, w.Code)

	var resp RevokeResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Equal(t, int64(1), resp.RevokedCount)

	// Sentinel should trigger IsRecorderRevoked.
	ctx := context.Background()
	ok, err := store.IsRecorderRevoked(ctx, "rec-Y")
	require.NoError(t, err)
	require.True(t, ok)
}

func TestHandlerRejectsGET(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	handler := RevokeTokensHandler(HandlerConfig{
		Store: store,
		Log:   slog.Default(),
	})

	req := httptest.NewRequest(http.MethodGet, "/revoke", nil)
	w := httptest.NewRecorder()

	handler(w, req, "rec-Z", "tenant-1", "admin-1")
	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestHandlerRejectsEmptyRecorderID(t *testing.T) {
	db := openTestDB(t)
	store := NewStore(db)

	handler := RevokeTokensHandler(HandlerConfig{
		Store: store,
		Log:   slog.Default(),
	})

	req := httptest.NewRequest(http.MethodPost, "/revoke", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()

	handler(w, req, "", "tenant-1", "admin-1")
	require.Equal(t, http.StatusBadRequest, w.Code)
}
