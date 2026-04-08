package streamurl

import (
	"net/netip"
)

// EndpointKind tags an endpoint with its routing tier so callers (and
// telemetry) can tell at a glance which path is being attempted.
type EndpointKind string

const (
	// EndpointLAN is a direct LAN URL to the Recorder. This is the
	// fastest, lowest-latency path and is preferred whenever the client
	// is determined to be on the Recorder's local network.
	EndpointLAN EndpointKind = "lan"

	// EndpointGateway is the Tier 2 customer-edge Gateway URL. The
	// gateway role (KAI-261) terminates client connections and proxies
	// to the Recorder over the customer's WAN. Used when the client is
	// off-LAN but the customer has Gateway enabled.
	EndpointGateway EndpointKind = "gateway"

	// EndpointCloudRelay is the Tier 3 Kaivue cloud relay URL. This is
	// the fallback path used when neither the LAN nor a customer-edge
	// Gateway is reachable.
	EndpointCloudRelay EndpointKind = "cloud-relay"
)

// Endpoint is a single candidate URL plus its tier tag.
type Endpoint struct {
	Kind EndpointKind
	URL  string
}

// Recorder is the subset of a Recorder record that the routing decision
// logic needs. It is intentionally a value type with no behavior so the
// caller (which has the full DB-backed Recorder model) can populate it
// without dragging persistence concerns into this package.
type Recorder struct {
	// LANCIDRs is the list of CIDR ranges that the Recorder considers
	// "on its LAN". A client whose source IP falls inside any of these
	// ranges should be served the LAN URL. Empty list means "no LAN
	// classification possible from IP alone".
	LANCIDRs []netip.Prefix

	// LANURL is the direct on-LAN URL for the Recorder
	// (e.g. "rtsps://recorder.local:8322/cam1"). May be empty if the
	// Recorder has no local URL configured, in which case LAN
	// candidates are suppressed even when the client is on-LAN.
	LANURL string

	// GatewayURL is the Tier 2 Gateway URL for this Recorder. Empty
	// suppresses the Gateway candidate.
	GatewayURL string

	// CloudRelayURL is the Tier 3 cloud relay URL for this Recorder.
	// Empty suppresses the cloud relay candidate.
	CloudRelayURL string

	// Tier2Enabled indicates the tenant/recorder has Tier 2 (Gateway)
	// turned on. When false, GatewayURL is ignored entirely.
	Tier2Enabled bool

	// Tier3Enabled indicates the tenant/recorder has Tier 3 (Cloud
	// Relay) turned on. When false, CloudRelayURL is ignored entirely.
	Tier3Enabled bool
}

// ClientHint carries the per-request signals that influence routing.
type ClientHint struct {
	// IP is the client's source IP, as observed by the Directory. The
	// zero value (an invalid netip.Addr) means "unknown" and the
	// CIDR-based LAN check is skipped.
	IP netip.Addr

	// IsLAN, when true, is an explicit assertion from the client that
	// it is on the Recorder's LAN (e.g. the Flutter app discovered the
	// Recorder over mDNS). It overrides a negative IP-based check, but
	// only promotes the LAN endpoint if a LANURL is actually configured.
	IsLAN bool
}

// Decide returns the ordered list of endpoint candidates the client
// should attempt for the given Recorder.
//
// Ordering:
//   - LAN first, if the client is on-LAN (by IP-in-CIDR or by IsLAN
//     hint) AND the Recorder has a LANURL configured.
//   - Then Gateway, if Tier2Enabled and GatewayURL is set.
//   - Then CloudRelay, if Tier3Enabled and CloudRelayURL is set.
//
// The result may be empty if no candidates are available; callers
// should treat that as "no route to recorder" and surface an error.
//
// Decide is pure: same inputs always produce the same output, no I/O.
func Decide(client ClientHint, rec Recorder) []Endpoint {
	out := make([]Endpoint, 0, 3)

	if onLAN(client, rec) && rec.LANURL != "" {
		out = append(out, Endpoint{Kind: EndpointLAN, URL: rec.LANURL})
	}

	if rec.Tier2Enabled && rec.GatewayURL != "" {
		out = append(out, Endpoint{Kind: EndpointGateway, URL: rec.GatewayURL})
	}

	if rec.Tier3Enabled && rec.CloudRelayURL != "" {
		out = append(out, Endpoint{Kind: EndpointCloudRelay, URL: rec.CloudRelayURL})
	}

	return out
}

// onLAN reports whether the client should be considered on the
// Recorder's LAN. The IsLAN hint wins over a negative IP check (the
// client knows its own network better than we do), but the IP check
// alone is sufficient when no hint is supplied.
func onLAN(client ClientHint, rec Recorder) bool {
	if client.IsLAN {
		return true
	}
	if !client.IP.IsValid() {
		return false
	}
	for _, cidr := range rec.LANCIDRs {
		if cidr.Contains(client.IP) {
			return true
		}
	}
	return false
}
