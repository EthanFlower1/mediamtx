package federation

import (
	"testing"
)

func TestWrapUnwrapToken(t *testing.T) {
	raw := "eyJwYXlsb2FkIjoiZGF0YSJ9.c2lnbmF0dXJl"

	wrapped := WrapToken(raw)
	if wrapped != "FED-v1.eyJwYXlsb2FkIjoiZGF0YSJ9.c2lnbmF0dXJl" {
		t.Fatalf("unexpected wrapped token: %s", wrapped)
	}

	version, unwrapped, err := UnwrapToken(wrapped)
	if err != nil {
		t.Fatalf("UnwrapToken: %v", err)
	}
	if version != "v1" {
		t.Fatalf("expected version v1, got %q", version)
	}
	if unwrapped != raw {
		t.Fatalf("round-trip mismatch: got %q, want %q", unwrapped, raw)
	}
}

func TestUnwrapToken_MissingPrefix(t *testing.T) {
	_, _, err := UnwrapToken("v1.payload.sig")
	if err == nil {
		t.Fatal("expected error for missing FED- prefix")
	}
}

func TestUnwrapToken_UnsupportedVersion(t *testing.T) {
	_, _, err := UnwrapToken("FED-v99.payload.sig")
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestUnwrapToken_MissingVersionSeparator(t *testing.T) {
	_, _, err := UnwrapToken("FED-v1")
	if err == nil {
		t.Fatal("expected error for missing version separator")
	}
}

func TestUnwrapToken_EmptyPayload(t *testing.T) {
	_, _, err := UnwrapToken("FED-v1.")
	if err == nil {
		t.Fatal("expected error for empty payload")
	}
}
