package pdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// HTTPDoer abstracts *http.Client for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Client communicates with the ProdataKey cloud API.
type Client struct {
	httpClient  HTTPDoer
	endpoint    string
	clientID    string
	clientSecret string
	panelID     string

	mu          sync.Mutex
	accessToken string
	tokenExpiry time.Time
}

// ClientConfig holds the parameters for constructing a PDK API client.
type ClientConfig struct {
	HTTPClient   HTTPDoer
	Endpoint     string
	ClientID     string
	ClientSecret string
	PanelID      string
}

// NewClient creates a new PDK API client. If HTTPClient is nil, http.DefaultClient is used.
func NewClient(cfg ClientConfig) *Client {
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{
		httpClient:   hc,
		endpoint:     cfg.Endpoint,
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		panelID:      cfg.PanelID,
	}
}

// Authenticate obtains an OAuth2 access token from the PDK API using client
// credentials. The token is cached and reused until close to expiry.
func (c *Client) Authenticate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.accessToken != "" && time.Now().Before(c.tokenExpiry.Add(-60*time.Second)) {
		return nil // token still valid
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/oauth/token",
		bytes.NewBufferString(form.Encode()))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIAuth, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIAuth, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrAPIAuth, resp.StatusCode, string(body))
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return fmt.Errorf("%w: decode token: %v", ErrAPIAuth, err)
	}
	c.accessToken = tok.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return nil
}

// ListDoors retrieves all doors for the configured panel.
func (c *Client) ListDoors(ctx context.Context) ([]PDKDoor, error) {
	if err := c.Authenticate(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/panels/%s/doors", c.endpoint, c.panelID), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrAPIRequest, resp.StatusCode, string(body))
	}

	var doors []PDKDoor
	if err := json.NewDecoder(resp.Body).Decode(&doors); err != nil {
		return nil, fmt.Errorf("%w: decode doors: %v", ErrAPIRequest, err)
	}
	return doors, nil
}

// GetDoor retrieves a single door by its PDK ID.
func (c *Client) GetDoor(ctx context.Context, doorID string) (*PDKDoor, error) {
	if err := c.Authenticate(ctx); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/panels/%s/doors/%s", c.endpoint, c.panelID, doorID), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrDoorNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrAPIRequest, resp.StatusCode, string(body))
	}

	var door PDKDoor
	if err := json.NewDecoder(resp.Body).Decode(&door); err != nil {
		return nil, fmt.Errorf("%w: decode door: %v", ErrAPIRequest, err)
	}
	return &door, nil
}

// ListEvents retrieves recent events for the configured panel within a time range.
func (c *Client) ListEvents(ctx context.Context, since, until time.Time) ([]PDKEvent, error) {
	if err := c.Authenticate(ctx); err != nil {
		return nil, err
	}

	u := fmt.Sprintf("%s/api/panels/%s/events?since=%s&until=%s",
		c.endpoint, c.panelID,
		url.QueryEscape(since.UTC().Format(time.RFC3339)),
		url.QueryEscape(until.UTC().Format(time.RFC3339)))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: status %d: %s", ErrAPIRequest, resp.StatusCode, string(body))
	}

	var events []PDKEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("%w: decode events: %v", ErrAPIRequest, err)
	}
	return events, nil
}

// UnlockDoor sends an unlock command to a specific door.
func (c *Client) UnlockDoor(ctx context.Context, doorID string) error {
	if err := c.Authenticate(ctx); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/panels/%s/doors/%s/unlock", c.endpoint, c.panelID, doorID), nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAPIRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%w: status %d: %s", ErrAPIRequest, resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) setAuth(req *http.Request) {
	c.mu.Lock()
	tok := c.accessToken
	c.mu.Unlock()
	req.Header.Set("Authorization", "Bearer "+tok)
}
