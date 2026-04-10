package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PrometheusQuerier executes instant queries against a Prometheus-compatible
// API. Mock this interface in tests.
type PrometheusQuerier interface {
	// Query executes a PromQL instant query and returns the scalar result.
	// For vector results with a single element, the value of that element is
	// returned. Returns an error if the result is empty or ambiguous.
	Query(ctx context.Context, query string) (float64, error)
}

// HTTPPrometheusQuerier queries Prometheus over HTTP.
type HTTPPrometheusQuerier struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPPrometheusQuerier creates a querier targeting the given Prometheus
// base URL (e.g. "http://prometheus:9090").
func NewHTTPPrometheusQuerier(baseURL string) *HTTPPrometheusQuerier {
	return &HTTPPrometheusQuerier{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Query implements PrometheusQuerier.
func (q *HTTPPrometheusQuerier) Query(ctx context.Context, query string) (float64, error) {
	u := fmt.Sprintf("%s/api/v1/query?query=%s", q.baseURL, url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("prometheus query: %w", err)
	}

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("prometheus query: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, fmt.Errorf("prometheus query read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus query: status %d: %s", resp.StatusCode, body)
	}

	return parsePromResponse(body)
}

// promResponse is the minimal shape of a Prometheus /api/v1/query response.
type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
}

type promData struct {
	ResultType string            `json:"resultType"`
	Result     json.RawMessage   `json:"result"`
}

type promVectorResult struct {
	Value [2]json.RawMessage `json:"value"`
}

func parsePromResponse(body []byte) (float64, error) {
	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return 0, fmt.Errorf("prometheus: unmarshal response: %w", err)
	}
	if pr.Status != "success" {
		return 0, fmt.Errorf("prometheus: query status %q", pr.Status)
	}

	switch pr.Data.ResultType {
	case "scalar":
		// Scalar: [timestamp, "value"]
		var pair [2]json.RawMessage
		if err := json.Unmarshal(pr.Data.Result, &pair); err != nil {
			return 0, fmt.Errorf("prometheus: unmarshal scalar: %w", err)
		}
		var s string
		if err := json.Unmarshal(pair[1], &s); err != nil {
			return 0, fmt.Errorf("prometheus: unmarshal scalar value: %w", err)
		}
		return strconv.ParseFloat(s, 64)

	case "vector":
		var results []promVectorResult
		if err := json.Unmarshal(pr.Data.Result, &results); err != nil {
			return 0, fmt.Errorf("prometheus: unmarshal vector: %w", err)
		}
		if len(results) == 0 {
			return 0, fmt.Errorf("prometheus: empty vector result for query")
		}
		if len(results) > 1 {
			return 0, fmt.Errorf("prometheus: ambiguous vector result (%d elements)", len(results))
		}
		var s string
		if err := json.Unmarshal(results[0].Value[1], &s); err != nil {
			return 0, fmt.Errorf("prometheus: unmarshal vector value: %w", err)
		}
		return strconv.ParseFloat(s, 64)

	default:
		return 0, fmt.Errorf("prometheus: unsupported result type %q", pr.Data.ResultType)
	}
}
