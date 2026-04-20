package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
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
	LastSeen         time.Time           `json:"last_seen"`
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

// DiscoveryConfig holds configuration for the Discovery engine.
type DiscoveryConfig struct {
	// StaleTimeout is how long a device can go unseen before removal.
	// Zero means no stale cleanup (devices persist until next scan).
	StaleTimeout time.Duration
	// ProbeInterval is the delay between repeated probe packets per interface.
	ProbeInterval time.Duration
	// ProbeCount is the number of probe packets sent per interface.
	ProbeCount int
	// ListenDuration is how long to listen for responses after sending probes.
	ListenDuration time.Duration
}

// DefaultDiscoveryConfig returns sensible defaults for production use.
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		StaleTimeout:   5 * time.Minute,
		ProbeInterval:  100 * time.Millisecond,
		ProbeCount:     3,
		ListenDuration: 4 * time.Second,
	}
}

// Discovery manages ONVIF WS-Discovery scans.
type Discovery struct {
	mu      sync.Mutex
	result  *ScanResult
	config  DiscoveryConfig
	devices map[string]*DiscoveredDevice // keyed by host IP for dedup across scans
}

// NewDiscovery returns a new Discovery instance with default config.
func NewDiscovery() *Discovery {
	return NewDiscoveryWithConfig(DefaultDiscoveryConfig())
}

// NewDiscoveryWithConfig returns a Discovery instance with the given config.
func NewDiscoveryWithConfig(cfg DiscoveryConfig) *Discovery {
	return &Discovery{
		config:  cfg,
		devices: make(map[string]*DiscoveredDevice),
	}
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
	now := time.Now()
	rawDevices := d.wsDiscoverDevices()

	// Merge newly discovered devices into the persistent device map,
	// deduplicating by host IP across interfaces and scans.
	d.mu.Lock()
	for i := range rawDevices {
		dev := &rawDevices[i]
		dev.LastSeen = now
		hostKey := normalizeHostKey(dev.XAddr)
		if hostKey == "" {
			continue
		}
		if existing, ok := d.devices[hostKey]; ok {
			// Update the existing entry — prefer richer metadata.
			existing.LastSeen = now
			existing.XAddr = dev.XAddr
			if dev.Manufacturer != "" {
				existing.Manufacturer = dev.Manufacturer
			}
			if dev.Model != "" {
				existing.Model = dev.Model
			}
			if dev.Firmware != "" {
				existing.Firmware = dev.Firmware
			}
		} else {
			d.devices[hostKey] = dev
		}
	}

	// Remove stale devices that haven't been seen within the timeout.
	if d.config.StaleTimeout > 0 {
		cutoff := now.Add(-d.config.StaleTimeout)
		for key, dev := range d.devices {
			if dev.LastSeen.Before(cutoff) {
				delete(d.devices, key)
			}
		}
	}
	d.mu.Unlock()

	// Enrich each newly-discovered device outside the lock.
	for i := range rawDevices {
		d.enrichDevice(&rawDevices[i])
	}

	// Build final device list from the persistent map, applying enrichment.
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.result == nil || d.result.ScanID != scanID {
		return
	}

	// Apply enrichment data back to persistent entries.
	for i := range rawDevices {
		dev := &rawDevices[i]
		hostKey := normalizeHostKey(dev.XAddr)
		if hostKey == "" {
			continue
		}
		if existing, ok := d.devices[hostKey]; ok {
			existing.AuthRequired = dev.AuthRequired
			existing.Profiles = dev.Profiles
			if dev.Manufacturer != "" {
				existing.Manufacturer = dev.Manufacturer
			}
			if dev.Model != "" {
				existing.Model = dev.Model
			}
			if dev.Firmware != "" {
				existing.Firmware = dev.Firmware
			}
		}
	}

	devices := make([]DiscoveredDevice, 0, len(d.devices))
	for _, dev := range d.devices {
		devices = append(devices, *dev)
	}

	d.result.Devices = devices
	d.result.Status = ScanStatusComplete
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

// normalizeHostKey extracts a canonical host (IP or hostname, without port)
// from an XAddr URL for deduplication across interfaces.
func normalizeHostKey(xaddr string) string {
	u, err := url.Parse(xaddr)
	if err != nil {
		return ""
	}
	host := u.Hostname() // strips port
	if host == "" {
		return ""
	}
	// Resolve to IP if possible for consistent dedup.
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	// Hostname — try to resolve.
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return host
	}
	return ips[0].String()
}

// xaddrToHost extracts the host:port from an ONVIF XAddr URL.
func xaddrToHost(xaddr string) string {
	u, err := url.Parse(xaddr)
	if err != nil {
		return ""
	}
	return u.Host
}

// discoverableInterfaces returns all non-loopback, up, IPv4-capable interfaces.
func discoverableInterfaces() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var result []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		hasIPv4 := false
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if ok && ipNet.IP.To4() != nil {
				hasIPv4 = true
				break
			}
		}
		if hasIPv4 {
			result = append(result, iface)
		}
	}

	return result
}

// interfaceIPv4Addr returns the first IPv4 address on the given interface.
func interfaceIPv4Addr(iface net.Interface) net.IP {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if ok && ipNet.IP.To4() != nil {
			return ipNet.IP
		}
	}
	return nil
}

func (d *Discovery) wsDiscoverDevices() []DiscoveredDevice {
	mcastAddr, err := net.ResolveUDPAddr("udp4", wsDiscoveryAddr)
	if err != nil {
		return nil
	}

	ifaces := discoverableInterfaces()
	if len(ifaces) == 0 {
		// Fallback: bind to all interfaces if enumeration fails.
		return d.probeOnAddr(nil, mcastAddr)
	}

	// Send probes on each interface concurrently, collect all responses.
	type ifaceResult struct {
		devices []DiscoveredDevice
	}
	results := make([]ifaceResult, len(ifaces))
	var wg sync.WaitGroup

	for i, iface := range ifaces {
		wg.Add(1)
		go func(idx int, ifc net.Interface) {
			defer wg.Done()
			localIP := interfaceIPv4Addr(ifc)
			if localIP == nil {
				return
			}
			localAddr := &net.UDPAddr{IP: localIP, Port: 0}
			devs := d.probeOnAddr(localAddr, mcastAddr)
			results[idx] = ifaceResult{devices: devs}
			if len(devs) > 0 {
				log.Printf("ONVIF discovery: found %d device(s) on interface %s (%s)",
					len(devs), ifc.Name, localIP)
			}
		}(i, iface)
	}
	wg.Wait()

	// Deduplicate across interfaces by host IP.
	seen := make(map[string]bool)
	var devices []DiscoveredDevice

	for _, r := range results {
		for _, dev := range r.devices {
			key := normalizeHostKey(dev.XAddr)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			devices = append(devices, dev)
		}
	}

	return devices
}

// probeOnAddr sends WS-Discovery probes from localAddr and collects responses.
// If localAddr is nil, binds to all interfaces (fallback).
func (d *Discovery) probeOnAddr(localAddr *net.UDPAddr, mcastAddr *net.UDPAddr) []DiscoveredDevice {
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return nil
	}
	defer conn.Close()

	messageID := uuid.New().String()
	probe := probeMessage(messageID)

	for i := 0; i < d.config.ProbeCount; i++ {
		_, _ = conn.WriteToUDP(probe, mcastAddr)
		if i < d.config.ProbeCount-1 {
			time.Sleep(d.config.ProbeInterval)
		}
	}

	conn.SetReadDeadline(time.Now().Add(d.config.ListenDuration))

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
