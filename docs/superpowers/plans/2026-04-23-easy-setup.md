# Easy Setup — mDNS Discovery, Approval Pairing, Cloud Onboarding

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate manual configuration from multi-recorder and cloud-connected deployments — Recorders auto-discover the Directory via mDNS, pair via admin approval (no token copy-paste), and cloud onboarding happens through the admin UI with customer-chosen site alias.

**Architecture:** Three independent features wired into existing infrastructure. (1) Recorder boot calls the existing mDNS listener to find the Directory, then uses the existing pending-pairing request flow instead of requiring a pre-shared token. (2) The Directory wires four already-written pairing handlers into boot.go routes. (3) A new admin API endpoint + React settings tab lets the admin enter cloud credentials and a site alias, which triggers the cloud connector at runtime.

**Tech Stack:** Go 1.22+, React/TypeScript (Vite), SQLite, `http.NewServeMux()`, existing mDNS library, existing pairing library

---

## File Structure

### Feature 1: Recorder mDNS Discovery + Request-Based Pairing

| File | Responsibility |
|------|----------------|
| `internal/recorder/boot.go` | Modify: add mDNS discovery fallback before Joiner |
| `internal/recorder/discovery/listener.go` | Already exists — no changes needed |
| `internal/directory/boot.go` | Modify: wire 4 pending-pairing HTTP routes |
| `internal/directory/pairing/handler.go` | Already has RequestPairingHandler, ApprovePendingHandler, DenyPendingHandler, PollTokenHandler — verify they compile |

### Feature 2: Cloud Onboarding API

| File | Responsibility |
|------|----------------|
| `internal/directory/adminapi/cloud.go` | New: GET/PUT /api/v1/admin/cloud — read/write cloud connector settings |
| `internal/directory/adminapi/cloud_test.go` | New: tests for cloud settings API |
| `internal/directory/boot.go` | Modify: wire cloud settings endpoint, add runtime connector start/stop |
| `internal/directory/db/migrations/0009_cloud_settings.up.sql` | New: cloud_settings key-value table |
| `internal/directory/db/migrations/0009_cloud_settings.down.sql` | New: rollback |

### Feature 3: React Admin UI — Cloud Settings Tab

| File | Responsibility |
|------|----------------|
| `ui/src/pages/Settings.tsx` | Modify: add 'cloud' tab to TABS array |
| `ui/src/components/CloudSettings.tsx` | New: cloud settings form component |

---

## Phase 1: Wire Existing Pairing Handlers into Directory Routes

### Task 1: Register pending-pairing endpoints in boot.go

**Files:**
- Modify: `internal/directory/boot.go` (after the existing pairing endpoints around line 410)

The four handlers (RequestPairingHandler, ApprovePendingHandler, DenyPendingHandler, PollTokenHandler) already exist in `internal/directory/pairing/handler.go` but are not registered as HTTP routes. We need to wire them.

- [ ] **Step 1: Read the existing handler signatures**

Read `internal/directory/pairing/handler.go` to confirm the exact signatures of the four unregistered handlers. Note their parameter requirements.

- [ ] **Step 2: Add the four routes to boot.go**

In `internal/directory/boot.go`, after the existing pairing endpoints (after the line with `ListPendingHandler`), add:

```go
	// Approval-based pairing — Recorder discovers Directory via mDNS,
	// requests pairing, admin approves in UI.
	mux.HandleFunc("/api/v1/pairing/request", pairing.RequestPairingHandler(pairingSvc, pendingStore))
	mux.HandleFunc("/api/v1/pairing/pending/approve", pairing.ApprovePendingHandler(
		pairingSvc, pendingStore,
		func(r *http.Request) (pairing.UserID, bool) {
			uid := r.Header.Get("X-User-ID")
			return pairing.UserID(uid), uid != ""
		},
		func(r *http.Request) (string, bool) {
			id := r.URL.Query().Get("id")
			return id, id != ""
		},
	))
	mux.HandleFunc("/api/v1/pairing/pending/deny", pairing.DenyPendingHandler(
		pairingSvc, pendingStore,
		func(r *http.Request) (pairing.UserID, bool) {
			uid := r.Header.Get("X-User-ID")
			return pairing.UserID(uid), uid != ""
		},
		func(r *http.Request) (string, bool) {
			id := r.URL.Query().Get("id")
			return id, id != ""
		},
	))
	mux.HandleFunc("/api/v1/pairing/request/poll", pairing.PollTokenHandler(
		pendingStore, pairing.NewStore(ddb),
		func(r *http.Request) (string, bool) {
			id := r.URL.Query().Get("id")
			return id, id != ""
		},
	))
```

NOTE: Read the actual handler signatures first. The above is based on the exploration report — the exact parameter types may differ. Adapt to match what's actually in `handler.go`. If the handlers take different arguments (e.g., a `*Service` instead of `*PendingStore`), use what the code expects.

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/directory/...
```

Expected: clean compilation. If any handler signature doesn't match, read `handler.go` and fix the call site.

- [ ] **Step 4: Commit**

```bash
git add internal/directory/boot.go
git commit -m "feat(directory): wire approval-based pairing endpoints"
```

---

## Phase 2: Recorder mDNS Discovery Fallback

### Task 2: Add mDNS discovery to Recorder boot

**Files:**
- Modify: `internal/recorder/boot.go`

Currently the Recorder requires `MTX_DIRECTORY_URL` (or gets it from the pairing token). We add a fallback: if no URL is configured and no pairing token is present, use mDNS to discover the Directory, then use the request-based pairing flow.

- [ ] **Step 1: Read the current Recorder boot sequence**

Read `internal/recorder/boot.go` fully. Identify:
- Where `PairingToken` is read (around line 293)
- Where `DirectoryURL` is resolved
- The Joiner integration point
- How `BootConfig` provides the token

Also read `internal/recorder/discovery/listener.go` to confirm the `Listen` function signature:
```go
func Listen(timeout time.Duration, iface *net.Interface, log *slog.Logger) (*DirectoryInfo, error)
```

- [ ] **Step 2: Add mDNS discovery + request-based pairing fallback**

In `internal/recorder/boot.go`, add the import:
```go
"github.com/bluenviron/mediamtx/internal/recorder/discovery"
```

Before the Joiner runs (around line 293), add a discovery block. The logic should be:

```go
// If no pairing token and no directory URL configured, try mDNS discovery
// and request-based pairing (admin approval flow).
if cfg.PairingToken == "" {
    log.Info("recorder: no pairing token provided, attempting mDNS discovery")

    dirInfo, err := discovery.Listen(30*time.Second, nil, log)
    if err != nil {
        return nil, fmt.Errorf("recorder: mDNS discovery failed (set MTX_PAIRING_TOKEN or MTX_DIRECTORY_URL manually): %w", err)
    }

    directoryURL := fmt.Sprintf("http://%s:%d", dirInfo.SourceIP, dirInfo.Port)
    log.Info("recorder: discovered Directory via mDNS",
        "url", directoryURL,
        "hostname", dirInfo.Hostname)

    // Request pairing approval from the Directory.
    hostname, _ := os.Hostname()
    requestID, err := requestPairing(ctx, directoryURL, hostname)
    if err != nil {
        return nil, fmt.Errorf("recorder: pairing request failed: %w", err)
    }

    log.Info("recorder: pairing requested, waiting for admin approval",
        "request_id", requestID)

    // Poll for approval (blocks until approved, denied, or context cancelled).
    token, err := pollForToken(ctx, directoryURL, requestID, log)
    if err != nil {
        return nil, fmt.Errorf("recorder: pairing approval failed: %w", err)
    }

    cfg.PairingToken = token
    log.Info("recorder: pairing approved, proceeding with token")
}
```

- [ ] **Step 3: Implement requestPairing helper**

Add this function to `internal/recorder/boot.go` (or a new file `internal/recorder/autopair.go` if boot.go is already large):

```go
// requestPairing sends a pairing request to the Directory and returns the request ID.
func requestPairing(ctx context.Context, directoryURL, hostname string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"recorder_hostname": hostname,
		"note":              "Auto-discovered via mDNS",
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		directoryURL+"/api/v1/pairing/request", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.ID, nil
}
```

- [ ] **Step 4: Implement pollForToken helper**

```go
// pollForToken polls the Directory for a pairing token after admin approval.
// Blocks until approved, denied, expired, or context cancelled.
func pollForToken(ctx context.Context, directoryURL, requestID string, log *slog.Logger) (string, error) {
	pollURL := fmt.Sprintf("%s/api/v1/pairing/request/poll?id=%s", directoryURL, requestID)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL, nil)
			if err != nil {
				return "", err
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Warn("recorder: poll failed, retrying", "error", err)
				continue
			}

			var result struct {
				Status string `json:"status"`
				Token  string `json:"token"`
			}
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusAccepted:
				// Still pending — keep polling.
				log.Debug("recorder: still waiting for approval")
				continue
			case http.StatusOK:
				if result.Token == "" {
					return "", fmt.Errorf("approved but no token returned")
				}
				return result.Token, nil
			case http.StatusForbidden:
				return "", fmt.Errorf("pairing denied by admin")
			case http.StatusGone:
				return "", fmt.Errorf("pairing request expired")
			default:
				return "", fmt.Errorf("unexpected poll status %d", resp.StatusCode)
			}
		}
	}
}
```

- [ ] **Step 5: Verify compilation**

```bash
go build ./internal/recorder/...
```

Expected: clean compilation.

- [ ] **Step 6: Commit**

```bash
git add internal/recorder/boot.go
git commit -m "feat(recorder): add mDNS discovery and approval-based pairing fallback"
```

---

## Phase 3: Cloud Settings API

### Task 3: DB migration for cloud settings

**Files:**
- Create: `internal/directory/db/migrations/0009_cloud_settings.up.sql`
- Create: `internal/directory/db/migrations/0009_cloud_settings.down.sql`

NOTE: Check the actual next migration number first — list existing files and use the next available number.

- [ ] **Step 1: Check existing migration numbers**

```bash
ls internal/directory/db/migrations/*.up.sql | tail -5
```

Use the next available number. The plan assumes 0009 but adjust accordingly.

- [ ] **Step 2: Write the up migration**

```sql
-- Cloud connector settings — persisted so they survive restarts.
-- Stored as key-value pairs for flexibility.
CREATE TABLE IF NOT EXISTS cloud_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

- [ ] **Step 3: Write the down migration**

```sql
DROP TABLE IF EXISTS cloud_settings;
```

- [ ] **Step 4: Commit**

```bash
git add internal/directory/db/migrations/
git commit -m "feat(directorydb): add cloud_settings table"
```

---

### Task 4: Cloud settings admin API endpoint

**Files:**
- Create: `internal/directory/adminapi/cloud.go`
- Create: `internal/directory/adminapi/cloud_test.go`

This endpoint lets the admin UI read and update cloud connector settings (URL, token, site alias).

- [ ] **Step 1: Write the failing test**

```go
// internal/directory/adminapi/cloud_test.go
package adminapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCloudSettingsGetEmpty(t *testing.T) {
	store := newTestCloudStore(t)
	h := CloudSettingsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cloud", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp CloudSettings
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Enabled {
		t.Fatal("expected enabled=false by default")
	}
	if resp.SiteAlias != "" {
		t.Fatalf("alias = %q, want empty", resp.SiteAlias)
	}
}

func TestCloudSettingsPutAndGet(t *testing.T) {
	store := newTestCloudStore(t)
	h := CloudSettingsHandler(store)

	// PUT settings
	body, _ := json.Marshal(CloudSettings{
		Enabled:   true,
		URL:       "wss://connect.raikada.com/ws/directory",
		Token:     "my-secret-token",
		SiteAlias: "my-office",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/cloud", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200, body = %s", w.Code, w.Body.String())
	}

	// GET settings back
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/cloud", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	var resp CloudSettings
	json.NewDecoder(w2.Body).Decode(&resp)

	if !resp.Enabled {
		t.Fatal("expected enabled=true")
	}
	if resp.URL != "wss://connect.raikada.com/ws/directory" {
		t.Fatalf("url = %q", resp.URL)
	}
	if resp.SiteAlias != "my-office" {
		t.Fatalf("alias = %q, want %q", resp.SiteAlias, "my-office")
	}
}

func TestCloudSettingsRequiresSiteAlias(t *testing.T) {
	store := newTestCloudStore(t)
	h := CloudSettingsHandler(store)

	body, _ := json.Marshal(CloudSettings{
		Enabled: true,
		URL:     "wss://connect.raikada.com/ws/directory",
		Token:   "token",
		// SiteAlias intentionally empty
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/cloud", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// newTestCloudStore creates an in-memory SQLite DB with the cloud_settings table.
func newTestCloudStore(t *testing.T) *CloudStore {
	t.Helper()
	db, err := NewTestDB(t)
	if err != nil {
		t.Fatalf("test db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS cloud_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return &CloudStore{db: db}
}
```

NOTE: The `NewTestDB` helper may not exist — if not, create the in-memory SQLite DB directly:
```go
import "database/sql"
import _ "modernc.org/sqlite"

func newTestCloudStore(t *testing.T) *CloudStore {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil { t.Fatal(err) }
    t.Cleanup(func() { db.Close() })
    _, err = db.Exec(`CREATE TABLE IF NOT EXISTS cloud_settings (
        key TEXT PRIMARY KEY, value TEXT NOT NULL,
        updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
    if err != nil { t.Fatal(err) }
    return &CloudStore{db: db}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/directory/adminapi/... -v -run TestCloudSettings
```

Expected: FAIL — types not defined.

- [ ] **Step 3: Write implementation**

```go
// internal/directory/adminapi/cloud.go
package adminapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// CloudSettings represents the cloud connector configuration.
type CloudSettings struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	SiteAlias string `json:"site_alias"`
}

// CloudStore reads/writes cloud settings from the directory SQLite DB.
type CloudStore struct {
	db *sql.DB
}

// NewCloudStore creates a new cloud settings store.
func NewCloudStore(db *sql.DB) *CloudStore {
	return &CloudStore{db: db}
}

// Get returns the current cloud settings. Returns zero-value CloudSettings
// if no settings are stored.
func (s *CloudStore) Get() (CloudSettings, error) {
	var cs CloudSettings
	rows, err := s.db.Query(`SELECT key, value FROM cloud_settings`)
	if err != nil {
		return cs, err
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return cs, err
		}
		switch k {
		case "enabled":
			cs.Enabled = v == "true"
		case "url":
			cs.URL = v
		case "token":
			cs.Token = v
		case "site_alias":
			cs.SiteAlias = v
		}
	}
	return cs, rows.Err()
}

// Put saves cloud settings. Upserts each key.
func (s *CloudStore) Put(cs CloudSettings) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO cloud_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	enabled := "false"
	if cs.Enabled {
		enabled = "true"
	}

	for _, kv := range []struct{ k, v string }{
		{"enabled", enabled},
		{"url", cs.URL},
		{"token", cs.Token},
		{"site_alias", cs.SiteAlias},
	} {
		if _, err := stmt.Exec(kv.k, kv.v, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// CloudSettingsHandler returns an http.Handler for GET/PUT /api/v1/admin/cloud.
func CloudSettingsHandler(store *CloudStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cs, err := store.Get()
			if err != nil {
				http.Error(w, `{"error":"failed to read settings"}`, http.StatusInternalServerError)
				return
			}
			// Mask the token in GET responses.
			if cs.Token != "" {
				cs.Token = "********"
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cs)

		case http.MethodPut:
			var cs CloudSettings
			if err := json.NewDecoder(r.Body).Decode(&cs); err != nil {
				http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
				return
			}

			// Validate: if enabling, require URL, token, and site alias.
			if cs.Enabled {
				if cs.URL == "" || cs.Token == "" || cs.SiteAlias == "" {
					http.Error(w, `{"error":"url, token, and site_alias are required when enabled"}`, http.StatusBadRequest)
					return
				}
			}

			// If token is masked, keep the existing one.
			if cs.Token == "********" {
				existing, _ := store.Get()
				cs.Token = existing.Token
			}

			if err := store.Put(cs); err != nil {
				http.Error(w, `{"error":"failed to save settings"}`, http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})

		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/directory/adminapi/... -v -run TestCloudSettings
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/directory/adminapi/cloud.go internal/directory/adminapi/cloud_test.go
git commit -m "feat(adminapi): add cloud settings GET/PUT endpoint"
```

---

### Task 5: Wire cloud settings endpoint and runtime connector management in boot.go

**Files:**
- Modify: `internal/directory/boot.go`

Wire the cloud settings endpoint and modify the connector startup to load settings from the DB on boot. Add a callback so the admin API can restart the connector when settings change.

- [ ] **Step 1: Add cloud settings route to the mux**

In `internal/directory/boot.go`, in the handler registration section (after the admin endpoints), add:

```go
	// Cloud connector settings — admin UI reads/writes these
	cloudStore := adminapi.NewCloudStore(ddb.RawDB())
	mux.Handle("/api/v1/admin/cloud", adminapi.CloudSettingsHandler(cloudStore))
```

NOTE: `ddb.RawDB()` may not exist — check the directorydb.DB type. It might expose the underlying `*sql.DB` as a field or method. If the type is `type DB struct { *sql.DB }`, you can use `ddb.DB` directly. Read the actual type definition and adapt.

- [ ] **Step 2: Load cloud settings from DB for connector startup**

Modify the cloud connector startup block (step 8, added in the previous plan) to read from the DB instead of only from BootConfig:

```go
	// ---------------------------------------------------------------
	// 8. Start cloud connector (from DB settings or config)
	// ---------------------------------------------------------------
	cloudSettings, _ := cloudStore.Get()

	// Config takes precedence over DB settings (for env var overrides).
	cloudURL := cfg.CloudConnectURL
	cloudToken := cfg.CloudConnectToken
	cloudAlias := cfg.CloudSiteAlias
	if cloudURL == "" && cloudSettings.Enabled {
		cloudURL = cloudSettings.URL
		cloudToken = cloudSettings.Token
		cloudAlias = cloudSettings.SiteAlias
	}

	if cloudURL != "" {
		log.Info("directory: starting cloud connector",
			"url", cloudURL, "alias", cloudAlias)

		cloudCtx, cloudCancel := context.WithCancel(context.Background())
		srv.cloudCancel = cloudCancel

		cc := cloudconnector.New(cloudconnector.Config{
			URL:   cloudURL,
			Token: cloudToken,
			Site: cloudconnector.SiteInfo{
				ID:    cloudAlias,
				Alias: cloudAlias,
				Capabilities: cloudconnector.Capabilities{
					Streams:  true,
					Playback: true,
					AI:       true,
				},
			},
			Logger: log.With(slog.String("component", "cloudconnector")),
		})
		srv.CloudConn = cc
		go cc.Run(cloudCtx)
	} else {
		log.Info("directory: cloud connector disabled (air-gapped mode)")
	}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/directory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/directory/boot.go
git commit -m "feat(directory): wire cloud settings API and load from DB on boot"
```

---

## Phase 4: React Admin UI — Cloud Settings Tab

### Task 6: Add CloudSettings React component

**Files:**
- Create: `ui/src/components/CloudSettings.tsx`

- [ ] **Step 1: Read the existing Settings.tsx to understand patterns**

Read `ui/src/pages/Settings.tsx` to see:
- How tabs are defined
- How existing settings panels are structured
- What API client pattern is used (`apiFetch` from `../api/client`)
- What CSS/styling patterns are used

- [ ] **Step 2: Create the CloudSettings component**

```tsx
// ui/src/components/CloudSettings.tsx
import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

interface CloudSettingsData {
  enabled: boolean
  url: string
  token: string
  site_alias: string
}

export default function CloudSettings() {
  const [settings, setSettings] = useState<CloudSettingsData>({
    enabled: false,
    url: '',
    token: '',
    site_alias: '',
  })
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')

  useEffect(() => {
    apiFetch('/admin/cloud')
      .then((res) => res.json())
      .then((data) => setSettings(data))
      .catch(() => setError('Failed to load cloud settings'))
  }, [])

  const handleSave = async () => {
    setError('')
    setMessage('')

    if (settings.enabled && !settings.site_alias.trim()) {
      setError('Site alias is required')
      return
    }

    setSaving(true)
    try {
      const res = await apiFetch('/admin/cloud', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      })
      if (!res.ok) {
        const data = await res.json()
        setError(data.error || 'Failed to save')
        return
      }
      setMessage('Settings saved. Restart the server to apply changes.')
    } catch {
      setError('Failed to save settings')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{ maxWidth: 600 }}>
      <h3>Remote Access</h3>
      <p style={{ color: '#888', marginBottom: 16 }}>
        Connect this site to the cloud for remote viewing, notifications, and AI features.
      </p>

      <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 16 }}>
        <input
          type="checkbox"
          checked={settings.enabled}
          onChange={(e) => setSettings({ ...settings, enabled: e.target.checked })}
        />
        Enable cloud connection
      </label>

      {settings.enabled && (
        <>
          <div style={{ marginBottom: 12 }}>
            <label>Site Alias</label>
            <input
              type="text"
              value={settings.site_alias}
              onChange={(e) => setSettings({ ...settings, site_alias: e.target.value })}
              placeholder="my-office"
              style={{ display: 'block', width: '100%', padding: 8, marginTop: 4 }}
            />
            <small style={{ color: '#888' }}>
              A unique name for this site. Used for remote access URLs.
            </small>
          </div>

          <div style={{ marginBottom: 12 }}>
            <label>Cloud URL</label>
            <input
              type="text"
              value={settings.url}
              onChange={(e) => setSettings({ ...settings, url: e.target.value })}
              placeholder="wss://connect.raikada.com/ws/directory"
              style={{ display: 'block', width: '100%', padding: 8, marginTop: 4 }}
            />
          </div>

          <div style={{ marginBottom: 12 }}>
            <label>Cloud Token</label>
            <input
              type="password"
              value={settings.token}
              onChange={(e) => setSettings({ ...settings, token: e.target.value })}
              placeholder="Enter cloud token"
              style={{ display: 'block', width: '100%', padding: 8, marginTop: 4 }}
            />
          </div>
        </>
      )}

      <button onClick={handleSave} disabled={saving} style={{ marginTop: 8 }}>
        {saving ? 'Saving...' : 'Save'}
      </button>

      {message && <p style={{ color: 'green', marginTop: 8 }}>{message}</p>}
      {error && <p style={{ color: 'red', marginTop: 8 }}>{error}</p>}
    </div>
  )
}
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/components/CloudSettings.tsx
git commit -m "feat(ui): add CloudSettings component for remote access configuration"
```

---

### Task 7: Add cloud tab to Settings page

**Files:**
- Modify: `ui/src/pages/Settings.tsx`

- [ ] **Step 1: Read Settings.tsx to find exact insertion points**

Read the file. Find:
- The `TABS` array definition
- The tab content rendering (likely a switch or conditional)
- The import section

- [ ] **Step 2: Add import**

At the top of Settings.tsx, add:

```tsx
import CloudSettings from '../components/CloudSettings'
```

- [ ] **Step 3: Add tab to TABS array**

Add `'cloud'` to the TabId type and add the tab entry. Insert it as the second tab (after 'system', before 'sysconfig') since remote access is a primary feature:

```tsx
{ id: 'cloud', label: 'Remote Access' },
```

- [ ] **Step 4: Add tab content**

In the tab content rendering section, add a case for 'cloud':

```tsx
{activeTab === 'cloud' && <CloudSettings />}
```

- [ ] **Step 5: Verify the UI builds**

```bash
cd ui && npm run build
```

Expected: clean build.

- [ ] **Step 6: Commit**

```bash
git add ui/src/pages/Settings.tsx
git commit -m "feat(ui): add Remote Access tab to Settings page"
```

---

## Summary

| Task | What It Does | Effort |
|------|-------------|--------|
| 1. Wire pairing routes | Enables approval-based pairing (handlers already exist) | Small |
| 2. Recorder mDNS fallback | Auto-discover Directory, request pairing, poll for approval | Medium |
| 3. Cloud settings migration | Persist cloud config in SQLite | Small |
| 4. Cloud settings API | GET/PUT endpoint for admin UI | Medium |
| 5. Boot integration | Load cloud settings from DB, wire endpoint | Small |
| 6. CloudSettings component | React form for remote access config | Medium |
| 7. Settings tab | Add "Remote Access" tab to admin UI | Small |

After completion, the setup experience is:
- **Single server:** `docker run mediamtx-nvr` — done
- **Multi-server:** Run Directory + Recorder(s) — Recorders auto-discover and request pairing, admin clicks Approve
- **Remote access:** Open Settings → Remote Access → enter alias → Save
