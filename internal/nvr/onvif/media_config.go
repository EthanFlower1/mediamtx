package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// ProfileInfo holds full profile details including configurations.
type ProfileInfo struct {
	Token        string              `json:"token"`
	Name         string              `json:"name"`
	VideoSource  *VideoSourceInfo    `json:"video_source,omitempty"`
	VideoEncoder *VideoEncoderConfig `json:"video_encoder,omitempty"`
	AudioEncoder *AudioEncoderConfig `json:"audio_encoder,omitempty"`
	PTZConfig    *PTZConfigInfo      `json:"ptz_config,omitempty"`
}

type VideoSourceInfo struct {
	Token     string  `json:"token"`
	Framerate float64 `json:"framerate"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
}

type VideoEncoderConfig struct {
	Token            string  `json:"token"`
	Name             string  `json:"name"`
	Encoding         string  `json:"encoding"`
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

type AudioEncoderConfig struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	Encoding   string `json:"encoding"`
	Bitrate    int    `json:"bitrate"`
	SampleRate int    `json:"sample_rate"`
}

type AudioEncoderOptions struct {
	Encodings   []string `json:"encodings"`
	BitrateList []int    `json:"bitrate_list"`
	SampleRates []int    `json:"sample_rate_list"`
}

type Resolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// VideoSourceConfig holds a video source configuration from the device.
type VideoSourceConfig struct {
	Token       string        `json:"token"`
	Name        string        `json:"name"`
	SourceToken string        `json:"source_token"`
	Bounds      *IntRectangle `json:"bounds,omitempty"`
}

// IntRectangle represents a rectangle with position and dimensions.
type IntRectangle struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// VideoSourceConfigOptions holds the available options for a video source configuration.
type VideoSourceConfigOptions struct {
	BoundsRange             *IntRectangleRange `json:"bounds_range,omitempty"`
	MaximumNumberOfProfiles int                `json:"maximum_number_of_profiles,omitempty"`
}

// IntRectangleRange represents min/max ranges for each rectangle dimension.
type IntRectangleRange struct {
	XRange      Range `json:"x_range"`
	YRange      Range `json:"y_range"`
	WidthRange  Range `json:"width_range"`
	HeightRange Range `json:"height_range"`
}


type PTZConfigInfo struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	NodeToken string `json:"node_token"`
}

func convertVideoEncoderConfig(vec *onvifgo.VideoEncoderConfiguration) *VideoEncoderConfig {
	cfg := &VideoEncoderConfig{
		Token:    vec.Token,
		Name:     vec.Name,
		Encoding: vec.Encoding,
		Quality:  vec.Quality,
	}
	if vec.Resolution != nil {
		cfg.Width = vec.Resolution.Width
		cfg.Height = vec.Resolution.Height
	}
	if vec.RateControl != nil {
		cfg.FrameRate = vec.RateControl.FrameRateLimit
		cfg.BitrateLimit = vec.RateControl.BitrateLimit
		cfg.EncodingInterval = vec.RateControl.EncodingInterval
	}
	if vec.H264 != nil {
		cfg.GovLength = vec.H264.GovLength
		cfg.H264Profile = vec.H264.H264Profile
	}
	return cfg
}

// GetProfilesFull returns all profiles with full configuration details.
func GetProfilesFull(xaddr, username, password string) ([]*ProfileInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	profiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}

	var result []*ProfileInfo
	for _, p := range profiles {
		info := &ProfileInfo{
			Token: p.Token,
			Name:  p.Name,
		}
		if p.VideoSourceConfiguration != nil {
			info.VideoSource = &VideoSourceInfo{
				Token: p.VideoSourceConfiguration.Token,
			}
		}
		if p.VideoEncoderConfiguration != nil {
			info.VideoEncoder = convertVideoEncoderConfig(p.VideoEncoderConfiguration)
		}
		if p.AudioEncoderConfiguration != nil {
			info.AudioEncoder = &AudioEncoderConfig{
				Token:      p.AudioEncoderConfiguration.Token,
				Name:       p.AudioEncoderConfiguration.Name,
				Encoding:   p.AudioEncoderConfiguration.Encoding,
				Bitrate:    p.AudioEncoderConfiguration.Bitrate,
				SampleRate: p.AudioEncoderConfiguration.SampleRate,
			}
		}
		if p.PTZConfiguration != nil {
			info.PTZConfig = &PTZConfigInfo{
				Token:     p.PTZConfiguration.Token,
				Name:      p.PTZConfiguration.Name,
				NodeToken: p.PTZConfiguration.NodeToken,
			}
		}
		result = append(result, info)
	}
	return result, nil
}

// GetVideoSourcesList returns all video sources from the device.
func GetVideoSourcesList(xaddr, username, password string) ([]*VideoSourceInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	sources, err := client.Dev.GetVideoSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("get video sources: %w", err)
	}

	var result []*VideoSourceInfo
	for _, s := range sources {
		info := &VideoSourceInfo{
			Token:     s.Token,
			Framerate: s.Framerate,
		}
		if s.Resolution != nil {
			info.Width = s.Resolution.Width
			info.Height = s.Resolution.Height
		}
		result = append(result, info)
	}
	return result, nil
}

// GetVideoEncoderConfig returns the video encoder configuration for a given token.
func GetVideoEncoderConfig(xaddr, username, password, configToken string) (*VideoEncoderConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	vec, err := client.Dev.GetVideoEncoderConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get video encoder config: %w", err)
	}
	return convertVideoEncoderConfig(vec), nil
}

// SetVideoEncoderConfig updates a video encoder configuration on the device.
func SetVideoEncoderConfig(xaddr, username, password string, cfg *VideoEncoderConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	vec := &onvifgo.VideoEncoderConfiguration{
		Token:    cfg.Token,
		Name:     cfg.Name,
		Encoding: cfg.Encoding,
		Quality:  cfg.Quality,
		Resolution: &onvifgo.VideoResolution{
			Width:  cfg.Width,
			Height: cfg.Height,
		},
	}

	ctx := context.Background()
	if err := client.Dev.SetVideoEncoderConfiguration(ctx, vec, true); err != nil {
		return fmt.Errorf("set video encoder config: %w", err)
	}
	return nil
}

// GetVideoEncoderOpts returns the available options for a video encoder configuration.
func GetVideoEncoderOpts(xaddr, username, password, configToken string) (*VideoEncoderOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	opts, err := client.Dev.GetVideoEncoderConfigurationOptions(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get video encoder options: %w", err)
	}

	result := &VideoEncoderOptions{}
	if opts.QualityRange != nil {
		result.QualityRange = Range{Min: int(opts.QualityRange.Min), Max: int(opts.QualityRange.Max)}
	}
	if opts.JPEG != nil {
		result.Encodings = append(result.Encodings, "JPEG")
		for _, r := range opts.JPEG.ResolutionsAvailable {
			result.Resolutions = append(result.Resolutions, Resolution{Width: r.Width, Height: r.Height})
		}
		if opts.JPEG.FrameRateRange != nil {
			result.FrameRateRange = Range{Min: int(opts.JPEG.FrameRateRange.Min), Max: int(opts.JPEG.FrameRateRange.Max)}
		}
	}
	if opts.H264 != nil {
		result.Encodings = append(result.Encodings, "H264")
		for _, r := range opts.H264.ResolutionsAvailable {
			result.Resolutions = append(result.Resolutions, Resolution{Width: r.Width, Height: r.Height})
		}
		if opts.H264.FrameRateRange != nil {
			result.FrameRateRange = Range{Min: int(opts.H264.FrameRateRange.Min), Max: int(opts.H264.FrameRateRange.Max)}
		}
		if opts.H264.GovLengthRange != nil {
			result.GovLengthRange = Range{Min: opts.H264.GovLengthRange.Min, Max: opts.H264.GovLengthRange.Max}
		}
		for _, p := range opts.H264.H264ProfilesSupported {
			result.H264Profiles = append(result.H264Profiles, string(p))
		}
		if opts.H264.EncodingIntervalRange != nil {
			result.EncodingIntervalRange = Range{Min: opts.H264.EncodingIntervalRange.Min, Max: opts.H264.EncodingIntervalRange.Max}
		}
	}
	return result, nil
}

// CreateMediaProfile creates a new media profile on the device.
func CreateMediaProfile(xaddr, username, password, name string) (*ProfileInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	p, err := client.Dev.CreateProfile(ctx, name, "")
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}
	return &ProfileInfo{Token: p.Token, Name: p.Name}, nil
}

// DeleteMediaProfile deletes a media profile from the device.
func DeleteMediaProfile(xaddr, username, password, token string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.DeleteProfile(ctx, token); err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	return nil
}

// GetAudioEncoderCfg returns the audio encoder configuration for a given token.
func GetAudioEncoderCfg(xaddr, username, password, configToken string) (*AudioEncoderConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	aec, err := client.Dev.GetAudioEncoderConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get audio encoder config: %w", err)
	}
	return &AudioEncoderConfig{
		Token:      aec.Token,
		Name:       aec.Name,
		Encoding:   aec.Encoding,
		Bitrate:    aec.Bitrate,
		SampleRate: aec.SampleRate,
	}, nil
}

// SetAudioEncoderCfg updates an audio encoder configuration on the device.
func SetAudioEncoderCfg(xaddr, username, password string, cfg *AudioEncoderConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	aec := &onvifgo.AudioEncoderConfiguration{
		Token:      cfg.Token,
		Name:       cfg.Name,
		Encoding:   cfg.Encoding,
		Bitrate:    cfg.Bitrate,
		SampleRate: cfg.SampleRate,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioEncoderConfiguration(ctx, aec, true); err != nil {
		return fmt.Errorf("set audio encoder config: %w", err)
	}
	return nil
}

// AddVideoEncoderToProfile adds a video encoder configuration to a profile.
func AddVideoEncoderToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddVideoEncoderConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add video encoder to profile: %w", err)
	}
	return nil
}

// RemoveVideoEncoderFromProfile removes the video encoder from a profile.
func RemoveVideoEncoderFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveVideoEncoderConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove video encoder from profile: %w", err)
	}
	return nil
}

// AddAudioEncoderToProfile adds an audio encoder configuration to a profile.
func AddAudioEncoderToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddAudioEncoderConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add audio encoder to profile: %w", err)
	}
	return nil
}

// RemoveAudioEncoderFromProfile removes the audio encoder from a profile.
func RemoveAudioEncoderFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveAudioEncoderConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove audio encoder from profile: %w", err)
	}
	return nil
}

// --- Media2 public wrappers ---

// CreateMedia2Profile creates a new media profile via Media2.
func CreateMedia2Profile(xaddr, username, password, name string) (*ProfileInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	token, retName, err := CreateProfile2(client, name)
	if err != nil {
		return nil, err
	}
	return &ProfileInfo{Token: token, Name: retName}, nil
}

// DeleteMedia2Profile deletes a media profile via Media2.
func DeleteMedia2Profile(xaddr, username, password, token string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return DeleteProfile2(client, token)
}

// AddMedia2Configuration adds a configuration to a profile via Media2.
func AddMedia2Configuration(xaddr, username, password, profileToken, configType, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return AddConfiguration2(client, profileToken, configType, configToken)
}

// RemoveMedia2Configuration removes a configuration from a profile via Media2.
func RemoveMedia2Configuration(xaddr, username, password, profileToken, configType, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return RemoveConfiguration2(client, profileToken, configType, configToken)
}

// GetVideoSourceConfigs returns all video source configurations via Media2.
func GetVideoSourceConfigs(xaddr, username, password string) ([]VideoSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return GetVideoSourceConfigurations2(client)
}

// SetVideoSourceConfig updates a video source configuration via Media2.
func SetVideoSourceConfig(xaddr, username, password string, cfg *VideoSourceConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return SetVideoSourceConfiguration2(client, cfg)
}

// GetVideoSourceConfigOpts returns the available options for a video source configuration via Media2.
func GetVideoSourceConfigOpts(xaddr, username, password, configToken, profileToken string) (*VideoSourceConfigOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return GetVideoSourceConfigurationOptions2(client, configToken, profileToken)
}

// GetAudioSourceConfigs returns all audio source configurations via Media2.
func GetAudioSourceConfigs(xaddr, username, password string) ([]AudioSourceConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}
	return GetAudioSourceConfigurations2(client)
}

// SetAudioSourceConfig updates an audio source configuration via Media2.
func SetAudioSourceConfig(xaddr, username, password string, cfg *AudioSourceConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}
	return SetAudioSourceConfiguration2(client, cfg)
}
