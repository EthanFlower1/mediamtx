package openpath

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPClient is the interface for making HTTP requests, allowing injection
// of test doubles.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client wraps the Alta REST API with OAuth2 token management and
// exponential-backoff retry.
type Client struct {
	cfg    Config
	http   HTTPClient
	log    *slog.Logger

	mu    sync.Mutex
	token Token
}

// NewClient constructs a Client for the given Config.
// The caller may pass a custom HTTPClient (e.g. for tests); nil defaults
// to http.DefaultClient.
func NewClient(cfg Config, httpClient HTTPClient, logger *slog.Logger) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:  cfg,
		http: httpClient,
		log:  logger.With(slog.String("component", "openpath.client")),
	}
}

// -----------------------------------------------------------------------
// OAuth2 token management
// -----------------------------------------------------------------------

// Authenticate obtains (or refreshes) an OAuth2 access token using the
// client_credentials grant. The token is cached and re-used until it
// approaches expiry (30-second buffer).
func (c *Client) Authenticate(ctx context.Context) (Token, error) {
	c.mu.Lock()
	if c.token.Valid() {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	tok, err := c.fetchToken(ctx)
	if err != nil {
		return Token{}, fmt.Errorf("openpath: authenticate: %w", err)
	}

	c.mu.Lock()
	c.token = tok
	c.mu.Unlock()

	c.log.InfoContext(ctx, "authenticated with Alta API",
		"org_id", c.cfg.OrgID,
		"expires_at", tok.ExpiresAt.Format(time.RFC3339),
	)
	return tok, nil
}

func (c *Client) fetchToken(ctx context.Context) (Token, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.EffectiveBaseURL()+"/auth/oauth2/token",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return Token{}, fmt.Errorf("token request returned %d: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"` // seconds
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return Token{}, fmt.Errorf("decode token response: %w", err)
	}

	return Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresAt:   time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}, nil
}

// -----------------------------------------------------------------------
// Alta API calls
// -----------------------------------------------------------------------

// ListDoors fetches the door inventory for the configured organisation.
func (c *Client) ListDoors(ctx context.Context) ([]AltaDoor, error) {
	tok, err := c.Authenticate(ctx)
	if err != nil {
		return nil, err
	}

	var result []AltaDoor
	err = c.doWithRetry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			fmt.Sprintf("%s/orgs/%s/doors", c.cfg.EffectiveBaseURL(), c.cfg.OrgID), nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)

		resp, err := c.http.Do(req)
		if err != nil {
			return &retryableError{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return &retryableError{err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)}
		}
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			return fmt.Errorf("list doors returned %d: %s", resp.StatusCode, body)
		}

		var respBody struct {
			Data []AltaDoor `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			return fmt.Errorf("decode doors response: %w", err)
		}
		result = respBody.Data
		return nil
	})
	return result, err
}

// AltaDoor is the door record returned by the Alta API.
type AltaDoor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TriggerLockdown sends a lockdown command to a specific door.
func (c *Client) TriggerLockdown(ctx context.Context, req LockdownRequest) error {
	tok, err := c.Authenticate(ctx)
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]string{
		"action": "lockdown",
		"reason": req.Reason,
	})

	return c.doWithRetry(ctx, func() error {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			fmt.Sprintf("%s/orgs/%s/doors/%s/actions", c.cfg.EffectiveBaseURL(), req.OrgID, req.DoorID),
			bytes.NewReader(body),
		)
		if err != nil {
			return err
		}
		httpReq.Header.Set("Authorization", tok.TokenType+" "+tok.AccessToken)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.http.Do(httpReq)
		if err != nil {
			return &retryableError{err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return &retryableError{err: fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)}
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			return fmt.Errorf("lockdown returned %d: %s", resp.StatusCode, respBody)
		}
		return nil
	})
}

// -----------------------------------------------------------------------
// Retry with exponential backoff
// -----------------------------------------------------------------------

// maxRetries is the maximum number of retry attempts for transient failures.
const maxRetries = 3

// retryableError signals that the operation should be retried.
type retryableError struct {
	err error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// doWithRetry executes fn with exponential backoff on retryable errors.
// Non-retryable errors are returned immediately.
func (c *Client) doWithRetry(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Only retry on retryable errors.
		if _, ok := lastErr.(*retryableError); !ok {
			return lastErr
		}

		if attempt < maxRetries {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * 500 * time.Millisecond
			c.log.WarnContext(ctx, "retrying Alta API call",
				"attempt", attempt+1,
				"backoff", backoff,
				"error", lastErr,
			)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}
	}
	return fmt.Errorf("openpath: exhausted %d retries: %w", maxRetries, lastErr)
}
