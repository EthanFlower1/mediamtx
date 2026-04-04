package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// AllowedScales defines the valid Scale values for ONVIF Profile G replay.
// Positive values play forward; negative values play in reverse (I-frame seeking).
var AllowedScales = []float64{-4, -2, -1, 1, 2, 4}

// ReplaySession holds the parameters needed to initiate an RTSP replay session
// against a Profile G device, including the RTSP headers (Range, Scale, Speed)
// required by ONVIF for trick-play.
type ReplaySession struct {
	// ReplayURI is the RTSP URI obtained from the device's replay service.
	ReplayURI string `json:"replay_uri"`

	// RecordingToken identifies the recording on the device.
	RecordingToken string `json:"recording_token"`

	// StartTime is the absolute time to begin playback (RFC 3339).
	StartTime time.Time `json:"start_time"`

	// Scale controls playback direction and speed.
	// Positive values play forward (1 = normal, 2 = 2x fast-forward, 4 = 4x).
	// Negative values play in reverse via I-frame seeking (-1 = normal reverse,
	// -2 = 2x reverse, -4 = 4x reverse).
	Scale float64 `json:"scale"`

	// Reverse is true when Scale < 0, provided as a convenience flag.
	Reverse bool `json:"reverse"`

	// RTSPHeaders contains the RTSP headers the client should include in the
	// PLAY request to activate the desired trick-play mode.
	RTSPHeaders map[string]string `json:"rtsp_headers"`
}

// BuildReplaySession constructs a ReplaySession from the given parameters.
// It validates the scale value against AllowedScales and generates the correct
// RTSP headers for ONVIF Profile G replay, including reverse playback via
// negative Scale values.
//
// Per ONVIF Profile G:
//   - Range header uses clock= format for absolute time positioning.
//   - Scale header controls direction and speed (-1, -2, -4 for reverse).
//   - Negative scale triggers I-frame seeking on the device: the device sends
//     only I-frames in reverse temporal order, enabling smooth rewind at
//     the requested speed.
func BuildReplaySession(replayURI, recordingToken string, startTime time.Time, scale float64) (*ReplaySession, error) {
	if replayURI == "" {
		return nil, fmt.Errorf("BuildReplaySession: replay URI is required")
	}
	if recordingToken == "" {
		return nil, fmt.Errorf("BuildReplaySession: recording token is required")
	}
	if startTime.IsZero() {
		return nil, fmt.Errorf("BuildReplaySession: start time is required")
	}
	if !isAllowedScale(scale) {
		return nil, fmt.Errorf("BuildReplaySession: invalid scale %.1f, allowed values: %v", scale, AllowedScales)
	}

	reverse := scale < 0

	// Build RTSP headers per ONVIF Profile G specification.
	headers := make(map[string]string)

	// Range header: clock= format for absolute time (RFC 7826 / ONVIF Profile G).
	// For reverse playback the range starts at startTime and goes backwards,
	// but the Range header still specifies the start position.
	headers["Range"] = fmt.Sprintf("clock=%s-", startTime.UTC().Format("20060102T150405.000Z"))

	// Scale header: controls playback speed and direction.
	// Negative values signal reverse playback with I-frame seeking.
	headers["Scale"] = formatScale(scale)

	return &ReplaySession{
		ReplayURI:      replayURI,
		RecordingToken: recordingToken,
		StartTime:      startTime,
		Scale:          scale,
		Reverse:        reverse,
		RTSPHeaders:    headers,
	}, nil
}

// isAllowedScale checks whether the given scale is in the AllowedScales list.
func isAllowedScale(s float64) bool {
	for _, allowed := range AllowedScales {
		if math.Abs(s-allowed) < 0.001 {
			return true
		}
	}
	return false
}

// formatScale renders a scale value as a string suitable for the RTSP Scale header.
// Integer-valued floats are rendered without a decimal point (e.g. "1", "-2").
func formatScale(s float64) string {
	if s == float64(int(s)) {
		return fmt.Sprintf("%d", int(s))
	}
	return fmt.Sprintf("%.1f", s)
}

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
