package recorderapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// RecorderClient queries a single recorder's internal API.
type RecorderClient struct {
	baseURL      string
	serviceToken string
	http         *http.Client
}

// NewRecorderClient creates a client for a single recorder.
func NewRecorderClient(baseURL, serviceToken string) *RecorderClient {
	return &RecorderClient{
		baseURL:      baseURL,
		serviceToken: serviceToken,
		http:         &http.Client{Timeout: 10 * time.Second},
	}
}

// RecorderHealth is the response from GET /internal/v1/health.
type RecorderHealth struct {
	Status      string  `json:"status"`
	RecorderID  string  `json:"recorder_id"`
	CameraCount int     `json:"camera_count"`
	DiskTotalGB float64 `json:"disk_total_gb"`
	DiskFreeGB  float64 `json:"disk_free_gb"`
}

// Health queries the recorder's health endpoint.
func (c *RecorderClient) Health(ctx context.Context) (*RecorderHealth, error) {
	var result RecorderHealth
	if err := c.get(ctx, "/internal/v1/health", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RecordingItem matches the recorder's recording query response.
type RecordingItem struct {
	ID        int64  `json:"id"`
	CameraID  string `json:"camera_id"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	FilePath  string `json:"file_path"`
	FileSize  int64  `json:"file_size"`
	Format    string `json:"format"`
}

// QueryRecordings queries recordings from a recorder.
func (c *RecorderClient) QueryRecordings(ctx context.Context, cameraID string, start, end time.Time) ([]json.RawMessage, error) {
	params := url.Values{
		"camera_id": {cameraID},
		"start":     {start.UTC().Format(time.RFC3339)},
		"end":       {end.UTC().Format(time.RFC3339)},
	}
	var result struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := c.get(ctx, "/internal/v1/recordings", params, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

// TimelineBlock matches the recorder's timeline response.
type TimelineBlock struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// QueryTimeline queries timeline blocks from a recorder.
func (c *RecorderClient) QueryTimeline(ctx context.Context, cameraID, date string) ([]json.RawMessage, error) {
	params := url.Values{
		"camera_id": {cameraID},
		"date":      {date},
	}
	var result struct {
		Blocks []json.RawMessage `json:"blocks"`
	}
	if err := c.get(ctx, "/internal/v1/timeline", params, &result); err != nil {
		return nil, err
	}
	return result.Blocks, nil
}

// QueryEvents queries events from a recorder.
func (c *RecorderClient) QueryEvents(ctx context.Context, cameraID, eventType string, start, end time.Time) ([]json.RawMessage, error) {
	params := url.Values{
		"camera_id": {cameraID},
		"start":     {start.UTC().Format(time.RFC3339)},
		"end":       {end.UTC().Format(time.RFC3339)},
	}
	if eventType != "" {
		params.Set("type", eventType)
	}
	var result struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := c.get(ctx, "/internal/v1/events", params, &result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

func (c *RecorderClient) get(ctx context.Context, path string, params url.Values, out any) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("recorder client: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.serviceToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("recorder client: %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("recorder client: %s returned %d", path, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("recorder client: decode %s: %w", path, err)
	}
	return nil
}
