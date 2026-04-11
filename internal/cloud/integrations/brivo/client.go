package brivo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// httpClient implements BrivoAPIClient using real HTTP calls to the Brivo
// REST API (https://apidocs.brivo.com).
type httpClient struct {
	cfg    OAuthConfig
	http   *http.Client
	apiURL string // base URL for Brivo API, default https://api.brivo.com/v1/api
}

// HTTPClientConfig holds config for the production Brivo HTTP client.
type HTTPClientConfig struct {
	OAuth  OAuthConfig
	APIURL string // override for testing; defaults to https://api.brivo.com/v1/api
}

// NewHTTPClient constructs a production BrivoAPIClient.
func NewHTTPClient(cfg HTTPClientConfig) BrivoAPIClient {
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = "https://api.brivo.com/v1/api"
	}
	return &httpClient{
		cfg:    cfg.OAuth,
		http:   &http.Client{Timeout: 30 * time.Second},
		apiURL: apiURL,
	}
}

func (c *httpClient) ExchangeCode(ctx context.Context, code, codeVerifier string) (TokenPair, error) {
	tokenURL := c.cfg.TokenURL
	if tokenURL == "" {
		tokenURL = "https://auth.brivo.com/oauth/token"
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.cfg.RedirectURL},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return TokenPair{}, fmt.Errorf("brivo: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return TokenPair{}, fmt.Errorf("brivo: token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return TokenPair{}, fmt.Errorf("brivo: token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return TokenPair{}, fmt.Errorf("brivo: decode token response: %w", err)
	}

	return TokenPair{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}

func (c *httpClient) RefreshToken(ctx context.Context, refreshToken string) (TokenPair, error) {
	tokenURL := c.cfg.TokenURL
	if tokenURL == "" {
		tokenURL = "https://auth.brivo.com/oauth/token"
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewBufferString(data.Encode()))
	if err != nil {
		return TokenPair{}, fmt.Errorf("brivo: build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return TokenPair{}, fmt.Errorf("brivo: refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return TokenPair{}, fmt.Errorf("brivo: token refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return TokenPair{}, fmt.Errorf("brivo: decode refresh response: %w", err)
	}

	return TokenPair{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		Scope:        tokenResp.Scope,
	}, nil
}

func (c *httpClient) ListSites(ctx context.Context, accessToken string) ([]BrivoSite, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/sites", nil)
	if err != nil {
		return nil, fmt.Errorf("brivo: build sites request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brivo: sites request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brivo: list sites failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []BrivoSite `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("brivo: decode sites response: %w", err)
	}
	return result.Data, nil
}

func (c *httpClient) ListDoors(ctx context.Context, accessToken string, siteID string) ([]BrivoDoor, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiURL+"/sites/"+siteID+"/access-points", nil)
	if err != nil {
		return nil, fmt.Errorf("brivo: build doors request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("brivo: doors request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brivo: list doors failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []BrivoDoor `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("brivo: decode doors response: %w", err)
	}
	return result.Data, nil
}

func (c *httpClient) SendEvent(ctx context.Context, accessToken string, event NVREvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("brivo: marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("brivo: build event request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("brivo: send event request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("brivo: send event failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
