package federation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	fedpki "github.com/bluenviron/mediamtx/internal/directory/pki/federation"
)

// --- test helpers ---

func setupTestDB(t *testing.T) *directorydb.DB {
	t.Helper()
	db, err := directorydb.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func setupTestCA(t *testing.T) *fedpki.FederationCA {
	t.Helper()
	dir := t.TempDir()
	masterKey := make([]byte, 32)
	if _, err := rand.Read(masterKey); err != nil {
		t.Fatal(err)
	}
	ca, err := fedpki.New(fedpki.Config{
		StateDir:  dir,
		MasterKey: masterKey,
		Mode:      fedpki.ModeAirGapped,
		SiteID:    "founding-site-001",
	})
	if err != nil {
		t.Fatalf("federation CA: %v", err)
	}
	t.Cleanup(func() { ca.Shutdown(context.Background()) })
	return ca
}

// mockJWKSProvider implements JWKSProvider for testing.
type mockJWKSProvider struct {
	jwks []byte
	err  error
}

func (m *mockJWKSProvider) LocalJWKS() ([]byte, error) {
	return m.jwks, m.err
}

func setupTestService(t *testing.T) (*Service, *fedpki.FederationCA) {
	t.Helper()
	db := setupTestDB(t)
	ca := setupTestCA(t)

	jwksData, _ := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{"kty": "OKP", "crv": "Ed25519", "kid": "founding-key-1"},
		},
	})

	svc, err := NewService(Config{
		DB:                db,
		FederationCA:      ca,
		JWKS:              &mockJWKSProvider{jwks: jwksData},
		SiteID:            "founding-site-001",
		DirectoryEndpoint: "https://dir.site-a.kaivue.local:8443",
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, ca
}

func verifyKeyFromCA(t *testing.T, ca *fedpki.FederationCA) ed25519.PublicKey {
	t.Helper()
	vk, err := ca.PeerTokenVerifyKey()
	if err != nil {
		t.Fatalf("PeerTokenVerifyKey: %v", err)
	}
	return vk
}

// --- token format tests ---

func TestInvite_TokenFormat(t *testing.T) {
	svc, _ := setupTestService(t)

	result, err := svc.Invite(context.Background(), "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	// Must start with FED-v1.
	if !strings.HasPrefix(result.FEDToken, "FED-v1.") {
		t.Fatalf("token must start with FED-v1., got prefix: %s", result.FEDToken[:min(20, len(result.FEDToken))])
	}

	// TokenID must start with "fed-".
	if !strings.HasPrefix(result.TokenID, "fed-") {
		t.Fatalf("token_id must start with fed-, got: %s", result.TokenID)
	}

	// PeerSiteID must be non-empty.
	if result.PeerSiteID == "" {
		t.Fatal("peer_site_id must be non-empty")
	}

	// ExpiresAt must be ~60 minutes in the future.
	expectedMin := time.Now().Add(59 * time.Minute)
	expectedMax := time.Now().Add(61 * time.Minute)
	if result.ExpiresAt.Before(expectedMin) || result.ExpiresAt.After(expectedMax) {
		t.Fatalf("expires_at outside expected range: %v", result.ExpiresAt)
	}
}

func TestInvite_TokenVersioned(t *testing.T) {
	svc, _ := setupTestService(t)

	result, err := svc.Invite(context.Background(), "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	// Unwrap should succeed and report v1.
	version, raw, err := UnwrapToken(result.FEDToken)
	if err != nil {
		t.Fatalf("UnwrapToken: %v", err)
	}
	if version != "v1" {
		t.Fatalf("expected version v1, got %q", version)
	}
	// Raw token should contain two segments (payload.sig).
	if parts := strings.SplitN(raw, ".", 2); len(parts) != 2 {
		t.Fatalf("raw token should have payload.sig, got %d parts", len(parts))
	}
}

// --- single-use enforcement ---

func TestJoin_SingleUse(t *testing.T) {
	svc, ca := setupTestService(t)
	ctx := context.Background()
	verifyKey := verifyKeyFromCA(t, ca)

	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	peerJWKS := `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"peer-key-1"}]}`

	// First join should succeed.
	joinReq := JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://dir.site-b.kaivue.local:8443",
		PeerName:     "Site B",
		PeerJWKS:     peerJWKS,
		PeerSiteID:   result.PeerSiteID,
	}
	_, err = svc.Join(ctx, joinReq, verifyKey)
	if err != nil {
		t.Fatalf("first Join: %v", err)
	}

	// Second join with the same token must fail.
	_, err = svc.Join(ctx, joinReq, verifyKey)
	if err == nil {
		t.Fatal("second Join should fail")
	}
	if !strings.Contains(err.Error(), "already redeemed") {
		t.Fatalf("expected 'already redeemed' error, got: %v", err)
	}
}

// --- TTL enforcement ---

func TestJoin_ExpiredToken(t *testing.T) {
	db := setupTestDB(t)
	ca := setupTestCA(t)

	jwksData, _ := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{"kty": "OKP", "crv": "Ed25519", "kid": "founding-key-1"},
		},
	})

	// Service clock set 2 hours in the past so minted tokens are already expired.
	pastTime := time.Now().Add(-2 * time.Hour)
	svc, err := NewService(Config{
		DB:                db,
		FederationCA:      ca,
		JWKS:              &mockJWKSProvider{jwks: jwksData},
		SiteID:            "founding-site-001",
		DirectoryEndpoint: "https://dir.site-a.kaivue.local:8443",
		Clock:             func() time.Time { return pastTime },
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	ctx := context.Background()
	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	verifyKey := verifyKeyFromCA(t, ca)
	_, err = svc.Join(ctx, JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://dir.site-b.kaivue.local:8443",
		PeerName:     "Site B",
		PeerJWKS:     `{"keys":[]}`,
		PeerSiteID:   result.PeerSiteID,
	}, verifyKey)
	if err == nil {
		t.Fatal("Join with expired token should fail")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expiry error, got: %v", err)
	}
}

// --- JWKS exchange ---

func TestJoin_JWKSExchange(t *testing.T) {
	svc, ca := setupTestService(t)
	ctx := context.Background()
	verifyKey := verifyKeyFromCA(t, ca)

	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	peerJWKS := `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"peer-key-1"}]}`
	joinReq := JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://dir.site-b.kaivue.local:8443",
		PeerName:     "Site B",
		PeerJWKS:     peerJWKS,
		PeerSiteID:   result.PeerSiteID,
	}

	joinResult, err := svc.Join(ctx, joinReq, verifyKey)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	// Verify founding JWKS was returned.
	if joinResult.FoundingJWKS == "" {
		t.Fatal("founding JWKS must be non-empty")
	}
	if joinResult.FoundingSiteID != "founding-site-001" {
		t.Fatalf("unexpected founding site ID: %s", joinResult.FoundingSiteID)
	}
	if joinResult.FoundingEndpoint != "https://dir.site-a.kaivue.local:8443" {
		t.Fatalf("unexpected founding endpoint: %s", joinResult.FoundingEndpoint)
	}
	if joinResult.CAFingerprint == "" {
		t.Fatal("CA fingerprint must be non-empty")
	}
	if joinResult.CARootPEM == "" {
		t.Fatal("CA root PEM must be non-empty")
	}

	// Verify peer JWKS was stored in federation_members.
	member, err := svc.store.GetMember(ctx, result.PeerSiteID)
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if member.JWKSJson != peerJWKS {
		t.Fatalf("member JWKS mismatch: got %s", member.JWKSJson)
	}
}

// --- federation_members write ---

func TestJoin_WritesFederationMember(t *testing.T) {
	svc, ca := setupTestService(t)
	ctx := context.Background()
	verifyKey := verifyKeyFromCA(t, ca)

	// Initially no members.
	members, err := svc.store.ListActiveMembers(ctx)
	if err != nil {
		t.Fatalf("ListActiveMembers: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("expected 0 members, got %d", len(members))
	}

	// Invite and join.
	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	_, err = svc.Join(ctx, JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://dir.site-b.kaivue.local:8443",
		PeerName:     "Site B",
		PeerJWKS:     `{"keys":[]}`,
		PeerSiteID:   result.PeerSiteID,
	}, verifyKey)
	if err != nil {
		t.Fatalf("Join: %v", err)
	}

	// Now there should be one member.
	members, err = svc.store.ListActiveMembers(ctx)
	if err != nil {
		t.Fatalf("ListActiveMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].SiteID != result.PeerSiteID {
		t.Fatalf("member site ID mismatch")
	}
	if members[0].Name != "Site B" {
		t.Fatalf("member name mismatch: %s", members[0].Name)
	}
	if members[0].Status != "active" {
		t.Fatalf("member status should be active, got %s", members[0].Status)
	}
}

// --- error cases ---

func TestJoin_InvalidPrefix(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	_, err := svc.Join(ctx, JoinRequest{
		FEDToken:     "NOTFED-v1.payload.sig",
		PeerEndpoint: "https://peer.local:8443",
		PeerJWKS:     `{"keys":[]}`,
	}, ed25519.PublicKey{})
	if err == nil {
		t.Fatal("expected error for invalid prefix")
	}
	if !strings.Contains(err.Error(), "FED- prefix") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJoin_MissingJWKS(t *testing.T) {
	svc, ca := setupTestService(t)
	ctx := context.Background()
	verifyKey := verifyKeyFromCA(t, ca)

	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	_, err = svc.Join(ctx, JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://peer.local:8443",
		PeerJWKS:     "", // missing
		PeerSiteID:   result.PeerSiteID,
	}, verifyKey)
	if err == nil {
		t.Fatal("expected error for missing JWKS")
	}
	if !strings.Contains(err.Error(), "JWKS is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJoin_SiteIDMismatch(t *testing.T) {
	svc, ca := setupTestService(t)
	ctx := context.Background()
	verifyKey := verifyKeyFromCA(t, ca)

	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	_, err = svc.Join(ctx, JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://peer.local:8443",
		PeerJWKS:     `{"keys":[]}`,
		PeerSiteID:   "wrong-site-id",
	}, verifyKey)
	if err == nil {
		t.Fatal("expected error for site ID mismatch")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJoin_WrongVerifyKey(t *testing.T) {
	svc, _ := setupTestService(t)
	ctx := context.Background()

	result, err := svc.Invite(ctx, "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	// Use a random key that won't match the CA's signing key.
	_, wrongKey, _ := ed25519.GenerateKey(rand.Reader)
	wrongPub := wrongKey.Public().(ed25519.PublicKey)

	_, err = svc.Join(ctx, JoinRequest{
		FEDToken:     result.FEDToken,
		PeerEndpoint: "https://peer.local:8443",
		PeerJWKS:     `{"keys":[]}`,
		PeerSiteID:   result.PeerSiteID,
	}, wrongPub)
	if err == nil {
		t.Fatal("expected error for wrong verify key")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got: %v", err)
	}
}

// --- HTTP handler tests ---

func TestInviteHandler_Success(t *testing.T) {
	svc, _ := setupTestService(t)

	handler := InviteHandler(svc, func(r *http.Request) (string, bool) {
		return "admin@site-a", true
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/invite", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp InviteResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Token, "FED-v1.") {
		t.Fatalf("token missing FED-v1 prefix: %s", resp.Token[:min(20, len(resp.Token))])
	}
	if resp.TokenID == "" {
		t.Fatal("token_id empty")
	}
	if resp.ExpiresIn != "1h0m0s" {
		t.Fatalf("unexpected expires_in: %s", resp.ExpiresIn)
	}
}

func TestInviteHandler_Unauthorized(t *testing.T) {
	svc, _ := setupTestService(t)

	handler := InviteHandler(svc, func(r *http.Request) (string, bool) {
		return "", false
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/invite", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestInviteHandler_MethodNotAllowed(t *testing.T) {
	svc, _ := setupTestService(t)
	handler := InviteHandler(svc, func(r *http.Request) (string, bool) {
		return "admin", true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federation/invite", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestJoinHandler_Success(t *testing.T) {
	svc, ca := setupTestService(t)
	verifyKey := verifyKeyFromCA(t, ca)

	inviteResult, err := svc.Invite(context.Background(), "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	handler := JoinHandler(svc, verifyKey, nil)

	body, _ := json.Marshal(JoinHTTPRequest{
		Token:        inviteResult.FEDToken,
		PeerEndpoint: "https://dir.site-b.kaivue.local:8443",
		PeerName:     "Site B",
		PeerJWKS:     `{"keys":[{"kty":"OKP","crv":"Ed25519","kid":"peer-key-1"}]}`,
		PeerSiteID:   inviteResult.PeerSiteID,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/join", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp JoinHTTPResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.FoundingSiteID != "founding-site-001" {
		t.Fatalf("unexpected founding site ID: %s", resp.FoundingSiteID)
	}
	if resp.FoundingJWKS == "" {
		t.Fatal("founding JWKS must be non-empty")
	}
	if resp.CAFingerprint == "" {
		t.Fatal("CA fingerprint must be non-empty")
	}
	if resp.CARootPEM == "" {
		t.Fatal("CA root PEM must be non-empty")
	}
}

func TestJoinHandler_AlreadyUsed(t *testing.T) {
	svc, ca := setupTestService(t)
	verifyKey := verifyKeyFromCA(t, ca)

	inviteResult, err := svc.Invite(context.Background(), "admin@site-a")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	handler := JoinHandler(svc, verifyKey, nil)
	body, _ := json.Marshal(JoinHTTPRequest{
		Token:        inviteResult.FEDToken,
		PeerEndpoint: "https://dir.site-b.kaivue.local:8443",
		PeerName:     "Site B",
		PeerJWKS:     `{"keys":[]}`,
		PeerSiteID:   inviteResult.PeerSiteID,
	})

	// First join succeeds.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/federation/join", strings.NewReader(string(body)))
	rec1 := httptest.NewRecorder()
	handler(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first join expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}

	// Second join fails with 409 Conflict.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/federation/join", strings.NewReader(string(body)))
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)
	if rec2.Code != http.StatusConflict {
		t.Fatalf("second join expected 409, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var errResp map[string]string
	if err := json.NewDecoder(rec2.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp["code"] != "TOKEN_ALREADY_USED" {
		t.Fatalf("expected TOKEN_ALREADY_USED, got %s", errResp["code"])
	}
}

func TestJoinHandler_MissingFields(t *testing.T) {
	svc, ca := setupTestService(t)
	verifyKey := verifyKeyFromCA(t, ca)
	handler := JoinHandler(svc, verifyKey, nil)

	tests := []struct {
		name string
		body JoinHTTPRequest
	}{
		{"missing token", JoinHTTPRequest{PeerEndpoint: "https://x", PeerJWKS: "{}"}},
		{"missing endpoint", JoinHTTPRequest{Token: "FED-v1.x.y", PeerJWKS: "{}"}},
		{"missing jwks", JoinHTTPRequest{Token: "FED-v1.x.y", PeerEndpoint: "https://x"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/federation/join", strings.NewReader(string(body)))
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestJoinHandler_MethodNotAllowed(t *testing.T) {
	svc, ca := setupTestService(t)
	verifyKey := verifyKeyFromCA(t, ca)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			handler := JoinHandler(svc, verifyKey, nil)
			req := httptest.NewRequest(method, "/api/v1/federation/join", nil)
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("expected 405 for %s, got %d", method, rec.Code)
			}
		})
	}
}

// --- store tests ---

func TestStore_MarkExpired(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Insert a token that expires in the past.
	err := store.InsertToken(ctx, "tok-expired", "blob", "peer-1", "admin",
		time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("InsertToken: %v", err)
	}
	// Insert a token that expires in the future.
	err = store.InsertToken(ctx, "tok-future", "blob", "peer-2", "admin",
		time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("InsertToken: %v", err)
	}

	n, err := store.MarkExpired(ctx)
	if err != nil {
		t.Fatalf("MarkExpired: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 expired, got %d", n)
	}

	tok1, err := store.GetToken(ctx, "tok-expired")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok1.Status != StatusExpired {
		t.Fatalf("expected expired, got %s", tok1.Status)
	}
	tok2, err := store.GetToken(ctx, "tok-future")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok2.Status != StatusPending {
		t.Fatalf("expected pending, got %s", tok2.Status)
	}
}

func TestStore_RedeemNotFound(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	err := store.RedeemToken(context.Background(), "nonexistent-token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestStore_RedeemExpired(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	err := store.InsertToken(ctx, "tok-past", "blob", "peer-1", "admin",
		time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("InsertToken: %v", err)
	}

	err = store.RedeemToken(ctx, "tok-past")
	if !errors.Is(err, ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestStore_MemberCRUD(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	// Insert.
	err := store.InsertMember(ctx, MemberRow{
		SiteID:        "site-b",
		Name:          "Site B",
		Endpoint:      "https://dir.site-b:8443",
		JWKSJson:      `{"keys":[]}`,
		CAFingerprint: "abc123",
	})
	if err != nil {
		t.Fatalf("InsertMember: %v", err)
	}

	// Get.
	m, err := store.GetMember(ctx, "site-b")
	if err != nil {
		t.Fatalf("GetMember: %v", err)
	}
	if m.Name != "Site B" {
		t.Fatalf("name mismatch: %s", m.Name)
	}
	if m.Status != "active" {
		t.Fatalf("status should be active: %s", m.Status)
	}

	// List.
	members, err := store.ListActiveMembers(ctx)
	if err != nil {
		t.Fatalf("ListActiveMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}

	// Update last seen.
	err = store.UpdateMemberLastSeen(ctx, "site-b")
	if err != nil {
		t.Fatalf("UpdateMemberLastSeen: %v", err)
	}

	// Not found.
	_, err = store.GetMember(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestNewService_Validation(t *testing.T) {
	db := setupTestDB(t)
	ca := setupTestCA(t)
	jwks := &mockJWKSProvider{jwks: []byte(`{"keys":[]}`)}

	tests := []struct {
		name string
		cfg  Config
	}{
		{"missing DB", Config{FederationCA: ca, JWKS: jwks, SiteID: "x", DirectoryEndpoint: "x"}},
		{"missing CA", Config{DB: db, JWKS: jwks, SiteID: "x", DirectoryEndpoint: "x"}},
		{"missing JWKS", Config{DB: db, FederationCA: ca, SiteID: "x", DirectoryEndpoint: "x"}},
		{"missing SiteID", Config{DB: db, FederationCA: ca, JWKS: jwks, DirectoryEndpoint: "x"}},
		{"missing Endpoint", Config{DB: db, FederationCA: ca, JWKS: jwks, SiteID: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewService(tt.cfg)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
