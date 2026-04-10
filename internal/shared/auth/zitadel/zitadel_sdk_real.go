//go:build zitadel_sdk

// This file is the KAI-220 handoff slot: replace the body of newSDKClient
// below with the real github.com/zitadel/zitadel-go/v3 client construction
// once the dependency is vendored. The adapter in adapter.go already calls
// into the sdkClient methods (doJSON etc.) and so needs no changes — keep
// this file's public surface identical to zitadel_sdk_stub.go.
//
// Intentionally kept as a compile-time shell until the real SDK lands:
// building with `-tags zitadel_sdk` today will fail with a clear error,
// which is preferable to a silent partial implementation.

package zitadel

import (
	"context"
	"errors"
	"net/http"
)

type sdkClient struct {
	domain string
	http   *http.Client
}

func newSDKClient(domain string, httpClient *http.Client) *sdkClient {
	return &sdkClient{domain: domain, http: httpClient}
}

var errRealSDKNotWired = errors.New(
	"zitadel: real SDK not wired — build without -tags zitadel_sdk, or " +
		"wire github.com/zitadel/zitadel-go/v3 here (KAI-220 handoff)")

func (c *sdkClient) doJSON(ctx context.Context, method, path, orgID string, in, out any) error {
	_ = ctx
	_ = method
	_ = path
	_ = orgID
	_ = in
	_ = out
	return errRealSDKNotWired
}

type sdkError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *sdkError) Error() string { return "zitadel: " + e.Code + ": " + e.Message }

func errAsSDK(err error) *sdkError {
	var se *sdkError
	if errors.As(err, &se) {
		return se
	}
	return nil
}

// Type shells so adapter.go compiles identically under both tags.
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

func escape(s string) string { return s }
