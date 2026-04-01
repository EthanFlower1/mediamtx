package onvif

import (
	"context"
	"fmt"

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
