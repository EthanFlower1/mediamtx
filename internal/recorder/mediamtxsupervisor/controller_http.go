package mediamtxsupervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPController is the production Controller. It speaks HTTP to the
// MediaMTX control API documented at /v3/config/paths/...
//
// MediaMTX exposes per-path replace endpoints rather than a bulk
// "set everything at once" endpoint, so HTTPController:
//
//  1. Lists existing paths under the configured prefix via
//     `GET /v3/config/paths/list`.
//  2. For every desired path: `POST /v3/config/paths/add/{name}` if it
//     does not exist, otherwise `PATCH /v3/config/paths/patch/{name}`.
//  3. For every existing-but-no-longer-desired path under the prefix:
//     `DELETE /v3/config/paths/delete/{name}`.
//
// All operations are hot reloads — MediaMTX applies them without a
// process restart. The controller never restarts the MediaMTX process
// itself; that is the sidecar supervisor's job.
type HTTPController struct {
	// BaseURL is the MediaMTX HTTP API root, e.g. "http://127.0.0.1:9997".
	// The controller per CLAUDE.md should always be loopback-bound.
	BaseURL string

	// Client is the HTTP client. nil means a default client with a
	// 5s timeout.
	Client *http.Client

	// PathPrefix is the prefix the supervisor owns. Any path on
	// the MediaMTX side that starts with this prefix is considered
	// Recorder-managed and may be deleted on reconcile. Must
	// match the RenderOptions.PathPrefix used by the supervisor.
	PathPrefix string

	// AuthToken is sent as `Authorization: Bearer <token>` if set.
	AuthToken string
}

func (c *HTTPController) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 5 * time.Second}
}

// ApplyPaths implements Controller.
func (c *HTTPController) ApplyPaths(ctx context.Context, set PathConfigSet) error {
	existing, err := c.listOwnedPaths(ctx)
	if err != nil {
		return fmt.Errorf("list paths: %w", err)
	}

	desired := make(map[string]PathConfig, len(set.Paths))
	for _, p := range set.Paths {
		desired[p.Name] = p
	}

	// Add or patch desired paths.
	for name, p := range desired {
		if _, ok := existing[name]; ok {
			if err := c.patchPath(ctx, p); err != nil {
				return fmt.Errorf("patch %q: %w", name, err)
			}
			continue
		}
		if err := c.addPath(ctx, p); err != nil {
			return fmt.Errorf("add %q: %w", name, err)
		}
	}

	// Delete owned paths that are no longer desired.
	for name := range existing {
		if _, keep := desired[name]; keep {
			continue
		}
		if err := c.deletePath(ctx, name); err != nil {
			return fmt.Errorf("delete %q: %w", name, err)
		}
	}
	return nil
}

// Healthy implements Controller.
func (c *HTTPController) Healthy(ctx context.Context) error {
	req, err := c.newReq(ctx, http.MethodGet, "/v3/config/global/get", nil)
	if err != nil {
		return err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("mediamtx /v3/config/global/get: %s", resp.Status)
	}
	return nil
}

// listOwnedPaths returns the names of MediaMTX paths whose name starts
// with the configured PathPrefix. Result map values are intentionally
// empty; we only need the keys.
func (c *HTTPController) listOwnedPaths(ctx context.Context) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	page := 0
	for {
		u := fmt.Sprintf("/v3/config/paths/list?itemsPerPage=200&page=%d", page)
		req, err := c.newReq(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.client().Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("list paths page %d: %s: %s",
				page, resp.Status, string(body))
		}
		var parsed struct {
			ItemCount int `json:"itemCount"`
			PageCount int `json:"pageCount"`
			Items     []struct {
				Name string `json:"name"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("decode list paths: %w", err)
		}
		for _, it := range parsed.Items {
			if strings.HasPrefix(it.Name, c.PathPrefix) {
				out[it.Name] = struct{}{}
			}
		}
		page++
		if page >= parsed.PageCount {
			break
		}
	}
	return out, nil
}

func (c *HTTPController) addPath(ctx context.Context, p PathConfig) error {
	return c.writePath(ctx, http.MethodPost, "/v3/config/paths/add/", p)
}

func (c *HTTPController) patchPath(ctx context.Context, p PathConfig) error {
	return c.writePath(ctx, http.MethodPatch, "/v3/config/paths/patch/", p)
}

func (c *HTTPController) writePath(ctx context.Context, method, prefix string, p PathConfig) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := c.newReq(ctx, method, prefix+url.PathEscape(p.Name), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %s: %s", method, prefix+p.Name, resp.Status, string(b))
	}
	return nil
}

func (c *HTTPController) deletePath(ctx context.Context, name string) error {
	req, err := c.newReq(ctx, http.MethodDelete, "/v3/config/paths/delete/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete %q: %s: %s", name, resp.Status, string(b))
	}
	return nil
}

func (c *HTTPController) newReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if c.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AuthToken)
	}
	return req, nil
}
