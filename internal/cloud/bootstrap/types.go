package bootstrap

import "errors"

// OIDCAppType distinguishes confidential server apps from public native/SPA apps.
type OIDCAppType int

const (
	// OIDCAppConfidential is a server-side app with a client secret.
	OIDCAppConfidential OIDCAppType = iota
	// OIDCAppNative is a native app using PKCE (no client secret).
	OIDCAppNative
	// OIDCAppSPA is a single-page app using PKCE (no client secret).
	OIDCAppSPA
)

// OIDCApp describes an OIDC application to register in Zitadel.
type OIDCApp struct {
	// Name is the human-readable application name in Zitadel.
	Name string
	// Type determines the OIDC flow (confidential vs PKCE).
	Type OIDCAppType
	// RedirectURIs are the allowed OAuth2 redirect URIs.
	RedirectURIs []string
	// PostLogoutRedirectURIs are allowed post-logout redirect URIs.
	PostLogoutRedirectURIs []string
}

// Result holds the outputs of a successful bootstrap run.
type Result struct {
	// PlatformOrgID is the Zitadel org ID for the platform.
	PlatformOrgID string
	// ServiceAccountID is the Zitadel user ID of the platform service account.
	ServiceAccountID string
	// ServiceAccountKeyJSON is the JWT profile key in JSON format. The caller
	// must persist this securely (e.g. Secrets Manager, file on disk).
	ServiceAccountKeyJSON []byte
	// Apps maps application name → OIDC client ID.
	Apps map[string]string
}

// Config configures the bootstrap procedure.
type Config struct {
	// PlatformOrgName is the display name for the platform org.
	PlatformOrgName string
	// ServiceAccountName is the username for the platform service account.
	ServiceAccountName string
	// Apps is the list of OIDC applications to register.
	Apps []OIDCApp
}

// DefaultConfig returns the standard bootstrap configuration for Kaivue.
func DefaultConfig() Config {
	return Config{
		PlatformOrgName:    "Kaivue Platform",
		ServiceAccountName: "kaivue-platform-sa",
		Apps: []OIDCApp{
			{
				Name:         "kaivue-directory",
				Type:         OIDCAppConfidential,
				RedirectURIs: []string{"https://localhost/callback"},
			},
			{
				Name:         "kaivue-recorder",
				Type:         OIDCAppConfidential,
				RedirectURIs: []string{"https://localhost/callback"},
			},
			{
				Name:         "kaivue-gateway",
				Type:         OIDCAppConfidential,
				RedirectURIs: []string{"https://localhost/callback"},
			},
			{
				Name:         "kaivue-flutter",
				Type:         OIDCAppNative,
				RedirectURIs: []string{
					"com.kaivue.app://callback",
					"com.kaivue.app.dev://callback",
				},
				PostLogoutRedirectURIs: []string{
					"com.kaivue.app://logout",
					"com.kaivue.app.dev://logout",
				},
			},
			{
				Name:         "kaivue-web",
				Type:         OIDCAppSPA,
				RedirectURIs: []string{"https://localhost:5173/callback"},
				PostLogoutRedirectURIs: []string{"https://localhost:5173/"},
			},
		},
	}
}

// Validate checks that the config has all required fields.
func (c Config) Validate() error {
	if c.PlatformOrgName == "" {
		return errors.New("bootstrap: PlatformOrgName is required")
	}
	if c.ServiceAccountName == "" {
		return errors.New("bootstrap: ServiceAccountName is required")
	}
	for i, app := range c.Apps {
		if app.Name == "" {
			return errors.New("bootstrap: Apps[" + itoa(i) + "].Name is required")
		}
		if len(app.RedirectURIs) == 0 {
			return errors.New("bootstrap: Apps[" + app.Name + "].RedirectURIs must not be empty")
		}
	}
	return nil
}

func itoa(i int) string {
	return string(rune('0' + i))
}
