# `internal/shared/errors`

The Kaivue Recording Server error-handling framework.

This package owns three things:

1. The **`Error`** type that flows through every backend service and out to
   customers as a JSON envelope.
2. The **stable error-code registry** in `codes.go` — a public, append-only,
   never-reused list of dotted identifiers like `auth.invalid_credentials`.
3. The **fail-closed / fail-open policy** that decides whether a given
   failure denies a request or keeps the system serving from cache.

Owner: cross-cutting / SRE. Linear: KAI-424.

---

## The wire envelope

Every customer-visible error — HTTP, Connect-Go, gRPC, Flutter client — is
serialized as the JSON below. Field names are part of the public API.

```json
{
  "code":           "auth.invalid_credentials",
  "message":        "The username or password is incorrect.",
  "correlation_id": "01HXYZABCDEF123456",
  "suggestion":     "Check your credentials or reset your password."
}
```

The wrapped `Cause` is **never** included on the wire — it lives only in
server-side logs.

---

## Fail-closed for security, fail-open for recording

This is the most important policy in the package. Internalize it.

| Domain          | Policy        | Reasoning                                                                                  |
|-----------------|---------------|--------------------------------------------------------------------------------------------|
| `auth.*`        | **closed**    | If we cannot verify a user, deny. Never admit on auth provider outage.                     |
| `permission.*`  | **closed**    | If we cannot verify a role, deny.                                                          |
| `tenant.*`      | **closed**    | Cross-tenant leaks are unrecoverable; deny by default.                                     |
| `stream.*`      | **open**      | Recording is the product's core promise. Keep recording from cached camera state.          |
| `billing.*`     | **open**      | Billing blips never cause data loss.                                                       |
| `notification.*`| **open**      | Outbound channel hiccups never cause data loss.                                            |

Use the helper at policy decision points:

```go
if errs.IsSecurityCritical(errs.CodeOf(err)) {
    return http.StatusForbidden
}
// Fail open: log, queue, serve from cache, keep recording.
```

> **Concrete consequence:** if Zitadel goes down, admin sign-ins fail (closed),
> but recorders MUST keep writing segments to disk from cached camera state
> and re-key encryption locally. The recording loop checks `stream.*` errors,
> not `auth.*` errors, before dropping frames.

The package's `TestFailOpenRecordingUnderAuthOutage` pins the classification
half of this policy. The recorder package owns the runtime half.

---

## Usage

### Constructing a new error

```go
import errs "github.com/bluenviron/mediamtx/internal/shared/errors"

return errs.New(
    errs.CodeAuthInvalidCredentials,
    "The username or password is incorrect.",
    errs.WithCorrelationIDFromContext(ctx),
    errs.WithSuggestion("Check your credentials or reset your password."),
)
```

### Wrapping an underlying cause

```go
row, err := db.QueryRow(ctx, "...").Scan(&t)
if err != nil {
    return errs.Wrap(err, errs.CodeTenantNotFound, "Tenant not found.",
        errs.WithCorrelationIDFromContext(ctx))
}
```

`Wrap` automatically propagates the correlation ID from a wrapped `*Error`
if you don't supply one explicitly.

### Checking codes

```go
if errs.CodeOf(err) == errs.CodeStreamTokenExpired {
    // re-mint and retry
}
// or, idiomatically with errors.Is:
if stderrors.Is(err, errs.New(errs.CodeStreamTokenExpired, "")) { ... }
```

`errors.Is` matches by `Code`, so the message string is irrelevant for
classification.

### Correlation IDs in middleware

```go
func CorrelationMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get("X-Correlation-ID")
        if id == "" {
            id = ulid.Make().String()
        }
        ctx := errs.ContextWithCorrelationID(r.Context(), id)
        w.Header().Set("X-Correlation-ID", id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}
```

Downstream handlers then use `errs.WithCorrelationIDFromContext(ctx)` so the
same ID lands in logs, traces, and the wire envelope.

---

## Adding a new error code

1. Add a `const Code... Code = "<domain>.<reason>"` in `codes.go`.
2. Append a `CodeInfo{...}` entry to `Registry` with a one-line description.
3. If the new code is in a new domain that should fail closed, also add
   the domain to `securityCriticalDomains` — and request review from
   `security-compliance`.
4. Run `go test ./internal/shared/errors/...`. The `TestNoCodeReuse` and
   `TestRegistryWellFormed` tests will fail the build if the code is
   malformed or duplicated.

## Retiring an error code

Codes are **append-only**. To retire one, do NOT delete the entry. Instead,
set `Retired: true` on its `Registry` row and leave the `const` in place.
The string remains forbidden for reuse forever.
