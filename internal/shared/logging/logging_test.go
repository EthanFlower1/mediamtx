package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// decode parses one JSON log line into a map for assertions.
func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(b), &m); err != nil {
		t.Fatalf("failed to parse log line %q: %v", string(b), err)
	}
	return m
}

func TestNew_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{
		Format:    FormatJSON,
		Writer:    &buf,
		Component: "directory",
		Subsystem: "indexer",
		Level:     slog.LevelDebug,
	})
	l.Info("hello", "k", "v")

	m := decode(t, buf.Bytes())
	if m["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", m["msg"])
	}
	if m[FieldComponent] != "directory" {
		t.Errorf("component = %v", m[FieldComponent])
	}
	if m[FieldSubsystem] != "indexer" {
		t.Errorf("subsystem = %v", m[FieldSubsystem])
	}
	if m["k"] != "v" {
		t.Errorf("k = %v", m["k"])
	}
}

func TestNew_TextOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Format: FormatText, Writer: &buf, Component: "rec"})
	l.Info("evt", "x", 1)

	out := buf.String()
	if !strings.Contains(out, "msg=evt") {
		t.Errorf("text output missing msg: %q", out)
	}
	if !strings.Contains(out, FieldComponent+"=rec") {
		t.Errorf("text output missing component: %q", out)
	}
	// Should NOT look like JSON.
	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("text output looks like JSON: %q", out)
	}
}

func TestRedaction_DefaultKeys(t *testing.T) {
	for _, key := range DefaultRedactKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			for _, variant := range []string{key, strings.ToUpper(key), titleCase(key)} {
				var buf bytes.Buffer
				l := New(Options{Format: FormatJSON, Writer: &buf})
				l.Info("auth", variant, "REPLACE_ME")

				m := decode(t, buf.Bytes())
				if got := m[variant]; got != RedactedValue {
					t.Errorf("variant %q: value = %v, want %s", variant, got, RedactedValue)
				}
			}
		})
	}
}

func TestRedaction_NestedGroup(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Format: FormatJSON, Writer: &buf})

	l.Info("rtsp connect",
		slog.Group("camera",
			slog.String("id", "cam-1"),
			slog.Group("creds",
				slog.String("username", "admin"),
				slog.String("password", "REPLACE_ME"),
				slog.String("rtsp_credentials", "REPLACE_ME"),
			),
		),
	)

	m := decode(t, buf.Bytes())
	camera, ok := m["camera"].(map[string]any)
	if !ok {
		t.Fatalf("missing camera group: %v", m)
	}
	if camera["id"] != "cam-1" {
		t.Errorf("id should pass through: %v", camera["id"])
	}
	creds, ok := camera["creds"].(map[string]any)
	if !ok {
		t.Fatalf("missing creds subgroup: %v", camera)
	}
	if creds["username"] != "admin" {
		t.Errorf("username should pass through: %v", creds["username"])
	}
	if creds["password"] != RedactedValue {
		t.Errorf("password not redacted: %v", creds["password"])
	}
	if creds["rtsp_credentials"] != RedactedValue {
		t.Errorf("rtsp_credentials not redacted: %v", creds["rtsp_credentials"])
	}
}

func TestRedaction_WithAttrs(t *testing.T) {
	// Sensitive fields attached via With() should also be redacted
	// (handler-level coverage, not just per-record).
	var buf bytes.Buffer
	l := New(Options{Format: FormatJSON, Writer: &buf})
	child := l.With("api_key", "REPLACE_ME", "user", "alice")
	child.Info("call")

	m := decode(t, buf.Bytes())
	if m["api_key"] != RedactedValue {
		t.Errorf("api_key not redacted: %v", m["api_key"])
	}
	if m["user"] != "alice" {
		t.Errorf("user should pass through: %v", m["user"])
	}
}

func TestRedaction_CustomAllowList(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{
		Format:     FormatJSON,
		Writer:     &buf,
		RedactKeys: []string{"custom_field"},
	})
	l.Info("evt",
		"custom_field", "REPLACE_ME",
		"password", "still-visible-because-not-in-list",
	)

	m := decode(t, buf.Bytes())
	if m["custom_field"] != RedactedValue {
		t.Errorf("custom_field not redacted")
	}
	// Default list overridden — password is no longer redacted.
	if m["password"] == RedactedValue {
		t.Errorf("custom list should override default; password unexpectedly redacted")
	}
}

func TestWithRequest_AttachesFields(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Format: FormatJSON, Writer: &buf})
	l = WithRequest(l, RequestFields{
		RequestID: "req-123",
		UserID:    "user-7",
		TenantID:  "tenant-9",
	})
	l.Info("ping")

	m := decode(t, buf.Bytes())
	if m[FieldRequestID] != "req-123" {
		t.Errorf("request_id = %v", m[FieldRequestID])
	}
	if m[FieldUserID] != "user-7" {
		t.Errorf("user_id = %v", m[FieldUserID])
	}
	if m[FieldTenantID] != "tenant-9" {
		t.Errorf("tenant_id = %v", m[FieldTenantID])
	}
}

func TestWithComponentAndSubsystem(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Format: FormatJSON, Writer: &buf})
	l = WithComponent(l, "recorder")
	l = WithSubsystem(l, "segmenter")
	l.Info("rolled segment")

	m := decode(t, buf.Bytes())
	if m[FieldComponent] != "recorder" {
		t.Errorf("component = %v", m[FieldComponent])
	}
	if m[FieldSubsystem] != "segmenter" {
		t.Errorf("subsystem = %v", m[FieldSubsystem])
	}
}

func TestContextRequestID(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-abc")
	if got := RequestIDFromContext(ctx); got != "req-abc" {
		t.Errorf("got %q", got)
	}
	// Empty id is a no-op.
	ctx2 := ContextWithRequestID(context.Background(), "")
	if got := RequestIDFromContext(ctx2); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestLoggerFromContext_InjectsRequestID(t *testing.T) {
	var buf bytes.Buffer
	base := New(Options{Format: FormatJSON, Writer: &buf})

	ctx := ContextWithRequestID(context.Background(), "req-xyz")
	ctx = ContextWithLogger(ctx, base)

	l := LoggerFromContext(ctx, nil)
	l.Info("handled")

	m := decode(t, buf.Bytes())
	if m[FieldRequestID] != "req-xyz" {
		t.Errorf("request_id = %v", m[FieldRequestID])
	}
}

func TestLoggerFromContext_FallbackUsedWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	fallback := New(Options{Format: FormatJSON, Writer: &buf})
	l := LoggerFromContext(context.Background(), fallback)
	l.Info("noctx")
	if buf.Len() == 0 {
		t.Fatal("expected fallback logger to emit a record")
	}
	m := decode(t, buf.Bytes())
	if m["msg"] != "noctx" {
		t.Errorf("msg = %v", m["msg"])
	}
}

func TestRedaction_DisabledOption(t *testing.T) {
	var buf bytes.Buffer
	l := New(Options{Format: FormatJSON, Writer: &buf, DisableRedaction: true})
	l.Info("evt", "password", "REPLACE_ME")

	m := decode(t, buf.Bytes())
	if m["password"] != "REPLACE_ME" {
		t.Errorf("expected raw value when redaction disabled, got %v", m["password"])
	}
}

// titleCase capitalizes only the first rune; we use it to generate a
// case variation in TestRedaction_DefaultKeys without pulling in the
// deprecated strings.Title.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
