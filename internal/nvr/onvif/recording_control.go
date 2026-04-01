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

// RecordingSource describes the source of an ONVIF recording.
type RecordingSource struct {
	SourceID    string `json:"source_id"`
	Name        string `json:"name"`
	Location    string `json:"location"`
	Description string `json:"description"`
	Address     string `json:"address"`
}

// RecordingConfiguration holds the configuration for an ONVIF recording container.
type RecordingConfiguration struct {
	RecordingToken       string          `json:"recording_token"`
	Source               RecordingSource `json:"source"`
	MaximumRetentionTime string          `json:"maximum_retention_time"`
	Content              string          `json:"content"`
}

// RecordingJobConfiguration holds the configuration for a recording job.
type RecordingJobConfiguration struct {
	JobToken       string `json:"job_token"`
	RecordingToken string `json:"recording_token"`
	Mode           string `json:"mode"`
	Priority       int    `json:"priority"`
}

// RecordingJobStateSource describes per-source state within a recording job.
type RecordingJobStateSource struct {
	SourceToken string `json:"source_token"`
	State       string `json:"state"`
}

// RecordingJobState holds the current state of a recording job.
type RecordingJobState struct {
	JobToken       string                    `json:"job_token"`
	RecordingToken string                    `json:"recording_token"`
	State          string                    `json:"state"`
	Sources        []RecordingJobStateSource `json:"sources"`
}

// --- SOAP XML types for recording control ---

type recordingControlEnvelope struct {
	XMLName xml.Name             `xml:"Envelope"`
	Body    recordingControlBody `xml:"Body"`
}

type recordingControlBody struct {
	CreateRecordingResponse          *createRecordingResponse          `xml:"CreateRecordingResponse"`
	DeleteRecordingResponse          *deleteRecordingResponse          `xml:"DeleteRecordingResponse"`
	GetRecordingConfigurationResponse *getRecordingConfigurationResponse `xml:"GetRecordingConfigurationResponse"`
	CreateRecordingJobResponse       *createRecordingJobResponse       `xml:"CreateRecordingJobResponse"`
	DeleteRecordingJobResponse       *deleteRecordingJobResponse       `xml:"DeleteRecordingJobResponse"`
	GetRecordingJobStateResponse     *getRecordingJobStateResponse     `xml:"GetRecordingJobStateResponse"`
	Fault                            *recordingControlFault            `xml:"Fault"`
}

type recordingControlFault struct {
	Faultstring string `xml:"faultstring"`
}

type createRecordingResponse struct {
	RecordingToken string `xml:"RecordingToken"`
}

type deleteRecordingResponse struct{}

type getRecordingConfigurationResponse struct {
	RecordingConfiguration recordingConfigurationXML `xml:"RecordingConfiguration"`
}

type recordingConfigurationXML struct {
	RecordingToken       string `xml:"token,attr"`
	Source               recordingSourceXML `xml:"Source"`
	MaximumRetentionTime string             `xml:"MaximumRetentionTime"`
	Content              string             `xml:"Content"`
}

type recordingSourceXML struct {
	SourceID    string `xml:"SourceID"`
	Name        string `xml:"Name"`
	Location    string `xml:"Location"`
	Description string `xml:"Description"`
	Address     string `xml:"Address"`
}

type createRecordingJobResponse struct {
	JobToken       string                     `xml:"JobToken"`
	JobConfiguration recordingJobConfigXML    `xml:"JobConfiguration"`
}

type recordingJobConfigXML struct {
	RecordingToken string `xml:"RecordingToken"`
	Mode           string `xml:"Mode"`
	Priority       int    `xml:"Priority"`
}

type deleteRecordingJobResponse struct{}

type getRecordingJobStateResponse struct {
	State recordingJobStateXML `xml:"State"`
}

type recordingJobStateXML struct {
	RecordingToken string                      `xml:"RecordingToken"`
	State          string                      `xml:"State"`
	Sources        recordingJobStateSourcesXML `xml:"Sources"`
}

type recordingJobStateSourcesXML struct {
	Items []recordingJobStateSourceXML `xml:"Source"`
}

type recordingJobStateSourceXML struct {
	SourceToken recordingJobSourceTokenXML `xml:"SourceToken"`
	State       string                     `xml:"State"`
}

type recordingJobSourceTokenXML struct {
	Token string `xml:"Token"`
}

// --- SOAP helpers ---

func recordingControlSoap(innerBody string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trc="http://www.onvif.org/ver10/recording/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Header></s:Header>
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, innerBody)
}

func doRecordingControlSOAP(ctx context.Context, controlURL, username, password, body string) ([]byte, error) {
	soapBody := recordingControlSoap(body)

	if username != "" {
		soapBody = injectWSSecurity(soapBody, username, password)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, controlURL, strings.NewReader(soapBody))
	if err != nil {
		return nil, fmt.Errorf("create recording control request: %w", err)
	}
	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("recording control http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("recording control read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recording control SOAP fault (HTTP %d): %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	return respBody, nil
}

// getRecordingControlURL returns the recording control service URL for the device.
func getRecordingControlURL(xaddr, username, password string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("connect to ONVIF device: %w", err)
	}

	controlURL := client.ServiceURL("recording_control")
	if controlURL == "" {
		// Fallback: some devices may register under "recording".
		controlURL = client.ServiceURL("recording")
	}
	if controlURL == "" {
		return "", fmt.Errorf("device does not support recording control service")
	}
	return controlURL, nil
}

// GetRecordingConfiguration returns the configuration for a specific recording on the device.
func GetRecordingConfiguration(xaddr, username, password, recordingToken string) (*RecordingConfiguration, error) {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:GetRecordingConfiguration>
      <trc:RecordingToken>%s</trc:RecordingToken>
    </trc:GetRecordingConfiguration>`, xmlEscape(recordingToken))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetRecordingConfigurationResponse == nil {
		return nil, fmt.Errorf("GetRecordingConfiguration: empty response")
	}

	rc := env.Body.GetRecordingConfigurationResponse.RecordingConfiguration
	return &RecordingConfiguration{
		RecordingToken:       rc.RecordingToken,
		Source: RecordingSource{
			SourceID:    rc.Source.SourceID,
			Name:        rc.Source.Name,
			Location:    rc.Source.Location,
			Description: rc.Source.Description,
			Address:     rc.Source.Address,
		},
		MaximumRetentionTime: rc.MaximumRetentionTime,
		Content:              rc.Content,
	}, nil
}

// CreateRecording creates a new recording container on the device's edge storage.
func CreateRecording(
	xaddr, username, password string,
	source RecordingSource, maxRetention, content string,
) (string, error) {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("CreateRecording: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:CreateRecording>
      <trc:RecordingConfiguration>
        <tt:Source>
          <tt:SourceID>%s</tt:SourceID>
          <tt:Name>%s</tt:Name>
          <tt:Location>%s</tt:Location>
          <tt:Description>%s</tt:Description>
          <tt:Address>%s</tt:Address>
        </tt:Source>
        <tt:MaximumRetentionTime>%s</tt:MaximumRetentionTime>
        <tt:Content>%s</tt:Content>
      </trc:RecordingConfiguration>
    </trc:CreateRecording>`,
		xmlEscape(source.SourceID),
		xmlEscape(source.Name),
		xmlEscape(source.Location),
		xmlEscape(source.Description),
		xmlEscape(source.Address),
		xmlEscape(maxRetention),
		xmlEscape(content))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return "", fmt.Errorf("CreateRecording: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("CreateRecording parse: %w", err)
	}
	if env.Body.Fault != nil {
		return "", fmt.Errorf("CreateRecording SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateRecordingResponse == nil {
		return "", fmt.Errorf("CreateRecording: empty response")
	}

	token := strings.TrimSpace(env.Body.CreateRecordingResponse.RecordingToken)
	if token == "" {
		return "", fmt.Errorf("CreateRecording: empty recording token in response")
	}
	return token, nil
}

// DeleteRecording deletes a recording container from the device's edge storage.
func DeleteRecording(xaddr, username, password, recordingToken string) error {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteRecording: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:DeleteRecording>
      <trc:RecordingToken>%s</trc:RecordingToken>
    </trc:DeleteRecording>`, xmlEscape(recordingToken))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return fmt.Errorf("DeleteRecording: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("DeleteRecording parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("DeleteRecording SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}

// CreateRecordingJob creates a recording job that records into the specified recording container.
// Mode should be "Active" (start recording) or "Idle" (create but don't start).
func CreateRecordingJob(
	xaddr, username, password, recordingToken, mode string,
	priority int,
) (*RecordingJobConfiguration, error) {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("CreateRecordingJob: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:CreateRecordingJob>
      <trc:JobConfiguration>
        <tt:RecordingToken>%s</tt:RecordingToken>
        <tt:Mode>%s</tt:Mode>
        <tt:Priority>%d</tt:Priority>
      </trc:JobConfiguration>
    </trc:CreateRecordingJob>`,
		xmlEscape(recordingToken),
		xmlEscape(mode),
		priority)

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("CreateRecordingJob: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("CreateRecordingJob parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("CreateRecordingJob SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateRecordingJobResponse == nil {
		return nil, fmt.Errorf("CreateRecordingJob: empty response")
	}

	resp := env.Body.CreateRecordingJobResponse
	return &RecordingJobConfiguration{
		JobToken:       strings.TrimSpace(resp.JobToken),
		RecordingToken: resp.JobConfiguration.RecordingToken,
		Mode:           resp.JobConfiguration.Mode,
		Priority:       resp.JobConfiguration.Priority,
	}, nil
}

// DeleteRecordingJob deletes a recording job from the device.
func DeleteRecordingJob(xaddr, username, password, jobToken string) error {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteRecordingJob: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:DeleteRecordingJob>
      <trc:JobToken>%s</trc:JobToken>
    </trc:DeleteRecordingJob>`, xmlEscape(jobToken))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return fmt.Errorf("DeleteRecordingJob: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("DeleteRecordingJob parse: %w", err)
	}
	if env.Body.Fault != nil {
		return fmt.Errorf("DeleteRecordingJob SOAP fault: %s", env.Body.Fault.Faultstring)
	}

	return nil
}

// GetRecordingJobState returns the current state of a recording job.
func GetRecordingJobState(xaddr, username, password, jobToken string) (*RecordingJobState, error) {
	controlURL, err := getRecordingControlURL(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingJobState: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	reqBody := fmt.Sprintf(`<trc:GetRecordingJobState>
      <trc:JobToken>%s</trc:JobToken>
    </trc:GetRecordingJobState>`, xmlEscape(jobToken))

	body, err := doRecordingControlSOAP(ctx, controlURL, username, password, reqBody)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingJobState: %w", err)
	}

	var env recordingControlEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("GetRecordingJobState parse: %w", err)
	}
	if env.Body.Fault != nil {
		return nil, fmt.Errorf("GetRecordingJobState SOAP fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.GetRecordingJobStateResponse == nil {
		return nil, fmt.Errorf("GetRecordingJobState: empty response")
	}

	st := env.Body.GetRecordingJobStateResponse.State
	var sources []RecordingJobStateSource
	for _, src := range st.Sources.Items {
		sources = append(sources, RecordingJobStateSource{
			SourceToken: src.SourceToken.Token,
			State:       src.State,
		})
	}

	return &RecordingJobState{
		JobToken:       jobToken,
		RecordingToken: st.RecordingToken,
		State:          st.State,
		Sources:        sources,
	}, nil
}
