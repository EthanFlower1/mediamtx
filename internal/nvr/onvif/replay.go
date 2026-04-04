package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// --- SOAP response types for replay ---

type replayEnvelope struct {
	XMLName xml.Name   `xml:"Envelope"`
	Body    replayBody `xml:"Body"`
}

type replayBody struct {
	GetReplayUriResponse *getReplayUriResponse `xml:"GetReplayUriResponse"`
	Fault                *replayFault          `xml:"Fault"`
}

type replayFault struct {
	Faultstring string `xml:"faultstring"`
}

type getReplayUriResponse struct {
	Uri string `xml:"Uri"`
}

// --- SOAP helper ---

func replaySoap(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trp="http://www.onvif.org/ver10/replay/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

// GetReplayUri returns the RTSP URI for replaying a recording from the camera's edge storage.
func GetReplayUri(xaddr, username, password, recordingToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("GetReplayUri: connect to ONVIF device: %w", err)
	}

	replayURL := client.ServiceURL("replay")
	if replayURL == "" {
		return "", fmt.Errorf("GetReplayUri: device does not support replay service")
	}

	reqBody := fmt.Sprintf(`<trp:GetReplayUri>
      <trp:StreamSetup>
        <tt:Stream>RTP-Unicast</tt:Stream>
        <tt:Transport>
          <tt:Protocol>RTSP</tt:Protocol>
        </tt:Transport>
      </trp:StreamSetup>
      <trp:RecordingToken>%s</trp:RecordingToken>
    </trp:GetReplayUri>`, xmlEscape(recordingToken))

	soapBody := replaySoap(reqBody)

	if username != "" {
		soapBody = injectWSSecurity(soapBody, username, password)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, replayURL, strings.NewReader(soapBody))
	if err != nil {
		return "", fmt.Errorf("GetReplayUri: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GetReplayUri: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("GetReplayUri: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GetReplayUri: SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var env replayEnvelope
	if err := xml.Unmarshal(respBody, &env); err != nil {
		return "", fmt.Errorf("GetReplayUri: parse response: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("GetReplayUri: SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetReplayUriResponse == nil {
		return "", fmt.Errorf("GetReplayUri: empty response")
	}

	uri := strings.TrimSpace(env.Body.GetReplayUriResponse.Uri)
	if uri == "" {
		return "", fmt.Errorf("GetReplayUri: empty URI in response")
	}

	return uri, nil
}

// ReplaySession describes the parameters needed to start an RTSP replay
// session against a camera's edge storage.  The caller uses the returned
// ReplayURI together with RTSP Range/Scale/Speed headers to control playback.
type ReplaySession struct {
	// ReplayURI is the RTSP URI for the recording replay.
	ReplayURI string `json:"replay_uri"`
	// RecordingToken is the ONVIF recording token being replayed.
	RecordingToken string `json:"recording_token"`
	// StartTime is the requested start of the playback range (RFC 3339).
	StartTime string `json:"start_time,omitempty"`
	// EndTime is the requested end of the playback range (RFC 3339).
	EndTime string `json:"end_time,omitempty"`
	// Scale controls trick-play speed (e.g. 2.0 = 2x forward, -1.0 = reverse).
	// A value of 0 means normal (1x) playback.
	Scale float64 `json:"scale,omitempty"`
	// Speed controls the delivery rate relative to real-time.
	// A value of 0 means server-default (typically 1.0).
	Speed float64 `json:"speed,omitempty"`
	// RangeHeader is the pre-formatted RTSP Range header value.
	RangeHeader string `json:"range_header,omitempty"`
	// ScaleHeader is the pre-formatted RTSP Scale header value.
	ScaleHeader string `json:"scale_header,omitempty"`
	// SpeedHeader is the pre-formatted RTSP Speed header value.
	SpeedHeader string `json:"speed_header,omitempty"`
}

// ReplaySessionRequest holds the parameters for starting a replay session.
type ReplaySessionRequest struct {
	// RecordingToken is the ONVIF recording to replay (required).
	RecordingToken string `json:"recording_token"`
	// StartTime is the desired playback start in RFC 3339 format (optional).
	StartTime string `json:"start_time,omitempty"`
	// EndTime is the desired playback end in RFC 3339 format (optional).
	EndTime string `json:"end_time,omitempty"`
	// Scale controls trick-play speed.  1.0 = normal, 2.0 = 2x, -1.0 = reverse.
	// Zero or omitted means normal playback.
	Scale float64 `json:"scale,omitempty"`
	// Speed controls the delivery rate (how fast data is sent over the network).
	// Zero or omitted means server default.
	Speed float64 `json:"speed,omitempty"`
}

// BuildReplaySession obtains the RTSP replay URI from the device and
// constructs the appropriate RTSP headers for Range, Scale, and Speed
// based on the request parameters.
func BuildReplaySession(xaddr, username, password string, req *ReplaySessionRequest) (*ReplaySession, error) {
	if req.RecordingToken == "" {
		return nil, fmt.Errorf("BuildReplaySession: recording_token is required")
	}

	replayURI, err := GetReplayUri(xaddr, username, password, req.RecordingToken)
	if err != nil {
		return nil, fmt.Errorf("BuildReplaySession: %w", err)
	}

	session := &ReplaySession{
		ReplayURI:      replayURI,
		RecordingToken: req.RecordingToken,
		StartTime:      req.StartTime,
		EndTime:        req.EndTime,
		Scale:          req.Scale,
		Speed:          req.Speed,
	}

	// Build RTSP Range header (clock mode per RFC 7826 / ONVIF Profile G).
	session.RangeHeader = buildRangeHeader(req.StartTime, req.EndTime)

	// Build Scale header for trick-play.
	if req.Scale != 0 {
		session.ScaleHeader = fmt.Sprintf("%.1f", req.Scale)
	}

	// Build Speed header for delivery rate control.
	if req.Speed != 0 {
		session.SpeedHeader = fmt.Sprintf("%.1f", req.Speed)
	}

	return session, nil
}

// buildRangeHeader formats an RTSP Range header using the "clock" format
// expected by ONVIF Profile G replay.
//
// Examples:
//
//	clock=20240101T120000Z-20240101T130000Z   (bounded)
//	clock=20240101T120000Z-                    (open-ended)
//	""                                          (no range = play from start)
func buildRangeHeader(startTime, endTime string) string {
	if startTime == "" {
		return ""
	}

	start, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		// Fall back to using the raw string if parsing fails.
		if endTime != "" {
			return fmt.Sprintf("clock=%s-%s", startTime, endTime)
		}
		return fmt.Sprintf("clock=%s-", startTime)
	}

	clockStart := start.UTC().Format("20060102T150405Z")

	if endTime == "" {
		return fmt.Sprintf("clock=%s-", clockStart)
	}

	end, err := time.Parse(time.RFC3339, endTime)
	if err != nil {
		return fmt.Sprintf("clock=%s-%s", clockStart, endTime)
	}

	clockEnd := end.UTC().Format("20060102T150405Z")
	return fmt.Sprintf("clock=%s-%s", clockStart, clockEnd)
}

// GetReplayCapabilities returns the replay service capabilities for a device.
func GetReplayCapabilities(xaddr, username, password string) (*ReplayCapabilities, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetReplayCapabilities: %w", err)
	}

	if !client.HasService("replay") {
		return nil, fmt.Errorf("GetReplayCapabilities: device does not support replay service")
	}

	if client.DetailedCapabilities != nil && client.DetailedCapabilities.Replay != nil {
		return client.DetailedCapabilities.Replay, nil
	}

	return &ReplayCapabilities{}, nil
}
