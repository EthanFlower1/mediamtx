package itsm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// roundTripFunc adapts a function to implement http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func pdSuccessResponse(dedupKey string) *http.Response {
	body, _ := json.Marshal(pagerDutyResponse{
		Status:   "success",
		Message:  "Event processed",
		DedupKey: dedupKey,
	})
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestPagerDutyClient_SendAlert(t *testing.T) {
	var captured pagerDutyEvent
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		return pdSuccessResponse("test-dedup-123"), nil
	})

	client, err := NewPagerDutyClient("test-routing-key",
		WithPagerDutyHTTPClient(mock),
	)
	if err != nil {
		t.Fatalf("NewPagerDutyClient: %v", err)
	}

	if client.Type() != ProviderPagerDuty {
		t.Fatalf("expected provider type %s, got %s", ProviderPagerDuty, client.Type())
	}

	alert := Alert{
		Summary:   "Camera offline: front-door",
		Source:    "raikada",
		Severity:  SeverityCritical,
		DedupKey:  "cam-front-door-offline",
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Group:     "cameras",
		Class:     "camera_offline",
		Details:   map[string]string{"camera_id": "front-door"},
	}

	result, err := client.SendAlert(context.Background(), alert)
	if err != nil {
		t.Fatalf("SendAlert: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected status success, got %s", result.Status)
	}
	if result.ExternalID != "test-dedup-123" {
		t.Errorf("expected external ID test-dedup-123, got %s", result.ExternalID)
	}
	if result.ProviderType != ProviderPagerDuty {
		t.Errorf("expected provider pagerduty, got %s", result.ProviderType)
	}

	// Verify the captured request payload.
	if captured.RoutingKey != "test-routing-key" {
		t.Errorf("routing key: got %s", captured.RoutingKey)
	}
	if captured.EventAction != "trigger" {
		t.Errorf("event action: got %s", captured.EventAction)
	}
	if captured.Payload.Severity != "critical" {
		t.Errorf("severity: got %s", captured.Payload.Severity)
	}
	if captured.Payload.Summary != "Camera offline: front-door" {
		t.Errorf("summary: got %s", captured.Payload.Summary)
	}
}

func TestPagerDutyClient_ResolveAlert(t *testing.T) {
	var captured pagerDutyEvent
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &captured)
		return pdSuccessResponse("cam-offline-123"), nil
	})

	client, err := NewPagerDutyClient("test-key", WithPagerDutyHTTPClient(mock))
	if err != nil {
		t.Fatalf("NewPagerDutyClient: %v", err)
	}

	result, err := client.ResolveAlert(context.Background(), "cam-offline-123")
	if err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	if captured.EventAction != "resolve" {
		t.Errorf("expected resolve action, got %s", captured.EventAction)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %s", result.Status)
	}
}

func TestPagerDutyClient_RateLimited(t *testing.T) {
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"throttle"}`))),
			Header:     make(http.Header),
		}, nil
	})

	client, err := NewPagerDutyClient("test-key", WithPagerDutyHTTPClient(mock))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SendAlert(context.Background(), Alert{
		Summary:   "test",
		Source:    "test",
		Severity:  SeverityInfo,
		Timestamp: time.Now(),
	})
	if err != ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestPagerDutyClient_ServerError(t *testing.T) {
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"status":"error"}`))),
			Header:     make(http.Header),
		}, nil
	})

	client, err := NewPagerDutyClient("test-key", WithPagerDutyHTTPClient(mock))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SendAlert(context.Background(), Alert{
		Summary:   "test",
		Source:    "test",
		Severity:  SeverityError,
		Timestamp: time.Now(),
	})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestPagerDutyClient_EmptyRoutingKey(t *testing.T) {
	_, err := NewPagerDutyClient("")
	if err == nil {
		t.Error("expected error for empty routing key")
	}
}

func TestPagerDutyClient_SummaryTruncation(t *testing.T) {
	longSummary := make([]byte, 2000)
	for i := range longSummary {
		longSummary[i] = 'a'
	}

	var captured pagerDutyEvent
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &captured)
		return pdSuccessResponse("trunc-test"), nil
	})

	client, err := NewPagerDutyClient("key", WithPagerDutyHTTPClient(mock))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SendAlert(context.Background(), Alert{
		Summary:   string(longSummary),
		Source:    "test",
		Severity:  SeverityInfo,
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(captured.Payload.Summary) != pagerDutyMaxSummaryLen {
		t.Errorf("expected truncated summary of %d chars, got %d",
			pagerDutyMaxSummaryLen, len(captured.Payload.Summary))
	}
}

func TestPagerDutyClient_CustomEndpoint(t *testing.T) {
	var capturedURL string
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return pdSuccessResponse("ep-test"), nil
	})

	client, err := NewPagerDutyClient("key",
		WithPagerDutyEndpoint("https://custom.pagerduty.example.com/v2/enqueue"),
		WithPagerDutyHTTPClient(mock),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SendAlert(context.Background(), Alert{
		Summary:   "test",
		Source:    "test",
		Severity:  SeverityInfo,
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if capturedURL != "https://custom.pagerduty.example.com/v2/enqueue" {
		t.Errorf("expected custom endpoint, got %s", capturedURL)
	}
}
