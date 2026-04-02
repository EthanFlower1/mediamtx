package onvif

import (
	"context"
	"fmt"
	"time"

	onvifgo "github.com/EthanFlower1/onvif-go"
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

// TrackConfiguration holds the configuration for a track within a recording.
type TrackConfiguration struct {
	TrackToken  string `json:"track_token"`
	TrackType   string `json:"track_type"`
	Description string `json:"description"`
}

// GetRecordingConfiguration returns the configuration for a specific recording on the device.
func GetRecordingConfiguration(xaddr, username, password, recordingToken string) (*RecordingConfiguration, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rc, err := client.Dev.GetRecordingConfiguration(ctx, recordingToken)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingConfiguration: %w", err)
	}

	return &RecordingConfiguration{
		RecordingToken:       recordingToken,
		Source: RecordingSource{
			SourceID:    rc.Source.SourceId,
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
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("CreateRecording: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	config := &onvifgo.RecordingConfiguration{
		Source: onvifgo.RecordingSourceInformation{
			SourceId:    source.SourceID,
			Name:        source.Name,
			Location:    source.Location,
			Description: source.Description,
			Address:     source.Address,
		},
		MaximumRetentionTime: maxRetention,
		Content:              content,
	}

	token, err := client.Dev.CreateRecording(ctx, config)
	if err != nil {
		return "", fmt.Errorf("CreateRecording: %w", err)
	}

	if token == "" {
		return "", fmt.Errorf("CreateRecording: empty recording token in response")
	}
	return token, nil
}

// DeleteRecording deletes a recording container from the device's edge storage.
func DeleteRecording(xaddr, username, password, recordingToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteRecording: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Dev.DeleteRecording(ctx, recordingToken); err != nil {
		return fmt.Errorf("DeleteRecording: %w", err)
	}

	return nil
}

// CreateRecordingJob creates a recording job that records into the specified recording container.
// Mode should be "Active" (start recording) or "Idle" (create but don't start).
func CreateRecordingJob(
	xaddr, username, password, recordingToken, mode string,
	priority int,
) (*RecordingJobConfiguration, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("CreateRecordingJob: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	config := &onvifgo.RecordingJobConfiguration{
		RecordingToken: recordingToken,
		Mode:           mode,
		Priority:       priority,
	}

	jobToken, actualConfig, err := client.Dev.CreateRecordingJob(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("CreateRecordingJob: %w", err)
	}

	return &RecordingJobConfiguration{
		JobToken:       jobToken,
		RecordingToken: actualConfig.RecordingToken,
		Mode:           actualConfig.Mode,
		Priority:       actualConfig.Priority,
	}, nil
}

// DeleteRecordingJob deletes a recording job from the device.
func DeleteRecordingJob(xaddr, username, password, jobToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteRecordingJob: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Dev.DeleteRecordingJob(ctx, jobToken); err != nil {
		return fmt.Errorf("DeleteRecordingJob: %w", err)
	}

	return nil
}

// GetRecordingJobState returns the current state of a recording job.
func GetRecordingJobState(xaddr, username, password, jobToken string) (*RecordingJobState, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingJobState: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	st, err := client.Dev.GetRecordingJobState(ctx, jobToken)
	if err != nil {
		return nil, fmt.Errorf("GetRecordingJobState: %w", err)
	}

	var sources []RecordingJobStateSource
	for _, src := range st.Sources {
		sourceToken := ""
		if src.SourceToken != nil {
			sourceToken = src.SourceToken.Token
		}
		sources = append(sources, RecordingJobStateSource{
			SourceToken: sourceToken,
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

// GetTrackConfiguration returns the configuration for a specific track within a recording.
func GetTrackConfiguration(xaddr, username, password, recordingToken, trackToken string) (*TrackConfiguration, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, fmt.Errorf("GetTrackConfiguration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tc, err := client.Dev.GetTrackConfiguration(ctx, recordingToken, trackToken)
	if err != nil {
		return nil, fmt.Errorf("GetTrackConfiguration: %w", err)
	}

	return &TrackConfiguration{
		TrackToken:  trackToken,
		TrackType:   tc.TrackType,
		Description: tc.Description,
	}, nil
}

// CreateTrack adds a new track to a recording on the device's edge storage.
// TrackType must be "Video", "Audio", or "Metadata".
func CreateTrack(xaddr, username, password, recordingToken, trackType, description string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("CreateTrack: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	config := &onvifgo.TrackConfiguration{
		TrackType:   trackType,
		Description: description,
	}

	token, err := client.Dev.CreateTrack(ctx, recordingToken, config)
	if err != nil {
		return "", fmt.Errorf("CreateTrack: %w", err)
	}

	if token == "" {
		return "", fmt.Errorf("CreateTrack: empty track token in response")
	}
	return token, nil
}

// DeleteTrack removes a track from a recording on the device's edge storage.
func DeleteTrack(xaddr, username, password, recordingToken, trackToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return fmt.Errorf("DeleteTrack: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Dev.DeleteTrack(ctx, recordingToken, trackToken); err != nil {
		return fmt.Errorf("DeleteTrack: %w", err)
	}

	return nil
}
