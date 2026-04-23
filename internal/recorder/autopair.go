// Package recorder — autopair implements the mDNS-based approval pairing
// helpers used when the Recorder boots without an explicit MTX_PAIRING_TOKEN.
//
// Flow:
//  1. requestPairing sends POST /api/v1/pairing/request to the Directory.
//  2. pollForToken polls GET /api/v1/pairing/request/{id}/token every 5 s
//     until the admin approves, denies, or the request expires.
package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// pairingRequestBody is the JSON body sent to the Directory's pairing
// request endpoint.
type pairingRequestBody struct {
	RecorderHostname string   `json:"recorder_hostname"`
	RequestedRoles   []string `json:"requested_roles,omitempty"`
	Note             string   `json:"note,omitempty"`
}

// pairingRequestResponse is the JSON response from POST /api/v1/pairing/request.
type pairingRequestResponse struct {
	ID        string `json:"id"`
	ExpiresIn string `json:"expires_in"`
	Message   string `json:"message"`
}

// pollTokenResponse is the JSON response from GET /api/v1/pairing/request/{id}/token
// when the request has been approved (200 OK).
type pollTokenResponse struct {
	Token     string `json:"token"`
	TokenID   string `json:"token_id"`
	ExpiresIn string `json:"expires_in"`
}

// pollPendingResponse is returned with 202 Accepted while awaiting approval.
type pollPendingResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// pollErrorResponse is returned with 403/410 on denial or expiry.
type pollErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// requestPairing sends a pairing request to the Directory and returns the
// pending request ID. The admin must approve this request before a token is
// issued.
func requestPairing(ctx context.Context, directoryURL, hostname string) (string, error) {
	body := pairingRequestBody{
		RecorderHostname: hostname,
		RequestedRoles:   []string{"recorder"},
		Note:             "Auto-discovered via mDNS",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal pairing request: %w", err)
	}

	url := directoryURL + "/api/v1/pairing/request"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, string(respBody))
	}

	var result pairingRequestResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("empty request ID in response from %s", url)
	}

	return result.ID, nil
}

const pollInterval = 5 * time.Second

// pollForToken polls the Directory's token endpoint until the admin approves
// (200), denies (403), or the request expires (410). It also respects context
// cancellation.
func pollForToken(ctx context.Context, directoryURL, requestID string, log *slog.Logger) (string, error) {
	url := fmt.Sprintf("%s/api/v1/pairing/request/%s/token", directoryURL, requestID)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("context cancelled while waiting for approval: %w", ctx.Err())
		case <-ticker.C:
			token, done, err := pollOnce(ctx, url)
			if err != nil {
				return "", err
			}
			if done {
				return token, nil
			}
			log.Debug("recorder: still waiting for admin approval", "request_id", requestID)
		}
	}
}

// pollOnce makes a single GET request to the token endpoint.
// Returns (token, true, nil) on approval, ("", false, nil) when still pending,
// or ("", false, err) on terminal failure.
func pollOnce(ctx context.Context, url string) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", false, fmt.Errorf("create poll request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("poll %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false, fmt.Errorf("read poll response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var result pollTokenResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return "", false, fmt.Errorf("decode approved token response: %w", err)
		}
		if result.Token == "" {
			return "", false, fmt.Errorf("empty token in approved response")
		}
		return result.Token, true, nil

	case http.StatusAccepted:
		// Still pending — keep polling.
		return "", false, nil

	case http.StatusForbidden:
		var errResp pollErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return "", false, fmt.Errorf("pairing denied by admin: %s", errResp.Message)

	case http.StatusGone:
		var errResp pollErrorResponse
		_ = json.Unmarshal(body, &errResp)
		return "", false, fmt.Errorf("pairing request expired: %s", errResp.Message)

	default:
		return "", false, fmt.Errorf("unexpected status %d from %s: %s", resp.StatusCode, url, string(body))
	}
}
