package driver

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	amcrest "github.com/EthanFlower1/amcrest-sdk"
	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// AmcrestDriver implements Driver using Amcrest/Dahua's proprietary HTTP CGI API.
type AmcrestDriver struct {
	client *amcrest.Client
	host   string
}

func NewAmcrestDriver(host, username, password string) (*AmcrestDriver, error) {
	client, err := amcrest.NewClient(host, username, password)
	if err != nil {
		return nil, fmt.Errorf("amcrest driver: %w", err)
	}
	return &AmcrestDriver{client: client, host: host}, nil
}

func (d *AmcrestDriver) Name() string { return "amcrest" }

// ── Video Encoder ───────────────────────────────────────────────────────────

func streamIndex(token string) int {
	trimmed := strings.TrimLeft(token, "0")
	if trimmed == "" {
		return 0
	}
	n, _ := strconv.Atoi(trimmed)
	return n
}

func streamFormat(idx int) string {
	if idx == 0 {
		return "MainFormat"
	}
	return "ExtraFormat"
}

func formatIndex(streamIdx int) int {
	if streamIdx <= 1 {
		return 0
	}
	return streamIdx - 1
}

func (d *AmcrestDriver) GetVideoEncoderConfig(ctx context.Context, token string) (*VideoEncoderConfig, error) {
	idx := streamIndex(token)
	raw, err := d.client.Video.GetEncodeConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get encode config: %w", err)
	}

	prefix := fmt.Sprintf("table.Encode[0].%s[%d].Video.", streamFormat(idx), formatIndex(idx))

	return &VideoEncoderConfig{
		Token:        token,
		Name:         kvString(raw, prefix+"Name"),
		Encoding:     amcrestToOnvifEncoding(kvString(raw, prefix+"Compression")),
		Width:        kvInt(raw, prefix+"Width"),
		Height:       kvInt(raw, prefix+"Height"),
		Quality:      kvFloat(raw, prefix+"Quality"),
		FrameRate:    kvInt(raw, prefix+"FPS"),
		BitrateLimit: kvInt(raw, prefix+"BitRate"),
		GovLength:    kvInt(raw, prefix+"GOP"),
		H264Profile:  kvString(raw, prefix+"Profile"),
	}, nil
}

func (d *AmcrestDriver) SetVideoEncoderConfig(ctx context.Context, cfg *VideoEncoderConfig) error {
	idx := streamIndex(cfg.Token)
	prefix := fmt.Sprintf("Encode[0].%s[%d].Video.", streamFormat(idx), formatIndex(idx))

	params := map[string]string{
		prefix + "Compression": onvifToAmcrestEncoding(cfg.Encoding),
		prefix + "FPS":         strconv.Itoa(cfg.FrameRate),
		prefix + "Width":       strconv.Itoa(cfg.Width),
		prefix + "Height":      strconv.Itoa(cfg.Height),
		prefix + "Quality":     strconv.FormatFloat(cfg.Quality, 'f', 0, 64),
	}
	if cfg.BitrateLimit > 0 {
		params[prefix+"BitRate"] = strconv.Itoa(cfg.BitrateLimit)
	}
	if cfg.GovLength > 0 {
		params[prefix+"GOP"] = strconv.Itoa(cfg.GovLength)
	}
	if cfg.H264Profile != "" {
		params[prefix+"Profile"] = cfg.H264Profile
	}

	return d.client.Video.SetEncodeConfig(ctx, params)
}

func (d *AmcrestDriver) GetVideoEncoderOptions(ctx context.Context, token string) (*VideoEncoderOptions, error) {
	idx := streamIndex(token)
	caps, err := d.client.Video.GetEncodeConfigCaps(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("amcrest get encode caps: %w", err)
	}

	prefix := fmt.Sprintf("caps.%s[%d].Video.", streamFormat(idx), formatIndex(idx))
	opts := &VideoEncoderOptions{}

	if v, ok := caps[prefix+"CompressionTypes"]; ok {
		for _, c := range strings.Split(v, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				opts.Encodings = append(opts.Encodings, amcrestToOnvifEncoding(c))
			}
		}
	}
	if v, ok := caps[prefix+"ResolutionTypes"]; ok {
		for _, r := range strings.Split(v, ",") {
			parts := strings.SplitN(strings.TrimSpace(r), "x", 2)
			if len(parts) == 2 {
				w, _ := strconv.Atoi(parts[0])
				h, _ := strconv.Atoi(parts[1])
				if w > 0 && h > 0 {
					opts.Resolutions = append(opts.Resolutions, Resolution{Width: w, Height: h})
				}
			}
		}
	}
	if v, ok := caps[prefix+"MaxFPS"]; ok {
		max, _ := strconv.Atoi(strings.TrimSpace(v))
		opts.FrameRateRange = Range{Min: 1, Max: max}
	}

	return opts, nil
}

// ── Imaging ─────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetImagingSettings(ctx context.Context, _ string) (*ImagingSettings, error) {
	raw, err := d.client.Camera.GetImageConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get image config: %w", err)
	}

	b := kvFloat(raw, "Brightness")
	c := kvFloat(raw, "Contrast")
	s := kvFloat(raw, "Saturation")
	sh := kvFloat(raw, "Sharpness")

	settings := &ImagingSettings{
		Brightness:      &b,
		Contrast:        &c,
		ColorSaturation: &s,
		Sharpness:       &sh,
	}

	dnRaw, dnErr := d.client.Camera.GetDayNightConfig(ctx)
	if dnErr == nil {
		switch strings.ToLower(kvString(dnRaw, "Mode")) {
		case "auto":
			settings.IrCutFilter = "AUTO"
		case "day", "color":
			settings.IrCutFilter = "ON"
		case "night", "blackwhite":
			settings.IrCutFilter = "OFF"
		}
	}

	return settings, nil
}

func (d *AmcrestDriver) SetImagingSettings(ctx context.Context, _ string, settings *ImagingSettings) error {
	params := map[string]string{}
	if settings.Brightness != nil {
		params["Brightness"] = strconv.FormatFloat(*settings.Brightness, 'f', 0, 64)
	}
	if settings.Contrast != nil {
		params["Contrast"] = strconv.FormatFloat(*settings.Contrast, 'f', 0, 64)
	}
	if settings.ColorSaturation != nil {
		params["Saturation"] = strconv.FormatFloat(*settings.ColorSaturation, 'f', 0, 64)
	}
	if settings.Sharpness != nil {
		params["Sharpness"] = strconv.FormatFloat(*settings.Sharpness, 'f', 0, 64)
	}
	if len(params) > 0 {
		return d.client.Camera.SetImageConfig(ctx, params)
	}
	return nil
}

func (d *AmcrestDriver) GetImagingOptions(ctx context.Context, _ string) (*onvif.ImagingOptions, error) {
	raw, err := d.client.Camera.GetVideoInOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get video in options: %w", err)
	}

	opts := &onvif.ImagingOptions{}

	// Amcrest VideoInOptions reports ranges like BrightnessRange, ContrastRange, etc.
	if v, ok := raw["BrightnessRange"]; ok {
		opts.Brightness = parseAmcrestRange(v)
	}
	if v, ok := raw["ContrastRange"]; ok {
		opts.Contrast = parseAmcrestRange(v)
	}
	if v, ok := raw["SaturationRange"]; ok {
		opts.Saturation = parseAmcrestRange(v)
	}
	if v, ok := raw["SharpnessRange"]; ok {
		opts.Sharpness = parseAmcrestRange(v)
	}

	opts.IrCutFilterModes = []string{"AUTO", "ON", "OFF"}

	return opts, nil
}

func (d *AmcrestDriver) GetImagingStatus(ctx context.Context, _ string) (*onvif.ImagingStatus, error) {
	raw, err := d.client.Camera.GetFocusStatus(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("amcrest get focus status: %w", err)
	}

	status := &onvif.ImagingStatus{}
	posStr := kvString(raw, "Status")
	if posStr != "" {
		pos, _ := strconv.ParseFloat(posStr, 64)
		moveStatus := "IDLE"
		if strings.EqualFold(kvString(raw, "MoveStatus"), "MOVING") {
			moveStatus = "MOVING"
		}
		status.FocusStatus = &onvif.FocusStatus{
			Position:   pos,
			MoveStatus: moveStatus,
		}
	}

	return status, nil
}

func (d *AmcrestDriver) GetImagingMoveOptions(_ context.Context, _ string) (*onvif.FocusMoveOptions, error) {
	// Amcrest cameras support continuous focus via AdjustFocusContinuously.
	return &onvif.FocusMoveOptions{
		Continuous: &onvif.ContinuousFocusMoveOptions{
			Speed: onvif.FloatRange{Min: -1, Max: 1},
		},
	}, nil
}

func (d *AmcrestDriver) MoveFocus(ctx context.Context, _ string, req *onvif.FocusMoveRequest) error {
	if req.Continuous != nil {
		// Map continuous speed to Amcrest focus adjustment.
		// Positive speed = focus far, negative = focus near.
		focus := int(req.Continuous.Speed * 8)
		return d.client.Camera.AdjustFocusContinuously(ctx, 0, focus, 0)
	}
	if req.Absolute != nil {
		focus := int(req.Absolute.Position)
		return d.client.Camera.AdjustFocus(ctx, 0, focus, 0)
	}
	if req.Relative != nil {
		focus := int(req.Relative.Distance * 8)
		return d.client.Camera.AdjustFocusContinuously(ctx, 0, focus, 0)
	}
	return fmt.Errorf("amcrest: no focus move parameters specified")
}

func (d *AmcrestDriver) StopFocus(ctx context.Context, _ string) error {
	// Stop by sending zero speed.
	return d.client.Camera.AdjustFocusContinuously(ctx, 0, 0, 0)
}

// ── PTZ ─────────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) PTZCommand(ctx context.Context, _ string, cmd *PTZCommand) error {
	switch cmd.Action {
	case "stop":
		return d.client.PTZ.Stop(ctx, 0, "All")
	case "continuous":
		// Map pan/tilt/zoom speeds to Amcrest PTZ control codes.
		if cmd.ZoomSpeed > 0 {
			return d.client.PTZ.Control(ctx, 0, "ZoomTele", int(cmd.ZoomSpeed*8), 0, 0)
		} else if cmd.ZoomSpeed < 0 {
			return d.client.PTZ.Control(ctx, 0, "ZoomWide", int(-cmd.ZoomSpeed*8), 0, 0)
		}
		// Combined pan/tilt.
		code := ptzDirectionCode(cmd.PanSpeed, cmd.TiltSpeed)
		speed := int(max(abs(cmd.PanSpeed), abs(cmd.TiltSpeed)) * 8)
		if speed < 1 {
			speed = 1
		}
		return d.client.PTZ.Control(ctx, 0, code, speed, 0, 0)
	case "goto_preset":
		idx, _ := strconv.Atoi(cmd.PresetID)
		return d.client.PTZ.GotoPreset(ctx, 0, idx)
	case "goto_home":
		return d.client.PTZ.GotoPreset(ctx, 0, 0)
	default:
		return fmt.Errorf("amcrest: unknown PTZ action %q", cmd.Action)
	}
}

func ptzDirectionCode(pan, tilt float64) string {
	switch {
	case pan > 0 && tilt > 0:
		return "RightUp"
	case pan > 0 && tilt < 0:
		return "RightDown"
	case pan > 0:
		return "Right"
	case pan < 0 && tilt > 0:
		return "LeftUp"
	case pan < 0 && tilt < 0:
		return "LeftDown"
	case pan < 0:
		return "Left"
	case tilt > 0:
		return "Up"
	case tilt < 0:
		return "Down"
	default:
		return "Up" // fallback
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

func (d *AmcrestDriver) GetPTZPresets(ctx context.Context, _ string) ([]PTZPreset, error) {
	raw, err := d.client.PTZ.GetPresets(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("amcrest get presets: %w", err)
	}
	// Amcrest returns preset list as a raw string. Parse it.
	var presets []PTZPreset
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "presets[") {
			// e.g., presets[0].Name=Preset1
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.HasSuffix(parts[0], ".Name") {
				idx := strings.TrimSuffix(strings.TrimPrefix(parts[0], "presets["), "].Name")
				presets = append(presets, PTZPreset{
					Token: idx,
					Name:  parts[1],
				})
			}
		}
	}
	return presets, nil
}

func (d *AmcrestDriver) GetPTZStatus(ctx context.Context, _ string) (*PTZStatus, error) {
	raw, err := d.client.PTZ.GetStatus(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("amcrest get ptz status: %w", err)
	}
	return &PTZStatus{
		Pan:  kvFloat(raw, "status.Postion[0]"),
		Tilt: kvFloat(raw, "status.Postion[1]"),
		Zoom: kvFloat(raw, "status.Postion[2]"),
	}, nil
}

// ── Events ──────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) SubscribeEvents(ctx context.Context) (*EventStream, error) {
	evtChan := make(chan Event, 64)

	amcrestEvtChan, stream, err := d.client.Event.Subscribe(ctx, []string{
		"VideoMotion", "VideoBlind", "VideoLoss",
		"CrossLineDetection", "CrossRegionDetection",
	}, 60)
	if err != nil {
		close(evtChan)
		return nil, fmt.Errorf("amcrest subscribe events: %w", err)
	}

	go func() {
		defer close(evtChan)
		for evt := range amcrestEvtChan {
			if evt.Code == "Heartbeat" {
				continue
			}
			evtChan <- Event{
				Type:      amcrestToEventType(evt.Code),
				Channel:   evt.Index,
				Active:    evt.Action == "Start",
				Timestamp: time.Now().UnixMilli(),
			}
		}
	}()

	return &EventStream{
		Events: evtChan,
		closer: stream,
	}, nil
}

// ── Snapshot ────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) CaptureSnapshot(ctx context.Context, channel int) (*Snapshot, error) {
	data, err := d.client.Snapshot.Get(ctx, channel)
	if err != nil {
		return nil, fmt.Errorf("amcrest capture snapshot: %w", err)
	}
	return &Snapshot{
		Data:        data,
		ContentType: "image/jpeg",
	}, nil
}

// ── Media Profiles ──────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetProfilesFull(ctx context.Context) ([]*onvif.ProfileInfo, error) {
	// Amcrest cameras expose streams as Main (0) + Extra (1..N).
	// We synthesize profile info from the encode configuration.
	raw, err := d.client.Video.GetEncodeConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get encode config for profiles: %w", err)
	}

	var profiles []*onvif.ProfileInfo

	// Main stream
	mainPrefix := "table.Encode[0].MainFormat[0].Video."
	profiles = append(profiles, &onvif.ProfileInfo{
		Token: "000",
		Name:  "Main Stream",
		VideoEncoder: &onvif.VideoEncoderConfig{
			Token:    "000",
			Name:     kvString(raw, mainPrefix+"Name"),
			Encoding: amcrestToOnvifEncoding(kvString(raw, mainPrefix+"Compression")),
			Width:    kvInt(raw, mainPrefix+"Width"),
			Height:   kvInt(raw, mainPrefix+"Height"),
		},
	})

	// Extra stream(s)
	extraPrefix := "table.Encode[0].ExtraFormat[0].Video."
	if kvString(raw, extraPrefix+"Compression") != "" {
		profiles = append(profiles, &onvif.ProfileInfo{
			Token: "001",
			Name:  "Sub Stream",
			VideoEncoder: &onvif.VideoEncoderConfig{
				Token:    "001",
				Name:     kvString(raw, extraPrefix+"Name"),
				Encoding: amcrestToOnvifEncoding(kvString(raw, extraPrefix+"Compression")),
				Width:    kvInt(raw, extraPrefix+"Width"),
				Height:   kvInt(raw, extraPrefix+"Height"),
			},
		})
	}

	return profiles, nil
}

func (d *AmcrestDriver) CreateMediaProfile(_ context.Context, _ string) (*onvif.ProfileInfo, error) {
	return nil, fmt.Errorf("amcrest: CreateMediaProfile not supported — Amcrest cameras have fixed stream profiles")
}

func (d *AmcrestDriver) DeleteMediaProfile(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: DeleteMediaProfile not supported — Amcrest cameras have fixed stream profiles")
}

func (d *AmcrestDriver) GetVideoSourcesList(ctx context.Context) ([]*onvif.VideoSourceInfo, error) {
	caps, err := d.client.Video.GetVideoInputCaps(ctx, 0)
	if err != nil {
		return nil, fmt.Errorf("amcrest get video input caps: %w", err)
	}

	width := kvInt(caps, "MaxWidth")
	height := kvInt(caps, "MaxHeight")
	if width == 0 {
		width = 1920
	}
	if height == 0 {
		height = 1080
	}

	return []*onvif.VideoSourceInfo{
		{
			Token:  "000",
			Width:  width,
			Height: height,
		},
	}, nil
}

func (d *AmcrestDriver) GetVideoSourceConfigs(_ context.Context) ([]onvif.VideoSourceConfig, error) {
	// Amcrest has a single fixed video source configuration.
	return []onvif.VideoSourceConfig{
		{
			Token:       "vsconf_000",
			Name:        "Video Source Config 0",
			SourceToken: "000",
		},
	}, nil
}

func (d *AmcrestDriver) SetVideoSourceConfig(_ context.Context, _ *onvif.VideoSourceConfig) error {
	return fmt.Errorf("amcrest: SetVideoSourceConfig not supported — Amcrest video source config is fixed")
}

func (d *AmcrestDriver) GetVideoSourceConfigOptions(_ context.Context, _, _ string) (*onvif.VideoSourceConfigOptions, error) {
	return &onvif.VideoSourceConfigOptions{
		MaximumNumberOfProfiles: 2,
	}, nil
}

func (d *AmcrestDriver) CreateMedia2Profile(_ context.Context, _ string) (*onvif.ProfileInfo, error) {
	return nil, fmt.Errorf("amcrest: Media2 profiles not supported by Amcrest API")
}

func (d *AmcrestDriver) DeleteMedia2Profile(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: Media2 profiles not supported by Amcrest API")
}

func (d *AmcrestDriver) AddMedia2Configuration(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("amcrest: Media2 configuration not supported by Amcrest API")
}

func (d *AmcrestDriver) RemoveMedia2Configuration(_ context.Context, _, _, _ string) error {
	return fmt.Errorf("amcrest: Media2 configuration not supported by Amcrest API")
}

func (d *AmcrestDriver) GetStreamUriMulticast(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("amcrest: multicast streaming not supported by Amcrest CGI API")
}

// ── Audio ───────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetAudioCapabilities(ctx context.Context) (*onvif.AudioCapabilities, error) {
	caps := &onvif.AudioCapabilities{}

	inputChannels, err := d.client.Audio.GetInputChannels(ctx)
	if err == nil && inputChannels > 0 {
		caps.AudioSources = inputChannels
	}

	outputChannels, err := d.client.Audio.GetOutputChannels(ctx)
	if err == nil && outputChannels > 0 {
		caps.AudioOutputs = outputChannels
		caps.HasBackchannel = true
	}

	return caps, nil
}

func (d *AmcrestDriver) GetAudioSources(ctx context.Context) ([]*onvif.AudioSourceInfo, error) {
	inputChannels, err := d.client.Audio.GetInputChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get audio input channels: %w", err)
	}

	var sources []*onvif.AudioSourceInfo
	for i := 0; i < inputChannels; i++ {
		sources = append(sources, &onvif.AudioSourceInfo{
			Token:    fmt.Sprintf("audio_%d", i),
			Channels: 1,
		})
	}
	return sources, nil
}

func (d *AmcrestDriver) GetAudioSourceConfigurations(ctx context.Context) ([]*onvif.AudioSourceConfig, error) {
	inputChannels, err := d.client.Audio.GetInputChannels(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get audio input channels: %w", err)
	}

	var configs []*onvif.AudioSourceConfig
	for i := 0; i < inputChannels; i++ {
		configs = append(configs, &onvif.AudioSourceConfig{
			Token:       fmt.Sprintf("asc_%d", i),
			Name:        fmt.Sprintf("Audio Source %d", i),
			SourceToken: fmt.Sprintf("audio_%d", i),
		})
	}
	return configs, nil
}

func (d *AmcrestDriver) GetAudioSourceConfiguration(_ context.Context, configToken string) (*onvif.AudioSourceConfig, error) {
	// Parse channel index from token pattern "asc_N".
	idx := 0
	if strings.HasPrefix(configToken, "asc_") {
		idx, _ = strconv.Atoi(strings.TrimPrefix(configToken, "asc_"))
	}
	return &onvif.AudioSourceConfig{
		Token:       configToken,
		Name:        fmt.Sprintf("Audio Source %d", idx),
		SourceToken: fmt.Sprintf("audio_%d", idx),
	}, nil
}

func (d *AmcrestDriver) SetAudioSourceConfiguration(_ context.Context, _ *onvif.AudioSourceConfig) error {
	return fmt.Errorf("amcrest: SetAudioSourceConfiguration not supported — Amcrest audio source config is fixed")
}

func (d *AmcrestDriver) GetAudioSourceConfigOptions(_ context.Context, _, _ string) (*onvif.AudioSourceConfigOptions, error) {
	return &onvif.AudioSourceConfigOptions{
		InputTokensAvailable: []string{"audio_0"},
	}, nil
}

func (d *AmcrestDriver) GetCompatibleAudioSourceConfigs(ctx context.Context, _ string) ([]*onvif.AudioSourceConfig, error) {
	return d.GetAudioSourceConfigurations(ctx)
}

func (d *AmcrestDriver) AddAudioSourceToProfile(_ context.Context, _, _ string) error {
	return fmt.Errorf("amcrest: AddAudioSourceToProfile not supported — Amcrest profiles have fixed audio bindings")
}

func (d *AmcrestDriver) RemoveAudioSourceFromProfile(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: RemoveAudioSourceFromProfile not supported — Amcrest profiles have fixed audio bindings")
}

func (d *AmcrestDriver) GetAudioSourceConfigs(ctx context.Context) ([]onvif.AudioSourceConfig, error) {
	ptrs, err := d.GetAudioSourceConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	var configs []onvif.AudioSourceConfig
	for _, p := range ptrs {
		configs = append(configs, *p)
	}
	return configs, nil
}

func (d *AmcrestDriver) SetAudioSourceConfig(_ context.Context, _ *onvif.AudioSourceConfig) error {
	return fmt.Errorf("amcrest: SetAudioSourceConfig not supported — Amcrest audio source config is fixed")
}

// ── Device Management ───────────────────────────────────────────────────────

func (d *AmcrestDriver) ProbeDevice(ctx context.Context) ([]onvif.MediaProfile, error) {
	// Probe by reading encode config to discover available streams.
	raw, err := d.client.Video.GetEncodeConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest probe device: %w", err)
	}

	var profiles []onvif.MediaProfile

	mainPrefix := "table.Encode[0].MainFormat[0].Video."
	mainEnc := kvString(raw, mainPrefix+"Compression")
	if mainEnc != "" {
		profiles = append(profiles, onvif.MediaProfile{
			Token:      "000",
			Name:       "Main Stream",
			VideoCodec: amcrestToOnvifEncoding(mainEnc),
			Width:      kvInt(raw, mainPrefix+"Width"),
			Height:     kvInt(raw, mainPrefix+"Height"),
		})
	}

	extraPrefix := "table.Encode[0].ExtraFormat[0].Video."
	extraEnc := kvString(raw, extraPrefix+"Compression")
	if extraEnc != "" {
		profiles = append(profiles, onvif.MediaProfile{
			Token:      "001",
			Name:       "Sub Stream",
			VideoCodec: amcrestToOnvifEncoding(extraEnc),
			Width:      kvInt(raw, extraPrefix+"Width"),
			Height:     kvInt(raw, extraPrefix+"Height"),
		})
	}

	return profiles, nil
}

func (d *AmcrestDriver) GetSystemDateAndTime(ctx context.Context) (*onvif.DateTimeInfo, error) {
	timeStr, err := d.client.System.GetCurrentTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get current time: %w", err)
	}

	info := &onvif.DateTimeInfo{
		Type:      "Manual",
		LocalTime: strings.TrimSpace(timeStr),
	}

	// Check NTP config to determine if time is synced via NTP.
	ntpCfg, ntpErr := d.client.Network.GetNTPConfig(ctx)
	if ntpErr == nil {
		if strings.EqualFold(kvString(ntpCfg, "Enable"), "true") {
			info.Type = "NTP"
		}
	}

	return info, nil
}

func (d *AmcrestDriver) SetSystemDateAndTime(ctx context.Context, req *onvif.SetDateTimeRequest) error {
	if req.Type == "NTP" {
		// Enable NTP rather than setting manual time.
		return d.client.Network.SetNTPConfig(ctx, map[string]string{
			"NTP.Enable": "true",
		})
	}

	if req.UTCDateTime != nil {
		timeStr := fmt.Sprintf("%04d-%d-%d %02d:%02d:%02d",
			req.UTCDateTime.Year, req.UTCDateTime.Month, req.UTCDateTime.Day,
			req.UTCDateTime.Hour, req.UTCDateTime.Minute, req.UTCDateTime.Second,
		)
		return d.client.System.SetCurrentTime(ctx, timeStr)
	}

	return fmt.Errorf("amcrest: SetSystemDateAndTime requires UTCDateTime or NTP type")
}

func (d *AmcrestDriver) GetDeviceHostname(ctx context.Context) (*onvif.HostnameInfo, error) {
	name, err := d.client.System.GetMachineName(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get hostname: %w", err)
	}
	return &onvif.HostnameInfo{
		Name: name,
	}, nil
}

func (d *AmcrestDriver) SetDeviceHostname(ctx context.Context, name string) error {
	return d.client.System.SetGeneralConfig(ctx, map[string]string{
		"General.MachineName": name,
	})
}

func (d *AmcrestDriver) DeviceReboot(ctx context.Context) (string, error) {
	if err := d.client.System.Reboot(ctx); err != nil {
		return "", fmt.Errorf("amcrest reboot: %w", err)
	}
	return "Rebooting", nil
}

func (d *AmcrestDriver) GetDeviceScopes(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("amcrest: WS-Discovery scopes not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) SetDeviceScopes(_ context.Context, _ []string) error {
	return fmt.Errorf("amcrest: WS-Discovery scopes not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) AddDeviceScopes(_ context.Context, _ []string) error {
	return fmt.Errorf("amcrest: WS-Discovery scopes not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) RemoveDeviceScopes(_ context.Context, _ []string) ([]string, error) {
	return nil, fmt.Errorf("amcrest: WS-Discovery scopes not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) GetDiscoveryMode(_ context.Context) (*onvif.DiscoveryModeInfo, error) {
	return nil, fmt.Errorf("amcrest: WS-Discovery mode not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) SetDiscoveryMode(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: WS-Discovery mode not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) GetSystemLog(ctx context.Context, _ string) (*onvif.SystemLogInfo, error) {
	raw, err := d.client.System.GetSystemInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get system log: %w", err)
	}

	// Flatten the system info map into a readable log string.
	var sb strings.Builder
	for k, v := range raw {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(v)
		sb.WriteString("\n")
	}

	return &onvif.SystemLogInfo{
		Content: sb.String(),
	}, nil
}

func (d *AmcrestDriver) GetSystemSupportInformation(ctx context.Context) (*onvif.SupportInfo, error) {
	deviceType, _ := d.client.System.GetDeviceType(ctx)
	hwVer, _ := d.client.System.GetHardwareVersion(ctx)
	swVer, _ := d.client.System.GetSoftwareVersion(ctx)
	serial, _ := d.client.System.GetSerialNumber(ctx)
	vendor, _ := d.client.System.GetVendor(ctx)

	content := fmt.Sprintf(
		"Vendor: %s\nDevice Type: %s\nHardware Version: %s\nSoftware Version: %s\nSerial Number: %s",
		vendor, deviceType, hwVer, swVer, serial,
	)

	return &onvif.SupportInfo{
		Content: content,
	}, nil
}

// ── Network ─────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetNetworkInterfaces(ctx context.Context) ([]*onvif.NetworkInterfaceInfo, error) {
	raw, err := d.client.Network.GetNetworkConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get network config: %w", err)
	}

	// Parse network config. Keys like "table.Network.eth0.IPAddress", "table.Network.eth0.SubnetMask", etc.
	ifaceMap := make(map[string]*onvif.NetworkInterfaceInfo)
	for k, v := range raw {
		// Expected: table.Network.<iface>.<field>
		parts := strings.SplitN(k, ".", 4)
		if len(parts) < 4 || parts[1] != "Network" {
			continue
		}
		ifaceName := parts[2]
		field := parts[3]

		iface, ok := ifaceMap[ifaceName]
		if !ok {
			iface = &onvif.NetworkInterfaceInfo{
				Token:   ifaceName,
				Enabled: true,
				IPv4:    &onvif.IPv4Config{Enabled: true},
			}
			ifaceMap[ifaceName] = iface
		}

		switch field {
		case "IPAddress":
			iface.IPv4.Address = v
		case "SubnetMask":
			iface.IPv4.Prefix = subnetToCIDR(v)
		case "DhcpEnable":
			iface.IPv4.DHCP = strings.EqualFold(v, "true")
		case "PhysicalAddress":
			iface.MAC = v
		}
	}

	var result []*onvif.NetworkInterfaceInfo
	for _, iface := range ifaceMap {
		result = append(result, iface)
	}

	if len(result) == 0 {
		// Fallback: return a synthetic interface from interfaces listing.
		result = append(result, &onvif.NetworkInterfaceInfo{
			Token:   "eth0",
			Enabled: true,
		})
	}

	return result, nil
}

func (d *AmcrestDriver) SetNetworkInterface(ctx context.Context, token string, req *onvif.SetNetworkInterfaceRequest) (bool, error) {
	params := map[string]string{}
	prefix := fmt.Sprintf("Network.%s.", token)

	if req.IPv4 != nil {
		if req.IPv4.Address != "" {
			params[prefix+"IPAddress"] = req.IPv4.Address
		}
		params[prefix+"DhcpEnable"] = strconv.FormatBool(req.IPv4.DHCP)
	}

	if len(params) == 0 {
		return false, nil
	}

	if err := d.client.Network.SetNetworkConfig(ctx, params); err != nil {
		return false, fmt.Errorf("amcrest set network interface: %w", err)
	}

	// Amcrest typically requires reboot after network changes.
	return true, nil
}

func (d *AmcrestDriver) GetNetworkProtocols(ctx context.Context) ([]*onvif.NetworkProtocolInfo, error) {
	// Query individual protocol configs from Amcrest.
	var protocols []*onvif.NetworkProtocolInfo

	// RTSP
	rtspCfg, err := d.client.Network.GetRTSPConfig(ctx)
	if err == nil {
		port, _ := strconv.Atoi(kvString(rtspCfg, "Port"))
		if port == 0 {
			port = 554
		}
		protocols = append(protocols, &onvif.NetworkProtocolInfo{
			Name:    "RTSP",
			Enabled: true,
			Port:    port,
		})
	}

	// HTTP (from the host)
	httpPort := 80
	if _, portStr, err := splitHostPort(d.host); err == nil && portStr != "" {
		httpPort, _ = strconv.Atoi(portStr)
	}
	protocols = append(protocols, &onvif.NetworkProtocolInfo{
		Name:    "HTTP",
		Enabled: true,
		Port:    httpPort,
	})

	return protocols, nil
}

func (d *AmcrestDriver) SetNetworkProtocols(ctx context.Context, protocols []*onvif.NetworkProtocolInfo) error {
	for _, p := range protocols {
		switch strings.ToUpper(p.Name) {
		case "RTSP":
			params := map[string]string{
				"RTSP.Port": strconv.Itoa(p.Port),
			}
			if err := d.client.Network.SetNTPConfig(ctx, params); err != nil {
				return fmt.Errorf("amcrest set RTSP port: %w", err)
			}
		}
	}
	return nil
}

func (d *AmcrestDriver) GetNetworkDefaultGateway(ctx context.Context) (*onvif.GatewayInfo, error) {
	raw, err := d.client.Network.GetNetworkConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get network gateway: %w", err)
	}

	info := &onvif.GatewayInfo{
		IPv4: []string{},
		IPv6: []string{},
	}

	for k, v := range raw {
		if strings.HasSuffix(k, ".DefaultGateway") && v != "" {
			info.IPv4 = append(info.IPv4, v)
		}
	}

	return info, nil
}

func (d *AmcrestDriver) SetNetworkDefaultGateway(ctx context.Context, gw *onvif.GatewayInfo) error {
	if len(gw.IPv4) == 0 {
		return fmt.Errorf("amcrest: no IPv4 gateway provided")
	}
	params := map[string]string{
		"Network.eth0.DefaultGateway": gw.IPv4[0],
	}
	return d.client.Network.SetNetworkConfig(ctx, params)
}

func (d *AmcrestDriver) GetDNSConfig(ctx context.Context) (*onvif.DNSInfo, error) {
	raw, err := d.client.Network.GetNetworkConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get DNS config: %w", err)
	}

	info := &onvif.DNSInfo{}
	for k, v := range raw {
		if strings.HasSuffix(k, ".DnsServer1") && v != "" {
			info.Servers = append(info.Servers, v)
		}
		if strings.HasSuffix(k, ".DnsServer2") && v != "" {
			info.Servers = append(info.Servers, v)
		}
	}

	return info, nil
}

func (d *AmcrestDriver) SetDNSConfig(ctx context.Context, req *onvif.SetDNSRequest) error {
	params := map[string]string{}
	if len(req.Servers) > 0 {
		params["Network.eth0.DnsServer1"] = req.Servers[0]
	}
	if len(req.Servers) > 1 {
		params["Network.eth0.DnsServer2"] = req.Servers[1]
	}
	return d.client.Network.SetNetworkConfig(ctx, params)
}

func (d *AmcrestDriver) GetNTPConfig(ctx context.Context) (*onvif.NTPInfo, error) {
	raw, err := d.client.Network.GetNTPConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get NTP config: %w", err)
	}

	info := &onvif.NTPInfo{
		FromDHCP: strings.EqualFold(kvString(raw, "Enable"), "false"),
	}

	// The Amcrest NTP config uses "Address" for the server.
	server := kvString(raw, "Address")
	if server != "" {
		info.Servers = append(info.Servers, server)
	}

	return info, nil
}

func (d *AmcrestDriver) SetNTPConfig(ctx context.Context, req *onvif.SetNTPRequest) error {
	params := map[string]string{
		"NTP.Enable": "true",
	}
	if len(req.Servers) > 0 {
		params["NTP.Address"] = req.Servers[0]
	}
	return d.client.Network.SetNTPConfig(ctx, params)
}

// ── Users ───────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetDeviceUsers(ctx context.Context) ([]*onvif.DeviceUser, error) {
	raw, err := d.client.User.GetAllUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get users: %w", err)
	}

	// Parse indexed response: users[0].Name=admin, users[0].Group=admin, etc.
	userMap := make(map[string]*onvif.DeviceUser)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], parts[1]
		// Extract index: users[N].Field
		if !strings.HasPrefix(key, "users[") {
			continue
		}
		bracketEnd := strings.Index(key, "]")
		if bracketEnd < 0 {
			continue
		}
		idx := key[6:bracketEnd]
		field := ""
		if len(key) > bracketEnd+2 {
			field = key[bracketEnd+2:]
		}

		user, ok := userMap[idx]
		if !ok {
			user = &onvif.DeviceUser{}
			userMap[idx] = user
		}

		switch field {
		case "Name":
			user.Username = val
		case "Group":
			user.Role = amcrestGroupToRole(val)
		}
	}

	var users []*onvif.DeviceUser
	for _, u := range userMap {
		if u.Username != "" {
			users = append(users, u)
		}
	}
	return users, nil
}

func (d *AmcrestDriver) CreateDeviceUser(ctx context.Context, username, password, role string) error {
	group := roleToAmcrestGroup(role)
	return d.client.User.AddUser(ctx, username, password, group)
}

func (d *AmcrestDriver) SetDeviceUser(ctx context.Context, username, password, role string) error {
	params := map[string]string{
		"user.Group": roleToAmcrestGroup(role),
	}
	if err := d.client.User.ModifyUser(ctx, username, params); err != nil {
		return fmt.Errorf("amcrest modify user: %w", err)
	}
	// Amcrest requires separate call to change password (needs old password).
	// Since we don't have old password here, we skip password change if it fails.
	// In practice the API consumer should use the specific password change endpoint.
	return nil
}

func (d *AmcrestDriver) DeleteDeviceUser(ctx context.Context, username string) error {
	return d.client.User.DeleteUser(ctx, username)
}

// ── Relay ───────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetRelayOutputs(_ context.Context) ([]onvif.RelayOutput, error) {
	// Amcrest cameras typically don't expose relay outputs via the CGI API.
	return nil, fmt.Errorf("amcrest: relay outputs not available via Amcrest CGI API")
}

func (d *AmcrestDriver) SetRelayOutputState(_ context.Context, _ string, _ bool) error {
	return fmt.Errorf("amcrest: relay output control not available via Amcrest CGI API")
}

// ── Analytics ───────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetRules(ctx context.Context, _ string) ([]onvif.AnalyticsRule, error) {
	raw, err := d.client.Analytics.GetRuleConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get analytics rules: %w", err)
	}

	// Parse Amcrest VideoAnalyseRule config into analytics rules.
	// Keys like: table.VideoAnalyseRule[0].Name=Rule1, table.VideoAnalyseRule[0].Type=CrossLine
	ruleMap := make(map[string]*onvif.AnalyticsRule)
	for k, v := range raw {
		// Extract index and field from table.VideoAnalyseRule[N].Field
		if !strings.Contains(k, "VideoAnalyseRule[") {
			continue
		}
		bracketStart := strings.Index(k, "[")
		bracketEnd := strings.Index(k, "]")
		if bracketStart < 0 || bracketEnd < 0 {
			continue
		}
		idx := k[bracketStart+1 : bracketEnd]
		dotIdx := strings.LastIndex(k, ".")
		if dotIdx < 0 {
			continue
		}
		field := k[dotIdx+1:]

		rule, ok := ruleMap[idx]
		if !ok {
			rule = &onvif.AnalyticsRule{Parameters: make(map[string]string)}
			ruleMap[idx] = rule
		}

		switch field {
		case "Name":
			rule.Name = v
		case "Type":
			rule.Type = v
		default:
			rule.Parameters[field] = v
		}
	}

	var rules []onvif.AnalyticsRule
	for _, r := range ruleMap {
		if r.Name != "" || r.Type != "" {
			rules = append(rules, *r)
		}
	}
	return rules, nil
}

func (d *AmcrestDriver) CreateRule(ctx context.Context, _ string, rule onvif.AnalyticsRule) error {
	// Amcrest uses setConfig for analytics rules. We append to VideoAnalyseRule.
	params := map[string]string{}
	// Use a high index to avoid overwriting existing rules.
	idx := 10
	prefix := fmt.Sprintf("VideoAnalyseRule[0].PtzRuleList[%d].", idx)
	params[prefix+"Name"] = rule.Name
	params[prefix+"Type"] = rule.Type
	for k, v := range rule.Parameters {
		params[prefix+k] = v
	}
	return d.client.Analytics.SetRuleConfig(ctx, params)
}

func (d *AmcrestDriver) ModifyRule(ctx context.Context, _ string, rule onvif.AnalyticsRule) error {
	// Find existing rule by name and update it.
	raw, err := d.client.Analytics.GetRuleConfig(ctx)
	if err != nil {
		return fmt.Errorf("amcrest get analytics rules for modify: %w", err)
	}

	// Find the index of the rule with the matching name.
	ruleIndex := -1
	for k, v := range raw {
		if strings.HasSuffix(k, ".Name") && v == rule.Name {
			bracketStart := strings.Index(k, "[")
			bracketEnd := strings.Index(k, "]")
			if bracketStart >= 0 && bracketEnd >= 0 {
				ruleIndex, _ = strconv.Atoi(k[bracketStart+1 : bracketEnd])
				break
			}
		}
	}

	if ruleIndex < 0 {
		return fmt.Errorf("amcrest: rule %q not found", rule.Name)
	}

	prefix := fmt.Sprintf("VideoAnalyseRule[0].PtzRuleList[%d].", ruleIndex)
	params := map[string]string{
		prefix + "Type": rule.Type,
	}
	for k, v := range rule.Parameters {
		params[prefix+k] = v
	}
	return d.client.Analytics.SetRuleConfig(ctx, params)
}

func (d *AmcrestDriver) DeleteRule(_ context.Context, _, _ string) error {
	return fmt.Errorf("amcrest: individual rule deletion not supported — use ModifyRule to disable instead")
}

func (d *AmcrestDriver) GetAnalyticsModules(ctx context.Context, _ string) ([]onvif.AnalyticsModule, error) {
	raw, err := d.client.Analytics.GetGlobalConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get analytics global config: %w", err)
	}

	// Parse the global config into modules.
	module := onvif.AnalyticsModule{
		Name:       "VideoAnalytics",
		Type:       "amcrest:VideoAnalyseGlobal",
		Parameters: make(map[string]string),
	}
	for k, v := range raw {
		// Strip the table prefix for cleaner parameter names.
		key := k
		if idx := strings.LastIndex(k, "."); idx >= 0 {
			key = k[idx+1:]
		}
		module.Parameters[key] = v
	}

	return []onvif.AnalyticsModule{module}, nil
}

// ── OSD ─────────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetOSDs(ctx context.Context, _ string) ([]onvif.OSD, error) {
	// Amcrest OSD is managed via VideoWidget and ChannelTitle configs.
	widgetRaw, err := d.client.Video.GetVideoWidget(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get video widget: %w", err)
	}

	var osds []onvif.OSD

	// Channel title OSD
	titleRaw, titleErr := d.client.Video.GetChannelTitle(ctx)
	if titleErr == nil {
		for k, v := range titleRaw {
			if strings.HasSuffix(k, ".Name") && v != "" {
				osds = append(osds, onvif.OSD{
					Token:            "title_0",
					VideoSourceToken: "000",
					Type:             "Text",
					TextString: &onvif.OSDText{
						IsPersistentText: true,
						Type:             "Plain",
						PlainText:        v,
					},
				})
				break
			}
		}
	}

	// Time OSD from VideoWidget config
	for k, v := range widgetRaw {
		if strings.Contains(k, "TimeTitle") && strings.HasSuffix(k, ".EncodeBlend") {
			if strings.EqualFold(v, "true") {
				osds = append(osds, onvif.OSD{
					Token:            "time_0",
					VideoSourceToken: "000",
					Type:             "Text",
					TextString: &onvif.OSDText{
						Type: "DateAndTime",
					},
				})
			}
			break
		}
	}

	return osds, nil
}

func (d *AmcrestDriver) GetOSDOptions(_ context.Context, _ string) (*onvif.OSDOptions, error) {
	return &onvif.OSDOptions{
		MaximumNumberOfOSDs: onvif.MaxOSDs{
			Total:       2,
			PlainText:   1,
			DateAndTime: 1,
		},
		Types:           []string{"Text"},
		PositionOptions: []string{"Custom"},
	}, nil
}

func (d *AmcrestDriver) CreateOSD(_ context.Context, _ onvif.OSDConfig) (string, error) {
	return "", fmt.Errorf("amcrest: OSD creation not supported — use SetOSD to modify existing overlays")
}

func (d *AmcrestDriver) SetOSD(ctx context.Context, cfg onvif.OSDConfig) error {
	if cfg.TextString != nil && cfg.TextString.PlainText != "" {
		// Update channel title.
		return d.client.Video.SetChannelTitle(ctx, 0, cfg.TextString.PlainText)
	}
	return fmt.Errorf("amcrest: only plain text OSD updates are supported")
}

func (d *AmcrestDriver) DeleteOSD(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: OSD deletion not supported — Amcrest OSD overlays are fixed; use SetOSD to clear text")
}

// ── Metadata ────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetMetadataConfigurations(_ context.Context) ([]*onvif.MetadataConfigInfo, error) {
	// Amcrest doesn't have a direct metadata configuration concept like ONVIF.
	// Return a synthetic config representing the event stream metadata.
	return []*onvif.MetadataConfigInfo{
		{
			Token:          "metadata_0",
			Name:           "Event Metadata",
			Analytics:      true,
			SessionTimeout: "PT60S",
		},
	}, nil
}

func (d *AmcrestDriver) GetMetadataConfiguration(_ context.Context, configToken string) (*onvif.MetadataConfigInfo, error) {
	return &onvif.MetadataConfigInfo{
		Token:          configToken,
		Name:           "Event Metadata",
		Analytics:      true,
		SessionTimeout: "PT60S",
	}, nil
}

func (d *AmcrestDriver) SetMetadataConfiguration(_ context.Context, _ *onvif.MetadataConfigInfo) error {
	return fmt.Errorf("amcrest: metadata configuration is managed by the event system and cannot be changed directly")
}

func (d *AmcrestDriver) AddMetadataToProfile(_ context.Context, _, _ string) error {
	return fmt.Errorf("amcrest: metadata-to-profile binding not supported — event metadata is always available")
}

func (d *AmcrestDriver) RemoveMetadataFromProfile(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: metadata-from-profile unbinding not supported — event metadata is always available")
}

// ── Multicast ───────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetMulticastConfig(_ context.Context, _ string) (*onvif.MulticastConfig, error) {
	return nil, fmt.Errorf("amcrest: multicast configuration not supported by Amcrest CGI API")
}

func (d *AmcrestDriver) SetMulticastConfig(_ context.Context, _ string, _ *onvif.MulticastConfig) error {
	return fmt.Errorf("amcrest: multicast configuration not supported by Amcrest CGI API")
}

// ── Edge Recording ──────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetRecordingConfiguration(ctx context.Context, _ string) (*onvif.RecordingConfiguration, error) {
	raw, err := d.client.Recording.GetRecordConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get recording config: %w", err)
	}

	return &onvif.RecordingConfiguration{
		RecordingToken: "rec_0",
		Source: onvif.RecordingSource{
			SourceID: "000",
			Name:     "Main",
		},
		Content: kvString(raw, "table.Record[0].MediaType"),
	}, nil
}

func (d *AmcrestDriver) CreateRecording(_ context.Context, _ onvif.RecordingSource, _, _ string) (string, error) {
	return "", fmt.Errorf("amcrest: edge recording creation not supported — use camera's built-in recording configuration")
}

func (d *AmcrestDriver) DeleteRecording(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: edge recording deletion not supported — recording is managed by camera firmware")
}

func (d *AmcrestDriver) CreateRecordingJob(_ context.Context, _, _ string, _ int) (*onvif.RecordingJobConfiguration, error) {
	return nil, fmt.Errorf("amcrest: recording jobs not supported — use camera's RecordMode configuration instead")
}

func (d *AmcrestDriver) DeleteRecordingJob(_ context.Context, _ string) error {
	return fmt.Errorf("amcrest: recording jobs not supported — use camera's RecordMode configuration instead")
}

func (d *AmcrestDriver) GetRecordingJobState(_ context.Context, _ string) (*onvif.RecordingJobState, error) {
	return nil, fmt.Errorf("amcrest: recording job state not supported — use camera's RecordMode configuration instead")
}

func (d *AmcrestDriver) CreateTrack(_ context.Context, _, _, _ string) (string, error) {
	return "", fmt.Errorf("amcrest: track creation not supported — recording tracks are fixed by camera firmware")
}

func (d *AmcrestDriver) DeleteTrack(_ context.Context, _, _ string) error {
	return fmt.Errorf("amcrest: track deletion not supported — recording tracks are fixed by camera firmware")
}

func (d *AmcrestDriver) GetTrackConfiguration(_ context.Context, _, _ string) (*onvif.TrackConfiguration, error) {
	return nil, fmt.Errorf("amcrest: track configuration not supported — recording tracks are fixed by camera firmware")
}

func (d *AmcrestDriver) GetRecordingSummary(ctx context.Context) (*onvif.EdgeRecordingSummary, error) {
	caps, err := d.client.Recording.GetCaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("amcrest get recording caps: %w", err)
	}

	total, _ := strconv.Atoi(kvString(caps, "MaxRecordChannels"))
	return &onvif.EdgeRecordingSummary{
		TotalRecordings: total,
	}, nil
}

func (d *AmcrestDriver) FindRecordings(ctx context.Context) ([]onvif.EdgeRecording, error) {
	files, err := d.client.Recording.FindFiles(ctx, amcrest.FindFilesOpts{
		Channel:   0,
		StartTime: time.Now().AddDate(0, 0, -1).Format("2006-01-02 15:04:05"),
		EndTime:   time.Now().Format("2006-01-02 15:04:05"),
		Type:      "dav",
	})
	if err != nil {
		return nil, fmt.Errorf("amcrest find recordings: %w", err)
	}

	var recordings []onvif.EdgeRecording
	for _, f := range files {
		recordings = append(recordings, onvif.EdgeRecording{
			RecordingToken: f.FilePath,
			SourceName:     fmt.Sprintf("Channel %d", f.Channel),
			EarliestTime:   f.StartTime,
			LatestTime:     f.EndTime,
		})
	}
	return recordings, nil
}

// ── Replay ──────────────────────────────────────────────────────────────────

func (d *AmcrestDriver) GetReplayUri(_ context.Context, recordingToken string) (string, error) {
	// Amcrest provides playback via RTSP with a special path format.
	host := d.host
	if h, _, err := splitHostPort(d.host); err == nil {
		host = h
	}
	// Amcrest playback URI format: rtsp://host:554/cam/playback?channel=0&starttime=...
	return fmt.Sprintf("rtsp://%s:554/cam/playback?channel=0&starttime=%s",
		host, url.QueryEscape(recordingToken)), nil
}

func (d *AmcrestDriver) BuildReplaySession(_ context.Context, replayURI, recordingToken string, startTime time.Time, scale float64) (*onvif.ReplaySession, error) {
	if replayURI == "" {
		return nil, fmt.Errorf("amcrest: replay URI is required")
	}
	if scale == 0 {
		scale = 1
	}

	headers := map[string]string{
		"Range": fmt.Sprintf("clock=%s-", startTime.UTC().Format("20060102T150405.000Z")),
	}
	if scale != 1 {
		headers["Scale"] = fmt.Sprintf("%.1f", scale)
	}

	return &onvif.ReplaySession{
		ReplayURI:      replayURI,
		RecordingToken: recordingToken,
		StartTime:      startTime,
		Scale:          scale,
		Reverse:        scale < 0,
		RTSPHeaders:    headers,
	}, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func amcrestToOnvifEncoding(enc string) string {
	switch strings.ToUpper(enc) {
	case "H.264", "H.264H", "H.264B":
		return "H264"
	case "H.265", "H.265+":
		return "H265"
	case "MJPEG", "JPEG":
		return "JPEG"
	default:
		return enc
	}
}

func onvifToAmcrestEncoding(enc string) string {
	switch strings.ToUpper(enc) {
	case "H264":
		return "H.264"
	case "H265":
		return "H.265"
	case "JPEG":
		return "MJPEG"
	default:
		return enc
	}
}

func amcrestToEventType(code string) string {
	switch code {
	case "VideoMotion":
		return "motion"
	case "VideoBlind":
		return "tamper"
	case "VideoLoss":
		return "signal_loss"
	case "CrossLineDetection":
		return "line_crossing"
	case "CrossRegionDetection":
		return "intrusion"
	default:
		return code
	}
}

func kvString(m map[string]string, key string) string {
	return strings.TrimSpace(m[key])
}

func kvInt(m map[string]string, key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(m[key]))
	return v
}

func kvFloat(m map[string]string, key string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(m[key]), 64)
	return v
}

func extractHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	return u.Hostname()
}

// splitHostPort splits a host string that may contain a port.
// It handles both "host:port" and plain "host" formats.
func splitHostPort(hostport string) (host, port string, err error) {
	lastColon := strings.LastIndex(hostport, ":")
	if lastColon < 0 {
		return hostport, "", nil
	}
	return hostport[:lastColon], hostport[lastColon+1:], nil
}

// parseAmcrestRange parses a range string like "0-100" into a FloatRange.
func parseAmcrestRange(s string) *onvif.FloatRange {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return nil
	}
	min, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	max, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return nil
	}
	return &onvif.FloatRange{Min: min, Max: max}
}

// subnetToCIDR converts a subnet mask string (e.g., "255.255.255.0") to a prefix length.
func subnetToCIDR(mask string) int {
	parts := strings.Split(mask, ".")
	if len(parts) != 4 {
		return 24 // default
	}
	bits := 0
	for _, p := range parts {
		n, _ := strconv.Atoi(p)
		for n > 0 {
			bits += n & 1
			n >>= 1
		}
	}
	return bits
}

// amcrestGroupToRole maps Amcrest user groups to ONVIF-style roles.
func amcrestGroupToRole(group string) string {
	switch strings.ToLower(group) {
	case "admin":
		return "Administrator"
	case "user":
		return "User"
	case "viewer", "anonymous":
		return "Anonymous"
	default:
		return group
	}
}

// roleToAmcrestGroup maps ONVIF-style roles to Amcrest user groups.
func roleToAmcrestGroup(role string) string {
	switch strings.ToLower(role) {
	case "administrator", "admin":
		return "admin"
	case "operator":
		return "user"
	case "user":
		return "user"
	case "anonymous":
		return "viewer"
	default:
		return "user"
	}
}
