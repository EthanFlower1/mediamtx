package r2

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Tier represents the cloud archive storage tier.
type Tier string

const (
	// TierHot is Cloudflare R2 standard — first ~30 days.
	TierHot Tier = "hot"
	// TierWarm is Cloudflare R2 infrequent-access — 30-90 days.
	TierWarm Tier = "warm"
	// TierCold is Cloudflare R2 cold tier — 90 days - 1 year.
	TierCold Tier = "cold"
)

// EncryptionMode selects the encryption strategy for objects uploaded to R2.
type EncryptionMode string

const (
	// EncryptionModeStandard uses Cloudflare R2 server-side encryption (default).
	// Cloudflare holds the keys. Simplest, suitable for most tenants.
	EncryptionModeStandard EncryptionMode = "standard"

	// EncryptionModeSSEKMS uses a customer-owned AWS KMS key. The platform
	// requests encryption via SSE-KMS headers. Customer can audit and revoke
	// KMS access. HIPAA / SOC2 use case.
	//
	// TODO(KAI-266): SSE-KMS header wiring is stubbed. Full implementation
	// requires the KMS key ARN at Client construction time.
	EncryptionModeSSEKMS EncryptionMode = "sse-kms"

	// EncryptionModeCSECMK performs client-side encryption using the
	// cryptostore before the bytes reach the network. The R2 object is pure
	// ciphertext. Even Kaivue staff cannot decrypt without the customer's master
	// key. Fail-closed: any cryptostore error aborts the upload.
	EncryptionModeCSECMK EncryptionMode = "cse-cmk"
)

// Config holds the R2 client configuration. Secrets are loaded from environment
// variables so they are never embedded in config files on disk.
//
// Config file path: configs/cloud/archive/r2.yaml (see that file for field docs).
// See also configs/cloud/archive/README.md for required bucket provisioning.
type Config struct {
	// AccountID is the Cloudflare account ID (not secret).
	// config key: archive.r2.account_id
	AccountID string

	// AccessKeyID is the R2 API token key ID.
	// Loaded from env: R2_ACCESS_KEY_ID (never hardcoded).
	AccessKeyID string

	// SecretAccessKey is the R2 API token secret.
	// Loaded from env: R2_SECRET_ACCESS_KEY. At rest this value is KMS-wrapped
	// (KAI-231 IaC writes an encrypted SSM parameter).
	SecretAccessKey string

	// Env distinguishes deployment environments: "prod", "staging", "dev".
	// Drives bucket naming: kaivue-{env}-{tier}.
	Env string

	// Region is the AWS region that signed URLs are issued for.
	// R2 is globally distributed but the S3-compat endpoint requires a region
	// string; "auto" is valid and recommended for R2.
	Region string

	// EncryptionMode selects how objects are encrypted at rest.
	// Default: EncryptionModeStandard.
	EncryptionMode EncryptionMode

	// KMSKeyARN is required when EncryptionMode == EncryptionModeSSEKMS.
	// TODO(KAI-266): stub — not implemented in this ticket.
	KMSKeyARN string
}

// Validate returns an error if any required field is missing or invalid.
func (c *Config) Validate() error {
	var errs []string
	if c.AccountID == "" {
		errs = append(errs, "account_id is required")
	}
	if c.AccessKeyID == "" {
		errs = append(errs, "access_key_id is required (set R2_ACCESS_KEY_ID env)")
	}
	if c.SecretAccessKey == "" {
		errs = append(errs, "secret_access_key is required (set R2_SECRET_ACCESS_KEY env)")
	}
	if c.Env == "" {
		errs = append(errs, "env is required (prod|staging|dev)")
	}
	switch c.EncryptionMode {
	case "", EncryptionModeStandard, EncryptionModeSSEKMS, EncryptionModeCSECMK:
		// valid
	default:
		errs = append(errs, fmt.Sprintf("unknown encryption_mode %q", c.EncryptionMode))
	}
	if c.EncryptionMode == EncryptionModeSSEKMS && c.KMSKeyARN == "" {
		errs = append(errs, "kms_key_arn is required for sse-kms mode")
	}
	if len(errs) > 0 {
		return errors.New("r2 config: " + strings.Join(errs, "; "))
	}
	return nil
}

// ConfigFromEnv constructs a Config by reading the standard R2 environment
// variables. Callers may override individual fields after calling this.
func ConfigFromEnv(accountID, env string) Config {
	region := os.Getenv("R2_REGION")
	if region == "" {
		region = "auto"
	}
	return Config{
		AccountID:      accountID,
		AccessKeyID:    os.Getenv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("R2_SECRET_ACCESS_KEY"),
		Env:            env,
		Region:         region,
		EncryptionMode: EncryptionModeStandard,
	}
}

// BucketName returns the canonical bucket name for the given tier and config.
// Format: kaivue-{env}-{tier}
// Example: kaivue-prod-hot, kaivue-staging-warm
func BucketName(cfg Config, tier Tier) string {
	return fmt.Sprintf("kaivue-%s-%s", cfg.Env, tier)
}
