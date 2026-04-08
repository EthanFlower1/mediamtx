// Package discovery implements the mDNS listener used by the Recorder during
// the auto-discovery phase of onboarding (KAI-245).
//
// The Recorder calls Listen with a timeout; it returns the first
// DirectoryInfo received on the mDNS multicast group. The caller then
// presents the discovered endpoint to the operator ("Detected Directory at
// <hostname>. Join?") before proceeding with the approval-request flow.
//
// Boundary rules: internal/recorder/... MUST NOT import internal/directory/...
// This package imports only internal/shared/... and stdlib.
package discovery

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

const (
	// ServiceType is the mDNS service type the Directory broadcasts.
	// Must match internal/directory/mdns.ServiceType.
	ServiceType = "_mediamtx-directory._tcp.local."

	mdnsIPv4Addr  = "224.0.0.251:5353"
	mdnsGroupIPv4 = "224.0.0.251"
	mdnsMCastPort = 5353
)

// ErrTimeout is returned by Listen when no Directory is found within the
// deadline.
var ErrTimeout = errors.New("discovery: no Directory found within timeout")

// DirectoryInfo holds the essential data extracted from a received mDNS
// DNS-SD announcement.
type DirectoryInfo struct {
	// Hostname is the mDNS .local hostname from the SRV target, e.g.
	// "kaivue-directory.local".
	Hostname string
	// Port is the TCP port from the SRV record.
	Port int
	// TXT carries the key=value pairs from the TXT record, e.g.
	// {"v": "1", "site": "building-a"}.
	TXT map[string]string
	// SourceIP is the source address of the UDP packet, useful for
	// fallback when the .local hostname is not resolvable.
	SourceIP net.IP
}

// Listen joins the mDNS multicast group and waits until a Directory
// announcement is received or the context deadline is exceeded.
//
// timeout controls how long to wait. The recommended value in the auto-
// discover flow is 30 s (spec §13.4).
//
// iface may be nil — in that case the OS picks the interface. For tests,
// pass the loopback interface.
func Listen(timeout time.Duration, iface *net.Interface, log *slog.Logger) (*DirectoryInfo, error) {
	if log == nil {
		log = slog.Default()
	}

	conn, err := net.ListenMulticastUDP("udp4", iface, &net.UDPAddr{
		IP:   net.ParseIP(mdnsGroupIPv4),
		Port: mdnsMCastPort,
	})
	if err != nil {
		return nil, fmt.Errorf("discovery: join multicast group: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(timeout)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return nil, fmt.Errorf("discovery: set deadline: %w", err)
	}

	buf := make([]byte, 4096)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Timeout() {
				return nil, ErrTimeout
			}
			return nil, fmt.Errorf("discovery: read: %w", err)
		}

		info, err := parseDNSSDPacket(buf[:n])
		if err != nil {
			log.Debug("discovery: parse failed, skipping packet",
				"src", src.String(),
				"error", err,
			)
			continue
		}
		if info == nil {
			// Packet did not contain a matching service type.
			continue
		}
		info.SourceIP = src.IP
		log.Info("discovery: Directory found",
			"hostname", info.Hostname,
			"port", info.Port,
			"src_ip", src.String(),
		)
		return info, nil
	}
}

// parseDNSSDPacket inspects a raw DNS message and, if it contains a PTR
// record for ServiceType, extracts the SRV target+port and TXT key=value pairs.
// Returns nil, nil if the packet is a valid DNS message but does not contain
// our service type.
func parseDNSSDPacket(pkt []byte) (*DirectoryInfo, error) {
	if len(pkt) < 12 {
		return nil, fmt.Errorf("packet too short (%d bytes)", len(pkt))
	}

	// Only accept authoritative DNS responses (QR=1, AA=1).
	flags := uint16(pkt[2])<<8 | uint16(pkt[3])
	if flags&0x8000 == 0 { // not a response
		return nil, nil
	}
	if flags&0x0400 == 0 { // not authoritative
		return nil, nil
	}

	ancount := int(uint16(pkt[6])<<8 | uint16(pkt[7]))
	if ancount == 0 {
		return nil, nil
	}

	// Skip question section (QDCOUNT).
	qdcount := int(uint16(pkt[4])<<8 | uint16(pkt[5]))
	off := 12
	for i := 0; i < qdcount; i++ {
		var err error
		off, err = skipName(pkt, off)
		if err != nil {
			return nil, err
		}
		off += 4 // QTYPE + QCLASS
	}

	// Parse answer records.
	var (
		foundService bool
		hostname     string
		port         int
		txt          = make(map[string]string)
	)

	for i := 0; i < ancount; i++ {
		if off >= len(pkt) {
			break
		}
		name, newOff, err := readName(pkt, off)
		if err != nil {
			return nil, fmt.Errorf("record %d name: %w", i, err)
		}
		off = newOff
		if off+10 > len(pkt) {
			break
		}
		rrtype := uint16(pkt[off])<<8 | uint16(pkt[off+1])
		// class := uint16(pkt[off+2])<<8 | uint16(pkt[off+3])
		// ttl   := binary.BigEndian.Uint32(pkt[off+4:off+8])
		rdlen := int(uint16(pkt[off+8])<<8 | uint16(pkt[off+9]))
		off += 10
		if off+rdlen > len(pkt) {
			break
		}
		rdata := pkt[off : off+rdlen]
		off += rdlen

		switch rrtype {
		case 12: // PTR
			target, _, err := readName(pkt, off-rdlen)
			if err != nil {
				continue
			}
			_ = target
			// Check if the owner name is our service type.
			if normalizeName(name) == normalizeName(ServiceType) {
				foundService = true
			}

		case 33: // SRV
			if rdlen < 6 {
				continue
			}
			// priority(2) + weight(2) + port(2) + target
			port = int(binary.BigEndian.Uint16(rdata[4:6]))
			target, _, err := readName(pkt, off-rdlen+6)
			if err != nil {
				continue
			}
			hostname = target

		case 16: // TXT
			txt = parseTXT(rdata)
		}
	}

	if !foundService || hostname == "" || port == 0 {
		return nil, nil
	}

	return &DirectoryInfo{
		Hostname: hostname,
		Port:     port,
		TXT:      txt,
	}, nil
}

// skipName skips a DNS name at offset off and returns the new offset.
func skipName(pkt []byte, off int) (int, error) {
	for {
		if off >= len(pkt) {
			return off, fmt.Errorf("name overflows packet at %d", off)
		}
		length := int(pkt[off])
		if length == 0 {
			return off + 1, nil
		}
		if length&0xC0 == 0xC0 {
			// Pointer (compression) — 2 bytes total.
			return off + 2, nil
		}
		off += 1 + length
	}
}

// readName decodes a DNS name (with possible pointer compression) starting at
// offset off. It returns the decoded name and the new offset after the name
// in the original packet (not after the pointer target).
func readName(pkt []byte, off int) (string, int, error) {
	var labels []string
	visited := make(map[int]bool)
	origOff := -1

	for {
		if off >= len(pkt) {
			return "", off, fmt.Errorf("name read past end of packet")
		}
		if visited[off] {
			return "", off, fmt.Errorf("name compression loop detected")
		}
		visited[off] = true

		length := int(pkt[off])
		if length == 0 {
			off++
			break
		}
		if length&0xC0 == 0xC0 {
			// Pointer compression.
			if off+1 >= len(pkt) {
				return "", off, fmt.Errorf("compression pointer overflows packet")
			}
			ptr := int(uint16(pkt[off]&0x3F)<<8 | uint16(pkt[off+1]))
			if origOff < 0 {
				origOff = off + 2
			}
			off = ptr
			continue
		}
		off++
		if off+length > len(pkt) {
			return "", off, fmt.Errorf("label overflows packet")
		}
		labels = append(labels, string(pkt[off:off+length]))
		off += length
	}

	if origOff >= 0 {
		off = origOff
	}
	return strings.Join(labels, ".") + ".", off, nil
}

// parseTXT converts DNS TXT RDATA bytes into a key=value map. Strings without
// "=" are stored with an empty value.
func parseTXT(rdata []byte) map[string]string {
	result := make(map[string]string)
	off := 0
	for off < len(rdata) {
		length := int(rdata[off])
		off++
		if off+length > len(rdata) {
			break
		}
		part := string(rdata[off : off+length])
		off += length
		if idx := strings.IndexByte(part, '='); idx >= 0 {
			result[part[:idx]] = part[idx+1:]
		} else {
			result[part] = ""
		}
	}
	return result
}

// normalizeName lowercases and ensures a trailing dot for comparison.
func normalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}
