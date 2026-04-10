package email

import (
	"fmt"
	"strings"
)

// DNSComputerConfig tunes ComputeDNSRecords output. All fields are
// set once at control-plane startup and come from Kaivue's SendGrid
// parent account configuration (KAI-357 infra module).
type DNSComputerConfig struct {
	// SPFInclude is the macro integrators include in their SPF
	// record, e.g. "include:u12345.wl.sendgrid.net". Comes from the
	// SendGrid parent account config.
	SPFInclude string

	// DMARCReportMailbox is the rua= address for aggregate reports,
	// e.g. "dmarc-reports@kaivue.io".
	DMARCReportMailbox string

	// DMARCPolicy is the p= value ("none" | "quarantine" | "reject").
	// Defaults to "quarantine" per Kaivue security baseline.
	DMARCPolicy string

	// TTL is the TTL the integrator should set on the records.
	// Defaults to 3600s if zero.
	TTL int
}

// Validate returns an error if the config is unusable.
func (c DNSComputerConfig) Validate() error {
	if strings.TrimSpace(c.SPFInclude) == "" {
		return fmt.Errorf("email: DNSComputerConfig.SPFInclude required")
	}
	if strings.TrimSpace(c.DMARCReportMailbox) == "" {
		return fmt.Errorf("email: DNSComputerConfig.DMARCReportMailbox required")
	}
	switch c.DMARCPolicy {
	case "", "none", "quarantine", "reject":
	default:
		return fmt.Errorf("email: DNSComputerConfig.DMARCPolicy %q invalid", c.DMARCPolicy)
	}
	return nil
}

// ComputeDNSRecords returns the SPF/DKIM/DMARC records an integrator
// must add to their DNS zone for the given domain + active DKIM key.
//
// The caller is responsible for passing the DKIM key that matches
// domain.ActiveSelector. Mismatched inputs return an error rather
// than silently producing a wrong DKIM record.
func ComputeDNSRecords(cfg DNSComputerConfig, domain Domain, activeKey DKIMKey) (DNSRecords, error) {
	if err := cfg.Validate(); err != nil {
		return DNSRecords{}, err
	}
	if strings.TrimSpace(domain.Domain) == "" {
		return DNSRecords{}, ErrMissingDomain
	}
	if activeKey.Selector != domain.ActiveSelector {
		return DNSRecords{}, fmt.Errorf(
			"email: active key selector %q does not match domain.ActiveSelector %q",
			activeKey.Selector, domain.ActiveSelector,
		)
	}
	if strings.TrimSpace(activeKey.PublicKeyPEM) == "" {
		return DNSRecords{}, fmt.Errorf("email: DKIM key %s has no public key material", activeKey.ID)
	}

	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 3600
	}
	policy := cfg.DMARCPolicy
	if policy == "" {
		policy = "quarantine"
	}

	// SPF: a single TXT record at the apex. We include the SendGrid
	// parent macro and end with ~all (soft-fail) to match Kaivue's
	// baseline. If an integrator needs a stricter policy they can
	// edit the record post-verification.
	spfValue := fmt.Sprintf("v=spf1 %s ~all", strings.TrimSpace(cfg.SPFInclude))

	// DKIM TXT is published at {selector}._domainkey.{domain}. The
	// key material goes in the p= tag; we strip PEM headers and
	// concatenate the base64 body.
	dkimB64, err := pemBodyBase64(activeKey.PublicKeyPEM)
	if err != nil {
		return DNSRecords{}, err
	}
	dkimValue := fmt.Sprintf("v=DKIM1; k=rsa; p=%s", dkimB64)
	dkimName := fmt.Sprintf("%s._domainkey.%s", activeKey.Selector, domain.Domain)

	// DMARC: conservative quarantine policy + rua aggregate reports.
	dmarcValue := fmt.Sprintf("v=DMARC1; p=%s; rua=mailto:%s; fo=1",
		policy, strings.TrimSpace(cfg.DMARCReportMailbox))
	dmarcName := "_dmarc." + domain.Domain

	return DNSRecords{
		SPF: TXTRecord{
			Name:  domain.Domain,
			Type:  "TXT",
			Value: spfValue,
			TTL:   ttl,
		},
		DKIM: TXTRecord{
			Name:  dkimName,
			Type:  "TXT",
			Value: dkimValue,
			TTL:   ttl,
		},
		DMARC: TXTRecord{
			Name:  dmarcName,
			Type:  "TXT",
			Value: dmarcValue,
			TTL:   ttl,
		},
	}, nil
}

// pemBodyBase64 strips -----BEGIN/END----- lines and returns the base64
// body concatenated onto a single line, which is the form DKIM DNS
// records expect in the p= tag.
func pemBodyBase64(pemText string) (string, error) {
	lines := strings.Split(pemText, "\n")
	var body strings.Builder
	inside := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-----BEGIN") {
			inside = true
			continue
		}
		if strings.HasPrefix(line, "-----END") {
			inside = false
			continue
		}
		if inside {
			body.WriteString(line)
		}
	}
	if body.Len() == 0 {
		return "", fmt.Errorf("email: PEM contained no body")
	}
	return body.String(), nil
}
