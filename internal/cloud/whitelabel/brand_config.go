// Package whitelabel holds per-integrator brand configuration, the asset
// storage interface, and the HTTP surface that lets the integrator portal
// (KAI-354, lead-web) round-trip brand config and upload brand assets.
//
// Scope note (KAI-353): this package is the *source of truth schema* plus an
// in-memory implementation sufficient for tests. The real S3/R2 backend
// (KAI-355) and encryption/KMS story are deferred. Handlers are exposed via
// Routes() but are intentionally not wired into the cloud router here.
package whitelabel

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// AssetKind enumerates the brand asset categories recognised by the
// white-label pipeline. New kinds require a matching entry in
// assetKindConstraints below.
type AssetKind string

const (
	AssetKindLogo   AssetKind = "logo"
	AssetKindSplash AssetKind = "splash"
	AssetKindIcon   AssetKind = "icon"
	AssetKindFont   AssetKind = "font"
)

// Valid returns true when the kind is one of the recognised asset kinds.
func (k AssetKind) Valid() bool {
	switch k {
	case AssetKindLogo, AssetKindSplash, AssetKindIcon, AssetKindFont:
		return true
	}
	return false
}

// ColorPalette is the minimum palette required for white-label surfaces.
// Additional ramp colors can be derived by the client at render time.
type ColorPalette struct {
	Primary    string `json:"primary"    validate:"required,hexcolor"`
	Secondary  string `json:"secondary"  validate:"required,hexcolor"`
	Accent     string `json:"accent"     validate:"required,hexcolor"`
	Background string `json:"background,omitempty" validate:"omitempty,hexcolor"`
	Foreground string `json:"foreground,omitempty" validate:"omitempty,hexcolor"`
}

// Typography carries font family identifiers. Actual font binaries live in
// the asset store as AssetKindFont entries and are referenced by Assets.Fonts.
type Typography struct {
	HeadingFamily string `json:"headingFamily" validate:"required,max=64"`
	BodyFamily    string `json:"bodyFamily"    validate:"required,max=64"`
	MonoFamily    string `json:"monoFamily,omitempty" validate:"omitempty,max=64"`
}

// BundleIDs holds the mobile store identifiers the build pipeline stamps
// into iOS and Android artifacts.
type BundleIDs struct {
	IOS     string `json:"ios"     validate:"required,bundleid"`
	Android string `json:"android" validate:"required,bundleid"`
}

// AssetRef is a pointer to a versioned object in the BrandAssetStore.
// StorageKey is the backend-specific key (e.g. S3 object key) and is opaque
// to callers; ContentType and Version are mirrored for convenience.
type AssetRef struct {
	Kind        AssetKind `json:"kind"`
	StorageKey  string    `json:"storageKey"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	Version     int       `json:"version"`
	UploadedAt  time.Time `json:"uploadedAt"`
}

// BrandAssets groups the refs for a single brand config version. Fonts is a
// slice because integrators commonly ship multiple weights/styles.
type BrandAssets struct {
	Logo   *AssetRef  `json:"logo,omitempty"`
	Splash *AssetRef  `json:"splash,omitempty"`
	Icon   *AssetRef  `json:"icon,omitempty"`
	Fonts  []AssetRef `json:"fonts,omitempty"`
}

// BrandConfig is the per-integrator source of truth for every white-label
// surface (mobile apps, marketing emails, portal). Version is a monotonic
// integer that advances on every PUT; older versions are retained so
// integrators can roll back.
type BrandConfig struct {
	TenantID     string       `json:"tenantId"     validate:"required,uuid"`
	Version      int          `json:"version"      validate:"gte=1"`
	AppName      string       `json:"appName"      validate:"required,min=1,max=64"`
	Colors       ColorPalette `json:"colors"`
	Typography   Typography   `json:"typography"`
	BundleIDs    BundleIDs    `json:"bundleIds"`
	SenderDomain string       `json:"senderDomain" validate:"required,fqdn"`
	ToSURL       string       `json:"tosUrl"       validate:"required,url"`
	PrivacyURL   string       `json:"privacyUrl"   validate:"required,url"`
	Assets       BrandAssets  `json:"assets"`
	UpdatedAt    time.Time    `json:"updatedAt"`
	UpdatedBy    string       `json:"updatedBy,omitempty"`
}

// Validation errors returned by BrandConfig.Validate. They are exported so
// callers (and tests) can do errors.Is comparisons.
var (
	ErrMissingTenantID    = errors.New("whitelabel: tenantId required")
	ErrInvalidVersion     = errors.New("whitelabel: version must be >= 1")
	ErrMissingAppName     = errors.New("whitelabel: appName required")
	ErrInvalidColor       = errors.New("whitelabel: color must be #RRGGBB or #RGB")
	ErrMissingTypography  = errors.New("whitelabel: typography families required")
	ErrInvalidBundleID    = errors.New("whitelabel: bundleId must be reverse-DNS (e.g. com.example.app)")
	ErrInvalidSenderFQDN  = errors.New("whitelabel: senderDomain must be a valid FQDN")
	ErrInvalidURL         = errors.New("whitelabel: ToSURL and PrivacyURL must be absolute https URLs")
	ErrUUID               = errors.New("whitelabel: tenantId must be a UUID")
)

var (
	// Matches #RGB or #RRGGBB.
	hexColorRE = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	// Reverse-DNS identifier. Two or more dot-separated labels, each label
	// starts with a letter, contains letters/digits/hyphens.
	bundleIDRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*(\.[A-Za-z][A-Za-z0-9-]*){1,}$`)
	// Permissive FQDN: two or more labels, total length <= 253.
	fqdnRE = regexp.MustCompile(`^(?:[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z]{2,}$`)
	// RFC 4122 UUID.
	uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
)

// Validate enforces the acceptance criteria of KAI-353. It returns the first
// error encountered; callers that want all errors at once should layer a
// multi-error on top.
func (c *BrandConfig) Validate() error {
	if strings.TrimSpace(c.TenantID) == "" {
		return ErrMissingTenantID
	}
	if !uuidRE.MatchString(c.TenantID) {
		return ErrUUID
	}
	if c.Version < 1 {
		return ErrInvalidVersion
	}
	if strings.TrimSpace(c.AppName) == "" || len(c.AppName) > 64 {
		return ErrMissingAppName
	}
	for _, col := range []string{c.Colors.Primary, c.Colors.Secondary, c.Colors.Accent} {
		if !hexColorRE.MatchString(col) {
			return fmt.Errorf("%w: %q", ErrInvalidColor, col)
		}
	}
	for _, col := range []string{c.Colors.Background, c.Colors.Foreground} {
		if col != "" && !hexColorRE.MatchString(col) {
			return fmt.Errorf("%w: %q", ErrInvalidColor, col)
		}
	}
	if strings.TrimSpace(c.Typography.HeadingFamily) == "" ||
		strings.TrimSpace(c.Typography.BodyFamily) == "" {
		return ErrMissingTypography
	}
	if !bundleIDRE.MatchString(c.BundleIDs.IOS) {
		return fmt.Errorf("%w: ios=%q", ErrInvalidBundleID, c.BundleIDs.IOS)
	}
	if !bundleIDRE.MatchString(c.BundleIDs.Android) {
		return fmt.Errorf("%w: android=%q", ErrInvalidBundleID, c.BundleIDs.Android)
	}
	if !fqdnRE.MatchString(c.SenderDomain) {
		return fmt.Errorf("%w: %q", ErrInvalidSenderFQDN, c.SenderDomain)
	}
	if err := validateHTTPSURL(c.ToSURL); err != nil {
		return fmt.Errorf("%w: tos: %v", ErrInvalidURL, err)
	}
	if err := validateHTTPSURL(c.PrivacyURL); err != nil {
		return fmt.Errorf("%w: privacy: %v", ErrInvalidURL, err)
	}
	return nil
}

func validateHTTPSURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("must be absolute https URL, got %q", raw)
	}
	return nil
}

// Clone returns a deep copy of the config so the caller can mutate the
// returned value without affecting the original (important for version
// snapshots kept by the store).
func (c *BrandConfig) Clone() *BrandConfig {
	if c == nil {
		return nil
	}
	out := *c
	if c.Assets.Fonts != nil {
		out.Assets.Fonts = append([]AssetRef(nil), c.Assets.Fonts...)
	}
	if c.Assets.Logo != nil {
		logo := *c.Assets.Logo
		out.Assets.Logo = &logo
	}
	if c.Assets.Splash != nil {
		splash := *c.Assets.Splash
		out.Assets.Splash = &splash
	}
	if c.Assets.Icon != nil {
		icon := *c.Assets.Icon
		out.Assets.Icon = &icon
	}
	return &out
}
