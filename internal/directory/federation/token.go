package federation

import (
	"fmt"
	"strings"
)

// TokenVersion is the current version of the FED- token format.
const TokenVersion = "v1"

// TokenPrefix is the UX-recognizable prefix prepended to every federation
// pairing token. Combined with the version it forms "FED-v1.".
const TokenPrefix = "FED-"

// WrapToken prepends the FED-v1 prefix to a raw signed enrollment token
// produced by FederationCA.MintPeerEnrollmentToken.
//
// Output format: FED-v1.<payload>.<sig>
func WrapToken(rawEnrollmentToken string) string {
	return TokenPrefix + TokenVersion + "." + rawEnrollmentToken
}

// UnwrapToken strips the FED-v1 prefix and returns the raw enrollment token.
// Returns an error if the prefix or version is missing/wrong.
func UnwrapToken(fedToken string) (version string, rawToken string, err error) {
	if !strings.HasPrefix(fedToken, TokenPrefix) {
		return "", "", fmt.Errorf("federation: token missing FED- prefix")
	}
	rest := fedToken[len(TokenPrefix):]

	// Split on first "." to get version.
	dotIdx := strings.Index(rest, ".")
	if dotIdx < 0 {
		return "", "", fmt.Errorf("federation: token missing version separator")
	}
	version = rest[:dotIdx]
	rawToken = rest[dotIdx+1:]

	if version != TokenVersion {
		return version, "", fmt.Errorf("federation: unsupported token version %q (expected %q)", version, TokenVersion)
	}
	if rawToken == "" {
		return version, "", fmt.Errorf("federation: empty token payload")
	}
	return version, rawToken, nil
}
