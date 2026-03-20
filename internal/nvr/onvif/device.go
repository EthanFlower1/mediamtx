package onvif

import "errors"

// ErrNotImplemented is returned by stub methods that are not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// Device wraps an ONVIF device connection for camera management.
type Device struct {
	XAddr    string
	Username string
	Password string
}

// NewDevice creates a new Device instance for the given ONVIF endpoint.
func NewDevice(xaddr, username, password string) *Device {
	return &Device{
		XAddr:    xaddr,
		Username: username,
		Password: password,
	}
}

// GetStreamURI returns the RTSP stream URI for the given profile token.
// This is a stub that will be implemented when ONVIF library support is available.
func (d *Device) GetStreamURI(profileToken string) (string, error) {
	return "", ErrNotImplemented
}
