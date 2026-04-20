package driver

import "strings"

// Resolve returns the best Driver for a camera based on its manufacturer.
// Each driver is self-contained — no fallback chain. If a vendor driver
// can't be created, we fall back to creating an ONVIF driver instead.
func Resolve(manufacturer, onvifEndpoint, username, password string) Driver {
	mfr := strings.ToLower(manufacturer)

	switch {
	case strings.Contains(mfr, "amcrest") || strings.Contains(mfr, "dahua"):
		host := extractHost(onvifEndpoint)
		d, err := NewAmcrestDriver(host, username, password)
		if err == nil {
			return d
		}
		// If Amcrest client creation fails, use ONVIF.
	}

	// Add future drivers here:
	// case strings.Contains(mfr, "hikvision"):
	//     return NewHikvisionDriver(...)
	// case strings.Contains(mfr, "axis"):
	//     return NewAxisDriver(...)
	// case strings.Contains(mfr, "reolink"):
	//     return NewReolinkDriver(...)

	return &OnvifDriver{
		Endpoint: onvifEndpoint,
		Username: username,
		Password: password,
	}
}
