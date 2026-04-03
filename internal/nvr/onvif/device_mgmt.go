package onvif

import (
	"context"
	"fmt"
	"net"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// DateTimeInfo holds system date/time information from a device.
type DateTimeInfo struct {
	Type           string `json:"type"`
	DaylightSaving bool   `json:"daylight_saving"`
	Timezone       string `json:"timezone"`
	UTCTime        string `json:"utc_time"`
	LocalTime      string `json:"local_time"`
}

// HostnameInfo holds device hostname information.
type HostnameInfo struct {
	FromDHCP bool   `json:"from_dhcp"`
	Name     string `json:"name"`
}

// NetworkInterfaceInfo holds information about a single network interface.
type NetworkInterfaceInfo struct {
	Token   string      `json:"token"`
	Enabled bool        `json:"enabled"`
	MAC     string      `json:"mac"`
	IPv4    *IPv4Config `json:"ipv4,omitempty"`
}

// IPv4Config holds IPv4 configuration for a network interface.
type IPv4Config struct {
	Enabled bool   `json:"enabled"`
	DHCP    bool   `json:"dhcp"`
	Address string `json:"address"`
	Prefix  int    `json:"prefix_length"`
}

// NetworkProtocolInfo holds information about a single network protocol.
type NetworkProtocolInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
}

// DNSInfo holds DNS configuration from a device.
type DNSInfo struct {
	FromDHCP bool     `json:"from_dhcp"`
	Servers  []string `json:"servers"`
}

// NTPInfo holds NTP configuration from a device.
type NTPInfo struct {
	FromDHCP bool     `json:"from_dhcp"`
	Servers  []string `json:"servers"`
}

// DeviceUser holds information about a user account on the device.
type DeviceUser struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// GetSystemDateAndTime retrieves the system date and time from the device.
// The library returns interface{} so we do a best-effort parse via type assertion.
func GetSystemDateAndTime(xaddr, user, pass string) (*DateTimeInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	raw, err := client.Dev.GetSystemDateAndTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetSystemDateAndTime: %w", err)
	}

	info := &DateTimeInfo{}

	if sdt, ok := raw.(*onvifgo.SystemDateTime); ok {
		info.Type = string(sdt.DateTimeType)
		info.DaylightSaving = sdt.DaylightSavings
		if sdt.TimeZone != nil {
			info.Timezone = sdt.TimeZone.TZ
		}
		if sdt.UTCDateTime != nil {
			info.UTCTime = fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d",
				sdt.UTCDateTime.Date.Year,
				sdt.UTCDateTime.Date.Month,
				sdt.UTCDateTime.Date.Day,
				sdt.UTCDateTime.Time.Hour,
				sdt.UTCDateTime.Time.Minute,
				sdt.UTCDateTime.Time.Second,
			)
		}
		if sdt.LocalDateTime != nil {
			info.LocalTime = fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d",
				sdt.LocalDateTime.Date.Year,
				sdt.LocalDateTime.Date.Month,
				sdt.LocalDateTime.Date.Day,
				sdt.LocalDateTime.Time.Hour,
				sdt.LocalDateTime.Time.Minute,
				sdt.LocalDateTime.Time.Second,
			)
		}
	}

	return info, nil
}

// GetDeviceHostname retrieves the hostname configuration from the device.
func GetDeviceHostname(xaddr, user, pass string) (*HostnameInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	h, err := client.Dev.GetHostname(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetHostname: %w", err)
	}

	return &HostnameInfo{
		FromDHCP: h.FromDHCP,
		Name:     h.Name,
	}, nil
}

// SetDeviceHostname sets the hostname on the device.
func SetDeviceHostname(xaddr, user, pass, name string) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	if err := client.Dev.SetHostname(ctx, name); err != nil {
		return fmt.Errorf("SetHostname: %w", err)
	}

	return nil
}

// DeviceReboot reboots the device and returns the reboot message.
func DeviceReboot(xaddr, user, pass string) (string, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return "", fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	msg, err := client.Dev.SystemReboot(ctx)
	if err != nil {
		return "", fmt.Errorf("SystemReboot: %w", err)
	}

	return msg, nil
}

// GetNetworkInterfaces retrieves all network interfaces from the device.
func GetNetworkInterfaces(xaddr, user, pass string) ([]*NetworkInterfaceInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	ifaces, err := client.Dev.GetNetworkInterfaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetNetworkInterfaces: %w", err)
	}

	result := make([]*NetworkInterfaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		info := &NetworkInterfaceInfo{
			Token:   iface.Token,
			Enabled: iface.Enabled,
			MAC:     iface.Info.HwAddress,
		}

		if iface.IPv4 != nil {
			cfg := &IPv4Config{
				Enabled: iface.IPv4.Enabled,
				DHCP:    iface.IPv4.Config.DHCP,
			}
			if len(iface.IPv4.Config.Manual) > 0 {
				cfg.Address = iface.IPv4.Config.Manual[0].Address
				cfg.Prefix = iface.IPv4.Config.Manual[0].PrefixLength
			}
			info.IPv4 = cfg
		}

		result = append(result, info)
	}

	return result, nil
}

// GetNetworkProtocols retrieves the network protocols configured on the device.
func GetNetworkProtocols(xaddr, user, pass string) ([]*NetworkProtocolInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	protos, err := client.Dev.GetNetworkProtocols(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetNetworkProtocols: %w", err)
	}

	result := make([]*NetworkProtocolInfo, 0, len(protos))
	for _, p := range protos {
		info := &NetworkProtocolInfo{
			Name:    string(p.Name),
			Enabled: p.Enabled,
		}
		if len(p.Port) > 0 {
			info.Port = p.Port[0]
		}
		result = append(result, info)
	}

	return result, nil
}

// SetNetworkProtocols sets the network protocols on the device.
func SetNetworkProtocols(xaddr, user, pass string, protocols []*NetworkProtocolInfo) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	protos := make([]*onvifgo.NetworkProtocol, 0, len(protocols))
	for _, p := range protocols {
		protos = append(protos, &onvifgo.NetworkProtocol{
			Name:    onvifgo.NetworkProtocolType(p.Name),
			Enabled: p.Enabled,
			Port:    []int{p.Port},
		})
	}

	if err := client.Dev.SetNetworkProtocols(ctx, protos); err != nil {
		return fmt.Errorf("SetNetworkProtocols: %w", err)
	}

	return nil
}

// GetDeviceUsers retrieves the user accounts configured on the device.
func GetDeviceUsers(xaddr, user, pass string) ([]*DeviceUser, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	users, err := client.Dev.GetUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetUsers: %w", err)
	}

	result := make([]*DeviceUser, 0, len(users))
	for _, u := range users {
		result = append(result, &DeviceUser{
			Username: u.Username,
			Role:     u.UserLevel,
		})
	}

	return result, nil
}

// CreateDeviceUser creates a new user account on the device.
func CreateDeviceUser(xaddr, adminUser, adminPass, username, password, role string) error {
	client, err := NewClient(xaddr, adminUser, adminPass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	users := []*onvifgo.User{
		{
			Username:  username,
			Password:  password,
			UserLevel: role,
		},
	}

	if err := client.Dev.CreateUsers(ctx, users); err != nil {
		return fmt.Errorf("CreateUsers: %w", err)
	}

	return nil
}

// DeleteDeviceUser deletes a user account from the device.
func DeleteDeviceUser(xaddr, adminUser, adminPass, username string) error {
	client, err := NewClient(xaddr, adminUser, adminPass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	if err := client.Dev.DeleteUsers(ctx, []string{username}); err != nil {
		return fmt.Errorf("DeleteUsers: %w", err)
	}

	return nil
}

// SetDeviceUser updates an existing user account on the device.
func SetDeviceUser(xaddr, adminUser, adminPass, username, password, role string) error {
	client, err := NewClient(xaddr, adminUser, adminPass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	u := &onvifgo.User{
		Username:  username,
		Password:  password,
		UserLevel: role,
	}

	if err := client.Dev.SetUser(ctx, u); err != nil {
		return fmt.Errorf("SetUser: %w", err)
	}

	return nil
}

// GetDeviceScopes retrieves the scopes configured on the device.
func GetDeviceScopes(xaddr, user, pass string) ([]string, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	scopes, err := client.Dev.GetScopes(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetScopes: %w", err)
	}

	result := make([]string, 0, len(scopes))
	for _, s := range scopes {
		result = append(result, s.ScopeItem)
	}

	return result, nil
}

// GetNTPConfig retrieves the NTP configuration from the device.
func GetNTPConfig(xaddr, user, pass string) (*NTPInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	ntp, err := client.Dev.GetNTP(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetNTP: %w", err)
	}

	info := &NTPInfo{
		FromDHCP: ntp.FromDHCP,
	}

	// Prefer DHCP NTP servers if available, otherwise manual.
	hosts := ntp.NTPFromDHCP
	if len(hosts) == 0 {
		hosts = ntp.NTPManual
	}

	for _, h := range hosts {
		switch h.Type {
		case "DNS":
			info.Servers = append(info.Servers, h.DNSname)
		case "IPv6":
			info.Servers = append(info.Servers, h.IPv6Address)
		default:
			info.Servers = append(info.Servers, h.IPv4Address)
		}
	}

	return info, nil
}

// SetDateTimeRequest is the request body for SetSystemDateAndTime.
type SetDateTimeRequest struct {
	Type           string   `json:"type"`
	DaylightSaving bool     `json:"daylight_saving"`
	Timezone       string   `json:"timezone"`
	UTCDateTime    *DateVal `json:"utc_date_time,omitempty"`
}

// DateVal holds a date/time value for set requests.
type DateVal struct {
	Year   int `json:"year"`
	Month  int `json:"month"`
	Day    int `json:"day"`
	Hour   int `json:"hour"`
	Minute int `json:"minute"`
	Second int `json:"second"`
}

// SetDNSRequest is the request body for SetDNS.
type SetDNSRequest struct {
	FromDHCP bool     `json:"from_dhcp"`
	Servers  []string `json:"servers"`
}

// SetNTPRequest is the request body for SetNTP.
type SetNTPRequest struct {
	FromDHCP bool     `json:"from_dhcp"`
	Servers  []string `json:"servers"`
}

// SetNetworkInterfaceRequest is the request body for SetNetworkInterfaces.
type SetNetworkInterfaceRequest struct {
	Enabled *bool       `json:"enabled,omitempty"`
	IPv4    *IPv4Config `json:"ipv4,omitempty"`
}

// SetNetworkInterfaceResponse is the response from SetNetworkInterfaces.
type SetNetworkInterfaceResponse struct {
	RebootNeeded bool `json:"reboot_needed"`
}

// GatewayInfo holds network gateway configuration.
type GatewayInfo struct {
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
}

// ScopesRequest is the request body for scope operations.
type ScopesRequest struct {
	Scopes []string `json:"scopes"`
}

// DiscoveryModeInfo holds the discovery mode for the device.
type DiscoveryModeInfo struct {
	Mode string `json:"mode"`
}

// SystemLogInfo holds system log content.
type SystemLogInfo struct {
	Content string `json:"content"`
}

// SupportInfo holds system support information.
type SupportInfo struct {
	Content string `json:"content"`
}

// GetDNSConfig retrieves the DNS configuration from the device.
func GetDNSConfig(xaddr, user, pass string) (*DNSInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	dns, err := client.Dev.GetDNS(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetDNS: %w", err)
	}

	info := &DNSInfo{
		FromDHCP: dns.FromDHCP,
	}

	// Prefer DHCP DNS servers if available, otherwise manual.
	addrs := dns.DNSFromDHCP
	if len(addrs) == 0 {
		addrs = dns.DNSManual
	}

	for _, a := range addrs {
		switch a.Type {
		case "IPv6":
			info.Servers = append(info.Servers, a.IPv6Address)
		default:
			info.Servers = append(info.Servers, a.IPv4Address)
		}
	}

	return info, nil
}

// ValidateIP returns an error if s is not a valid IPv4 or IPv6 address.
func ValidateIP(s string) error {
	if net.ParseIP(s) == nil {
		return fmt.Errorf("invalid IP address: %s", s)
	}
	return nil
}

// SetSystemDateAndTime sets the system date and time on the device.
func SetSystemDateAndTime(xaddr, user, pass string, req *SetDateTimeRequest) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	sdt := &onvifgo.SystemDateTime{
		DateTimeType:    onvifgo.SetDateTimeType(req.Type),
		DaylightSavings: req.DaylightSaving,
		TimeZone:        &onvifgo.TimeZone{TZ: req.Timezone},
	}

	if req.UTCDateTime != nil {
		sdt.UTCDateTime = &onvifgo.DateTime{
			Date: onvifgo.Date{
				Year:  req.UTCDateTime.Year,
				Month: req.UTCDateTime.Month,
				Day:   req.UTCDateTime.Day,
			},
			Time: onvifgo.Time{
				Hour:   req.UTCDateTime.Hour,
				Minute: req.UTCDateTime.Minute,
				Second: req.UTCDateTime.Second,
			},
		}
	}

	if err := client.Dev.SetSystemDateAndTime(ctx, sdt); err != nil {
		return fmt.Errorf("SetSystemDateAndTime: %w", err)
	}

	return nil
}

// SetDNSConfig sets the DNS configuration on the device.
func SetDNSConfig(xaddr, user, pass string, req *SetDNSRequest) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	manual := make([]onvifgo.IPAddress, 0, len(req.Servers))
	for _, s := range req.Servers {
		ip := net.ParseIP(s)
		addr := onvifgo.IPAddress{Type: "IPv4", IPv4Address: s}
		if ip != nil && ip.To4() == nil {
			addr.Type = "IPv6"
			addr.IPv6Address = s
			addr.IPv4Address = ""
		}
		manual = append(manual, addr)
	}

	if err := client.Dev.SetDNS(ctx, req.FromDHCP, nil, manual); err != nil {
		return fmt.Errorf("SetDNS: %w", err)
	}

	return nil
}

// SetNTPConfig sets the NTP configuration on the device.
func SetNTPConfig(xaddr, user, pass string, req *SetNTPRequest) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	manual := make([]onvifgo.NetworkHost, 0, len(req.Servers))
	for _, s := range req.Servers {
		host := onvifgo.NetworkHost{}
		ip := net.ParseIP(s)
		if ip == nil {
			host.Type = "DNS"
			host.DNSname = s
		} else if ip.To4() != nil {
			host.Type = "IPv4"
			host.IPv4Address = s
		} else {
			host.Type = "IPv6"
			host.IPv6Address = s
		}
		manual = append(manual, host)
	}

	if err := client.Dev.SetNTP(ctx, req.FromDHCP, manual); err != nil {
		return fmt.Errorf("SetNTP: %w", err)
	}

	return nil
}

// SetNetworkInterface sets the configuration of a network interface on the device.
// Returns true if the device requires a reboot for changes to take effect.
func SetNetworkInterface(xaddr, user, pass, token string, req *SetNetworkInterfaceRequest) (bool, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return false, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	config := &onvifgo.NetworkInterfaceSetConfiguration{
		Enabled: req.Enabled,
	}

	if req.IPv4 != nil {
		config.IPv4 = &onvifgo.IPv4NetworkInterfaceSetConfiguration{
			Enabled: &req.IPv4.Enabled,
			DHCP:    &req.IPv4.DHCP,
		}
	}

	reboot, err := client.Dev.SetNetworkInterfaces(ctx, token, config)
	if err != nil {
		return false, fmt.Errorf("SetNetworkInterfaces: %w", err)
	}

	return reboot, nil
}

// GetNetworkDefaultGateway retrieves the default gateway from the device.
func GetNetworkDefaultGateway(xaddr, user, pass string) (*GatewayInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	gw, err := client.Dev.GetNetworkDefaultGateway(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetNetworkDefaultGateway: %w", err)
	}

	info := &GatewayInfo{
		IPv4: gw.IPv4Address,
		IPv6: gw.IPv6Address,
	}
	if info.IPv4 == nil {
		info.IPv4 = []string{}
	}
	if info.IPv6 == nil {
		info.IPv6 = []string{}
	}

	return info, nil
}

// SetNetworkDefaultGateway sets the default gateway on the device.
func SetNetworkDefaultGateway(xaddr, user, pass string, req *GatewayInfo) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	gw := &onvifgo.NetworkGateway{
		IPv4Address: req.IPv4,
		IPv6Address: req.IPv6,
	}

	if err := client.Dev.SetNetworkDefaultGateway(ctx, gw); err != nil {
		return fmt.Errorf("SetNetworkDefaultGateway: %w", err)
	}

	return nil
}

// SetDeviceScopes replaces all configurable scopes on the device.
func SetDeviceScopes(xaddr, user, pass string, scopes []string) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	if err := client.Dev.SetScopes(ctx, scopes); err != nil {
		return fmt.Errorf("SetScopes: %w", err)
	}

	return nil
}

// AddDeviceScopes adds scopes to the device.
func AddDeviceScopes(xaddr, user, pass string, scopes []string) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	if err := client.Dev.AddScopes(ctx, scopes); err != nil {
		return fmt.Errorf("AddScopes: %w", err)
	}

	return nil
}

// RemoveDeviceScopes removes scopes from the device.
// Returns the remaining scopes after removal.
func RemoveDeviceScopes(xaddr, user, pass string, scopes []string) ([]string, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	remaining, err := client.Dev.RemoveScopes(ctx, scopes)
	if err != nil {
		return nil, fmt.Errorf("RemoveScopes: %w", err)
	}

	return remaining, nil
}

// GetDiscoveryMode retrieves the WS-Discovery mode from the device.
func GetDiscoveryMode(xaddr, user, pass string) (*DiscoveryModeInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	mode, err := client.Dev.GetDiscoveryMode(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetDiscoveryMode: %w", err)
	}

	return &DiscoveryModeInfo{Mode: string(mode)}, nil
}

// SetDiscoveryMode sets the WS-Discovery mode on the device.
func SetDiscoveryMode(xaddr, user, pass, mode string) error {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	if err := client.Dev.SetDiscoveryMode(ctx, onvifgo.DiscoveryMode(mode)); err != nil {
		return fmt.Errorf("SetDiscoveryMode: %w", err)
	}

	return nil
}

// GetSystemLog retrieves the system log from the device.
func GetSystemLog(xaddr, user, pass, logType string) (*SystemLogInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	log, err := client.Dev.GetSystemLog(ctx, onvifgo.SystemLogType(logType))
	if err != nil {
		return nil, fmt.Errorf("GetSystemLog: %w", err)
	}

	return &SystemLogInfo{Content: log.String}, nil
}

// GetSystemSupportInformation retrieves support information from the device.
func GetSystemSupportInformation(xaddr, user, pass string) (*SupportInfo, error) {
	client, err := NewClient(xaddr, user, pass)
	if err != nil {
		return nil, fmt.Errorf("connect to device: %w", err)
	}

	ctx := context.Background()

	info, err := client.Dev.GetSystemSupportInformation(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetSystemSupportInformation: %w", err)
	}

	return &SupportInfo{Content: info.String}, nil
}
