// Package logging provides a structured logging facade built on Go's
// standard log/slog package for the Kaivue Recording Server.
//
// It standardizes:
//   - JSON output in production, text (optionally colorized) in development
//   - A consistent set of fields injected on every record
//     (request_id, user_id, tenant_id, component, subsystem)
//   - Sensitive-field redaction via a wrapping slog.Handler
//   - Context helpers for request-scoped loggers and request IDs
//
// Importers should use this package instead of constructing slog handlers
// directly so that all services emit comparable, redaction-safe logs.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Format selects the encoding for log output.
type Format string

const (
	// FormatJSON emits one JSON object per record. Use in production.
	FormatJSON Format = "json"
	// FormatText emits the slog text format. Use in development.
	FormatText Format = "text"
)

// Standard field names used across the codebase. Centralizing these as
// constants prevents drift between subsystems.
const (
	FieldRequestID = "request_id"
	FieldUserID    = "user_id"
	FieldTenantID  = "tenant_id"
	FieldComponent = "component"
	FieldSubsystem = "subsystem"
)

// RedactedValue is the literal string substituted for any attribute whose
// key matches the redaction allow-list.
const RedactedValue = "[REDACTED]"

// DefaultRedactKeys is the default case-insensitive set of attribute keys
// whose values must be redacted before they reach any sink.
//
// Keep this list intentionally broad: it is cheaper to over-redact than to
// leak credentials into a log aggregator.
var DefaultRedactKeys = []string{
	"password",
	"passwd",
	"token",
	"secret",
	"key",
	"credential",
	"credentials",
	"authorization",
	"bearer",
	"rtsp_credentials",
	"jwt",
	"api_key",
	"client_secret",
}

// Options configures a logger constructed via New.
type Options struct {
	// Format selects JSON vs text output. Empty defaults to JSON.
	Format Format
	// Level is the minimum slog level to emit. Zero value = LevelInfo.
	Level slog.Level
	// Writer is the sink. Nil defaults to os.Stdout.
	Writer io.Writer
	// AddSource toggles slog source location capture.
	AddSource bool
	// Component populated as the FieldComponent on every record.
	Component string
	// Subsystem populated as the FieldSubsystem on every record.
	Subsystem string
	// RedactKeys overrides the default redaction list. If nil,
	// DefaultRedactKeys is used. Pass an empty (non-nil) slice to
	// disable redaction entirely (not recommended).
	RedactKeys []string
	// DisableRedaction skips wrapping in the redactor. Intended for
	// tests only.
	DisableRedaction bool
}

// New constructs an *slog.Logger configured per Options.
//
// The returned logger always wraps its underlying handler in the
// redactor (unless explicitly disabled), and pre-applies any
// Component / Subsystem attributes provided.
func New(opts Options) *slog.Logger {
	if opts.Writer == nil {
		opts.Writer = os.Stdout
	}
	if opts.Format == "" {
		opts.Format = FormatJSON
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     opts.Level,
		AddSource: opts.AddSource,
	}

	var base slog.Handler
	switch opts.Format {
	case FormatText:
		base = slog.NewTextHandler(opts.Writer, handlerOpts)
	default:
		base = slog.NewJSONHandler(opts.Writer, handlerOpts)
	}

	if !opts.DisableRedaction {
		base = NewRedactingHandler(base, opts.RedactKeys)
	}

	logger := slog.New(base)

	attrs := make([]any, 0, 4)
	if opts.Component != "" {
		attrs = append(attrs, slog.String(FieldComponent, opts.Component))
	}
	if opts.Subsystem != "" {
		attrs = append(attrs, slog.String(FieldSubsystem, opts.Subsystem))
	}
	if len(attrs) > 0 {
		logger = logger.With(attrs...)
	}
	return logger
}

// WithComponent returns a child logger with FieldComponent set.
func WithComponent(l *slog.Logger, component string) *slog.Logger {
	return l.With(slog.String(FieldComponent, component))
}

// WithSubsystem returns a child logger with FieldSubsystem set.
func WithSubsystem(l *slog.Logger, subsystem string) *slog.Logger {
	return l.With(slog.String(FieldSubsystem, subsystem))
}

// RequestFields carries the per-request identifiers attached to a logger.
// Fields with empty values are omitted from the resulting record so that
// JSON sinks do not see noisy `""` entries.
type RequestFields struct {
	RequestID string
	UserID    string
	TenantID  string
}

// WithRequest returns a child logger with the per-request identifiers set.
func WithRequest(l *slog.Logger, f RequestFields) *slog.Logger {
	attrs := make([]any, 0, 3)
	if f.RequestID != "" {
		attrs = append(attrs, slog.String(FieldRequestID, f.RequestID))
	}
	if f.UserID != "" {
		attrs = append(attrs, slog.String(FieldUserID, f.UserID))
	}
	if f.TenantID != "" {
		attrs = append(attrs, slog.String(FieldTenantID, f.TenantID))
	}
	if len(attrs) == 0 {
		return l
	}
	return l.With(attrs...)
}

// --- Context plumbing -----------------------------------------------------

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyLogger
)

// ContextWithRequestID returns a child context carrying the supplied
// request id. Empty ids are ignored.
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyRequestID, id)
}

// RequestIDFromContext extracts the request id previously stored via
// ContextWithRequestID. Returns "" if none.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// ContextWithLogger stores a logger on the context for later retrieval.
func ContextWithLogger(ctx context.Context, l *slog.Logger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if l == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyLogger, l)
}

// LoggerFromContext returns the logger stored on the context, or the
// fallback. If a request id is present on the context but the logger does
// not yet carry one, the request id is added.
func LoggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if ctx == nil {
		return fallback
	}
	l, _ := ctx.Value(ctxKeyLogger).(*slog.Logger)
	if l == nil {
		l = fallback
	}
	if l == nil {
		l = defaultLogger()
	}
	if rid := RequestIDFromContext(ctx); rid != "" {
		l = l.With(slog.String(FieldRequestID, rid))
	}
	return l
}

var (
	defaultOnce sync.Once
	defaultLog  *slog.Logger
)

func defaultLogger() *slog.Logger {
	defaultOnce.Do(func() {
		format := FormatJSON
		if isDevEnv() {
			format = FormatText
		}
		defaultLog = New(Options{Format: format, Level: slog.LevelInfo})
	})
	return defaultLog
}

func isDevEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("KAIVUE_ENV")))
	return v == "dev" || v == "development" || v == "local"
}
