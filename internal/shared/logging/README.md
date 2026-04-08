# `internal/shared/logging`

Structured logging facade for the Kaivue Recording Server, built on Go's
standard `log/slog` package. Use this package instead of constructing
`slog` handlers directly so that every component emits comparable,
redaction-safe records.

## What you get

- **JSON in production, text in dev** — selected via `Options.Format`
  (`FormatJSON` / `FormatText`).
- **Consistent fields** on every record:
  `request_id`, `user_id`, `tenant_id`, `component`, `subsystem`.
- **Sensitive-field redaction** via a `slog.Handler` wrapper. Default
  allow-list blocks `password`, `passwd`, `token`, `secret`, `key`,
  `credential(s)`, `authorization`, `bearer`, `rtsp_credentials`, `jwt`,
  `api_key`, `client_secret` (case-insensitive). Matching descends into
  nested `slog.Group` values.
- **Context plumbing** for request-scoped loggers and request IDs.

## Quick start

```go
import (
    "log/slog"
    "github.com/bluenviron/mediamtx/internal/shared/logging"
)

logger := logging.New(logging.Options{
    Format:    logging.FormatJSON,
    Level:     slog.LevelInfo,
    Component: "directory",
    Subsystem: "indexer",
})

logger.Info("camera registered", "camera_id", "cam-42")
```

## Per-subsystem child loggers

```go
recLogger := logging.WithComponent(logger, "recorder")
segLogger := logging.WithSubsystem(recLogger, "segmenter")
segLogger.Info("rolled segment", "duration_ms", 5000)
```

## Per-request loggers (HTTP / Connect-Go middleware)

```go
func Middleware(base *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            reqID := r.Header.Get("X-Request-ID")
            if reqID == "" {
                reqID = newRequestID()
            }

            l := logging.WithRequest(base, logging.RequestFields{
                RequestID: reqID,
                UserID:    userIDFromAuth(r),
                TenantID:  tenantIDFromAuth(r),
            })

            ctx := logging.ContextWithRequestID(r.Context(), reqID)
            ctx = logging.ContextWithLogger(ctx, l)

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// Downstream code
func handle(ctx context.Context) {
    l := logging.LoggerFromContext(ctx, nil)
    l.Info("processing")
}
```

`LoggerFromContext` automatically attaches the context's `request_id`
even if the stored logger doesn't already carry it, so request IDs
always flow through to the sink.

## Redaction

Any attribute whose key (case-insensitive) is in the allow-list has its
value rewritten to `"[REDACTED]"`. This includes attributes added via
`logger.With(...)` and attributes inside nested `slog.Group` values:

```go
logger.Info("rtsp connect",
    slog.Group("camera",
        slog.String("id", "cam-1"),
        slog.Group("creds",
            slog.String("username", "admin"),
            slog.String("password", "REPLACE_ME"), // -> [REDACTED]
        ),
    ),
)
```

### Customizing the allow-list

Pass `RedactKeys` in `Options` to **replace** the default list:

```go
logger := logging.New(logging.Options{
    Format:     logging.FormatJSON,
    RedactKeys: append(logging.DefaultRedactKeys, "session_cookie", "stripe_pk"),
})
```

To disable redaction entirely (not recommended outside tests), set
`Options.DisableRedaction = true`.

## Defaults

- `Format` defaults to `FormatJSON`.
- `Writer` defaults to `os.Stdout`.
- `Level` defaults to `slog.LevelInfo` (zero value).
- Redaction is **on** unless `DisableRedaction` is set.

## Conventions

- Use the package-level `Field*` constants (`FieldRequestID`,
  `FieldUserID`, `FieldTenantID`, `FieldComponent`, `FieldSubsystem`)
  whenever attaching these fields manually so the names never drift.
- Never put raw secrets in test fixtures; use the literal `"REPLACE_ME"`.
- Logs that flow into a central aggregator (Loki etc.) **must** go
  through this package — direct `slog.NewJSONHandler` use bypasses
  redaction.
