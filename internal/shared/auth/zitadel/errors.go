package zitadel

import (
	"context"
	"errors"
	"net/http"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// translateAuthError collapses a Zitadel SDK error into the sentinel that
// AuthenticateLocal / CompleteSSOFlow / RefreshSession / VerifyToken all
// return. The fail-closed contract forbids leaking which check failed, so
// every failure from a credential-bearing call maps to
// ErrInvalidCredentials / ErrTokenInvalid, regardless of cause.
//
// See the IdentityProvider contract in internal/shared/auth/provider.go.
func translateAuthError(err error, fallback error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if se := errAsSDK(err); se != nil {
		// Any 4xx from the auth-call endpoints collapses to the fallback
		// sentinel. 5xx leaks *nothing* about credentials either: the
		// caller can retry.
		if se.HTTPStatus >= 400 && se.HTTPStatus < 500 {
			return fallback
		}
		return fallback
	}
	return fallback
}

// translateUserError maps Zitadel errors on user-CRUD calls to the
// user-management sentinels. These calls ARE permitted to distinguish
// not-found / conflict / tenant-mismatch because the caller is already
// authenticated — the user-enumeration concern only applies to the
// credential-bearing endpoints.
func translateUserError(err error) error {
	if err == nil {
		return nil
	}
	if se := errAsSDK(err); se != nil {
		switch se.HTTPStatus {
		case http.StatusNotFound:
			return auth.ErrUserNotFound
		case http.StatusConflict:
			return auth.ErrUserExists
		case http.StatusForbidden:
			return auth.ErrTenantMismatch
		}
		if se.Code == "NOT_FOUND" {
			return auth.ErrUserNotFound
		}
		if se.Code == "ALREADY_EXISTS" {
			return auth.ErrUserExists
		}
	}
	return err
}

// translateProviderError maps errors on ConfigureProvider / TestProvider.
func translateProviderError(err error) error {
	if err == nil {
		return nil
	}
	if se := errAsSDK(err); se != nil {
		if se.HTTPStatus == http.StatusNotFound || se.Code == "NOT_FOUND" {
			return auth.ErrProviderNotFound
		}
	}
	return auth.ErrProviderTestFailed
}
