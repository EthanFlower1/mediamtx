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
