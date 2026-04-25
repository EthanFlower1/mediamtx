# Move Pairing Token to Shared — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the cross-boundary import `recorder/pairing → directory/pairing` by extracting the `PairingToken` type and codec functions to `internal/shared/pairing/`.

**Architecture:** `PairingToken` is the wire format exchanged between Directory and Recorder during enrollment. It's a pure data type with encode/decode/sign/verify functions and zero dependencies on either role's internals. Moving it to `shared/` makes both sides import the token contract from neutral ground, satisfying the depguard boundary.

**Tech Stack:** Go, ed25519, HKDF

---

## File Map

| File | Change | What |
|------|--------|------|
| `internal/shared/pairing/token.go` | Create | `PairingToken` struct, `UserID`, `TokenTTL`, `Encode`, `Decode`, `DecodeTokenUnsafe`, `NewSigningKey`, `VerifyPublicKey` |
| `internal/shared/pairing/token_test.go` | Create | Token encode/decode round-trip tests (copied from `directory/pairing/token_test.go`) |
| `internal/directory/pairing/token.go` | Delete | Replaced by shared/pairing |
| `internal/directory/pairing/token_test.go` | Delete | Replaced by shared/pairing |
| `internal/directory/pairing/service.go` | Modify | Import `shared/pairing` instead of local token type |
| `internal/directory/pairing/handler.go` | Modify | Same import switch |
| `internal/directory/pairing/store.go` | Modify | Same if it references PairingToken |
| `internal/directory/pairing/pending.go` | Modify | Same if it references PairingToken |
| `internal/directory/pairing/sweeper.go` | Modify | Same if it references PairingToken |
| `internal/recorder/pairing/join.go` | Modify | Import `shared/pairing` instead of `directory/pairing` |
| `internal/recorder/pairing/join_test.go` | Modify | Same import switch |
| `internal/shared/runtime/allinone.go` | Modify | If it references directory/pairing types |

---

## Task 1: Create `internal/shared/pairing/token.go`

**Files:**
- Create: `internal/shared/pairing/token.go`
- Create: `internal/shared/pairing/token_test.go`

- [ ] **Step 1: Copy token.go to shared/pairing**

```bash
mkdir -p internal/shared/pairing
cp internal/directory/pairing/token.go internal/shared/pairing/token.go
```

The file is self-contained — it only imports stdlib (`crypto`, `encoding`, `time`) and `golang.org/x/crypto/hkdf`. No internal package dependencies. Change nothing except verify the package declaration says `package pairing`.

- [ ] **Step 2: Copy token_test.go to shared/pairing**

```bash
cp internal/directory/pairing/token_test.go internal/shared/pairing/token_test.go
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/shared/pairing/... -v -count=1
```

Expected: all tests pass — the token is fully self-contained.

- [ ] **Step 4: Commit**

```bash
git add internal/shared/pairing/
git commit -m "feat: extract PairingToken to internal/shared/pairing

PairingToken is the wire contract between Directory and Recorder.
Moving it to shared/ eliminates the cross-boundary import."
```

---

## Task 2: Switch directory/pairing to import from shared/pairing

**Files:**
- Modify: `internal/directory/pairing/service.go`
- Modify: `internal/directory/pairing/handler.go`
- Modify: `internal/directory/pairing/store.go`
- Modify: `internal/directory/pairing/pending.go`
- Modify: `internal/directory/pairing/sweeper.go`
- Modify: `internal/directory/pairing/pending_handler.go`
- Modify: `internal/directory/pairing/metrics.go`
- Modify: `internal/directory/pairing/checkin_handler_test.go`
- Modify: `internal/directory/pairing/pending_test.go`
- Delete: `internal/directory/pairing/token.go`
- Delete: `internal/directory/pairing/token_test.go`

- [ ] **Step 1: Find all files in directory/pairing that reference local token types**

```bash
grep -rn 'PairingToken\|UserID\|TokenTTL\|DecodeTokenUnsafe\|NewSigningKey\|VerifyPublicKey' internal/directory/pairing/ --include='*.go' -l
```

- [ ] **Step 2: Add import alias in each file**

For each file that references these types, add:

```go
sharedpairing "github.com/bluenviron/mediamtx/internal/shared/pairing"
```

Then prefix all references: `PairingToken` → `sharedpairing.PairingToken`, `UserID` → `sharedpairing.UserID`, etc.

Alternatively, since the package name is the same (`pairing`), if the file already uses `package pairing` declarations, these types can be referenced without a prefix — they're in a different package though, so an import is needed. Use the alias approach to be explicit.

- [ ] **Step 3: Delete the local token files**

```bash
git rm internal/directory/pairing/token.go internal/directory/pairing/token_test.go
```

- [ ] **Step 4: Verify build and tests**

```bash
go build ./internal/directory/pairing/...
go test ./internal/directory/pairing/... -v -count=1
```

- [ ] **Step 5: Commit**

```bash
git add internal/directory/pairing/
git commit -m "refactor: switch directory/pairing to use shared/pairing token

Deleted local token.go — PairingToken now imported from shared/."
```

---

## Task 3: Switch recorder/pairing to import from shared/pairing

**Files:**
- Modify: `internal/recorder/pairing/join.go`
- Modify: `internal/recorder/pairing/join_test.go`

- [ ] **Step 1: Update join.go import**

In `internal/recorder/pairing/join.go`, replace:

```go
dirpairing "github.com/bluenviron/mediamtx/internal/directory/pairing"
```

with:

```go
sharedpairing "github.com/bluenviron/mediamtx/internal/shared/pairing"
```

Then replace all `dirpairing.` references with `sharedpairing.`:
- `dirpairing.PairingToken` → `sharedpairing.PairingToken`
- `dirpairing.DecodeTokenUnsafe` → `sharedpairing.DecodeTokenUnsafe`
- `dirpairing.Decode` → `sharedpairing.Decode`

- [ ] **Step 2: Update join_test.go import**

Same replacement in the test file.

- [ ] **Step 3: Verify build and tests**

```bash
go build ./internal/recorder/pairing/...
go test ./internal/recorder/pairing/... -v -count=1
```

- [ ] **Step 4: Verify no cross-boundary imports remain**

```bash
grep -r '"github.com/bluenviron/mediamtx/internal/directory' internal/recorder/ --include='*.go'
```

Expected: zero results.

- [ ] **Step 5: Commit**

```bash
git add internal/recorder/pairing/
git commit -m "refactor: switch recorder/pairing to use shared/pairing token

Eliminates the last cross-boundary import between recorder and directory."
```

---

## Task 4: Update allinone.go if needed and verify

**Files:**
- Modify: `internal/shared/runtime/allinone.go` (if it references directory/pairing)

- [ ] **Step 1: Check if allinone.go has the cross-boundary import**

```bash
grep 'directory/pairing\|recorder/pairing' internal/shared/runtime/allinone.go
```

If it imports either, update to use `shared/pairing` for any token-related references.

- [ ] **Step 2: Full boundary verification**

```bash
echo "=== recorder importing directory ===" 
grep -r '"github.com/bluenviron/mediamtx/internal/directory' internal/recorder/ --include='*.go' -l
echo "=== directory importing recorder ==="
grep -r '"github.com/bluenviron/mediamtx/internal/recorder' internal/directory/ --include='*.go' -l
```

Expected: zero results for both.

- [ ] **Step 3: Full build and test**

```bash
go build ./...
go test ./internal/shared/pairing/... ./internal/directory/pairing/... ./internal/recorder/pairing/... -v -count=1
```

Expected: clean build, all tests pass.

- [ ] **Step 4: Commit if any changes**

```bash
git add -A
git commit -m "chore: verify zero cross-boundary imports between directory and recorder"
```
