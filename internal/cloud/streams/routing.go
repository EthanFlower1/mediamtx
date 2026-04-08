package streams

import (
	"net"
)

// EndpointKind classifies the connectivity path a client should use.
//
// LAN-direct is lowest latency and is offered when the client appears to be
// on the same private network as the Recorder. ManagedRelay is always
// included as a fallback; it routes through the cloud relay tier.
// SelfHostedPublic (TODO(KAI-258)) is offered when the Recorder has a
// public IP or DNS name pinned to it.
type EndpointKind string

const (
	// EndpointKindLANDirect routes the client directly to the Recorder's
	// LAN address. Lowest latency (~5 ms), only viable when client and
	// Recorder share the same private subnet.
	EndpointKindLANDirect EndpointKind = "lan_direct"

	// EndpointKindSelfHostedPublic routes via the Recorder's own public-
	// facing address. Medium latency (~40 ms). TODO(KAI-258): not yet
	// emitted by the simplified router; requires the full routing logic.
	EndpointKindSelfHostedPublic EndpointKind = "self_hosted_public"

	// EndpointKindManagedRelay routes via the Kaivue cloud relay tier.
	// Highest latency (~80 ms) but always reachable. Always included as
	// a fallback.
	EndpointKindManagedRelay EndpointKind = "managed_relay"
)

// ClientInfo carries network metadata about the requesting client. It is
// derived from the inbound HTTP request by the service layer.
type ClientInfo struct {
	// SourceIP is the client's effective remote IP, with proxy headers
	// already resolved (X-Forwarded-For, X-Real-IP).
	SourceIP net.IP
}

// EndpointChoice is one URL-route a client may use to reach a stream.
// The service mints a separate JWT for each endpoint.
type EndpointChoice struct {
	Kind               EndpointKind
	BaseURL            string // scheme + host + path prefix, no token
	EstimatedLatencyMS int
}

// Camera is the minimal projection of a camera record that the router needs.
// The full Camera struct lives in the (not-yet-landed) KAI-249 camera
// registry; this interface decouples the router from that package.
//
// TODO(KAI-249): replace with the real Camera type once the camera registry
// lands and is available on main.
type Camera struct {
	// ID is the tenant-scoped stable camera identifier.
	ID string
	// RecorderID is the ID of the Recorder that holds this camera's stream.
	RecorderID string
	// RecorderLANSubnets is the set of RFC-1918 subnets the Recorder's
	// NICs are attached to. Populated by the Recorder heartbeat.
	RecorderLANSubnets []string
	// RecorderLANBaseURL is the scheme + host the Recorder exposes on its
	// LAN interface (e.g. "https://192.168.1.10:8443").
	RecorderLANBaseURL string
	// RelayBaseURL is the cloud-managed relay URL for this Recorder
	// (e.g. "https://relay-us-east-2.kaivue.io/r/<recorder_id>").
	RelayBaseURL string
}

// Router selects the set of endpoints to offer for a given client + camera
// combination.
//
// This is the simplified version (KAI-255). The full routing logic —
// covering self_hosted_public, NAT traversal, TURN/ICE negotiation, and
// per-client capability probing — is deferred to KAI-258.
//
// TODO(KAI-258): replace ChooseEndpoints with the full routing algorithm.
type Router struct {
	// RelayBaseURL is the default managed-relay base URL used when the
	// Camera does not have its own relay URL pinned. Typically
	// "https://relay-us-east-2.kaivue.io".
	RelayBaseURL string
}

// ChooseEndpoints returns the list of endpoint choices for a client + camera
// pair in preference order (lowest latency first).
//
// Simplified logic (KAI-255):
//  1. If client SourceIP is in an RFC-1918 range OR appears to be in one of
//     the camera's Recorder LAN subnets → offer lan_direct first.
//  2. Always offer managed_relay as a fallback.
//
// TODO(KAI-258): add self_hosted_public, TURN/ICE probing, and per-protocol
// capability detection.
func (r *Router) ChooseEndpoints(client ClientInfo, cam Camera) []EndpointChoice {
	var choices []EndpointChoice

	if isPrivateIP(client.SourceIP) || inAnySubnet(client.SourceIP, cam.RecorderLANSubnets) {
		lanURL := cam.RecorderLANBaseURL
		if lanURL == "" {
			lanURL = "https://recorder.lan" // TODO(KAI-249): populated from camera registry
		}
		choices = append(choices, EndpointChoice{
			Kind:               EndpointKindLANDirect,
			BaseURL:            lanURL,
			EstimatedLatencyMS: 5,
		})
	}

	// TODO(KAI-258): insert self_hosted_public here when Recorder has a
	// verified public address.

	relayURL := cam.RelayBaseURL
	if relayURL == "" {
		relayURL = r.RelayBaseURL
	}
	if relayURL == "" {
		relayURL = "https://relay.kaivue.io" // last-resort fallback
	}
	choices = append(choices, EndpointChoice{
		Kind:               EndpointKindManagedRelay,
		BaseURL:            relayURL,
		EstimatedLatencyMS: 80,
	})

	return choices
}

// RFC-1918 private address ranges.
var privateRanges = func() []*net.IPNet {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"fc00::/7", // IPv6 ULA
		"127.0.0.0/8",
		"::1/128",
	}
	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}()

func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range privateRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func inAnySubnet(ip net.IP, cidrs []string) bool {
	if ip == nil {
		return false
	}
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
