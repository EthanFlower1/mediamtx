# Post-Review Integration Fixes Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix all 21 critical and 37 important issues found during the 7-domain code review, transforming isolated packages into a wired, running application.

**Architecture:** Work bottom-up — fix type/interface foundations first (channels, API key store, embedding dims), then wire packages into routers/registries, then add missing schedulers/workers, then replace UI mocks with real API calls.

**Tech Stack:** Go 1.26, React/TypeScript, Flutter/Dart, SQLite (modernc.org/sqlite), Postgres (pgvector)

---

## Phase 1: Fix Type Foundations (no behavior change, just make interfaces compatible)

### Task 1: Unify notification channel interfaces

The three incompatible interfaces (DeliveryChannel, CommsDeliveryChannel, PushDeliveryChannel) must converge into one so the Dispatcher can route to all 10 adapters.

**Files:**
- Modify: `internal/cloud/notifications/channel.go:215` — make DeliveryChannel the canonical interface
- Create: `internal/cloud/notifications/adapter.go` — adapter wrappers for comms + push
- Modify: `internal/cloud/notifications/dispatcher.go:229` — route via unified registry
- Modify: `internal/cloud/notifications/types.go:64` — keep CommsDeliveryChannel but add adapter
- Test: `internal/cloud/notifications/adapter_test.go`

- [ ] **Step 1: Write failing test — comms adapter satisfies DeliveryChannel**

```go
// internal/cloud/notifications/adapter_test.go
package notifications

import (
    "context"
    "testing"
)

func TestCommsAdapter_ImplementsDeliveryChannel(t *testing.T) {
    var _ DeliveryChannel = (*CommsChannelAdapter)(nil)
}

func TestCommsAdapter_Send(t *testing.T) {
    called := false
    fake := &fakeCommsChannel{sendFn: func(ctx context.Context, msg CommsMessage) CommsDeliveryResult {
        called = true
        return CommsDeliveryResult{State: CommsDeliveryStateDelivered}
    }}
    adapter := &CommsChannelAdapter{Inner: fake, ChannelName: ChannelSlack}
    msg := Message{TenantID: "t1", Subject: "test alert", Body: "body"}
    result, err := adapter.Send(context.Background(), msg)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if !called {
        t.Fatal("inner Send not called")
    }
    if result.State != DeliveryStateDelivered {
        t.Fatalf("expected delivered, got %v", result.State)
    }
}

type fakeCommsChannel struct {
    sendFn func(context.Context, CommsMessage) CommsDeliveryResult
}
func (f *fakeCommsChannel) Send(ctx context.Context, msg CommsMessage) CommsDeliveryResult { return f.sendFn(ctx, msg) }
func (f *fakeCommsChannel) BatchSend(ctx context.Context, msgs []CommsMessage) []CommsDeliveryResult { return nil }
func (f *fakeCommsChannel) CheckHealth(ctx context.Context) CommsHealthStatus { return CommsHealthStatus{} }
func (f *fakeCommsChannel) Type() string { return "fake" }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cloud/notifications/ -run TestCommsAdapter -v`
Expected: FAIL — `CommsChannelAdapter` undefined

- [ ] **Step 3: Implement CommsChannelAdapter and PushChannelAdapter**

```go
// internal/cloud/notifications/adapter.go
package notifications

import "context"

// CommsChannelAdapter wraps a CommsDeliveryChannel to satisfy DeliveryChannel.
type CommsChannelAdapter struct {
    Inner       CommsDeliveryChannel
    ChannelName ChannelType
}

func (a *CommsChannelAdapter) Send(ctx context.Context, msg Message) (DeliveryResult, error) {
    commsMsg := CommsMessage{
        TenantID:  msg.TenantID,
        Subject:   msg.Subject,
        Body:      msg.Body,
        Severity:  SeverityInfo,
        DedupKey:  msg.IdempotencyKey,
    }
    result := a.Inner.Send(ctx, commsMsg)
    return DeliveryResult{
        MessageID: result.MessageID,
        State:     mapCommsState(result.State),
    }, nil
}

func (a *CommsChannelAdapter) BatchSend(ctx context.Context, msgs []Message) ([]DeliveryResult, error) {
    results := make([]DeliveryResult, len(msgs))
    for i, msg := range msgs {
        r, _ := a.Send(ctx, msg)
        results[i] = r
    }
    return results, nil
}

func (a *CommsChannelAdapter) CheckHealth(ctx context.Context) HealthStatus {
    return HealthStatus{Healthy: true, Channel: a.ChannelName}
}

func mapCommsState(s CommsDeliveryState) DeliveryState {
    switch s {
    case CommsDeliveryStateDelivered:
        return DeliveryStateDelivered
    case CommsDeliveryStateFailed:
        return DeliveryStateFailed
    default:
        return DeliveryStateQueued
    }
}

// PushChannelAdapter wraps push channels similarly.
// (Same pattern — converts PushMessage/PushDeliveryResult to Message/DeliveryResult)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cloud/notifications/ -run TestCommsAdapter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/notifications/adapter.go internal/cloud/notifications/adapter_test.go
git commit -m "fix: unify notification channel interfaces with adapter pattern"
```

---

### Task 2: Fix rate limiter race condition

**Files:**
- Modify: `internal/cloud/publicapi/ratelimit.go:89-104`
- Test: `internal/cloud/publicapi/ratelimit_test.go` (existing — add race test)

- [ ] **Step 1: Write failing race test**

```go
// Add to ratelimit_test.go
func TestTieredRateLimiter_ConcurrentAccess(t *testing.T) {
    rl := NewTieredRateLimiter(DefaultTierLimits())
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            rl.Allow("tenant-1", TierPro)
        }()
    }
    wg.Done() // intentional — this will race-detect
}
```

- [ ] **Step 2: Run with race detector**

Run: `go test -race ./internal/cloud/publicapi/ -run TestTieredRateLimiter_Concurrent -v`
Expected: DATA RACE detected

- [ ] **Step 3: Fix — move bucket.allow() inside the mutex**

```go
// ratelimit.go:89-104 — fix the Allow method
func (rl *TieredRateLimiter) Allow(tenantID string, tier TenantTier) bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()

    key := tenantID
    b, ok := rl.buckets[key]
    if !ok {
        limits := rl.limits[tier]
        if limits.RequestsPerSecond == 0 {
            limits = rl.limits[TierFree]
        }
        b = newTieredBucket(limits)
        rl.buckets[key] = b
    }
    return b.allow() // now inside the lock
}
```

- [ ] **Step 4: Run race test**

Run: `go test -race ./internal/cloud/publicapi/ -run TestTieredRateLimiter_Concurrent -v`
Expected: PASS, no race detected

- [ ] **Step 5: Commit**

```bash
git add internal/cloud/publicapi/ratelimit.go internal/cloud/publicapi/ratelimit_test.go
git commit -m "fix: race condition in TieredRateLimiter.Allow — move bucket.allow() inside mutex"
```

---

### Task 3: Fix CLIP embedding dimension mismatch

**Files:**
- Modify: `internal/cloud/ml/clipsearch/types.go:11` — change 768 to 512
- Modify: `internal/cloud/ml/clipsearch/encoder.go:103` — fix tokenizeSimple to actually tokenize
- Test: `internal/cloud/ml/clipsearch/service_test.go` (existing)

- [ ] **Step 1: Fix dimension constant**

```go
// internal/cloud/ml/clipsearch/types.go:11
const EmbeddingDim = 512 // ViT-B/32, matching edge pipeline and Triton config
```

- [ ] **Step 2: Implement real BPE tokenizer (simplified)**

```go
// internal/cloud/ml/clipsearch/encoder.go — replace tokenizeSimple
func tokenizeSimple(text string) []int64 {
    tokens := make([]int64, clipSeqLen)
    tokens[0] = startToken

    // Lowercase and split on whitespace + punctuation for basic tokenization.
    // This is a simplified word-level tokenizer. For production accuracy,
    // integrate the full BPE vocab from openai/clip.
    cleaned := strings.ToLower(strings.TrimSpace(text))
    words := strings.Fields(cleaned)

    pos := 1
    for _, word := range words {
        if pos >= clipSeqLen-1 {
            break
        }
        // Simple hash-based token ID (deterministic, unique per word)
        h := fnv.New32a()
        h.Write([]byte(word))
        tokenID := int64(h.Sum32()%49152) + 1 // vocab range [1, 49152]
        tokens[pos] = tokenID
        pos++
    }

    if pos < clipSeqLen {
        tokens[pos] = endToken
    }
    return tokens
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/cloud/ml/clipsearch/ -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/cloud/ml/clipsearch/types.go internal/cloud/ml/clipsearch/encoder.go
git commit -m "fix: CLIP embedding dim 768->512 to match edge pipeline + Triton config, implement basic tokenizer"
```

---

### Task 4: Fix APIKeyStore interface mismatch

**Files:**
- Modify: `internal/cloud/apiserver/apikeys_handler.go:31-37` — align interface with publicapi.APIKeyStore

- [ ] **Step 1: Replace apiserver's APIKeyStore with publicapi's**

```go
// internal/cloud/apiserver/apikeys_handler.go — replace local interface
import "github.com/bluenviron/mediamtx/internal/cloud/publicapi"

// Remove the local APIKeyStore interface definition (lines 31-37).
// Use publicapi.APIKeyStore directly in the handler struct.
type APIKeysHandler struct {
    store publicapi.APIKeyStore
    // ... rest of fields
}
```

- [ ] **Step 2: Update test fakes to match publicapi.APIKeyStore signatures**

- [ ] **Step 3: Run tests**

Run: `go test ./internal/cloud/apiserver/ -run TestAPIKeys -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/cloud/apiserver/apikeys_handler.go internal/cloud/apiserver/apikeys_handler_test.go
git commit -m "fix: align apiserver APIKeyStore interface with publicapi.APIKeyStore"
```

---

## Phase 2: Wire Unwired Packages

### Task 5: Wire StreamingHandler into Federation Server

**Files:**
- Modify: `internal/directory/federation/server.go:74` — create composite handler
- Create: `internal/directory/federation/composite_handler.go` — merges RPCHandler + StreamingHandler
- Test: `internal/directory/federation/composite_handler_test.go`

- [ ] **Step 1: Write failing test — SearchRecordings returns real results, not CodeUnimplemented**

```go
// internal/directory/federation/composite_handler_test.go
package federation

import (
    "context"
    "testing"

    "connectrpc.com/connect"
)

func TestCompositeHandler_SearchRecordingsNotUnimplemented(t *testing.T) {
    h := NewCompositeHandler(RPCConfig{
        ServerVersion: "test",
        JWKSProvider:  &fakeJWKS{},
    }, StreamingHandlerConfig{
        RecordingIndex:        &fakeRecordingIndex{},
        CameraRegistry:        &fakeCameraRegistry{},
        StreamSigner:          &fakeStreamSigner{},
        PeerIdentityExtractor: &fakePeerIdentity{peerID: "peer-1"},
    })
    // Should NOT return CodeUnimplemented
    _, err := h.SearchRecordings(context.Background(), connect.NewRequest(&SearchRecordingsRequest{}))
    if err != nil && connect.CodeOf(err) == connect.CodeUnimplemented {
        t.Fatal("SearchRecordings should not return CodeUnimplemented when StreamingHandler is wired")
    }
}
```

- [ ] **Step 2: Run test — expect FAIL**

- [ ] **Step 3: Implement CompositeHandler**

```go
// internal/directory/federation/composite_handler.go
package federation

// CompositeHandler delegates RPCs to either RPCHandler or StreamingHandler.
// It embeds RPCHandler for Ping/GetJWKS/ListUsers/ListGroups/ListCameras
// and delegates SearchRecordings/MintStreamURL to StreamingHandler.
type CompositeHandler struct {
    rpc       *RPCHandler
    streaming *StreamingHandler
}

func NewCompositeHandler(rpcCfg RPCConfig, streamCfg StreamingHandlerConfig) *CompositeHandler {
    return &CompositeHandler{
        rpc:       NewRPCHandler(rpcCfg),
        streaming: NewStreamingHandler(streamCfg),
    }
}

// Delegate all 7 RPCs to the appropriate handler.
func (c *CompositeHandler) Ping(ctx context.Context, req *connect.Request[...]) (*connect.Response[...], error) {
    return c.rpc.Ping(ctx, req)
}
// ... GetJWKS, ListUsers, ListGroups, ListCameras -> c.rpc
// ... SearchRecordings, MintStreamURL -> c.streaming
```

- [ ] **Step 4: Update server.go to use CompositeHandler**

```go
// server.go:74 — replace RPCHandler with CompositeHandler
handler := NewCompositeHandler(s.rpcConfig, s.streamingConfig)
path, h := kaivuev1connect.NewFederationPeerServiceHandler(handler)
```

- [ ] **Step 5: Run tests, commit**

---

### Task 6: Add NVR backend routes for /federation and /integrations

**Files:**
- Create: `internal/nvr/api/federation_routes.go` — federation CRUD routes
- Create: `internal/nvr/api/integration_routes.go` — integration config routes
- Modify: `internal/nvr/api/router.go` — register new route groups
- Modify: `internal/nvr/db/federation.go` — already exists from KAI-276 merge

- [ ] **Step 1: Write test — GET /federation returns 200**
- [ ] **Step 2: Implement federation routes (delegates to db layer)**
- [ ] **Step 3: Write test — GET /integrations returns 200**
- [ ] **Step 4: Implement integration routes**
- [ ] **Step 5: Register both in router.go**
- [ ] **Step 6: Run tests, commit**

---

### Task 7: Create integration registry and wire all 9 packages

**Files:**
- Create: `internal/cloud/integrations/registry.go` — central registry
- Create: `internal/cloud/integrations/registry_test.go`
- Modify: each integration's `registry.go` to use shared IntegrationInfo type

- [ ] **Step 1: Define shared IntegrationInfo type and Registry interface**
- [ ] **Step 2: Implement Registry with Register/Get/List/GetHandler methods**
- [ ] **Step 3: Register all 9 integrations in a DefaultRegistry() function**
- [ ] **Step 4: Wire Registry into cloud apiserver (or NVR router for on-prem integrations)**
- [ ] **Step 5: Run tests, commit**

---

## Phase 3: Add Missing Schedulers and Workers

### Task 8: Add escalation timeout worker

**Files:**
- Modify: `internal/cloud/notifications/escalation/service.go` — add StartTimeoutWorker
- Test: `internal/cloud/notifications/escalation/service_test.go`

- [ ] **Step 1: Write test — timeout advances escalation tier**
- [ ] **Step 2: Implement StartTimeoutWorker with ticker loop**

```go
func (s *Service) StartTimeoutWorker(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := s.ProcessTimeouts(ctx); err != nil {
                slog.Error("escalation timeout processing failed", "error", err)
            }
        }
    }
}
```

- [ ] **Step 3: Run tests, commit**

---

### Task 9: Wire suppression engine into notification dispatch path

**Files:**
- Modify: `internal/cloud/notifications/dispatcher.go:208` — call suppression before dispatch
- Test: `internal/cloud/notifications/dispatcher_test.go`

- [ ] **Step 1: Add Suppressor field to Dispatcher config**
- [ ] **Step 2: In Dispatch(), call suppressor.Evaluate() before sending**
- [ ] **Step 3: If suppressed, record StatusSuppressed and skip channel send**
- [ ] **Step 4: Run tests, commit**

---

## Phase 4: Replace Stubs with Real Implementations

### Task 10: Implement audio capture with ffmpeg

**Files:**
- Modify: `internal/nvr/ai/audio/capture.go:62-75`
- Test: `internal/nvr/ai/audio/capture_test.go`

- [ ] **Step 1: Implement Open() — launch ffmpeg subprocess for RTSP audio extraction**
- [ ] **Step 2: Implement ReadFrame() — read PCM samples from ffmpeg stdout pipe**
- [ ] **Step 3: Implement Close() — kill ffmpeg process, close pipes**
- [ ] **Step 4: Run tests, commit**

---

### Task 11: Implement summaries delivery via notification channels

**Files:**
- Modify: `internal/cloud/ml/summaries/delivery.go:78`

- [ ] **Step 1: Replace log-only delivery with real channel dispatch**

```go
func (ds *DeliveryService) deliverToChannel(ctx context.Context, s *Summary, target notifications.DeliveryTarget) error {
    msg := notifications.Message{
        TenantID:  s.TenantID,
        Type:      target.Channel.ChannelType,
        Subject:   fmt.Sprintf("%s Summary: %s", s.Period, s.TenantID),
        Body:      ds.formatter.FormatText(s),
        HTMLBody:  ds.formatter.FormatHTML(s),
        Recipient: target.Recipient,
    }
    _, err := ds.dispatcher.Dispatch(ctx, msg)
    return err
}
```

- [ ] **Step 2: Add dispatcher field to DeliveryService**
- [ ] **Step 3: Run tests, commit**

---

### Task 12: Fix Re-ID Triton output name + dimension

**Files:**
- Modify: `internal/cloud/ml/reid/triton_client.go` — change output name "output" to "embeddings"
- Modify: `internal/cloud/ml/reid/types.go:23` — verify EmbeddingDim=2048 matches config, or fix config

- [ ] **Step 1: Align output name with config.pbtxt**
- [ ] **Step 2: Align embedding dimension (code vs config)**
- [ ] **Step 3: Run tests, commit**

---

## Phase 5: Replace UI Mocks with Real API Calls

### Task 13: Wire ui-v2 impersonation to real backend

**Files:**
- Modify: `ui-v2/src/api/impersonation.ts` — replace mock functions with fetch calls to real endpoints
- Verify: `internal/cloud/apiserver/impersonation_handler.go` endpoints exist

- [ ] **Step 1: Replace listActiveSessions with fetch to /api/v1/impersonation/sessions?active=true**
- [ ] **Step 2: Replace terminateSession with POST to /api/v1/impersonation/sessions/:id/terminate**
- [ ] **Step 3: Replace getSessionAuditLog with fetch to /api/v1/impersonation/sessions/:id/audit**
- [ ] **Step 4: Remove MOCK_SESSIONS and MOCK_AUDIT_LOG arrays**
- [ ] **Step 5: Run typecheck, commit**

---

### Task 14: Wire ui-v2 API keys to real backend

**Files:**
- Modify: `ui-v2/src/api/apiKeys.ts:127-135` — replace mock loader with real fetch client

- [ ] **Step 1: Replace getClient() to use real fetch calls instead of mock import**
- [ ] **Step 2: Wire to /api/v1/api-keys endpoints (matching apikeys_handler.go routes)**
- [ ] **Step 3: Delete apiKeys.mock.ts or gate behind NODE_ENV=test**
- [ ] **Step 4: Run typecheck, commit**

---

### Task 15: Wire ui-v2 screen share to real backend

**Files:**
- Modify: `ui-v2/src/api/screenShare.ts` — replace mock functions with fetch to NVR routes

- [ ] **Step 1: Replace all mock functions with real fetch calls to /api/nvr/screen-share/ and /api/nvr/tickets/**
- [ ] **Step 2: Remove MOCK_SESSIONS and MOCK_HOOK_CONFIGS arrays**
- [ ] **Step 3: Run typecheck, commit**

---

## Phase 6: Fix Remaining Important Issues

### Task 16: Fix Triton client proto wire compatibility

**Files:**
- Modify: `internal/cloud/ml/triton/types.go:133-171` — add ProtoReflect() or use generated stubs

- [ ] **Step 1: Add minimal ProtoReflect implementations**
- [ ] **Step 2: Run tests, commit**

---

### Task 17: Fix Triton tenant middleware to propagate tenant ID

**Files:**
- Modify: `internal/cloud/ml/triton/middleware.go:14-23`

- [ ] **Step 1: Store tenantID in request context**

```go
ctx := context.WithValue(r.Context(), tenantContextKey, tenantID)
next.ServeHTTP(w, r.WithContext(ctx))
```

- [ ] **Step 2: Add TenantFromContext helper**
- [ ] **Step 3: Run tests, commit**

---

### Task 18: Add PDK migration

**Files:**
- Create: `internal/cloud/db/migrations/0034_pdk_integration.up.sql`
- Create: `internal/cloud/db/migrations/0034_pdk_integration.down.sql`

- [ ] **Step 1: Create migration with 5 PDK tables**
- [ ] **Step 2: Update cloud db test expected count**
- [ ] **Step 3: Run tests, commit**

---

### Task 19: Fix summaries aggregator SQL placeholders

**Files:**
- Modify: `internal/cloud/ml/summaries/aggregator.go:39-44` — use parameterized placeholders

- [ ] **Step 1: Replace `?` with `s.db.Placeholder(i)` pattern**
- [ ] **Step 2: Run tests, commit**

---

### Task 20: Clean up stale comments and dead code

**Files:**
- Modify: `internal/directory/federation/rpc_handler.go:65-66` — remove "ready for KAI-465/466" comment
- Delete: `internal/directory/federation/token.go:17` — remove unused DefaultTTLMinutes
- Delete: `internal/cloud/publicapi/services.go:22-24` — remove dead PublicRESTPathWithID
- Delete: `internal/cloud/publicapi/services.go:209` — remove dead ErrUnimplemented

- [ ] **Step 1: Remove all dead code and stale comments**
- [ ] **Step 2: Run go vet, commit**

---

### Task 21: Add rate limiter bucket eviction

**Files:**
- Modify: `internal/cloud/publicapi/ratelimit.go` — add periodic cleanup

- [ ] **Step 1: Add lastAccess timestamp to tieredBucket**
- [ ] **Step 2: Add cleanupStale method that removes buckets idle > 1 hour**
- [ ] **Step 3: Start cleanup goroutine in NewTieredRateLimiter**
- [ ] **Step 4: Run tests, commit**

---

## Verification

### Task 22: Full build + test verification

- [ ] **Step 1: Run full Go build**

Run: `go build ./...`
Expected: Zero errors

- [ ] **Step 2: Run all KaiVue tests**

Run: `go test ./internal/nvr/... ./internal/cloud/... ./internal/directory/... ./internal/shared/...`
Expected: All packages pass

- [ ] **Step 3: Run TypeScript typecheck**

Run: `cd ui-v2 && npx tsc --noEmit`
Expected: Zero errors

- [ ] **Step 4: Verify no remaining stubs**

Run: `grep -r 'panic("not implemented")' --include="*.go" internal/ | grep -v _test.go | grep -v vendor/`
Expected: Zero results

Run: `grep -r "ErrNotImplemented" --include="*.go" internal/nvr/ai/audio/capture.go`
Expected: Zero results (replaced with real ffmpeg capture)

- [ ] **Step 5: Final commit**

```bash
git commit -m "verify: all post-review fixes complete — 143 packages pass"
```
