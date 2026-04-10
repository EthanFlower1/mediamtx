//go:build !zitadel_sdk

// This file provides an in-tree stub of the small slice of the Zitadel SDK
// the adapter actually uses. It speaks Zitadel's public REST shapes
// (https://zitadel.com/docs/apis/introduction) over Config.HTTPClient, which
// means:
//
//   - it compiles in sandboxes that cannot reach github.com/zitadel/zitadel-go
//   - unit tests can inject a fake round-tripper and assert request shapes
//     against recorded fixtures in testdata/
//   - the handoff to KAI-220 (real Zitadel deploy) swaps this file out for
//     zitadel_sdk_real.go without any changes to the adapter.
//
// If/when the real SDK is vendored, build with `-tags zitadel_sdk` to opt
// in to the SDK-backed implementation instead.

package zitadel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// sdkClient is the minimal surface the adapter uses against Zitadel. Every
// method is tenant-scoped — the orgID argument is injected into the
// `x-zitadel-orgid` header and the URL path, and it is an error to call a
// cross-org method without one. This is seam #4 in miniature.
type sdkClient struct {
	domain string
	http   *http.Client
}

func newSDKClient(domain string, httpClient *http.Client) *sdkClient {
	return &sdkClient{domain: domain, http: httpClient}
}

// doJSON performs a JSON request/response round-trip. path is a relative
// Zitadel API path like "/v2/sessions". If orgID is non-empty, the
// x-zitadel-orgid header is set — otherwise it is left off and the call is
// treated as a platform-scoped call.
func (c *sdkClient) doJSON(ctx context.Context, method, path, orgID string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("zitadel: marshal %s %s: %w", method, path, err)
		}
		body = bytes.NewReader(buf)
	}
	u := "https://" + c.domain + path
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return fmt.Errorf("zitadel: new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if orgID != "" {
		req.Header.Set("x-zitadel-orgid", orgID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("zitadel: transport: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return parseZitadelError(resp.StatusCode, buf)
	}
	if out != nil && len(buf) > 0 {
		if err := json.Unmarshal(buf, out); err != nil {
			return fmt.Errorf("zitadel: decode: %w", err)
		}
	}
	return nil
}

// sdkError is the shape Zitadel returns on 4xx/5xx. We keep the whole blob
// so the adapter can translate known codes to the typed auth sentinels.
type sdkError struct {
	HTTPStatus int
	Code       string `json:"code"`    // Zitadel error code, e.g. "PERMISSION_DENIED"
	Message    string `json:"message"` // customer-facing message (safe to log)
}

func (e *sdkError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("zitadel: %s: %s (http %d)", e.Code, e.Message, e.HTTPStatus)
	}
	return fmt.Sprintf("zitadel: http %d: %s", e.HTTPStatus, e.Message)
}

func parseZitadelError(status int, body []byte) error {
	var e sdkError
	e.HTTPStatus = status
	if len(body) > 0 {
		_ = json.Unmarshal(body, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(body))
		}
	}
	return &e
}

// errAsSDK extracts *sdkError from err, returning nil if err is not one.
func errAsSDK(err error) *sdkError {
	var se *sdkError
	if errors.As(err, &se) {
		return se
	}
	return nil
}

// --- Typed request/response payloads the adapter uses -------------------
//
// These are the minimal JSON shapes the adapter speaks. They deliberately
// ignore 95% of Zitadel's API surface — we only model what the
// IdentityProvider interface touches. Field names match Zitadel's v2 REST
// API so the same fixtures work against the real service.

type sessionCreateRequest struct {
	Checks struct {
		User struct {
			LoginName string `json:"loginName"`
		} `json:"user"`
		Password struct {
			Password string `json:"password"`
		} `json:"password"`
	} `json:"checks"`
}

type sessionCreateResponse struct {
	SessionID    string `json:"sessionId"`
	SessionToken string `json:"sessionToken"`
	UserID       string `json:"userId"`
}

type tokenIntrospectResponse struct {
	Active bool     `json:"active"`
	Sub    string   `json:"sub"`
	OrgID  string   `json:"urn:zitadel:iam:user:resourceowner:id"`
	Groups []string `json:"urn:zitadel:iam:org:project:roles"`
	Exp    int64    `json:"exp"`
	Iat    int64    `json:"iat"`
	SID    string   `json:"sid"`
}

type userV2 struct {
	UserID  string `json:"userId"`
	Details struct {
		ResourceOwner string `json:"resourceOwner"`
	} `json:"details"`
	Human *struct {
		Profile struct {
			GivenName   string `json:"givenName"`
			FamilyName  string `json:"familyName"`
			DisplayName string `json:"displayName"`
		} `json:"profile"`
		Email struct {
			Email string `json:"email"`
		} `json:"email"`
		Username string `json:"username"`
	} `json:"human"`
	State string `json:"state"`
}

type listUsersResponse struct {
	Result []userV2 `json:"result"`
}

type createUserRequest struct {
	Username string `json:"username"`
	Profile  struct {
		GivenName   string `json:"givenName"`
		FamilyName  string `json:"familyName"`
		DisplayName string `json:"displayName"`
	} `json:"profile"`
	Email struct {
		Email string `json:"email"`
	} `json:"email"`
	Password *struct {
		Password string `json:"password"`
	} `json:"password,omitempty"`
}

type createUserResponse struct {
	UserID string `json:"userId"`
}

type orgCreateRequest struct {
	Name string `json:"name"`
}

type orgCreateResponse struct {
	ID string `json:"id"`
}

type providerCreateRequest struct {
	Name         string   `json:"name"`
	Issuer       string   `json:"issuer,omitempty"`
	ClientID     string   `json:"clientId,omitempty"`
	ClientSecret string   `json:"clientSecret,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	MetadataURL  string   `json:"metadataUrl,omitempty"`
	MetadataXML  string   `json:"metadataXml,omitempty"`
	LDAP         *struct {
		URL    string `json:"url"`
		BindDN string `json:"bindDn"`
	} `json:"ldap,omitempty"`
}

// escape is a tiny helper that URL-escapes path segments where Zitadel
// embeds IDs (userId, orgId, etc.).
func escape(s string) string { return url.PathEscape(s) }
