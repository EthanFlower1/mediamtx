package onvif

import (
	"testing"
)

func TestMetadataConfigInfoFields(t *testing.T) {
	info := &MetadataConfigInfo{
		Token:          "meta-token-1",
		Name:           "MetadataConfig1",
		UseCount:       3,
		Analytics:      true,
		SessionTimeout: "1m0s",
	}

	if info.Token != "meta-token-1" {
		t.Fatalf("expected token %q, got %q", "meta-token-1", info.Token)
	}
	if info.Name != "MetadataConfig1" {
		t.Fatalf("expected name %q, got %q", "MetadataConfig1", info.Name)
	}
	if info.UseCount != 3 {
		t.Fatalf("expected use_count %d, got %d", 3, info.UseCount)
	}
	if !info.Analytics {
		t.Fatal("expected analytics to be true")
	}
	if info.SessionTimeout != "1m0s" {
		t.Fatalf("expected session_timeout %q, got %q", "1m0s", info.SessionTimeout)
	}
}
