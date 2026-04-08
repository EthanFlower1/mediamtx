package mdns

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBroadcasterSendsAndStops verifies that:
//  1. NewBroadcaster starts without error.
//  2. Stop terminates cleanly and is idempotent.
//
// We verify the packet via the buildDNSSDPacket helper rather than a live
// multicast socket to avoid port-5353 conflicts with mDNSResponder on macOS.
func TestBroadcasterSendsAndStops(t *testing.T) {
	b, err := NewBroadcaster(BroadcasterConfig{
		InstanceName: "test-directory",
		Port:         8443,
		TXTRecords:   map[string]string{"site": "test"},
	})
	require.NoError(t, err)

	b.Stop()
	// Stop is idempotent.
	b.Stop()
}

// TestBuildDNSSDPacket verifies the packet builder for a known input without
// requiring network access.
func TestBuildDNSSDPacket(t *testing.T) {
	pkt := buildDNSSDPacket("test-dir", 8443, map[string]string{"v": "1"}, 120)
	require.GreaterOrEqual(t, len(pkt), 12, "packet must include DNS header")

	// Header: ANCOUNT = 3
	ancount := uint16(pkt[6])<<8 | uint16(pkt[7])
	require.Equal(t, uint16(3), ancount)

	// QR and AA bits.
	flags := uint16(pkt[2])<<8 | uint16(pkt[3])
	require.True(t, flags&0x8000 != 0, "QR")
	require.True(t, flags&0x0400 != 0, "AA")
}

// TestEncodeDNSName checks round-trip property: the encoded name always ends
// with a zero byte (root label) and has one length byte per label.
func TestEncodeDNSName(t *testing.T) {
	cases := []struct {
		name   string
		labels []string
	}{
		{"_mediamtx-directory._tcp.local.", []string{"_mediamtx-directory", "_tcp", "local"}},
		{"a.b.c.", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		encoded := encodeDNSName(tc.name)
		// Verify it ends with 0x00.
		require.Equal(t, byte(0x00), encoded[len(encoded)-1], "missing root label for %q", tc.name)
		// Walk labels.
		off := 0
		for _, lbl := range tc.labels {
			require.Equal(t, byte(len(lbl)), encoded[off], "label len for %q in %q", lbl, tc.name)
			off += 1 + len(lbl)
		}
	}
}

// loopbackInterface returns the first loopback network interface found.
func loopbackInterface(t *testing.T) *net.Interface {
	t.Helper()
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagLoopback != 0 {
			return &ifaces[i]
		}
	}
	t.Skip("no loopback interface — skipping multicast test")
	return nil
}
