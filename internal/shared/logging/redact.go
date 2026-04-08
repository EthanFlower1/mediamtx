package logging

import (
	"context"
	"log/slog"
	"strings"
)

// RedactingHandler wraps another slog.Handler and replaces the values of
// any attributes whose keys match a case-insensitive allow-list with
// RedactedValue. Redaction recurses into nested slog.Group attributes.
type RedactingHandler struct {
	inner   slog.Handler
	keys    map[string]struct{}
	groups  []string // for With/WithGroup pass-through bookkeeping (unused for redaction)
}

// NewRedactingHandler returns a handler that redacts sensitive attribute
// values before delegating to inner. If keys is nil, DefaultRedactKeys
// is used. If keys is non-nil but empty, no redaction is applied.
func NewRedactingHandler(inner slog.Handler, keys []string) *RedactingHandler {
	if keys == nil {
		keys = DefaultRedactKeys
	}
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[strings.ToLower(k)] = struct{}{}
	}
	return &RedactingHandler{inner: inner, keys: set}
}

// Enabled implements slog.Handler.
func (h *RedactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

// Handle implements slog.Handler. It walks the record's attributes,
// rewriting any sensitive ones, then forwards a new Record to the inner
// handler.
func (h *RedactingHandler) Handle(ctx context.Context, r slog.Record) error {
	if len(h.keys) == 0 {
		return h.inner.Handle(ctx, r)
	}
	clone := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clone.AddAttrs(h.redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clone)
}

// WithAttrs implements slog.Handler. Attributes attached via With are
// also subject to redaction.
func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(h.keys) == 0 {
		return &RedactingHandler{inner: h.inner.WithAttrs(attrs), keys: h.keys, groups: h.groups}
	}
	redacted := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		redacted[i] = h.redactAttr(a)
	}
	return &RedactingHandler{
		inner:  h.inner.WithAttrs(redacted),
		keys:   h.keys,
		groups: h.groups,
	}
}

// WithGroup implements slog.Handler.
func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{
		inner:  h.inner.WithGroup(name),
		keys:   h.keys,
		groups: append(append([]string{}, h.groups...), name),
	}
}

// redactAttr returns a copy of a with sensitive values replaced. Group
// values are descended into recursively. LogValuer values are resolved
// before checking, so deferred sensitive values are still caught.
func (h *RedactingHandler) redactAttr(a slog.Attr) slog.Attr {
	if h.isSensitive(a.Key) {
		return slog.String(a.Key, RedactedValue)
	}
	v := a.Value
	// Resolve LogValuer so we can inspect & redact the materialized value.
	if v.Kind() == slog.KindLogValuer {
		v = v.Resolve()
	}
	if v.Kind() == slog.KindGroup {
		inner := v.Group()
		out := make([]slog.Attr, len(inner))
		for i, g := range inner {
			out[i] = h.redactAttr(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	return slog.Attr{Key: a.Key, Value: v}
}

func (h *RedactingHandler) isSensitive(key string) bool {
	_, ok := h.keys[strings.ToLower(key)]
	return ok
}
