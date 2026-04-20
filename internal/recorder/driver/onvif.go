package driver

import (
	"context"
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// OnvifDriver implements Driver using ONVIF protocol calls.
type OnvifDriver struct {
	Endpoint string
	Username string
	Password string
}

func (d *OnvifDriver) Name() string { return "onvif" }

// ── Video Encoder ───────────────────────────────────────────────────────────

func (d *OnvifDriver) GetVideoEncoderConfig(ctx context.Context, token string) (*VideoEncoderConfig, error) {
	cfg, err := onvif.GetVideoEncoderConfig(d.Endpoint, d.Username, d.Password, token)
	if err != nil {
		return nil, err
	}
	return &VideoEncoderConfig{
		Token:            cfg.Token,
		Name:             cfg.Name,
		Encoding:         cfg.Encoding,
		Width:            cfg.Width,
		Height:           cfg.Height,
		Quality:          cfg.Quality,
		FrameRate:        cfg.FrameRate,
		BitrateLimit:     cfg.BitrateLimit,
		EncodingInterval: cfg.EncodingInterval,
		GovLength:        cfg.GovLength,
		H264Profile:      cfg.H264Profile,
	}, nil
}

func (d *OnvifDriver) SetVideoEncoderConfig(ctx context.Context, cfg *VideoEncoderConfig) error {
	return onvif.SetVideoEncoderConfig(d.Endpoint, d.Username, d.Password, &onvif.VideoEncoderConfig{
		Token:            cfg.Token,
		Name:             cfg.Name,
		Encoding:         cfg.Encoding,
		Width:            cfg.Width,
		Height:           cfg.Height,
		Quality:          cfg.Quality,
		FrameRate:        cfg.FrameRate,
		BitrateLimit:     cfg.BitrateLimit,
		EncodingInterval: cfg.EncodingInterval,
		GovLength:        cfg.GovLength,
		H264Profile:      cfg.H264Profile,
	})
}

func (d *OnvifDriver) GetVideoEncoderOptions(ctx context.Context, token string) (*VideoEncoderOptions, error) {
	opts, err := onvif.GetVideoEncoderOpts(d.Endpoint, d.Username, d.Password, token)
	if err != nil {
		return nil, err
	}
	resolutions := make([]Resolution, len(opts.Resolutions))
	for i, r := range opts.Resolutions {
		resolutions[i] = Resolution{Width: r.Width, Height: r.Height}
	}
	return &VideoEncoderOptions{
		Encodings:             opts.Encodings,
		Resolutions:           resolutions,
		FrameRateRange:        Range{Min: opts.FrameRateRange.Min, Max: opts.FrameRateRange.Max},
		QualityRange:          Range{Min: opts.QualityRange.Min, Max: opts.QualityRange.Max},
		BitrateRange:          Range{Min: opts.BitrateRange.Min, Max: opts.BitrateRange.Max},
		GovLengthRange:        Range{Min: opts.GovLengthRange.Min, Max: opts.GovLengthRange.Max},
		H264Profiles:          opts.H264Profiles,
		EncodingIntervalRange: Range{Min: opts.EncodingIntervalRange.Min, Max: opts.EncodingIntervalRange.Max},
	}, nil
}

// ── Imaging ─────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetImagingSettings(ctx context.Context, videoSourceToken string) (*ImagingSettings, error) {
	raw, err := onvif.GetImagingSettings(d.Endpoint, d.Username, d.Password, videoSourceToken)
	if err != nil {
		return nil, err
	}
	b := raw.Brightness
	c := raw.Contrast
	s := raw.Saturation
	sh := raw.Sharpness
	settings := &ImagingSettings{
		Brightness:      &b,
		Contrast:        &c,
		ColorSaturation: &s,
		Sharpness:       &sh,
		IrCutFilter:     raw.IrCutFilter,
	}
	if raw.WideDynamicRange != nil {
		settings.WideDynamicRange = &WDR{
			Mode:  raw.WideDynamicRange.Mode,
			Level: raw.WideDynamicRange.Level,
		}
	}
	return settings, nil
}

func (d *OnvifDriver) SetImagingSettings(ctx context.Context, videoSourceToken string, settings *ImagingSettings) error {
	raw := &onvif.ImagingSettings{
		IrCutFilter: settings.IrCutFilter,
	}
	if settings.Brightness != nil {
		raw.Brightness = *settings.Brightness
	}
	if settings.ColorSaturation != nil {
		raw.Saturation = *settings.ColorSaturation
	}
	if settings.Contrast != nil {
		raw.Contrast = *settings.Contrast
	}
	if settings.Sharpness != nil {
		raw.Sharpness = *settings.Sharpness
	}
	if settings.WideDynamicRange != nil {
		raw.WideDynamicRange = &onvif.WideDynamicRange{
			Mode:  settings.WideDynamicRange.Mode,
			Level: settings.WideDynamicRange.Level,
		}
	}
	return onvif.SetImagingSettings(d.Endpoint, d.Username, d.Password, videoSourceToken, raw)
}

func (d *OnvifDriver) GetImagingOptions(ctx context.Context, videoSourceToken string) (*onvif.ImagingOptions, error) {
	return onvif.GetImagingOptions(d.Endpoint, d.Username, d.Password, videoSourceToken)
}

func (d *OnvifDriver) GetImagingStatus(ctx context.Context, videoSourceToken string) (*onvif.ImagingStatus, error) {
	return onvif.GetImagingStatus(d.Endpoint, d.Username, d.Password, videoSourceToken)
}

func (d *OnvifDriver) GetImagingMoveOptions(ctx context.Context, videoSourceToken string) (*onvif.FocusMoveOptions, error) {
	return onvif.GetImagingMoveOptions(d.Endpoint, d.Username, d.Password, videoSourceToken)
}

func (d *OnvifDriver) MoveFocus(ctx context.Context, videoSourceToken string, req *onvif.FocusMoveRequest) error {
	return onvif.MoveFocus(d.Endpoint, d.Username, d.Password, videoSourceToken, req)
}

func (d *OnvifDriver) StopFocus(ctx context.Context, videoSourceToken string) error {
	return onvif.StopFocus(d.Endpoint, d.Username, d.Password, videoSourceToken)
}

// ── PTZ ─────────────────────────────────────────────────────────────────────

func (d *OnvifDriver) PTZCommand(_ context.Context, profileToken string, cmd *PTZCommand) error {
	ptz, err := onvif.NewPTZController(d.Endpoint, d.Username, d.Password)
	if err != nil {
		return err
	}
	switch cmd.Action {
	case "stop":
		return ptz.Stop(profileToken)
	case "continuous":
		return ptz.ContinuousMove(profileToken, cmd.PanSpeed, cmd.TiltSpeed, cmd.ZoomSpeed)
	case "goto_preset":
		return ptz.GotoPreset(profileToken, cmd.PresetID)
	case "goto_home":
		return ptz.GotoPreset(profileToken, "1") // preset 1 as home
	default:
		return fmt.Errorf("onvif: unknown PTZ action %q", cmd.Action)
	}
}

func (d *OnvifDriver) GetPTZPresets(_ context.Context, profileToken string) ([]PTZPreset, error) {
	ptz, err := onvif.NewPTZController(d.Endpoint, d.Username, d.Password)
	if err != nil {
		return nil, err
	}
	raw, err := ptz.GetPresets(profileToken)
	if err != nil {
		return nil, err
	}
	presets := make([]PTZPreset, len(raw))
	for i, p := range raw {
		presets[i] = PTZPreset{Token: p.Token, Name: p.Name}
	}
	return presets, nil
}

func (d *OnvifDriver) GetPTZStatus(_ context.Context, profileToken string) (*PTZStatus, error) {
	ptz, err := onvif.NewPTZController(d.Endpoint, d.Username, d.Password)
	if err != nil {
		return nil, err
	}
	raw, err := ptz.GetStatus(profileToken)
	if err != nil {
		return nil, err
	}
	return &PTZStatus{
		Pan:  raw.PanPosition,
		Tilt: raw.TiltPosition,
		Zoom: raw.ZoomPosition,
	}, nil
}

// ── Events ──────────────────────────────────────────────────────────────────

func (d *OnvifDriver) SubscribeEvents(ctx context.Context) (*EventStream, error) {
	return nil, fmt.Errorf("onvif: event subscription managed by scheduler")
}

// ── Snapshot ────────────────────────────────────────────────────────────────

func (d *OnvifDriver) CaptureSnapshot(ctx context.Context, channel int) (*Snapshot, error) {
	return nil, fmt.Errorf("onvif: snapshot requires RTSP/snapshot URI from camera model")
}

// ── Media Profiles ──────────────────────────────────────────────────────────

func (d *OnvifDriver) GetProfilesFull(ctx context.Context) ([]*onvif.ProfileInfo, error) {
	return onvif.GetProfilesFull(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) CreateMediaProfile(ctx context.Context, name string) (*onvif.ProfileInfo, error) {
	return onvif.CreateMediaProfile(d.Endpoint, d.Username, d.Password, name)
}

func (d *OnvifDriver) DeleteMediaProfile(ctx context.Context, token string) error {
	return onvif.DeleteMediaProfile(d.Endpoint, d.Username, d.Password, token)
}

func (d *OnvifDriver) GetVideoSourcesList(ctx context.Context) ([]*onvif.VideoSourceInfo, error) {
	return onvif.GetVideoSourcesList(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetVideoSourceConfigs(ctx context.Context) ([]onvif.VideoSourceConfig, error) {
	return onvif.GetVideoSourceConfigs(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetVideoSourceConfig(ctx context.Context, cfg *onvif.VideoSourceConfig) error {
	return onvif.SetVideoSourceConfig(d.Endpoint, d.Username, d.Password, cfg)
}

func (d *OnvifDriver) GetVideoSourceConfigOptions(ctx context.Context, configToken, profileToken string) (*onvif.VideoSourceConfigOptions, error) {
	return onvif.GetVideoSourceConfigOpts(d.Endpoint, d.Username, d.Password, configToken, profileToken)
}

func (d *OnvifDriver) CreateMedia2Profile(ctx context.Context, name string) (*onvif.ProfileInfo, error) {
	return onvif.CreateMedia2Profile(d.Endpoint, d.Username, d.Password, name)
}

func (d *OnvifDriver) DeleteMedia2Profile(ctx context.Context, token string) error {
	return onvif.DeleteMedia2Profile(d.Endpoint, d.Username, d.Password, token)
}

func (d *OnvifDriver) AddMedia2Configuration(ctx context.Context, profileToken, configType, configToken string) error {
	return onvif.AddMedia2Configuration(d.Endpoint, d.Username, d.Password, profileToken, configType, configToken)
}

func (d *OnvifDriver) RemoveMedia2Configuration(ctx context.Context, profileToken, configType, configToken string) error {
	return onvif.RemoveMedia2Configuration(d.Endpoint, d.Username, d.Password, profileToken, configType, configToken)
}

func (d *OnvifDriver) GetStreamUriMulticast(ctx context.Context, profileToken string) (string, error) {
	return onvif.GetStreamUriMulticast(d.Endpoint, d.Username, d.Password, profileToken)
}

// ── Audio ───────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetAudioCapabilities(ctx context.Context) (*onvif.AudioCapabilities, error) {
	return onvif.GetAudioCapabilities(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetAudioSources(ctx context.Context) ([]*onvif.AudioSourceInfo, error) {
	return onvif.GetAudioSources(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetAudioSourceConfigurations(ctx context.Context) ([]*onvif.AudioSourceConfig, error) {
	return onvif.GetAudioSourceConfigurations(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetAudioSourceConfiguration(ctx context.Context, configToken string) (*onvif.AudioSourceConfig, error) {
	return onvif.GetAudioSourceConfiguration(d.Endpoint, d.Username, d.Password, configToken)
}

func (d *OnvifDriver) SetAudioSourceConfiguration(ctx context.Context, cfg *onvif.AudioSourceConfig) error {
	return onvif.SetAudioSourceConfiguration(d.Endpoint, d.Username, d.Password, cfg)
}

func (d *OnvifDriver) GetAudioSourceConfigOptions(ctx context.Context, configToken, profileToken string) (*onvif.AudioSourceConfigOptions, error) {
	return onvif.GetAudioSourceConfigOptions(d.Endpoint, d.Username, d.Password, configToken, profileToken)
}

func (d *OnvifDriver) GetCompatibleAudioSourceConfigs(ctx context.Context, profileToken string) ([]*onvif.AudioSourceConfig, error) {
	return onvif.GetCompatibleAudioSourceConfigs(d.Endpoint, d.Username, d.Password, profileToken)
}

func (d *OnvifDriver) AddAudioSourceToProfile(ctx context.Context, profileToken, configToken string) error {
	return onvif.AddAudioSourceToProfile(d.Endpoint, d.Username, d.Password, profileToken, configToken)
}

func (d *OnvifDriver) RemoveAudioSourceFromProfile(ctx context.Context, profileToken string) error {
	return onvif.RemoveAudioSourceFromProfile(d.Endpoint, d.Username, d.Password, profileToken)
}

func (d *OnvifDriver) GetAudioSourceConfigs(ctx context.Context) ([]onvif.AudioSourceConfig, error) {
	return onvif.GetAudioSourceConfigs(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetAudioSourceConfig(ctx context.Context, cfg *onvif.AudioSourceConfig) error {
	return onvif.SetAudioSourceConfig(d.Endpoint, d.Username, d.Password, cfg)
}

// ── Device Management ───────────────────────────────────────────────────────

func (d *OnvifDriver) ProbeDevice(ctx context.Context) ([]onvif.MediaProfile, error) {
	return onvif.ProbeDevice(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetSystemDateAndTime(ctx context.Context) (*onvif.DateTimeInfo, error) {
	return onvif.GetSystemDateAndTime(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetSystemDateAndTime(ctx context.Context, req *onvif.SetDateTimeRequest) error {
	return onvif.SetSystemDateAndTime(d.Endpoint, d.Username, d.Password, req)
}

func (d *OnvifDriver) GetDeviceHostname(ctx context.Context) (*onvif.HostnameInfo, error) {
	return onvif.GetDeviceHostname(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetDeviceHostname(ctx context.Context, name string) error {
	return onvif.SetDeviceHostname(d.Endpoint, d.Username, d.Password, name)
}

func (d *OnvifDriver) DeviceReboot(ctx context.Context) (string, error) {
	return onvif.DeviceReboot(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetDeviceScopes(ctx context.Context) ([]string, error) {
	return onvif.GetDeviceScopes(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetDeviceScopes(ctx context.Context, scopes []string) error {
	return onvif.SetDeviceScopes(d.Endpoint, d.Username, d.Password, scopes)
}

func (d *OnvifDriver) AddDeviceScopes(ctx context.Context, scopes []string) error {
	return onvif.AddDeviceScopes(d.Endpoint, d.Username, d.Password, scopes)
}

func (d *OnvifDriver) RemoveDeviceScopes(ctx context.Context, scopes []string) ([]string, error) {
	return onvif.RemoveDeviceScopes(d.Endpoint, d.Username, d.Password, scopes)
}

func (d *OnvifDriver) GetDiscoveryMode(ctx context.Context) (*onvif.DiscoveryModeInfo, error) {
	return onvif.GetDiscoveryMode(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetDiscoveryMode(ctx context.Context, mode string) error {
	return onvif.SetDiscoveryMode(d.Endpoint, d.Username, d.Password, mode)
}

func (d *OnvifDriver) GetSystemLog(ctx context.Context, logType string) (*onvif.SystemLogInfo, error) {
	return onvif.GetSystemLog(d.Endpoint, d.Username, d.Password, logType)
}

func (d *OnvifDriver) GetSystemSupportInformation(ctx context.Context) (*onvif.SupportInfo, error) {
	return onvif.GetSystemSupportInformation(d.Endpoint, d.Username, d.Password)
}

// ── Network ─────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetNetworkInterfaces(ctx context.Context) ([]*onvif.NetworkInterfaceInfo, error) {
	return onvif.GetNetworkInterfaces(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetNetworkInterface(ctx context.Context, token string, req *onvif.SetNetworkInterfaceRequest) (bool, error) {
	return onvif.SetNetworkInterface(d.Endpoint, d.Username, d.Password, token, req)
}

func (d *OnvifDriver) GetNetworkProtocols(ctx context.Context) ([]*onvif.NetworkProtocolInfo, error) {
	return onvif.GetNetworkProtocols(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetNetworkProtocols(ctx context.Context, protocols []*onvif.NetworkProtocolInfo) error {
	return onvif.SetNetworkProtocols(d.Endpoint, d.Username, d.Password, protocols)
}

func (d *OnvifDriver) GetNetworkDefaultGateway(ctx context.Context) (*onvif.GatewayInfo, error) {
	return onvif.GetNetworkDefaultGateway(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetNetworkDefaultGateway(ctx context.Context, gw *onvif.GatewayInfo) error {
	return onvif.SetNetworkDefaultGateway(d.Endpoint, d.Username, d.Password, gw)
}

func (d *OnvifDriver) GetDNSConfig(ctx context.Context) (*onvif.DNSInfo, error) {
	return onvif.GetDNSConfig(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetDNSConfig(ctx context.Context, req *onvif.SetDNSRequest) error {
	return onvif.SetDNSConfig(d.Endpoint, d.Username, d.Password, req)
}

func (d *OnvifDriver) GetNTPConfig(ctx context.Context) (*onvif.NTPInfo, error) {
	return onvif.GetNTPConfig(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetNTPConfig(ctx context.Context, req *onvif.SetNTPRequest) error {
	return onvif.SetNTPConfig(d.Endpoint, d.Username, d.Password, req)
}

// ── Users ───────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetDeviceUsers(ctx context.Context) ([]*onvif.DeviceUser, error) {
	return onvif.GetDeviceUsers(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) CreateDeviceUser(ctx context.Context, username, password, role string) error {
	return onvif.CreateDeviceUser(d.Endpoint, d.Username, d.Password, username, password, role)
}

func (d *OnvifDriver) SetDeviceUser(ctx context.Context, username, password, role string) error {
	return onvif.SetDeviceUser(d.Endpoint, d.Username, d.Password, username, password, role)
}

func (d *OnvifDriver) DeleteDeviceUser(ctx context.Context, username string) error {
	return onvif.DeleteDeviceUser(d.Endpoint, d.Username, d.Password, username)
}

// ── Relay ───────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetRelayOutputs(ctx context.Context) ([]onvif.RelayOutput, error) {
	return onvif.GetRelayOutputs(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) SetRelayOutputState(ctx context.Context, token string, active bool) error {
	return onvif.SetRelayOutputState(d.Endpoint, d.Username, d.Password, token, active)
}

// ── Analytics ───────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetRules(ctx context.Context, configToken string) ([]onvif.AnalyticsRule, error) {
	return onvif.GetRules(d.Endpoint, d.Username, d.Password, configToken)
}

func (d *OnvifDriver) CreateRule(ctx context.Context, configToken string, rule onvif.AnalyticsRule) error {
	return onvif.CreateRule(d.Endpoint, d.Username, d.Password, configToken, rule)
}

func (d *OnvifDriver) ModifyRule(ctx context.Context, configToken string, rule onvif.AnalyticsRule) error {
	return onvif.ModifyRule(d.Endpoint, d.Username, d.Password, configToken, rule)
}

func (d *OnvifDriver) DeleteRule(ctx context.Context, configToken, ruleName string) error {
	return onvif.DeleteRule(d.Endpoint, d.Username, d.Password, configToken, ruleName)
}

func (d *OnvifDriver) GetAnalyticsModules(ctx context.Context, configToken string) ([]onvif.AnalyticsModule, error) {
	return onvif.GetAnalyticsModules(d.Endpoint, d.Username, d.Password, configToken)
}

// ── OSD ─────────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetOSDs(ctx context.Context, configToken string) ([]onvif.OSD, error) {
	return onvif.GetOSDs(d.Endpoint, d.Username, d.Password, configToken)
}

func (d *OnvifDriver) GetOSDOptions(ctx context.Context, configToken string) (*onvif.OSDOptions, error) {
	return onvif.GetOSDOptions(d.Endpoint, d.Username, d.Password, configToken)
}

func (d *OnvifDriver) CreateOSD(ctx context.Context, cfg onvif.OSDConfig) (string, error) {
	return onvif.CreateOSD(d.Endpoint, d.Username, d.Password, cfg)
}

func (d *OnvifDriver) SetOSD(ctx context.Context, cfg onvif.OSDConfig) error {
	return onvif.SetOSD(d.Endpoint, d.Username, d.Password, cfg)
}

func (d *OnvifDriver) DeleteOSD(ctx context.Context, osdToken string) error {
	return onvif.DeleteOSD(d.Endpoint, d.Username, d.Password, osdToken)
}

// ── Metadata ────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetMetadataConfigurations(ctx context.Context) ([]*onvif.MetadataConfigInfo, error) {
	return onvif.GetMetadataConfigurations(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) GetMetadataConfiguration(ctx context.Context, configToken string) (*onvif.MetadataConfigInfo, error) {
	return onvif.GetMetadataConfiguration(d.Endpoint, d.Username, d.Password, configToken)
}

func (d *OnvifDriver) SetMetadataConfiguration(ctx context.Context, cfg *onvif.MetadataConfigInfo) error {
	return onvif.SetMetadataConfiguration(d.Endpoint, d.Username, d.Password, cfg)
}

func (d *OnvifDriver) AddMetadataToProfile(ctx context.Context, profileToken, configToken string) error {
	return onvif.AddMetadataToProfile(d.Endpoint, d.Username, d.Password, profileToken, configToken)
}

func (d *OnvifDriver) RemoveMetadataFromProfile(ctx context.Context, profileToken string) error {
	return onvif.RemoveMetadataFromProfile(d.Endpoint, d.Username, d.Password, profileToken)
}

// ── Multicast ───────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetMulticastConfig(ctx context.Context, profileToken string) (*onvif.MulticastConfig, error) {
	return onvif.GetMulticastConfig(d.Endpoint, d.Username, d.Password, profileToken)
}

func (d *OnvifDriver) SetMulticastConfig(ctx context.Context, profileToken string, cfg *onvif.MulticastConfig) error {
	return onvif.SetMulticastConfig(d.Endpoint, d.Username, d.Password, profileToken, cfg)
}

// ── Edge Recording ──────────────────────────────────────────────────────────

func (d *OnvifDriver) GetRecordingConfiguration(ctx context.Context, recordingToken string) (*onvif.RecordingConfiguration, error) {
	return onvif.GetRecordingConfiguration(d.Endpoint, d.Username, d.Password, recordingToken)
}

func (d *OnvifDriver) CreateRecording(ctx context.Context, source onvif.RecordingSource, maxRetention, content string) (string, error) {
	return onvif.CreateRecording(d.Endpoint, d.Username, d.Password, source, maxRetention, content)
}

func (d *OnvifDriver) DeleteRecording(ctx context.Context, recordingToken string) error {
	return onvif.DeleteRecording(d.Endpoint, d.Username, d.Password, recordingToken)
}

func (d *OnvifDriver) CreateRecordingJob(ctx context.Context, recordingToken, mode string, priority int) (*onvif.RecordingJobConfiguration, error) {
	return onvif.CreateRecordingJob(d.Endpoint, d.Username, d.Password, recordingToken, mode, priority)
}

func (d *OnvifDriver) DeleteRecordingJob(ctx context.Context, jobToken string) error {
	return onvif.DeleteRecordingJob(d.Endpoint, d.Username, d.Password, jobToken)
}

func (d *OnvifDriver) GetRecordingJobState(ctx context.Context, jobToken string) (*onvif.RecordingJobState, error) {
	return onvif.GetRecordingJobState(d.Endpoint, d.Username, d.Password, jobToken)
}

func (d *OnvifDriver) CreateTrack(ctx context.Context, recordingToken, trackType, description string) (string, error) {
	return onvif.CreateTrack(d.Endpoint, d.Username, d.Password, recordingToken, trackType, description)
}

func (d *OnvifDriver) DeleteTrack(ctx context.Context, recordingToken, trackToken string) error {
	return onvif.DeleteTrack(d.Endpoint, d.Username, d.Password, recordingToken, trackToken)
}

func (d *OnvifDriver) GetTrackConfiguration(ctx context.Context, recordingToken, trackToken string) (*onvif.TrackConfiguration, error) {
	return onvif.GetTrackConfiguration(d.Endpoint, d.Username, d.Password, recordingToken, trackToken)
}

func (d *OnvifDriver) GetRecordingSummary(ctx context.Context) (*onvif.EdgeRecordingSummary, error) {
	return onvif.GetRecordingSummary(d.Endpoint, d.Username, d.Password)
}

func (d *OnvifDriver) FindRecordings(ctx context.Context) ([]onvif.EdgeRecording, error) {
	return onvif.FindRecordings(d.Endpoint, d.Username, d.Password)
}

// ── Replay ──────────────────────────────────────────────────────────────────

func (d *OnvifDriver) GetReplayUri(ctx context.Context, recordingToken string) (string, error) {
	return onvif.GetReplayUri(d.Endpoint, d.Username, d.Password, recordingToken)
}

func (d *OnvifDriver) BuildReplaySession(ctx context.Context, replayURI, recordingToken string, startTime time.Time, scale float64) (*onvif.ReplaySession, error) {
	return onvif.BuildReplaySession(replayURI, recordingToken, startTime, scale)
}
