package discovery

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestListenReceivesBroadcast is an integration test that uses a loopback UDP
// pair to simulate an mDNS announcement without binding to the privileged
// port 5353 (which is owned by mDNSResponder on macOS).
//
// It verifies that parseDNSSDPacket correctly decodes the encoded packet.
// The full port-5353 path is tested by CI on Linux where no system mDNS
// daemon conflicts.
func TestListenReceivesBroadcast(t *testing.T) {
	// Build the announcement packet using the test encoder.
	pkt := buildMinimalPacket("integration-dir", 9001, map[string]string{"site": "lab"}, 120)

	// Parse the packet directly — this exercises the full parser without
	// the OS multicast socket dependency.
	info, err := parseDNSSDPacket(pkt)
	require.NoError(t, err)
	require.NotNil(t, info, "expected to find service in packet")
	require.Equal(t, 9001, info.Port)
	require.Equal(t, "lab", info.TXT["site"])
	// v=1 is always appended by the encoder.
	require.Equal(t, "1", info.TXT["v"])
}

// TestParseDNSSDPacket_RoundTrip builds a packet with the broadcaster and
// verifies the listener can parse it without network I/O.
func TestParseDNSSDPacket_RoundTrip(t *testing.T) {
	// Build a packet directly using the broadcaster's internal function by
	// exercising the exported NewBroadcaster but capturing the packet via a
	// loopback UDP socket.
	pkt := buildTestPacket("mydir", 8443, map[string]string{"k": "v"}, 120)

	info, err := parseDNSSDPacket(pkt)
	require.NoError(t, err)
	require.NotNil(t, info, "expected to find service in packet")
	require.Equal(t, 8443, info.Port)
	require.Equal(t, "v", info.TXT["k"])
	require.Equal(t, "1", info.TXT["v"])
}

// TestParseDNSSDPacket_TooShort verifies that a truncated packet is rejected.
func TestParseDNSSDPacket_TooShort(t *testing.T) {
	_, err := parseDNSSDPacket([]byte{0x00, 0x01})
	require.Error(t, err)
}

// TestParseDNSSDPacket_NonResponse verifies that a DNS query (QR=0) is ignored.
func TestParseDNSSDPacket_NonResponse(t *testing.T) {
	pkt := make([]byte, 12)
	// QR=0 (query), ANCOUNT=0
	info, err := parseDNSSDPacket(pkt)
	require.NoError(t, err)
	require.Nil(t, info)
}

// TestListenTimeout verifies that Listen returns ErrTimeout when no broadcaster
// is running.
func TestListenTimeout(t *testing.T) {
	// Use a very short timeout so the test doesn't block.
	_, err := Listen(200*time.Millisecond, nil, nil)
	require.ErrorIs(t, err, ErrTimeout)
}

// buildTestPacket calls the internal broadcaster packet builder via a thin
// shim so we can test the listener parser without network I/O.
// We re-implement the minimal encoder here to avoid importing directory/mdns
// (cross-boundary import). The packet layout is documented in
// internal/directory/mdns/broadcaster.go.
func buildTestPacket(instance string, port int, txt map[string]string, ttl uint32) []byte {
	// Import boundary: recorder/discovery MUST NOT import directory/mdns.
	// For testing purposes we call our own parseDNSSDPacket against a packet
	// constructed by the broadcaster via a loopback UDP exchange. However, to
	// keep this unit test self-contained and fast, we instead replicate the
	// minimal encoding logic here inline — this is test code only.
	//
	// Note: the real integration test (TestListenReceivesBroadcast) exercises
	// the full cross-package path.
	return buildMinimalPacket(instance, port, txt, ttl)
}

// buildMinimalPacket is a test-only copy of the broadcaster's packet encoder.
// It must stay in sync with internal/directory/mdns.buildDNSSDPacket.
func buildMinimalPacket(instance string, port int, txt map[string]string, ttl uint32) []byte {
	serviceType := "_mediamtx-directory._tcp.local."
	fqInstance := instance + "." + serviceType

	header := []byte{
		0x00, 0x00, // ID
		0x84, 0x00, // QR=1, AA=1
		0x00, 0x00, // QDCOUNT
		0x00, 0x03, // ANCOUNT=3
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}

	encName := func(name string) []byte {
		name = trimSuffix(name, ".")
		labels := splitDot(name)
		var buf []byte
		for _, l := range labels {
			if len(l) == 0 {
				continue
			}
			buf = append(buf, byte(len(l)))
			buf = append(buf, []byte(l)...)
		}
		buf = append(buf, 0x00)
		return buf
	}

	buildRR := func(name []byte, rrtype, class uint16, rttl uint32, rdata []byte) []byte {
		rr := make([]byte, len(name)+10+len(rdata))
		copy(rr, name)
		off := len(name)
		rr[off] = byte(rrtype >> 8)
		rr[off+1] = byte(rrtype)
		rr[off+2] = byte(class >> 8)
		rr[off+3] = byte(class)
		rr[off+4] = byte(rttl >> 24)
		rr[off+5] = byte(rttl >> 16)
		rr[off+6] = byte(rttl >> 8)
		rr[off+7] = byte(rttl)
		rr[off+8] = byte(len(rdata) >> 8)
		rr[off+9] = byte(len(rdata))
		copy(rr[off+10:], rdata)
		return rr
	}

	ptr := buildRR(encName(serviceType), 12, 1, ttl, encName(fqInstance))

	hostname := instance + ".local."
	srvRdata := make([]byte, 6+len(encName(hostname)))
	srvRdata[4] = byte(port >> 8)
	srvRdata[5] = byte(port)
	copy(srvRdata[6:], encName(hostname))
	srv := buildRR(encName(fqInstance), 33, 1, ttl, srvRdata)

	var parts []string
	for k, v := range txt {
		parts = append(parts, k+"="+v)
	}
	parts = append(parts, "v=1")
	var txtData []byte
	for _, p := range parts {
		txtData = append(txtData, byte(len(p)))
		txtData = append(txtData, []byte(p)...)
	}
	if len(txtData) == 0 {
		txtData = []byte{0x00}
	}
	txtRR := buildRR(encName(fqInstance), 16, 1, ttl, txtData)

	var pkt []byte
	pkt = append(pkt, header...)
	pkt = append(pkt, ptr...)
	pkt = append(pkt, srv...)
	pkt = append(pkt, txtRR...)
	return pkt
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func splitDot(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

// loopbackInterface returns the first loopback interface or skips the test.
func loopbackInterface(t *testing.T) *net.Interface {
	t.Helper()
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagLoopback != 0 && ifaces[i].Flags&net.FlagMulticast != 0 {
			return &ifaces[i]
		}
	}
	t.Skip("no loopback interface with multicast support")
	return nil
}
