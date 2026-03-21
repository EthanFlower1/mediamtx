package onvif

import (
	"fmt"

	onviflib "github.com/use-go/onvif"
)

// Client wraps an ONVIF device connection and exposes service discovery.
type Client struct {
	Dev      *onviflib.Device
	Services map[string]string
	Username string
	Password string
}

// NewClient connects to an ONVIF device at xaddr with the given credentials
// and returns a Client ready for service calls.
func NewClient(xaddr, username, password string) (*Client, error) {
	host := xaddrToHost(xaddr)
	if host == "" {
		host = xaddr
	}
	dev, err := onviflib.NewDevice(onviflib.DeviceParams{
		Xaddr:    host,
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}
	return &Client{
		Dev:      dev,
		Services: dev.GetServices(),
		Username: username,
		Password: password,
	}, nil
}

// HasService reports whether the device advertises the named ONVIF service.
func (c *Client) HasService(name string) bool {
	_, ok := c.Services[name]
	return ok
}

// ServiceURL returns the endpoint URL for the named service, or "" if absent.
func (c *Client) ServiceURL(name string) string {
	return c.Services[name]
}

// Capabilities summarises which ONVIF services the device supports.
type Capabilities struct {
	Media            bool `json:"media"`
	Media2           bool `json:"media2"`
	PTZ              bool `json:"ptz"`
	Imaging          bool `json:"imaging"`
	Events           bool `json:"events"`
	Analytics        bool `json:"analytics"`
	DeviceIO         bool `json:"device_io"`
	Recording        bool `json:"recording"`
	Replay           bool `json:"replay"`
	AudioBackchannel bool `json:"audio_backchannel"`
}

// GetCapabilities returns a Capabilities struct based on the device's
// advertised services. AudioBackchannel detection requires querying audio
// outputs and is not yet implemented (always false).
func (c *Client) GetCapabilities() Capabilities {
	return Capabilities{
		Media:     c.HasService("media"),
		Media2:    c.HasService("media2"),
		PTZ:       c.HasService("ptz"),
		Imaging:   c.HasService("imaging"),
		Events:    c.HasService("events") || c.HasService("event"),
		Analytics: c.HasService("analytics"),
		DeviceIO:  c.HasService("deviceio"),
		Recording: c.HasService("recording"),
		Replay:    c.HasService("replay"),
	}
}
