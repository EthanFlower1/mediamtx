// Package driver provides a camera-vendor abstraction layer.
//
// Each vendor driver (ONVIF, Amcrest, Hikvision, etc.) implements the full
// Driver interface. The resolver picks exactly one driver per camera based on
// manufacturer. There is no fallback chain — each driver is self-contained.
package driver

import (
	"context"
	"io"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/onvif"
)

// ── Video Encoder ───────────────────────────────────────────────────────────

type VideoEncoderConfig struct {
	Token            string  `json:"token"`
	Name             string  `json:"name"`
	Encoding         string  `json:"encoding"`          // H264, JPEG, H265, MPEG4
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	Quality          float64 `json:"quality"`
	FrameRate        int     `json:"frame_rate"`
	BitrateLimit     int     `json:"bitrate_limit"`
	EncodingInterval int     `json:"encoding_interval"`
	GovLength        int     `json:"gov_length,omitempty"`
	H264Profile      string  `json:"h264_profile,omitempty"`
}

type VideoEncoderOptions struct {
	Encodings             []string     `json:"encodings"`
	Resolutions           []Resolution `json:"resolutions"`
	FrameRateRange        Range        `json:"frame_rate_range"`
	QualityRange          Range        `json:"quality_range"`
	BitrateRange          Range        `json:"bitrate_range,omitempty"`
	GovLengthRange        Range        `json:"gov_length_range,omitempty"`
	H264Profiles          []string     `json:"h264_profiles,omitempty"`
	EncodingIntervalRange Range        `json:"encoding_interval_range,omitempty"`
}

// ── Imaging ─────────────────────────────────────────────────────────────────

type ImagingSettings struct {
	Brightness       *float64 `json:"brightness,omitempty"`
	ColorSaturation  *float64 `json:"color_saturation,omitempty"`
	Contrast         *float64 `json:"contrast,omitempty"`
	Sharpness        *float64 `json:"sharpness,omitempty"`
	IrCutFilter      string   `json:"ir_cut_filter,omitempty"` // AUTO, ON, OFF
	WideDynamicRange *WDR     `json:"wide_dynamic_range,omitempty"`
}

type WDR struct {
	Mode  string  `json:"mode"`  // ON, OFF
	Level float64 `json:"level"`
}

// ── PTZ ─────────────────────────────────────────────────────────────────────

type PTZCommand struct {
	Action    string  `json:"action"`    // continuous, stop, goto_preset, goto_home
	PanSpeed  float64 `json:"pan,omitempty"`
	TiltSpeed float64 `json:"tilt,omitempty"`
	ZoomSpeed float64 `json:"zoom,omitempty"`
	PresetID  string  `json:"preset_id,omitempty"`
}

type PTZPreset struct {
	Token string  `json:"token"`
	Name  string  `json:"name"`
	Pan   float64 `json:"pan,omitempty"`
	Tilt  float64 `json:"tilt,omitempty"`
	Zoom  float64 `json:"zoom,omitempty"`
}

type PTZStatus struct {
	Pan       float64 `json:"pan"`
	Tilt      float64 `json:"tilt"`
	Zoom      float64 `json:"zoom"`
	PanTilt   string  `json:"pan_tilt_status,omitempty"`   // IDLE, MOVING
	ZoomState string  `json:"zoom_status,omitempty"`       // IDLE, MOVING
}

// ── Events ──────────────────────────────────────────────────────────────────

type Event struct {
	Type      string `json:"type"`       // motion, tamper, line_crossing, etc.
	Channel   int    `json:"channel"`
	Active    bool   `json:"active"`
	Timestamp int64  `json:"timestamp_ms"`
}

// EventStream is a channel-based event source. Call Close() to stop.
type EventStream struct {
	Events <-chan Event
	closer io.Closer
}

func (es *EventStream) Close() error {
	if es.closer != nil {
		return es.closer.Close()
	}
	return nil
}

// ── Snapshot ────────────────────────────────────────────────────────────────

type Snapshot struct {
	Data        []byte `json:"-"`
	ContentType string `json:"content_type"`
}

// ── Shared types ────────────────────────────────────────────────────────────

type Resolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// ── Sub-interfaces ──────────────────────────────────────────────────────────

// VideoEncoder provides video encoder configuration operations.
type VideoEncoder interface {
	GetVideoEncoderConfig(ctx context.Context, token string) (*VideoEncoderConfig, error)
	SetVideoEncoderConfig(ctx context.Context, cfg *VideoEncoderConfig) error
	GetVideoEncoderOptions(ctx context.Context, token string) (*VideoEncoderOptions, error)
}

// Imaging provides image settings and focus control.
type Imaging interface {
	GetImagingSettings(ctx context.Context, videoSourceToken string) (*ImagingSettings, error)
	SetImagingSettings(ctx context.Context, videoSourceToken string, settings *ImagingSettings) error
	GetImagingOptions(ctx context.Context, videoSourceToken string) (*onvif.ImagingOptions, error)
	GetImagingStatus(ctx context.Context, videoSourceToken string) (*onvif.ImagingStatus, error)
	GetImagingMoveOptions(ctx context.Context, videoSourceToken string) (*onvif.FocusMoveOptions, error)
	MoveFocus(ctx context.Context, videoSourceToken string, req *onvif.FocusMoveRequest) error
	StopFocus(ctx context.Context, videoSourceToken string) error
}

// PTZ provides pan-tilt-zoom control.
type PTZ interface {
	PTZCommand(ctx context.Context, profileToken string, cmd *PTZCommand) error
	GetPTZPresets(ctx context.Context, profileToken string) ([]PTZPreset, error)
	GetPTZStatus(ctx context.Context, profileToken string) (*PTZStatus, error)
}

// Events provides event subscription.
type Events interface {
	SubscribeEvents(ctx context.Context) (*EventStream, error)
}

// Snapshots provides snapshot capture.
type Snapshots interface {
	CaptureSnapshot(ctx context.Context, channel int) (*Snapshot, error)
}

// MediaProfiles provides media profile and video source management.
type MediaProfiles interface {
	GetProfilesFull(ctx context.Context) ([]*onvif.ProfileInfo, error)
	CreateMediaProfile(ctx context.Context, name string) (*onvif.ProfileInfo, error)
	DeleteMediaProfile(ctx context.Context, token string) error
	GetVideoSourcesList(ctx context.Context) ([]*onvif.VideoSourceInfo, error)
	GetVideoSourceConfigs(ctx context.Context) ([]onvif.VideoSourceConfig, error)
	SetVideoSourceConfig(ctx context.Context, cfg *onvif.VideoSourceConfig) error
	GetVideoSourceConfigOptions(ctx context.Context, configToken, profileToken string) (*onvif.VideoSourceConfigOptions, error)
	CreateMedia2Profile(ctx context.Context, name string) (*onvif.ProfileInfo, error)
	DeleteMedia2Profile(ctx context.Context, token string) error
	AddMedia2Configuration(ctx context.Context, profileToken, configType, configToken string) error
	RemoveMedia2Configuration(ctx context.Context, profileToken, configType, configToken string) error
	GetStreamUriMulticast(ctx context.Context, profileToken string) (string, error)
}

// Audio provides audio source and configuration management.
type Audio interface {
	GetAudioCapabilities(ctx context.Context) (*onvif.AudioCapabilities, error)
	GetAudioSources(ctx context.Context) ([]*onvif.AudioSourceInfo, error)
	GetAudioSourceConfigurations(ctx context.Context) ([]*onvif.AudioSourceConfig, error)
	GetAudioSourceConfiguration(ctx context.Context, configToken string) (*onvif.AudioSourceConfig, error)
	SetAudioSourceConfiguration(ctx context.Context, cfg *onvif.AudioSourceConfig) error
	GetAudioSourceConfigOptions(ctx context.Context, configToken, profileToken string) (*onvif.AudioSourceConfigOptions, error)
	GetCompatibleAudioSourceConfigs(ctx context.Context, profileToken string) ([]*onvif.AudioSourceConfig, error)
	AddAudioSourceToProfile(ctx context.Context, profileToken, configToken string) error
	RemoveAudioSourceFromProfile(ctx context.Context, profileToken string) error
	GetAudioSourceConfigs(ctx context.Context) ([]onvif.AudioSourceConfig, error)
	SetAudioSourceConfig(ctx context.Context, cfg *onvif.AudioSourceConfig) error
}

// DeviceManagement provides device-level management operations.
type DeviceManagement interface {
	ProbeDevice(ctx context.Context) ([]onvif.MediaProfile, error)
	GetSystemDateAndTime(ctx context.Context) (*onvif.DateTimeInfo, error)
	SetSystemDateAndTime(ctx context.Context, req *onvif.SetDateTimeRequest) error
	GetDeviceHostname(ctx context.Context) (*onvif.HostnameInfo, error)
	SetDeviceHostname(ctx context.Context, name string) error
	DeviceReboot(ctx context.Context) (string, error)
	GetDeviceScopes(ctx context.Context) ([]string, error)
	SetDeviceScopes(ctx context.Context, scopes []string) error
	AddDeviceScopes(ctx context.Context, scopes []string) error
	RemoveDeviceScopes(ctx context.Context, scopes []string) ([]string, error)
	GetDiscoveryMode(ctx context.Context) (*onvif.DiscoveryModeInfo, error)
	SetDiscoveryMode(ctx context.Context, mode string) error
	GetSystemLog(ctx context.Context, logType string) (*onvif.SystemLogInfo, error)
	GetSystemSupportInformation(ctx context.Context) (*onvif.SupportInfo, error)
}

// Network provides network configuration operations.
type Network interface {
	GetNetworkInterfaces(ctx context.Context) ([]*onvif.NetworkInterfaceInfo, error)
	SetNetworkInterface(ctx context.Context, token string, req *onvif.SetNetworkInterfaceRequest) (bool, error)
	GetNetworkProtocols(ctx context.Context) ([]*onvif.NetworkProtocolInfo, error)
	SetNetworkProtocols(ctx context.Context, protocols []*onvif.NetworkProtocolInfo) error
	GetNetworkDefaultGateway(ctx context.Context) (*onvif.GatewayInfo, error)
	SetNetworkDefaultGateway(ctx context.Context, gw *onvif.GatewayInfo) error
	GetDNSConfig(ctx context.Context) (*onvif.DNSInfo, error)
	SetDNSConfig(ctx context.Context, req *onvif.SetDNSRequest) error
	GetNTPConfig(ctx context.Context) (*onvif.NTPInfo, error)
	SetNTPConfig(ctx context.Context, req *onvif.SetNTPRequest) error
}

// Users provides user account management.
type Users interface {
	GetDeviceUsers(ctx context.Context) ([]*onvif.DeviceUser, error)
	CreateDeviceUser(ctx context.Context, username, password, role string) error
	SetDeviceUser(ctx context.Context, username, password, role string) error
	DeleteDeviceUser(ctx context.Context, username string) error
}

// Relay provides relay output control.
type Relay interface {
	GetRelayOutputs(ctx context.Context) ([]onvif.RelayOutput, error)
	SetRelayOutputState(ctx context.Context, token string, active bool) error
}

// Analytics provides analytics rule and module management.
type Analytics interface {
	GetRules(ctx context.Context, configToken string) ([]onvif.AnalyticsRule, error)
	CreateRule(ctx context.Context, configToken string, rule onvif.AnalyticsRule) error
	ModifyRule(ctx context.Context, configToken string, rule onvif.AnalyticsRule) error
	DeleteRule(ctx context.Context, configToken, ruleName string) error
	GetAnalyticsModules(ctx context.Context, configToken string) ([]onvif.AnalyticsModule, error)
}

// OSD provides on-screen display overlay management.
type OSD interface {
	GetOSDs(ctx context.Context, configToken string) ([]onvif.OSD, error)
	GetOSDOptions(ctx context.Context, configToken string) (*onvif.OSDOptions, error)
	CreateOSD(ctx context.Context, cfg onvif.OSDConfig) (string, error)
	SetOSD(ctx context.Context, cfg onvif.OSDConfig) error
	DeleteOSD(ctx context.Context, osdToken string) error
}

// Metadata provides metadata configuration management.
type Metadata interface {
	GetMetadataConfigurations(ctx context.Context) ([]*onvif.MetadataConfigInfo, error)
	GetMetadataConfiguration(ctx context.Context, configToken string) (*onvif.MetadataConfigInfo, error)
	SetMetadataConfiguration(ctx context.Context, cfg *onvif.MetadataConfigInfo) error
	AddMetadataToProfile(ctx context.Context, profileToken, configToken string) error
	RemoveMetadataFromProfile(ctx context.Context, profileToken string) error
}

// Multicast provides multicast streaming configuration.
type Multicast interface {
	GetMulticastConfig(ctx context.Context, profileToken string) (*onvif.MulticastConfig, error)
	SetMulticastConfig(ctx context.Context, profileToken string, cfg *onvif.MulticastConfig) error
}

// EdgeRecording provides on-camera (edge) recording management.
type EdgeRecording interface {
	GetRecordingConfiguration(ctx context.Context, recordingToken string) (*onvif.RecordingConfiguration, error)
	CreateRecording(ctx context.Context, source onvif.RecordingSource, maxRetention, content string) (string, error)
	DeleteRecording(ctx context.Context, recordingToken string) error
	CreateRecordingJob(ctx context.Context, recordingToken, mode string, priority int) (*onvif.RecordingJobConfiguration, error)
	DeleteRecordingJob(ctx context.Context, jobToken string) error
	GetRecordingJobState(ctx context.Context, jobToken string) (*onvif.RecordingJobState, error)
	CreateTrack(ctx context.Context, recordingToken, trackType, description string) (string, error)
	DeleteTrack(ctx context.Context, recordingToken, trackToken string) error
	GetTrackConfiguration(ctx context.Context, recordingToken, trackToken string) (*onvif.TrackConfiguration, error)
	GetRecordingSummary(ctx context.Context) (*onvif.EdgeRecordingSummary, error)
	FindRecordings(ctx context.Context) ([]onvif.EdgeRecording, error)
}

// Replay provides RTSP replay/playback from edge recordings.
type Replay interface {
	GetReplayUri(ctx context.Context, recordingToken string) (string, error)
	BuildReplaySession(ctx context.Context, replayURI, recordingToken string, startTime time.Time, scale float64) (*onvif.ReplaySession, error)
}

// ── Driver interface ────────────────────────────────────────────────────────

// Driver is the complete interface for camera operations. Each vendor driver
// implements ALL methods. There is no fallback — the resolver picks one driver
// per camera and that driver handles everything.
type Driver interface {
	// Name returns the driver identifier (e.g., "onvif", "amcrest").
	Name() string

	VideoEncoder
	Imaging
	PTZ
	Events
	Snapshots
	MediaProfiles
	Audio
	DeviceManagement
	Network
	Users
	Relay
	Analytics
	OSD
	Metadata
	Multicast
	EdgeRecording
	Replay
}
