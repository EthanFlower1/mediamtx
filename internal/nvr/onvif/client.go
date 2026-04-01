package onvif

import (
	"context"
	"fmt"
	"strings"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// Client wraps an ONVIF device connection and exposes service discovery.
type Client struct {
	Dev      *onvifgo.Client
	Services map[string]string
	Username string
	Password string
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

	services := buildServiceMap(ctx, dev)

	return &Client{
		Dev:      dev,
		Services: services,
		Username: username,
		Password: password,
	}, nil
}

// buildServiceMap queries the device for service endpoints and maps
// namespace URLs to friendly names used by the custom SOAP code.
func buildServiceMap(ctx context.Context, dev *onvifgo.Client) map[string]string {
	services := make(map[string]string)

	svcs, err := dev.GetServices(ctx, false)
	if err != nil {
		return services
	}

	for _, svc := range svcs {
		ns := strings.ToLower(svc.Namespace)
		switch {
		case strings.Contains(ns, "ver20/media"):
			services["media2"] = svc.XAddr
		case strings.Contains(ns, "media"):
			services["media"] = svc.XAddr
		case strings.Contains(ns, "ptz"):
			services["ptz"] = svc.XAddr
		case strings.Contains(ns, "imaging"):
			services["imaging"] = svc.XAddr
		case strings.Contains(ns, "events") || strings.Contains(ns, "event"):
			services["events"] = svc.XAddr
		case strings.Contains(ns, "analytics"):
			services["analytics"] = svc.XAddr
		case strings.Contains(ns, "deviceio"):
			services["deviceio"] = svc.XAddr
		case strings.Contains(ns, "recording"):
			services["recording"] = svc.XAddr
		case strings.Contains(ns, "replay"):
			services["replay"] = svc.XAddr
		case strings.Contains(ns, "search"):
			services["search"] = svc.XAddr
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
