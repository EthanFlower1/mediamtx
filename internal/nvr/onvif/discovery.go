package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	onvifgo "github.com/EthanFlower1/onvif-go"
)

// MediaProfile represents a media profile on an ONVIF device.
type MediaProfile struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	StreamURI  string `json:"stream_uri"`
	VideoCodec string `json:"video_codec,omitempty"`
	AudioCodec string `json:"audio_codec,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	VideoSourceToken string `json:"video_source_token,omitempty"`
}

// DiscoveredDevice represents an ONVIF device found during a WS-Discovery scan.
type DiscoveredDevice struct {
	XAddr            string         `json:"xaddr"`
	Manufacturer     string         `json:"manufacturer"`
	Model            string         `json:"model"`
	Firmware         string         `json:"firmware"`
	AuthRequired     bool           `json:"auth_required"`
	ExistingCameraID string              `json:"existing_camera_id,omitempty"`
	Profiles         []MediaProfile      `json:"profiles,omitempty"`
	Channels         []DiscoveredChannel `json:"channels,omitempty"`
}

// DiscoveredChannel represents a single video channel on a multi-sensor device.
type DiscoveredChannel struct {
	VideoSourceToken string         `json:"video_source_token"`
	Name             string         `json:"name"`
	Profiles         []MediaProfile `json:"profiles"`
}

// ScanStatus represents the current state of a discovery scan.
type ScanStatus string

const (
	ScanStatusScanning ScanStatus = "scanning"
	ScanStatusComplete ScanStatus = "complete"
)

// ScanResult holds the state and results of a discovery scan.
type ScanResult struct {
	ScanID  string             `json:"scan_id"`
	Status  ScanStatus         `json:"status"`
	Devices []DiscoveredDevice `json:"devices"`
}

// Discovery manages ONVIF WS-Discovery scans.
type Discovery struct {
	mu     sync.Mutex
	result *ScanResult
}

// NewDiscovery returns a new Discovery instance.
func NewDiscovery() *Discovery {
	return &Discovery{}
}

// StartScan begins an asynchronous ONVIF discovery scan.
func (d *Discovery) StartScan() (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result != nil && d.result.Status == ScanStatusScanning {
		return "", ErrScanInProgress
	}

	scanID := uuid.New().String()
	d.result = &ScanResult{
		ScanID:  scanID,
		Status:  ScanStatusScanning,
		Devices: []DiscoveredDevice{},
	}

	go d.runScan(scanID)

	return scanID, nil
}

// GetStatus returns a copy of the current scan result.
func (d *Discovery) GetStatus() *ScanResult {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result == nil {
		return nil
	}

	devices := make([]DiscoveredDevice, len(d.result.Devices))
	copy(devices, d.result.Devices)
	return &ScanResult{
		ScanID:  d.result.ScanID,
		Status:  d.result.Status,
		Devices: devices,
	}
}

// GetResults returns the discovered devices from the most recent scan.
func (d *Discovery) GetResults() []DiscoveredDevice {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result == nil {
		return []DiscoveredDevice{}
	}

	devices := make([]DiscoveredDevice, len(d.result.Devices))
	copy(devices, d.result.Devices)
	return devices
}

const wsDiscoveryAddr = "239.255.255.250:3702"

func probeMessage(messageID string) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing"
            xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery"
            xmlns:dn="http://www.onvif.org/ver10/network/wsdl">
  <s:Header>
    <a:Action s:mustUnderstand="1">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
    <a:MessageID>uuid:%s</a:MessageID>
    <a:ReplyTo>
      <a:Address>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address>
    </a:ReplyTo>
    <a:To s:mustUnderstand="1">urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <d:Probe>
      <d:Types>dn:NetworkVideoTransmitter</d:Types>
    </d:Probe>
  </s:Body>
</s:Envelope>`, messageID))
}

type probeMatchEnvelope struct {
	XMLName xml.Name       `xml:"Envelope"`
	Body    probeMatchBody `xml:"Body"`
}

type probeMatchBody struct {
	ProbeMatches *probeMatches `xml:"ProbeMatches"`
}

type probeMatches struct {
	Matches []probeMatch `xml:"ProbeMatch"`
}

type probeMatch struct {
	XAddrs string `xml:"XAddrs"`
	Scopes string `xml:"Scopes"`
}

func (d *Discovery) runScan(scanID string) {
	devices := d.wsDiscoverDevices()

	// Enrich each device with profiles and stream URIs (no auth needed for many cameras).
	for i := range devices {
		d.enrichDevice(&devices[i])
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result != nil && d.result.ScanID == scanID {
		d.result.Devices = devices
		d.result.Status = ScanStatusComplete
	}
}

// enrichDevice connects to an ONVIF device and fetches its info, profiles, and stream URIs.
func (d *Discovery) enrichDevice(dev *DiscoveredDevice) {
	onvifDev, err := onvifgo.NewClient(dev.XAddr)
	if err != nil {
		dev.AuthRequired = true
		return
	}

	ctx := context.Background()
	if err := onvifDev.Initialize(ctx); err != nil {
		dev.AuthRequired = true
		return
	}

	// Fetch device info to fill in manufacturer/model/firmware if not from scopes.
	info, err := onvifDev.GetDeviceInformation(ctx)
	if err == nil && info != nil {
		if dev.Manufacturer == "" {
			dev.Manufacturer = info.Manufacturer
		}
		if dev.Model == "" {
			dev.Model = info.Model
		}
		if dev.Firmware == "" {
			dev.Firmware = info.FirmwareVersion
		}
	}

	// Fetch media profiles.
	rawProfiles, err := onvifDev.GetProfiles(ctx)
	if err != nil {
		dev.AuthRequired = true
		return
	}

	for _, p := range rawProfiles {
		mp := profileToMediaProfile(p)

		// Get RTSP stream URI.
		streamResp, err := onvifDev.GetStreamURI(ctx, p.Token)
		if err == nil && streamResp != nil {
			mp.StreamURI = streamResp.URI
		}

		dev.Profiles = append(dev.Profiles, mp)
	}

	// Group profiles by video source to detect multi-channel devices.
	if len(dev.Profiles) > 0 {
		channels := GroupProfilesByVideoSource(dev.Profiles)
		if len(channels) > 1 {
			dev.Channels = channels
		}
	}
}

// xaddrToHost extracts the host:port from an ONVIF XAddr URL.
func xaddrToHost(xaddr string) string {
	u, err := url.Parse(xaddr)
	if err != nil {
		return ""
	}
	return u.Host
}

func (d *Discovery) wsDiscoverDevices() []DiscoveredDevice {
	addr, err := net.ResolveUDPAddr("udp4", wsDiscoveryAddr)
	if err != nil {
		return nil
	}

	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil
	}
	defer conn.Close()

	messageID := uuid.New().String()
	probe := probeMessage(messageID)

	for i := 0; i < 3; i++ {
		_, _ = conn.WriteToUDP(probe, addr)
		time.Sleep(100 * time.Millisecond)
	}

	conn.SetReadDeadline(time.Now().Add(4 * time.Second))

	seen := make(map[string]bool)
	var devices []DiscoveredDevice

	buf := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}

		var env probeMatchEnvelope
		if xmlErr := xml.Unmarshal(buf[:n], &env); xmlErr != nil {
			continue
		}

		if env.Body.ProbeMatches == nil {
			continue
		}

		for _, match := range env.Body.ProbeMatches.Matches {
			for _, xaddr := range strings.Fields(match.XAddrs) {
				if seen[xaddr] {
					continue
				}
				seen[xaddr] = true

				dev := DiscoveredDevice{XAddr: xaddr}
				parseScopes(match.Scopes, &dev)
				devices = append(devices, dev)
			}
		}
	}

	return devices
}

func parseScopes(scopes string, dev *DiscoveredDevice) {
	for _, scope := range strings.Fields(scopes) {
		scope = strings.TrimRight(scope, "/")
		parts := strings.Split(scope, "/")
		if len(parts) < 2 {
			continue
		}
		value := parts[len(parts)-1]
		category := parts[len(parts)-2]

		switch strings.ToLower(category) {
		case "name":
			if dev.Model == "" {
				dev.Model = value
			}
		case "hardware":
			dev.Model = value
		case "manufacturer":
			dev.Manufacturer = value
		case "firmware":
			dev.Firmware = value
		}
	}
}

// GroupProfilesByVideoSource groups profiles by their VideoSourceToken.
// Returns one DiscoveredChannel per unique video source, sorted by token.
// If all profiles have empty VideoSourceToken, returns a single channel.
func GroupProfilesByVideoSource(profiles []MediaProfile) []DiscoveredChannel {
	groups := make(map[string][]MediaProfile)
	var order []string

	for _, p := range profiles {
		token := p.VideoSourceToken
		if _, seen := groups[token]; !seen {
			order = append(order, token)
		}
		groups[token] = append(groups[token], p)
	}

	sort.Strings(order)

	channels := make([]DiscoveredChannel, 0, len(order))
	for i, token := range order {
		channels = append(channels, DiscoveredChannel{
			VideoSourceToken: token,
			Name:             fmt.Sprintf("Channel %d", i+1),
			Profiles:         groups[token],
		})
	}
	return channels
}
