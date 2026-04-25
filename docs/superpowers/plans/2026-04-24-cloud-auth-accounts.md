# Cloud Auth & Account Management — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Secure cloud remote access with per-tenant authentication and provide a signup flow so customers can create accounts, get API keys, and connect their on-prem NVR to the cloud.

**Architecture:** The cloud broker replaces its single shared token with per-tenant API key validation using the existing `apikeys.Store`. A public signup endpoint creates a tenant + initial admin + API key in one call. The API key is what customers enter in Settings → Remote Access on their NVR. Remote viewers authenticate via the on-prem NVR's existing JWT auth, tunneled transparently through frp. The cloud only authenticates the **site connection**, not individual viewers.

**Tech Stack:** Go 1.26+, SQLite (cloud broker DB for MVP — PostgreSQL later), existing `publicapi/apikeys` package, `golang-jwt/jwt/v5`, bcrypt

---

## File Structure

### Cloud Broker

| File | Responsibility |
|------|----------------|
| `cmd/cloudbroker/main.go` | Modify: replace single-token auth with API key store |
| `internal/cloud/broker/auth.go` | New: API key validation + tenant resolution for broker |
| `internal/cloud/broker/auth_test.go` | New: tests |
| `internal/cloud/broker/signup.go` | New: POST /api/v1/signup endpoint |
| `internal/cloud/broker/signup_test.go` | New: tests |
| `internal/cloud/broker/accounts.go` | New: GET /api/v1/account, GET /api/v1/account/keys |
| `internal/cloud/broker/accounts_test.go` | New: tests |
| `internal/cloud/broker/store.go` | New: SQLite store for tenants + API keys (MVP) |
| `internal/cloud/broker/store_test.go` | New: tests |

### On-Prem (no changes needed for auth)

The on-prem NVR already has JWT auth. Remote viewers go through the frp tunnel which proxies to the NVR's existing auth endpoints. The only on-prem change: the cloud token entered in Settings → Remote Access becomes the customer's API key instead of a shared secret.

---

## Phase 1: Cloud Broker Database

### Task 1: Create broker SQLite store for tenants and API keys

**Files:**
- Create: `internal/cloud/broker/store.go`
- Create: `internal/cloud/broker/store_test.go`

The cloud broker needs its own lightweight datastore. For MVP we use SQLite (same pattern as the on-prem Directory). This stores tenants and API keys.

- [ ] **Step 1: Write the failing test**

```go
// internal/cloud/broker/store_test.go
package broker

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func testDB(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCreateTenantAndLookup(t *testing.T) {
	s := testDB(t)

	id, err := s.CreateTenant("My Company", "admin@example.com")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if id == "" {
		t.Fatal("empty tenant ID")
	}

	tenant, err := s.GetTenant(id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if tenant.Name != "My Company" {
		t.Fatalf("name = %q", tenant.Name)
	}
	if tenant.Email != "admin@example.com" {
		t.Fatalf("email = %q", tenant.Email)
	}
}

func TestCreateAPIKeyAndValidate(t *testing.T) {
	s := testDB(t)

	tenantID, _ := s.CreateTenant("Test Co", "test@example.com")

	plainKey, err := s.CreateAPIKey(tenantID, "default")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if plainKey == "" {
		t.Fatal("empty key")
	}

	// Validate returns the tenant ID.
	gotTenantID, err := s.ValidateAPIKey(plainKey)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if gotTenantID != tenantID {
		t.Fatalf("tenant = %q, want %q", gotTenantID, tenantID)
	}
}

func TestValidateInvalidKey(t *testing.T) {
	s := testDB(t)
	_, err := s.ValidateAPIKey("kvue_not_a_real_key")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestListTenantKeys(t *testing.T) {
	s := testDB(t)
	tenantID, _ := s.CreateTenant("Test Co", "t@example.com")
	s.CreateAPIKey(tenantID, "key1")
	s.CreateAPIKey(tenantID, "key2")

	keys, err := s.ListAPIKeys(tenantID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("len = %d, want 2", len(keys))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cloud/broker/... -v -run "TestCreate|TestValidate|TestList"
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Write implementation**

```go
// internal/cloud/broker/store.go
package broker

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Tenant represents a customer account.
type Tenant struct {
	ID        string
	Name      string
	Email     string
	CreatedAt time.Time
}

// APIKeyInfo is the non-secret metadata about an API key.
type APIKeyInfo struct {
	ID        string
	TenantID  string
	Name      string
	Prefix    string
	CreatedAt time.Time
}

// Store manages tenants and API keys in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates the tables and returns a ready store.
func NewStore(db *sql.DB) (*Store, error) {
	schema := `
	CREATE TABLE IF NOT EXISTS tenants (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		email TEXT NOT NULL UNIQUE,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL REFERENCES tenants(id),
		name TEXT NOT NULL DEFAULT 'default',
		key_hash TEXT NOT NULL UNIQUE,
		key_prefix TEXT NOT NULL,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db}, nil
}

// CreateTenant creates a new tenant and returns its ID.
func (s *Store) CreateTenant(name, email string) (string, error) {
	id := generateID()
	_, err := s.db.Exec(
		"INSERT INTO tenants (id, name, email) VALUES (?, ?, ?)",
		id, name, email,
	)
	if err != nil {
		return "", err
	}
	return id, nil
}

// GetTenant returns a tenant by ID.
func (s *Store) GetTenant(id string) (*Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(
		"SELECT id, name, email, created_at FROM tenants WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &t.Email, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// GetTenantByEmail returns a tenant by email.
func (s *Store) GetTenantByEmail(email string) (*Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(
		"SELECT id, name, email, created_at FROM tenants WHERE email = ?", email,
	).Scan(&t.ID, &t.Name, &t.Email, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CreateAPIKey generates a new API key for a tenant. Returns the plaintext
// key (shown once) and stores the SHA-256 hash.
func (s *Store) CreateAPIKey(tenantID, name string) (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	plainKey := "kvue_" + hex.EncodeToString(raw)
	prefix := plainKey[:10]

	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	id := generateID()
	_, err := s.db.Exec(
		"INSERT INTO api_keys (id, tenant_id, name, key_hash, key_prefix) VALUES (?, ?, ?, ?, ?)",
		id, tenantID, name, keyHash, prefix,
	)
	if err != nil {
		return "", err
	}
	return plainKey, nil
}

// ValidateAPIKey checks a plaintext key and returns the tenant ID it belongs to.
func (s *Store) ValidateAPIKey(plainKey string) (string, error) {
	hash := sha256.Sum256([]byte(plainKey))
	keyHash := hex.EncodeToString(hash[:])

	var tenantID string
	err := s.db.QueryRow(
		"SELECT tenant_id FROM api_keys WHERE key_hash = ?", keyHash,
	).Scan(&tenantID)
	if err != nil {
		return "", fmt.Errorf("invalid API key")
	}
	return tenantID, nil
}

// ListAPIKeys returns all keys for a tenant (no secrets).
func (s *Store) ListAPIKeys(tenantID string) ([]APIKeyInfo, error) {
	rows, err := s.db.Query(
		"SELECT id, tenant_id, name, key_prefix, created_at FROM api_keys WHERE tenant_id = ?",
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKeyInfo
	for rows.Next() {
		var k APIKeyInfo
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Name, &k.Prefix, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cloud/broker/... -v -run "TestCreate|TestValidate|TestList"
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/broker/store.go internal/cloud/broker/store_test.go
git commit -m "feat(broker): add SQLite store for tenants and API keys"
```

---

## Phase 2: Signup Endpoint

### Task 2: Create signup handler

**Files:**
- Create: `internal/cloud/broker/signup.go`
- Create: `internal/cloud/broker/signup_test.go`

POST /api/v1/signup creates a tenant, generates an API key, and returns both. This is the first thing a new customer does.

- [ ] **Step 1: Write the failing test**

```go
// internal/cloud/broker/signup_test.go
package broker

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSignupSuccess(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)
	handler := SignupHandler(store)

	body, _ := json.Marshal(SignupRequest{
		CompanyName: "Acme Security",
		Email:       "admin@acme.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp SignupResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.TenantID == "" {
		t.Fatal("empty tenant_id")
	}
	if resp.APIKey == "" {
		t.Fatal("empty api_key")
	}
	if len(resp.APIKey) < 10 {
		t.Fatalf("api_key too short: %q", resp.APIKey)
	}

	// Key should be valid.
	tenantID, err := store.ValidateAPIKey(resp.APIKey)
	if err != nil {
		t.Fatalf("validate key: %v", err)
	}
	if tenantID != resp.TenantID {
		t.Fatalf("tenant mismatch")
	}
}

func TestSignupDuplicateEmail(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)
	handler := SignupHandler(store)

	body, _ := json.Marshal(SignupRequest{
		CompanyName: "First",
		Email:       "same@example.com",
	})

	// First signup succeeds.
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first signup: %d", w1.Code)
	}

	// Second signup with same email fails.
	body2, _ := json.Marshal(SignupRequest{CompanyName: "Second", Email: "same@example.com"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate email: %d, want 409", w2.Code)
	}
}

func TestSignupMissingFields(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)
	handler := SignupHandler(store)

	body, _ := json.Marshal(SignupRequest{CompanyName: "", Email: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/signup", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cloud/broker/... -v -run TestSignup
```

- [ ] **Step 3: Write implementation**

```go
// internal/cloud/broker/signup.go
package broker

import (
	"encoding/json"
	"net/http"
	"strings"
)

// SignupRequest is the body for POST /api/v1/signup.
type SignupRequest struct {
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
}

// SignupResponse is returned on successful signup.
type SignupResponse struct {
	TenantID string `json:"tenant_id"`
	APIKey   string `json:"api_key"`
	Message  string `json:"message"`
}

// SignupHandler returns an http.Handler for POST /api/v1/signup.
func SignupHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		var req SignupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		req.Email = strings.TrimSpace(strings.ToLower(req.Email))
		req.CompanyName = strings.TrimSpace(req.CompanyName)

		if req.Email == "" || req.CompanyName == "" {
			http.Error(w, `{"error":"company_name and email are required"}`, http.StatusBadRequest)
			return
		}

		// Check for duplicate email.
		if existing, _ := store.GetTenantByEmail(req.Email); existing != nil {
			http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
			return
		}

		tenantID, err := store.CreateTenant(req.CompanyName, req.Email)
		if err != nil {
			http.Error(w, `{"error":"failed to create account"}`, http.StatusInternalServerError)
			return
		}

		apiKey, err := store.CreateAPIKey(tenantID, "default")
		if err != nil {
			http.Error(w, `{"error":"failed to create API key"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(SignupResponse{
			TenantID: tenantID,
			APIKey:   apiKey,
			Message:  "Account created. Use this API key as your cloud token in Settings → Remote Access. Save it — it won't be shown again.",
		})
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cloud/broker/... -v -run TestSignup
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/broker/signup.go internal/cloud/broker/signup_test.go
git commit -m "feat(broker): add signup endpoint for tenant + API key creation"
```

---

## Phase 3: Per-Tenant Broker Auth

### Task 3: Replace shared token with API key validation

**Files:**
- Create: `internal/cloud/broker/auth.go`
- Create: `internal/cloud/broker/auth_test.go`
- Modify: `cmd/cloudbroker/main.go`

The broker's `Authenticate` callback currently checks against a single hardcoded token. Replace it with API key validation from the store.

- [ ] **Step 1: Write the failing test**

```go
// internal/cloud/broker/auth_test.go
package broker

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAuthenticateValidKey(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)

	tenantID, _ := store.CreateTenant("Test Co", "test@test.com")
	apiKey, _ := store.CreateAPIKey(tenantID, "default")

	auth := NewAuthenticator(store)

	gotTenantID, ok := auth.Authenticate(apiKey)
	if !ok {
		t.Fatal("expected valid")
	}
	if gotTenantID != tenantID {
		t.Fatalf("tenant = %q, want %q", gotTenantID, tenantID)
	}
}

func TestAuthenticateInvalidKey(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)

	auth := NewAuthenticator(store)

	_, ok := auth.Authenticate("kvue_bad_key_here")
	if ok {
		t.Fatal("expected invalid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/cloud/broker/... -v -run TestAuthenticate
```

- [ ] **Step 3: Write implementation**

```go
// internal/cloud/broker/auth.go
package broker

// Authenticator validates API keys against the store.
type Authenticator struct {
	store *Store
}

// NewAuthenticator creates an authenticator backed by the store.
func NewAuthenticator(store *Store) *Authenticator {
	return &Authenticator{store: store}
}

// Authenticate validates an API key and returns the tenant ID.
// This matches the signature expected by connect.BrokerConfig.Authenticate.
func (a *Authenticator) Authenticate(token string) (string, bool) {
	tenantID, err := a.store.ValidateAPIKey(token)
	if err != nil {
		return "", false
	}
	return tenantID, true
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/cloud/broker/... -v -run TestAuthenticate
```

Expected: all 2 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/broker/auth.go internal/cloud/broker/auth_test.go
git commit -m "feat(broker): add API key authenticator for per-tenant auth"
```

---

### Task 4: Wire store, auth, and signup into cloud broker

**Files:**
- Modify: `cmd/cloudbroker/main.go`

Replace the hardcoded single-token auth with the new store-backed authenticator, add the signup endpoint, and persist the store to disk.

- [ ] **Step 1: Read `cmd/cloudbroker/main.go` and modify**

Add imports:
```go
"database/sql"
"github.com/bluenviron/mediamtx/internal/cloud/broker"
_ "modernc.org/sqlite"
```

Add a new flag:
```go
dbPath := flag.String("db", "/opt/raikada/broker.db", "SQLite database path")
```

After flag parsing, create the store:
```go
db, err := sql.Open("sqlite", *dbPath)
if err != nil {
    log.Error("failed to open database", "error", err)
    os.Exit(1)
}
defer db.Close()

store, err := broker.NewStore(db)
if err != nil {
    log.Error("failed to init store", "error", err)
    os.Exit(1)
}

auth := broker.NewAuthenticator(store)
```

Replace the `Authenticate` callback in the broker config:
```go
Authenticate: auth.Authenticate,
```

Update the `StreamProxyConfig.TenantID` — it can no longer be a single static value. Modify the stream proxy to look up tenant from the request's API key or from the registry session. For MVP, accept tenant_id as a query param (the resolve endpoint already returns it).

Add signup endpoint to the mux:
```go
mux.Handle("/api/v1/signup", broker.SignupHandler(store))
```

- [ ] **Step 2: Remove the `-token` flag default or keep as fallback**

Keep the `-token` flag as a legacy fallback. If set, also create a "bootstrap" tenant with that token:
```go
if *token != "" {
    // Create bootstrap tenant if it doesn't exist.
    if _, err := store.GetTenantByEmail("bootstrap@raikada.com"); err != nil {
        tenantID, _ := store.CreateTenant("Bootstrap", "bootstrap@raikada.com")
        // Create a key with the exact token value for backwards compat.
        // (This requires adding a CreateAPIKeyWithValue method to the store.)
    }
}
```

For MVP, just keep the token flag as a simple fallback in the Authenticate callback:
```go
Authenticate: func(t string) (string, bool) {
    // Try API key store first.
    if tenantID, ok := auth.Authenticate(t); ok {
        return tenantID, true
    }
    // Fallback to legacy token.
    if *token != "" && t == *token {
        return *tenantID, true
    }
    return "", false
},
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./cmd/cloudbroker/...
```

- [ ] **Step 4: Commit**

```bash
git add cmd/cloudbroker/main.go
git commit -m "feat(cloudbroker): wire per-tenant auth and signup endpoint"
```

---

## Phase 4: Account Management Endpoints

### Task 5: Add account info and key management endpoints

**Files:**
- Create: `internal/cloud/broker/accounts.go`
- Create: `internal/cloud/broker/accounts_test.go`

Authenticated endpoints for customers to view their account and manage API keys.

- [ ] **Step 1: Write the failing test**

```go
// internal/cloud/broker/accounts_test.go
package broker

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAccountInfoEndpoint(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)

	tenantID, _ := store.CreateTenant("Test Co", "test@test.com")
	apiKey, _ := store.CreateAPIKey(tenantID, "default")

	handler := AccountHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	if resp["name"] != "Test Co" {
		t.Fatalf("name = %v", resp["name"])
	}
	if resp["email"] != "test@test.com" {
		t.Fatalf("email = %v", resp["email"])
	}
}

func TestAccountInfoUnauthorized(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)

	handler := AccountHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestListKeysEndpoint(t *testing.T) {
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)

	tenantID, _ := store.CreateTenant("Test Co", "test@test.com")
	apiKey, _ := store.CreateAPIKey(tenantID, "default")
	store.CreateAPIKey(tenantID, "mobile")

	handler := ListKeysHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/keys", nil)
	req.Header.Set("X-API-Key", apiKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp struct {
		Keys []APIKeyInfo `json:"keys"`
	}
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Keys) != 2 {
		t.Fatalf("keys = %d, want 2", len(resp.Keys))
	}
}
```

- [ ] **Step 2: Write implementation**

```go
// internal/cloud/broker/accounts.go
package broker

import (
	"encoding/json"
	"net/http"
)

// authenticateRequest extracts and validates the API key from the request.
// Returns the tenant ID or writes a 401 and returns empty string.
func authenticateRequest(store *Store, w http.ResponseWriter, r *http.Request) string {
	key := r.Header.Get("X-API-Key")
	if key == "" {
		key = r.Header.Get("Authorization")
		if len(key) > 7 && key[:7] == "Bearer " {
			key = key[7:]
		}
	}
	if key == "" {
		http.Error(w, `{"error":"API key required"}`, http.StatusUnauthorized)
		return ""
	}

	tenantID, err := store.ValidateAPIKey(key)
	if err != nil {
		http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
		return ""
	}
	return tenantID
}

// AccountHandler returns tenant info for the authenticated API key holder.
func AccountHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := authenticateRequest(store, w, r)
		if tenantID == "" {
			return
		}

		tenant, err := store.GetTenant(tenantID)
		if err != nil {
			http.Error(w, `{"error":"tenant not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"tenant_id":  tenant.ID,
			"name":       tenant.Name,
			"email":      tenant.Email,
			"created_at": tenant.CreatedAt,
		})
	})
}

// ListKeysHandler returns all API keys for the authenticated tenant.
func ListKeysHandler(store *Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenantID := authenticateRequest(store, w, r)
		if tenantID == "" {
			return
		}

		keys, err := store.ListAPIKeys(tenantID)
		if err != nil {
			http.Error(w, `{"error":"failed to list keys"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": keys,
		})
	})
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/cloud/broker/... -v -run "TestAccount|TestListKeys"
```

Expected: all 3 tests PASS.

- [ ] **Step 4: Wire into cloudbroker main.go**

Add to the mux in `cmd/cloudbroker/main.go`:

```go
mux.Handle("/api/v1/account", broker.AccountHandler(store))
mux.Handle("/api/v1/account/keys", broker.ListKeysHandler(store))
```

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/broker/accounts.go internal/cloud/broker/accounts_test.go cmd/cloudbroker/main.go
git commit -m "feat(broker): add account info and API key management endpoints"
```

---

## Phase 5: Integration Test

### Task 6: End-to-end test: signup → connect → resolve

**Files:**
- Create: `internal/cloud/broker/integration_test.go`

Test the full flow: signup for an account, get an API key, connect a Directory with that key, resolve the site.

- [ ] **Step 1: Write the integration test**

```go
// internal/cloud/broker/integration_test.go
package broker

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"context"

	"github.com/bluenviron/mediamtx/internal/cloud/connect"
	"github.com/bluenviron/mediamtx/internal/directory/cloudconnector"
	_ "modernc.org/sqlite"
)

func TestSignupConnectResolve(t *testing.T) {
	// 1. Set up store + broker + signup.
	db, _ := sql.Open("sqlite", ":memory:")
	defer db.Close()
	store, _ := NewStore(db)

	registry := connect.NewRegistry()
	auth := NewAuthenticator(store)

	brokerHandler := connect.NewBroker(connect.BrokerConfig{
		Registry:     registry,
		Authenticate: auth.Authenticate,
		Logger:       slog.Default(),
	})

	resolveHandler := connect.ResolveHandler(connect.ResolveConfig{
		Registry: registry,
	})

	mux := http.NewServeMux()
	mux.Handle("/api/v1/signup", SignupHandler(store))
	mux.Handle("/ws/directory", brokerHandler)
	mux.Handle("/connect/resolve", resolveHandler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 2. Signup.
	signupBody, _ := json.Marshal(SignupRequest{
		CompanyName: "Integration Test Co",
		Email:       "int@test.com",
	})
	signupResp, err := http.Post(srv.URL+"/api/v1/signup", "application/json", bytes.NewReader(signupBody))
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	defer signupResp.Body.Close()

	if signupResp.StatusCode != http.StatusCreated {
		t.Fatalf("signup status = %d", signupResp.StatusCode)
	}

	var signup SignupResponse
	json.NewDecoder(signupResp.Body).Decode(&signup)
	t.Logf("tenant=%s key=%s...%s", signup.TenantID, signup.APIKey[:10], signup.APIKey[len(signup.APIKey)-5:])

	// 3. Connect a Directory with the API key.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/directory"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	connector := cloudconnector.New(cloudconnector.Config{
		URL:   wsURL,
		Token: signup.APIKey,
		Site: cloudconnector.SiteInfo{
			ID:    "test-site",
			Alias: "my-office",
		},
		HeartbeatInterval: 30 * time.Second,
		Logger:            slog.Default(),
	})

	go connector.Run(ctx)
	time.Sleep(500 * time.Millisecond)

	// 4. Resolve the site using the tenant ID from signup.
	resolveURL := srv.URL + "/connect/resolve?tenant_id=" + signup.TenantID + "&alias=my-office"
	resolveResp, err := http.Get(resolveURL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	defer resolveResp.Body.Close()

	if resolveResp.StatusCode != http.StatusOK {
		t.Fatalf("resolve status = %d", resolveResp.StatusCode)
	}

	var plan connect.ConnectionPlan
	json.NewDecoder(resolveResp.Body).Decode(&plan)

	if plan.SiteAlias != "my-office" {
		t.Fatalf("alias = %q", plan.SiteAlias)
	}
	if plan.Status != "online" {
		t.Fatalf("status = %q", plan.Status)
	}

	t.Logf("signup → connect → resolve: OK (site=%s, alias=%s)", plan.SiteID, plan.SiteAlias)

	// 5. Disconnect and verify cleanup.
	cancel()
	time.Sleep(500 * time.Millisecond)

	_, ok := registry.LookupByAlias(signup.TenantID, "my-office")
	if ok {
		t.Fatal("site should be removed after disconnect")
	}
}
```

- [ ] **Step 2: Run test**

```bash
go test ./internal/cloud/broker/... -v -run TestSignupConnectResolve -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/cloud/broker/integration_test.go
git commit -m "test(broker): add e2e integration test for signup → connect → resolve flow"
```

---

## Summary

| Task | What It Delivers |
|------|-----------------|
| 1. Store | SQLite for tenants + API keys with hash-based validation |
| 2. Signup | POST /api/v1/signup → creates tenant + returns API key |
| 3. Auth | Replace shared token with per-tenant API key validation |
| 4. Wiring | Connect store, auth, signup to the cloud broker binary |
| 5. Accounts | GET /api/v1/account, GET /api/v1/account/keys |
| 6. Integration | E2E test: signup → connect Directory → resolve site |

**Customer flow after implementation:**
1. Customer visits `connect.raikada.com/api/v1/signup` and creates account
2. Gets back an API key (`kvue_...`)
3. Enters API key in NVR Settings → Remote Access
4. NVR connects to cloud using the key
5. Customer views cameras remotely at `{alias}.raikada.com`
6. Different customers are isolated — can't see each other's sites
