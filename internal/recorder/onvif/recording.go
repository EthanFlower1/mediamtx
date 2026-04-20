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

// EdgeRecording represents a single recording stored on a camera's edge storage (SD card).
type EdgeRecording struct {
	RecordingToken string `json:"recording_token"`
	SourceName     string `json:"source_name"`
	EarliestTime   string `json:"earliest_time"`
	LatestTime     string `json:"latest_time"`
}

// EdgeRecordingSummary provides an overview of all recordings on the camera.
type EdgeRecordingSummary struct {
	TotalRecordings int    `json:"total_recordings"`
	EarliestTime    string `json:"earliest_time"`
	LatestTime      string `json:"latest_time"`
}

// --- SOAP response types for recording search ---

type recordingSearchEnvelope struct {
	XMLName xml.Name            `xml:"Envelope"`
	Body    recordingSearchBody `xml:"Body"`
}

type recordingSearchBody struct {
	GetRecordingSummaryResponse *getRecordingSummaryResponse `xml:"GetRecordingSummaryResponse"`
	FindRecordingsResponse      *findRecordingsResponse      `xml:"FindRecordingsResponse"`
	GetRecordingSearchResultsResponse *getRecordingSearchResultsResponse `xml:"GetRecordingSearchResultsResponse"`
	Fault                       *recordingSearchFault        `xml:"Fault"`
}

type recordingSearchFault struct {
	Faultstring string `xml:"faultstring"`
}

type getRecordingSummaryResponse struct {
	Summary recordingSummaryXML `xml:"Summary"`
}

type recordingSummaryXML struct {
	DataFrom       string `xml:"DataFrom"`
	DataUntil      string `xml:"DataUntil"`
	NumberRecordings int  `xml:"NumberRecordings"`
}

type findRecordingsResponse struct {
	SearchToken string `xml:"SearchToken"`
}

type getRecordingSearchResultsResponse struct {
	ResultList recordingResultList `xml:"ResultList"`
}

type recordingResultList struct {
	SearchState      string                   `xml:"SearchState"`
	RecordingInformation []recordingInformationXML `xml:"RecordingInformation"`
}

type recordingInformationXML struct {
	RecordingToken string           `xml:"RecordingToken"`
	Source         recordingSource  `xml:"Source"`
	EarliestRecording string       `xml:"EarliestRecording"`
	LatestRecording   string       `xml:"LatestRecording"`
}

type recordingSource struct {
	SourceId    string `xml:"SourceId"`
	Name        string `xml:"Name"`
	Location    string `xml:"Location"`
	Description string `xml:"Description"`
}

// --- SOAP helper ---

func recordingSearchSoap(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tse="http://www.onvif.org/ver10/search/wsdl">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

func doRecordingSearchSOAP(ctx context.Context, searchURL, username, password, body string) ([]byte, error) {
	soapBody := recordingSearchSoap(body)

	if username != "" {
		soapBody = injectWSSecurity(soapBody, username, password)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("create recording search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recording search http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("recording search read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recording search SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// getSearchServiceURL returns the recording search service URL for the device.
func getSearchServiceURL(xaddr, username, password string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("connect to ONVIF device: %w", err)
	}

	searchURL := client.ServiceURL("recording")
	if searchURL == "" {
		// Some devices register the search service under "search" instead of "recording".
		searchURL = client.ServiceURL("search")
	}
	if searchURL == "" {
		return "", fmt.Errorf("device does not support recording search service")
	}
	return searchURL, nil
}

// GetRecordingSummary returns a summary of recordings stored on the camera's edge storage.
func GetRecordingSummary(xaddr, username, password string) (*EdgeRecordingSummary, error) {
	searchURL, err := getSearchServiceURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingSummary: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := `<tse:GetRecordingSummary/>`
	body, err := doRecordingSearchSOAP(ctx, searchURL, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingSummary: %w", err)
	}

	var env recordingSearchEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetRecordingSummary parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetRecordingSummary SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetRecordingSummaryResponse == nil {
		return nil, fmt.Errorf("GetRecordingSummary: empty response")
	}

	s := env.Body.GetRecordingSummaryResponse.Summary
	return &EdgeRecordingSummary{
		TotalRecordings: s.NumberRecordings,
		EarliestTime:    s.DataFrom,
		LatestTime:      s.DataUntil,
	}, nil
}

// FindRecordings discovers all recordings stored on the camera's edge storage using
// the session-based search pattern: FindRecordings -> poll GetRecordingSearchResults.
func FindRecordings(xaddr, username, password string) ([]EdgeRecording, error) {
	searchURL, err := getSearchServiceURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("FindRecordings: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Initiate a FindRecordings search session.
	findBody := `<tse:FindRecordings>
      <tse:Scope>
        <tse:IncludedSources/>
      </tse:Scope>
    </tse:FindRecordings>`

	body, err := doRecordingSearchSOAP(ctx, searchURL, username, password, findBody)
	if err != nil {
		return nil, fmt.Errorf("FindRecordings initiate: %w", err)
	}

	var findEnv recordingSearchEnvelope
	if err := xml.Unmarshal(body, &findEnv); err != nil {
		return nil, fmt.Errorf("FindRecordings parse search token: %w", err)
	}
	if findEnv.Body.Fault != nil {
		return nil, fmt.Errorf("FindRecordings SOAP fault: %s", findEnv.Body.Fault.Faultstring)
	}
	if findEnv.Body.FindRecordingsResponse == nil {
		return nil, fmt.Errorf("FindRecordings: empty response (no search token)")
	}

	searchToken := findEnv.Body.FindRecordingsResponse.SearchToken
	if searchToken == "" {
		return nil, fmt.Errorf("FindRecordings: empty search token")
	}

	// Step 2: Poll GetRecordingSearchResults until SearchState is "Completed".
	var allRecordings []EdgeRecording
	for {
		select {
		case <-ctx.Done():
			return allRecordings, fmt.Errorf("FindRecordings: timed out waiting for search results")
		default:
		}

		getResultsBody := fmt.Sprintf(`<tse:GetRecordingSearchResults>
      <tse:SearchToken>%s</tse:SearchToken>
      <tse:MinResults>1</tse:MinResults>
      <tse:MaxResults>50</tse:MaxResults>
      <tse:WaitTime>PT5S</tse:WaitTime>
    </tse:GetRecordingSearchResults>`, xmlEscape(searchToken))

		resultBody, err := doRecordingSearchSOAP(ctx, searchURL, username, password, getResultsBody)
		if err != nil {
			return allRecordings, fmt.Errorf("FindRecordings poll results: %w", err)
		}

		var resultEnv recordingSearchEnvelope
		if err := xml.Unmarshal(resultBody, &resultEnv); err != nil {
			return allRecordings, fmt.Errorf("FindRecordings parse results: %w", err)
		}
		if resultEnv.Body.Fault != nil {
			return allRecordings, fmt.Errorf("FindRecordings results SOAP fault: %s", resultEnv.Body.Fault.Faultstring)
		}
		if resultEnv.Body.GetRecordingSearchResultsResponse == nil {
			return allRecordings, fmt.Errorf("FindRecordings: empty search results response")
		}

		resultList := resultEnv.Body.GetRecordingSearchResultsResponse.ResultList

		for _, ri := range resultList.RecordingInformation {
			allRecordings = append(allRecordings, EdgeRecording{
				RecordingToken: ri.RecordingToken,
				SourceName:     ri.Source.Name,
				EarliestTime:   ri.EarliestRecording,
				LatestTime:     ri.LatestRecording,
			})
		}

		if strings.EqualFold(resultList.SearchState, "Completed") {
			break
		}

		// Brief pause before next poll to avoid hammering the device.
		time.Sleep(500 * time.Millisecond)
	}

	return allRecordings, nil
}
