package recorderapi

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const recorderIDKey contextKey = "recorder_id"

// RecorderIDFromContext extracts the authenticated recorder ID from the request context.
func RecorderIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(recorderIDKey).(string)
	return id, ok && id != ""
}

// BearerAuthMiddleware validates the Authorization: Bearer <token> header
// against the store's service tokens. On success, it injects the recorder_id
// into the request context.
func BearerAuthMiddleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, `{"error":"missing bearer token"}`, http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")

			recorderID, err := store.ValidateToken(r.Context(), token)
			if err != nil {
				http.Error(w, `{"error":"invalid or unknown token"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), recorderIDKey, recorderID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// BearerOrHeaderAuth creates a recorder authenticator function compatible with
// the existing recordercontrol/ingest patterns. It tries bearer token first,
// then falls back to X-Recorder-ID header (for backward compatibility during migration).
func BearerOrHeaderAuth(store *Store) func(r *http.Request) (string, bool) {
	return func(r *http.Request) (string, bool) {
		// Try bearer token first.
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if id, err := store.ValidateToken(r.Context(), token); err == nil {
				return id, true
			}
		}
		// Fall back to X-Recorder-ID header.
		id := r.Header.Get("X-Recorder-ID")
		return id, id != ""
	}
}
