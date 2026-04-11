package itsm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func ogSuccessResponse() *http.Response {
	body, _ := json.Marshal(opsgenieResponse{
		Result:    "Request will be processed",
		RequestID: "og-req-abc123",
	})
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestOpsgenieClient_SendAlert(t *testing.T) {
	var captured opsgenieCreateAlert
	var capturedAuthHeader string
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedAuthHeader = req.Header.Get("Authorization")
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		return ogSuccessResponse(), nil
	})

	client, err := NewOpsgenieClient("og-api-key-123",
		WithOpsgenieHTTPClient(mock),
	)
	if err != nil {
		t.Fatalf("NewOpsgenieClient: %v", err)
	}

	if client.Type() != ProviderOpsgenie {
		t.Fatalf("expected provider type %s, got %s", ProviderOpsgenie, client.Type())
	}

	alert := Alert{
		Summary:   "Storage full on recorder-01",
		Source:    "mediamtx-nvr",
		Severity:  SeverityError,
		DedupKey:  "recorder-01-storage-full",
		Timestamp: time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC),
		Group:     "storage",
		Class:     "storage_full",
		Details:   map[string]string{"recorder": "recorder-01", "usage": "98%"},
	}

	result, err := client.SendAlert(context.Background(), alert)
	if err != nil {
		t.Fatalf("SendAlert: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("expected status success, got %s", result.Status)
	}
	if result.ExternalID != "og-req-abc123" {
		t.Errorf("expected external ID og-req-abc123, got %s", result.ExternalID)
	}
	if result.ProviderType != ProviderOpsgenie {
		t.Errorf("expected provider opsgenie, got %s", result.ProviderType)
	}

	// Verify request payload.
	if capturedAuthHeader != "GenieKey og-api-key-123" {
		t.Errorf("auth header: got %s", capturedAuthHeader)
	}
	if captured.Priority != "P2" {
		t.Errorf("priority: expected P2, got %s", captured.Priority)
	}
	if captured.Alias != "recorder-01-storage-full" {
		t.Errorf("alias: got %s", captured.Alias)
	}
	if len(captured.Tags) != 1 || captured.Tags[0] != "storage_full" {
		t.Errorf("tags: got %v", captured.Tags)
	}
}

func TestOpsgenieClient_ResolveAlert(t *testing.T) {
	var capturedURL string
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return ogSuccessResponse(), nil
	})

	client, err := NewOpsgenieClient("og-key",
		WithOpsgenieHTTPClient(mock),
		WithOpsgenieEndpoint("https://test.opsgenie.example.com/v2/alerts"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.ResolveAlert(context.Background(), "my-dedup-key")
	if err != nil {
		t.Fatalf("ResolveAlert: %v", err)
	}

	expected := "https://test.opsgenie.example.com/v2/alerts/my-dedup-key/close?identifierType=alias"
	if capturedURL != expected {
		t.Errorf("expected URL %s, got %s", expected, capturedURL)
	}
}

func TestOpsgenieClient_RateLimited(t *testing.T) {
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"message":"rate limited"}`))),
			Header:     make(http.Header),
		}, nil
	})

	client, err := NewOpsgenieClient("og-key", WithOpsgenieHTTPClient(mock))
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

func TestOpsgenieClient_EmptyAPIKey(t *testing.T) {
	_, err := NewOpsgenieClient("")
	if err == nil {
		t.Error("expected error for empty API key")
	}
}

func TestOpsgenieClient_EUEndpoint(t *testing.T) {
	var capturedURL string
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		return ogSuccessResponse(), nil
	})

	client, err := NewOpsgenieClient("og-key",
		WithOpsgenieEU(),
		WithOpsgenieHTTPClient(mock),
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

	if !strings.HasPrefix(capturedURL, opsgenieEUEndpoint) {
		t.Errorf("expected EU endpoint prefix, got %s", capturedURL)
	}
}

func TestOpsgenieClient_PriorityMapping(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityCritical, "P1"},
		{SeverityError, "P2"},
		{SeverityWarning, "P3"},
		{SeverityInfo, "P5"},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			got := ogPriority(tt.severity)
			if got != tt.expected {
				t.Errorf("ogPriority(%s) = %s, want %s", tt.severity, got, tt.expected)
			}
		})
	}
}

func TestOpsgenieClient_MessageTruncation(t *testing.T) {
	longMsg := strings.Repeat("x", 200)

	var captured opsgenieCreateAlert
	mock := newMockHTTPClient(func(req *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(body, &captured)
		return ogSuccessResponse(), nil
	})

	client, err := NewOpsgenieClient("key", WithOpsgenieHTTPClient(mock))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.SendAlert(context.Background(), Alert{
		Summary:   longMsg,
		Source:    "test",
		Severity:  SeverityInfo,
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(captured.Message) != 130 {
		t.Errorf("expected truncated message of 130 chars, got %d", len(captured.Message))
	}
}
