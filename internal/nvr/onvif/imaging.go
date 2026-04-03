package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// ErrImagingNotSupported is returned when the camera doesn't expose an imaging service.
var ErrImagingNotSupported = fmt.Errorf("camera does not support ONVIF imaging service")

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// ImagingSettings represents the full adjustable image settings of a camera.
type ImagingSettings struct {
	Brightness            float64                `json:"brightness"`
	Contrast              float64                `json:"contrast"`
	Saturation            float64                `json:"saturation"`
	Sharpness             float64                `json:"sharpness"`
	BacklightCompensation *BacklightCompensation `json:"backlightCompensation,omitempty"`
	Exposure              *ExposureSettings      `json:"exposure,omitempty"`
	Focus                 *FocusSettings         `json:"focus,omitempty"`
	WideDynamicRange      *WideDynamicRange      `json:"wideDynamicRange,omitempty"`
	WhiteBalance          *WhiteBalanceSettings  `json:"whiteBalance,omitempty"`
	IrCutFilter           string                 `json:"irCutFilter,omitempty"` // "ON", "OFF", "AUTO"
}

// BacklightCompensation holds backlight compensation settings.
type BacklightCompensation struct {
	Mode  string  `json:"mode"`  // "OFF", "ON"
	Level float64 `json:"level"`
}

// ExposureSettings holds exposure control settings.
type ExposureSettings struct {
	Mode            string  `json:"mode"`                      // "AUTO", "MANUAL"
	Priority        string  `json:"priority,omitempty"`        // "LowNoise", "FrameRate"
	MinExposureTime float64 `json:"minExposureTime,omitempty"`
	MaxExposureTime float64 `json:"maxExposureTime,omitempty"`
	MinGain         float64 `json:"minGain,omitempty"`
	MaxGain         float64 `json:"maxGain,omitempty"`
	MinIris         float64 `json:"minIris,omitempty"`
	MaxIris         float64 `json:"maxIris,omitempty"`
	ExposureTime    float64 `json:"exposureTime,omitempty"`
	Gain            float64 `json:"gain,omitempty"`
	Iris            float64 `json:"iris,omitempty"`
}

// FocusSettings holds focus configuration.
type FocusSettings struct {
	AutoFocusMode string  `json:"autoFocusMode"` // "AUTO", "MANUAL"
	DefaultSpeed  float64 `json:"defaultSpeed,omitempty"`
	NearLimit     float64 `json:"nearLimit,omitempty"`
	FarLimit      float64 `json:"farLimit,omitempty"`
}

// WideDynamicRange holds WDR settings.
type WideDynamicRange struct {
	Mode  string  `json:"mode"`  // "OFF", "ON"
	Level float64 `json:"level"`
}

// WhiteBalanceSettings holds white balance configuration.
type WhiteBalanceSettings struct {
	Mode   string  `json:"mode"` // "AUTO", "MANUAL"
	CrGain float64 `json:"crGain,omitempty"`
	CbGain float64 `json:"cbGain,omitempty"`
}

// ---------------------------------------------------------------------------
// Options types (valid ranges)
// ---------------------------------------------------------------------------

// FloatRange represents a min/max range for a float value.
type FloatRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// ImagingOptions represents the valid ranges and modes for all imaging settings.
type ImagingOptions struct {
	Brightness            *FloatRange                    `json:"brightness,omitempty"`
	Contrast              *FloatRange                    `json:"contrast,omitempty"`
	Saturation            *FloatRange                    `json:"saturation,omitempty"`
	Sharpness             *FloatRange                    `json:"sharpness,omitempty"`
	BacklightCompensation *BacklightCompensationOptions  `json:"backlightCompensation,omitempty"`
	Exposure              *ExposureOptions               `json:"exposure,omitempty"`
	Focus                 *FocusOptions                  `json:"focus,omitempty"`
	WideDynamicRange      *WideDynamicRangeOptions       `json:"wideDynamicRange,omitempty"`
	WhiteBalance          *WhiteBalanceOptions           `json:"whiteBalance,omitempty"`
	IrCutFilterModes      []string                       `json:"irCutFilterModes,omitempty"`
}

// BacklightCompensationOptions holds valid ranges for backlight compensation.
type BacklightCompensationOptions struct {
	Modes []string    `json:"modes,omitempty"`
	Level *FloatRange `json:"level,omitempty"`
}

// ExposureOptions holds valid ranges for exposure settings.
type ExposureOptions struct {
	Modes           []string    `json:"modes,omitempty"`
	Priorities      []string    `json:"priorities,omitempty"`
	MinExposureTime *FloatRange `json:"minExposureTime,omitempty"`
	MaxExposureTime *FloatRange `json:"maxExposureTime,omitempty"`
	MinGain         *FloatRange `json:"minGain,omitempty"`
	MaxGain         *FloatRange `json:"maxGain,omitempty"`
	MinIris         *FloatRange `json:"minIris,omitempty"`
	MaxIris         *FloatRange `json:"maxIris,omitempty"`
	ExposureTime    *FloatRange `json:"exposureTime,omitempty"`
	Gain            *FloatRange `json:"gain,omitempty"`
	Iris            *FloatRange `json:"iris,omitempty"`
}

// FocusOptions holds valid ranges for focus settings.
type FocusOptions struct {
	AutoFocusModes []string    `json:"autoFocusModes,omitempty"`
	DefaultSpeed   *FloatRange `json:"defaultSpeed,omitempty"`
	NearLimit      *FloatRange `json:"nearLimit,omitempty"`
	FarLimit       *FloatRange `json:"farLimit,omitempty"`
}

// WideDynamicRangeOptions holds valid ranges for WDR settings.
type WideDynamicRangeOptions struct {
	Modes []string    `json:"modes,omitempty"`
	Level *FloatRange `json:"level,omitempty"`
}

// WhiteBalanceOptions holds valid ranges for white balance settings.
type WhiteBalanceOptions struct {
	Modes  []string    `json:"modes,omitempty"`
	YrGain *FloatRange `json:"yrGain,omitempty"`
	YbGain *FloatRange `json:"ybGain,omitempty"`
}

// ---------------------------------------------------------------------------
// Focus move types
// ---------------------------------------------------------------------------

// FocusMoveOptions describes what kinds of focus movements the camera supports.
type FocusMoveOptions struct {
	Absolute   *AbsoluteFocusMoveOptions   `json:"absolute,omitempty"`
	Relative   *RelativeFocusMoveOptions   `json:"relative,omitempty"`
	Continuous *ContinuousFocusMoveOptions `json:"continuous,omitempty"`
}

// AbsoluteFocusMoveOptions holds ranges for absolute focus moves.
type AbsoluteFocusMoveOptions struct {
	Position FloatRange `json:"position"`
	Speed    FloatRange `json:"speed"`
}

// RelativeFocusMoveOptions holds ranges for relative focus moves.
type RelativeFocusMoveOptions struct {
	Distance FloatRange `json:"distance"`
	Speed    FloatRange `json:"speed"`
}

// ContinuousFocusMoveOptions holds ranges for continuous focus moves.
type ContinuousFocusMoveOptions struct {
	Speed FloatRange `json:"speed"`
}

// FocusMoveRequest describes a focus move command.
type FocusMoveRequest struct {
	Absolute   *AbsoluteFocusMove   `json:"absolute,omitempty"`
	Relative   *RelativeFocusMove   `json:"relative,omitempty"`
	Continuous *ContinuousFocusMove `json:"continuous,omitempty"`
}

// AbsoluteFocusMove is an absolute focus position command.
type AbsoluteFocusMove struct {
	Position float64  `json:"position"`
	Speed    *float64 `json:"speed,omitempty"`
}

// RelativeFocusMove is a relative focus distance command.
type RelativeFocusMove struct {
	Distance float64  `json:"distance"`
	Speed    *float64 `json:"speed,omitempty"`
}

// ContinuousFocusMove is a continuous focus speed command.
type ContinuousFocusMove struct {
	Speed float64 `json:"speed"`
}

// ---------------------------------------------------------------------------
// Status types
// ---------------------------------------------------------------------------

// ImagingStatus contains the current status of the imaging system.
type ImagingStatus struct {
	FocusStatus *FocusStatus `json:"focusStatus,omitempty"`
}

// FocusStatus contains focus position and movement status.
type FocusStatus struct {
	Position   float64 `json:"position"`
	MoveStatus string  `json:"moveStatus"` // "IDLE", "MOVING", "UNKNOWN"
	Error      string  `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Public functions
// ---------------------------------------------------------------------------

// newImagingClient creates an ONVIF client and validates imaging support.
// It also resolves the video source token if not provided.
func newImagingClient(xaddr, username, password, videoSourceToken string) (*Client, string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, "", fmt.Errorf("connect to ONVIF device: %w", err)
	}
	if !client.HasService("imaging") {
		return nil, "", ErrImagingNotSupported
	}
	if videoSourceToken == "" || videoSourceToken == "000" {
		ctx := context.Background()
		token, err := getVideoSourceToken(ctx, client.Dev)
		if err != nil {
			return nil, "", fmt.Errorf("get video source token: %w", err)
		}
		videoSourceToken = token
	}
	return client, videoSourceToken, nil
}

// GetImagingSettings retrieves the current imaging settings from an ONVIF camera.
func GetImagingSettings(xaddr, username, password, videoSourceToken string) (*ImagingSettings, error) {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return nil, err
	}

	raw, err := client.Dev.GetImagingSettings(context.Background(), vsToken)
	if err != nil {
		return nil, fmt.Errorf("get imaging settings: %w", err)
	}

	return settingsFromRaw(raw), nil
}

// SetImagingSettings applies imaging settings to an ONVIF camera.
func SetImagingSettings(xaddr, username, password, videoSourceToken string, settings *ImagingSettings) error {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return err
	}

	raw := settingsToRaw(settings)

	if err := client.Dev.SetImagingSettings(context.Background(), vsToken, raw, true); err != nil {
		return fmt.Errorf("set imaging settings: %w", err)
	}
	return nil
}

// GetImagingOptions retrieves the valid ranges and modes for all imaging settings.
func GetImagingOptions(xaddr, username, password, videoSourceToken string) (*ImagingOptions, error) {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return nil, err
	}

	raw, err := client.Dev.GetOptions(context.Background(), vsToken)
	if err != nil {
		return nil, fmt.Errorf("get imaging options: %w", err)
	}

	return optionsFromRaw(raw), nil
}

// GetImagingMoveOptions retrieves the focus move options for the camera.
func GetImagingMoveOptions(xaddr, username, password, videoSourceToken string) (*FocusMoveOptions, error) {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return nil, err
	}

	raw, err := client.Dev.GetMoveOptions(context.Background(), vsToken)
	if err != nil {
		return nil, fmt.Errorf("get imaging move options: %w", err)
	}

	return moveOptionsFromRaw(raw), nil
}

// MoveFocus issues a focus move command to the camera.
func MoveFocus(xaddr, username, password, videoSourceToken string, req *FocusMoveRequest) error {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return err
	}

	fm := &onvifgo.FocusMove{}
	if req.Absolute != nil {
		fm.Absolute = &onvifgo.AbsoluteFocusMove{
			Position: req.Absolute.Position,
			Speed:    req.Absolute.Speed,
		}
	}
	if req.Relative != nil {
		fm.Relative = &onvifgo.RelativeFocusMove{
			Distance: req.Relative.Distance,
			Speed:    req.Relative.Speed,
		}
	}
	if req.Continuous != nil {
		fm.Continuous = &onvifgo.ContinuousFocusMove{
			Speed: req.Continuous.Speed,
		}
	}

	if err := client.Dev.Move(context.Background(), vsToken, fm); err != nil {
		return fmt.Errorf("move focus: %w", err)
	}
	return nil
}

// StopFocus stops any in-progress focus movement.
func StopFocus(xaddr, username, password, videoSourceToken string) error {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return err
	}

	if err := client.Dev.StopFocus(context.Background(), vsToken); err != nil {
		return fmt.Errorf("stop focus: %w", err)
	}
	return nil
}

// GetImagingStatus retrieves the current imaging status (focus position, etc.).
func GetImagingStatus(xaddr, username, password, videoSourceToken string) (*ImagingStatus, error) {
	client, vsToken, err := newImagingClient(xaddr, username, password, videoSourceToken)
	if err != nil {
		return nil, err
	}

	raw, err := client.Dev.GetImagingStatus(context.Background(), vsToken)
	if err != nil {
		return nil, fmt.Errorf("get imaging status: %w", err)
	}

	result := &ImagingStatus{}
	if raw.FocusStatus != nil {
		result.FocusStatus = &FocusStatus{
			Position:   raw.FocusStatus.Position,
			MoveStatus: raw.FocusStatus.MoveStatus,
			Error:      raw.FocusStatus.Error,
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Conversion helpers
// ---------------------------------------------------------------------------

func settingsFromRaw(raw *onvifgo.ImagingSettings) *ImagingSettings {
	s := &ImagingSettings{
		Brightness: derefFloat(raw.Brightness),
		Contrast:   derefFloat(raw.Contrast),
		Saturation: derefFloat(raw.ColorSaturation),
		Sharpness:  derefFloat(raw.Sharpness),
	}
	if raw.BacklightCompensation != nil {
		s.BacklightCompensation = &BacklightCompensation{
			Mode:  raw.BacklightCompensation.Mode,
			Level: raw.BacklightCompensation.Level,
		}
	}
	if raw.Exposure != nil {
		s.Exposure = &ExposureSettings{
			Mode:            raw.Exposure.Mode,
			Priority:        raw.Exposure.Priority,
			MinExposureTime: raw.Exposure.MinExposureTime,
			MaxExposureTime: raw.Exposure.MaxExposureTime,
			MinGain:         raw.Exposure.MinGain,
			MaxGain:         raw.Exposure.MaxGain,
			MinIris:         raw.Exposure.MinIris,
			MaxIris:         raw.Exposure.MaxIris,
			ExposureTime:    raw.Exposure.ExposureTime,
			Gain:            raw.Exposure.Gain,
			Iris:            raw.Exposure.Iris,
		}
	}
	if raw.Focus != nil {
		s.Focus = &FocusSettings{
			AutoFocusMode: raw.Focus.AutoFocusMode,
			DefaultSpeed:  raw.Focus.DefaultSpeed,
			NearLimit:     raw.Focus.NearLimit,
			FarLimit:      raw.Focus.FarLimit,
		}
	}
	if raw.WideDynamicRange != nil {
		s.WideDynamicRange = &WideDynamicRange{
			Mode:  raw.WideDynamicRange.Mode,
			Level: raw.WideDynamicRange.Level,
		}
	}
	if raw.WhiteBalance != nil {
		s.WhiteBalance = &WhiteBalanceSettings{
			Mode:   raw.WhiteBalance.Mode,
			CrGain: raw.WhiteBalance.CrGain,
			CbGain: raw.WhiteBalance.CbGain,
		}
	}
	if raw.IrCutFilter != nil {
		s.IrCutFilter = *raw.IrCutFilter
	}
	return s
}

func settingsToRaw(s *ImagingSettings) *onvifgo.ImagingSettings {
	raw := &onvifgo.ImagingSettings{
		Brightness:      &s.Brightness,
		Contrast:        &s.Contrast,
		ColorSaturation: &s.Saturation,
		Sharpness:       &s.Sharpness,
	}
	if s.BacklightCompensation != nil {
		raw.BacklightCompensation = &onvifgo.BacklightCompensation{
			Mode:  s.BacklightCompensation.Mode,
			Level: s.BacklightCompensation.Level,
		}
	}
	if s.Exposure != nil {
		raw.Exposure = &onvifgo.Exposure{
			Mode:            s.Exposure.Mode,
			Priority:        s.Exposure.Priority,
			MinExposureTime: s.Exposure.MinExposureTime,
			MaxExposureTime: s.Exposure.MaxExposureTime,
			MinGain:         s.Exposure.MinGain,
			MaxGain:         s.Exposure.MaxGain,
			MinIris:         s.Exposure.MinIris,
			MaxIris:         s.Exposure.MaxIris,
			ExposureTime:    s.Exposure.ExposureTime,
			Gain:            s.Exposure.Gain,
			Iris:            s.Exposure.Iris,
		}
	}
	if s.Focus != nil {
		raw.Focus = &onvifgo.FocusConfiguration{
			AutoFocusMode: s.Focus.AutoFocusMode,
			DefaultSpeed:  s.Focus.DefaultSpeed,
			NearLimit:     s.Focus.NearLimit,
			FarLimit:      s.Focus.FarLimit,
		}
	}
	if s.WideDynamicRange != nil {
		raw.WideDynamicRange = &onvifgo.WideDynamicRange{
			Mode:  s.WideDynamicRange.Mode,
			Level: s.WideDynamicRange.Level,
		}
	}
	if s.WhiteBalance != nil {
		raw.WhiteBalance = &onvifgo.WhiteBalance{
			Mode:   s.WhiteBalance.Mode,
			CrGain: s.WhiteBalance.CrGain,
			CbGain: s.WhiteBalance.CbGain,
		}
	}
	if s.IrCutFilter != "" {
		raw.IrCutFilter = &s.IrCutFilter
	}
	return raw
}

func convertFloatRange(r *onvifgo.FloatRange) *FloatRange {
	if r == nil {
		return nil
	}
	return &FloatRange{Min: r.Min, Max: r.Max}
}

func optionsFromRaw(raw *onvifgo.ImagingOptions) *ImagingOptions {
	o := &ImagingOptions{
		Brightness:       convertFloatRange(raw.Brightness),
		Contrast:         convertFloatRange(raw.Contrast),
		Saturation:       convertFloatRange(raw.ColorSaturation),
		Sharpness:        convertFloatRange(raw.Sharpness),
		IrCutFilterModes: raw.IrCutFilterModes,
	}
	if raw.BacklightCompensation != nil {
		o.BacklightCompensation = &BacklightCompensationOptions{
			Modes: raw.BacklightCompensation.Mode,
			Level: convertFloatRange(raw.BacklightCompensation.Level),
		}
	}
	if raw.Exposure != nil {
		o.Exposure = &ExposureOptions{
			Modes:           raw.Exposure.Mode,
			Priorities:      raw.Exposure.Priority,
			MinExposureTime: convertFloatRange(raw.Exposure.MinExposureTime),
			MaxExposureTime: convertFloatRange(raw.Exposure.MaxExposureTime),
			MinGain:         convertFloatRange(raw.Exposure.MinGain),
			MaxGain:         convertFloatRange(raw.Exposure.MaxGain),
			MinIris:         convertFloatRange(raw.Exposure.MinIris),
			MaxIris:         convertFloatRange(raw.Exposure.MaxIris),
			ExposureTime:    convertFloatRange(raw.Exposure.ExposureTime),
			Gain:            convertFloatRange(raw.Exposure.Gain),
			Iris:            convertFloatRange(raw.Exposure.Iris),
		}
	}
	if raw.Focus != nil {
		o.Focus = &FocusOptions{
			AutoFocusModes: raw.Focus.AutoFocusModes,
			DefaultSpeed:   convertFloatRange(raw.Focus.DefaultSpeed),
			NearLimit:      convertFloatRange(raw.Focus.NearLimit),
			FarLimit:       convertFloatRange(raw.Focus.FarLimit),
		}
	}
	if raw.WideDynamicRange != nil {
		o.WideDynamicRange = &WideDynamicRangeOptions{
			Modes: raw.WideDynamicRange.Mode,
			Level: convertFloatRange(raw.WideDynamicRange.Level),
		}
	}
	if raw.WhiteBalance != nil {
		o.WhiteBalance = &WhiteBalanceOptions{
			Modes:  raw.WhiteBalance.Mode,
			YrGain: convertFloatRange(raw.WhiteBalance.YrGain),
			YbGain: convertFloatRange(raw.WhiteBalance.YbGain),
		}
	}
	return o
}

func moveOptionsFromRaw(raw *onvifgo.MoveOptions) *FocusMoveOptions {
	opts := &FocusMoveOptions{}
	if raw.Absolute != nil {
		opts.Absolute = &AbsoluteFocusMoveOptions{
			Position: FloatRange{Min: raw.Absolute.Position.Min, Max: raw.Absolute.Position.Max},
			Speed:    FloatRange{Min: raw.Absolute.Speed.Min, Max: raw.Absolute.Speed.Max},
		}
	}
	if raw.Relative != nil {
		opts.Relative = &RelativeFocusMoveOptions{
			Distance: FloatRange{Min: raw.Relative.Distance.Min, Max: raw.Relative.Distance.Max},
			Speed:    FloatRange{Min: raw.Relative.Speed.Min, Max: raw.Relative.Speed.Max},
		}
	}
	if raw.Continuous != nil {
		opts.Continuous = &ContinuousFocusMoveOptions{
			Speed: FloatRange{Min: raw.Continuous.Speed.Min, Max: raw.Continuous.Speed.Max},
		}
	}
	return opts
}

// getVideoSourceToken discovers the first video source token from the camera.
func getVideoSourceToken(ctx context.Context, dev *onvifgo.Client) (string, error) {
	sources, err := dev.GetVideoSources(ctx)
	if err != nil {
		return "", fmt.Errorf("get video sources: %w", err)
	}
	if len(sources) == 0 {
		return "", fmt.Errorf("no video sources found")
	}
	return sources[0].Token, nil
}

// derefFloat safely dereferences a *float64, returning 0 if nil.
func derefFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
