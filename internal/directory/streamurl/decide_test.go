package streamurl

import (
	"net/netip"
	"reflect"
	"testing"
)

func mustPrefix(t *testing.T, s string) netip.Prefix {
	t.Helper()
	p, err := netip.ParsePrefix(s)
	if err != nil {
		t.Fatalf("ParsePrefix(%q): %v", s, err)
	}
	return p
}

func mustAddr(t *testing.T, s string) netip.Addr {
	t.Helper()
	a, err := netip.ParseAddr(s)
	if err != nil {
		t.Fatalf("ParseAddr(%q): %v", s, err)
	}
	return a
}

func TestDecide(t *testing.T) {
	tests := []struct {
		name   string
		client ClientHint
		rec    Recorder
		want   []Endpoint
	}{
		{
			name: "client IP inside LAN CIDR -> LAN first then gateway then cloud",
			client: ClientHint{
				IP: mustAddr(t, "10.0.0.42"),
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  true,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointLAN, URL: "rtsps://rec.local:8322/cam1"},
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
		{
			name: "client outside LAN, Tier2 enabled -> Gateway first",
			client: ClientHint{
				IP: mustAddr(t, "203.0.113.5"),
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  true,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
		{
			name: "client outside, Tier2 disabled, Tier3 enabled -> cloud relay only",
			client: ClientHint{
				IP: mustAddr(t, "203.0.113.5"),
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  false,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
		{
			name: "is_lan=true override promotes LAN even when IP is not in any CIDR",
			client: ClientHint{
				IP:    mustAddr(t, "198.51.100.9"),
				IsLAN: true,
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  true,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointLAN, URL: "rtsps://rec.local:8322/cam1"},
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
		{
			name: "is_lan=true but no LANURL configured -> LAN suppressed",
			client: ClientHint{
				IsLAN: true,
			},
			rec: Recorder{
				LANCIDRs:     []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:       "",
				GatewayURL:   "https://gw.example.com/cam1",
				Tier2Enabled: true,
			},
			want: []Endpoint{
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
			},
		},
		{
			name: "multi-CIDR recorder, client matches second CIDR -> LAN first",
			client: ClientHint{
				IP: mustAddr(t, "192.168.20.7"),
			},
			rec: Recorder{
				LANCIDRs: []netip.Prefix{
					mustPrefix(t, "10.0.0.0/24"),
					mustPrefix(t, "192.168.20.0/24"),
					mustPrefix(t, "172.16.0.0/12"),
				},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  true,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointLAN, URL: "rtsps://rec.local:8322/cam1"},
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
		{
			name: "IPv6 LAN CIDR match",
			client: ClientHint{
				IP: mustAddr(t, "fd00::1234"),
			},
			rec: Recorder{
				LANCIDRs:     []netip.Prefix{mustPrefix(t, "fd00::/8")},
				LANURL:       "rtsps://[fd00::1]:8322/cam1",
				GatewayURL:   "https://gw.example.com/cam1",
				Tier2Enabled: true,
			},
			want: []Endpoint{
				{Kind: EndpointLAN, URL: "rtsps://[fd00::1]:8322/cam1"},
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
			},
		},
		{
			name: "all tiers disabled and no LAN -> empty",
			client: ClientHint{
				IP: mustAddr(t, "203.0.113.5"),
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  false,
				Tier3Enabled:  false,
			},
			want: []Endpoint{},
		},
		{
			name: "client IP unknown, no hint -> Gateway then Cloud",
			client: ClientHint{
				// IP is the zero netip.Addr (Invalid).
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				LANURL:        "rtsps://rec.local:8322/cam1",
				GatewayURL:    "https://gw.example.com/cam1",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  true,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointGateway, URL: "https://gw.example.com/cam1"},
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
		{
			name: "Tier2 enabled but GatewayURL empty -> Gateway suppressed",
			client: ClientHint{
				IP: mustAddr(t, "203.0.113.5"),
			},
			rec: Recorder{
				LANCIDRs:      []netip.Prefix{mustPrefix(t, "10.0.0.0/24")},
				GatewayURL:    "",
				CloudRelayURL: "https://relay.kaivue.io/cam1",
				Tier2Enabled:  true,
				Tier3Enabled:  true,
			},
			want: []Endpoint{
				{Kind: EndpointCloudRelay, URL: "https://relay.kaivue.io/cam1"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decide(tt.client, tt.rec)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Decide()\n got = %#v\nwant = %#v", got, tt.want)
			}
		})
	}
}
