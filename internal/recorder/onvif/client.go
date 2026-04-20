package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// Client wraps an ONVIF device connection and exposes service discovery.
type Client struct {
	Dev                  *onvifgo.Client
	Services             map[string]string
	ServiceInfos         []ServiceInfo
	DetailedCapabilities *DetailedCapabilities
	Username             string
	Password             string
}

// NewClient connects to an ONVIF device at xaddr with the given credentials
// and returns a Client ready for service calls.
func NewClient(xaddr, username, password string) (*Client, error) {
	var opts []onvifgo.ClientOption
	if username != "" {
		opts = append(opts, onvifgo.WithCredentials(username, password))
	}

	dev, err := onvifgo.NewClient(xaddr, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	ctx := context.Background()
	if err := dev.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("connect to ONVIF device: %w", err)
	}

	serviceInfos := getServicesDetailed(ctx, dev)
	services := buildServiceMapFromInfos(serviceInfos)

	c := &Client{
		Dev:          dev,
		Services:     services,
		ServiceInfos: serviceInfos,
		Username:     username,
		Password:     password,
	}

	c.DetailedCapabilities = queryDetailedCapabilities(ctx, c)

	return c, nil
}

// buildServiceMapFromInfos builds a friendly-name → XAddr map from ServiceInfo entries.
func buildServiceMapFromInfos(infos []ServiceInfo) map[string]string {
	services := make(map[string]string)
	for _, info := range infos {
		name := friendlyServiceName(info.Namespace)
		if name != "" {
			services[name] = info.XAddr
		}
	}
	return services
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
