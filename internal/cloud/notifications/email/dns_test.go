package email_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications/email"
)

const (
	// Fixture public key (not a real DKIM key — shape only).
	testPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDTestPublicKeyBodyForDKIMRecord
TxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxx
TxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxTxxQIDAQAB
-----END PUBLIC KEY-----`
)

func TestComputeDNSRecords_AllThreeProduced(t *testing.T) {
	cfg := email.DNSComputerConfig{
		SPFInclude:         "include:u12345.wl.sendgrid.net",
		DMARCReportMailbox: "dmarc@kaivue.io",
		DMARCPolicy:        "quarantine",
		TTL:                3600,
	}

	now := time.Now().UTC()
	dom := email.Domain{
		ID:                "dom-1",
		TenantID:          "tenant-a",
		Domain:            "alerts.acme.com",
		SendGridSubuser:   "sg-acme",
		ActiveSelector:    email.SelectorS1,
		VerificationState: email.VerificationPending,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	key := email.DKIMKey{
		ID:               "key-1",
		TenantID:         "tenant-a",
		DomainID:         dom.ID,
		Selector:         email.SelectorS1,
		PublicKeyPEM:     testPublicKeyPEM,
		CryptostoreKeyID: "cs-1",
		KeySizeBits:      2048,
		CreatedAt:        now,
	}

	recs, err := email.ComputeDNSRecords(cfg, dom, key)
	if err != nil {
		t.Fatalf("ComputeDNSRecords: %v", err)
	}

	// SPF at apex
	if recs.SPF.Name != "alerts.acme.com" {
		t.Errorf("SPF name = %q, want apex", recs.SPF.Name)
	}
	if !strings.HasPrefix(recs.SPF.Value, "v=spf1 ") {
		t.Errorf("SPF value should start with v=spf1, got %q", recs.SPF.Value)
	}
	if !strings.Contains(recs.SPF.Value, "include:u12345.wl.sendgrid.net") {
		t.Errorf("SPF value missing SendGrid include: %q", recs.SPF.Value)
	}
	if !strings.HasSuffix(recs.SPF.Value, "~all") {
		t.Errorf("SPF value should end with ~all, got %q", recs.SPF.Value)
	}

	// DKIM at s1._domainkey.domain
	if recs.DKIM.Name != "s1._domainkey.alerts.acme.com" {
		t.Errorf("DKIM name = %q, want s1._domainkey.alerts.acme.com", recs.DKIM.Name)
	}
	if !strings.HasPrefix(recs.DKIM.Value, "v=DKIM1; k=rsa; p=") {
		t.Errorf("DKIM value preamble wrong: %q", recs.DKIM.Value)
	}
	// p= body must not contain PEM headers or newlines.
	if strings.Contains(recs.DKIM.Value, "BEGIN") || strings.Contains(recs.DKIM.Value, "\n") {
		t.Errorf("DKIM value leaked PEM structure: %q", recs.DKIM.Value)
	}

	// DMARC at _dmarc.domain
	if recs.DMARC.Name != "_dmarc.alerts.acme.com" {
		t.Errorf("DMARC name = %q, want _dmarc.alerts.acme.com", recs.DMARC.Name)
	}
	if !strings.Contains(recs.DMARC.Value, "p=quarantine") {
		t.Errorf("DMARC value missing policy: %q", recs.DMARC.Value)
	}
	if !strings.Contains(recs.DMARC.Value, "rua=mailto:dmarc@kaivue.io") {
		t.Errorf("DMARC value missing rua: %q", recs.DMARC.Value)
	}
}

func TestComputeDNSRecords_SelectorMismatchRejected(t *testing.T) {
	cfg := email.DNSComputerConfig{
		SPFInclude:         "include:test.sendgrid.net",
		DMARCReportMailbox: "dmarc@kaivue.io",
	}
	dom := email.Domain{
		Domain:         "alerts.acme.com",
		ActiveSelector: email.SelectorS1,
	}
	key := email.DKIMKey{
		// Mismatched selector — should be rejected to prevent
		// publishing a stale DKIM record.
		Selector:     email.SelectorS2,
		PublicKeyPEM: testPublicKeyPEM,
	}
	if _, err := email.ComputeDNSRecords(cfg, dom, key); err == nil {
		t.Fatal("expected error on selector mismatch")
	}
}

func TestComputeDNSRecords_ConfigValidation(t *testing.T) {
	dom := email.Domain{Domain: "alerts.acme.com", ActiveSelector: email.SelectorS1}
	key := email.DKIMKey{Selector: email.SelectorS1, PublicKeyPEM: testPublicKeyPEM}

	bad := []email.DNSComputerConfig{
		{SPFInclude: "", DMARCReportMailbox: "dmarc@x"},
		{SPFInclude: "include:x", DMARCReportMailbox: ""},
		{SPFInclude: "include:x", DMARCReportMailbox: "dmarc@x", DMARCPolicy: "invalid"},
	}
	for i, cfg := range bad {
		if _, err := email.ComputeDNSRecords(cfg, dom, key); err == nil {
			t.Errorf("case %d: expected validation error, got nil", i)
		}
	}
}

func TestGenerateDKIMKeypair_Shape(t *testing.T) {
	if testing.Short() {
		t.Skip("skip RSA generation in -short mode")
	}
	kp, err := email.GenerateDKIMKeypair()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if kp.KeySizeBits != 2048 {
		t.Errorf("KeySizeBits = %d, want 2048", kp.KeySizeBits)
	}
	if !strings.Contains(kp.PrivateKeyPEM, "-----BEGIN RSA PRIVATE KEY-----") {
		t.Errorf("private key PEM malformed: %q", kp.PrivateKeyPEM)
	}
	if !strings.Contains(kp.PublicKeyPEM, "-----BEGIN PUBLIC KEY-----") {
		t.Errorf("public key PEM malformed: %q", kp.PublicKeyPEM)
	}
}
