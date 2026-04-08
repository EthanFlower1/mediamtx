// Package mdns implements the mDNS service advertisement for the Kaivue
// Directory role. It broadcasts _mediamtx-directory._tcp.local so that new
// Recorders on the same LAN can discover the Directory without manual token
// paste (KAI-245).
//
// Boundary rules: this package is internal/directory/... and MUST NOT import
// internal/recorder/.... Only internal/shared/... is permitted as a cross-role
// import.
//
// Implementation notes:
//   - We use a hand-rolled multicast DNS sender rather than adding a new
//     external dependency; golang.org/x/net/ipv4 is already in go.mod
//     (indirect via pion).
//   - The broadcaster sends unsolicited announcements every announcePeriod.
//     This is sufficient for discovery within a ~30 s window; responding to
//     mDNS queries (RFC 6762 §6) is intentionally out of scope for v1 — the
//     listener does not send queries, it just listens for announcements.
//   - On shutdown the broadcaster sends a "goodbye" packet (TTL=0) per
//     RFC 6762 §11.3 so listeners can stop waiting immediately.
package mdns

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// ServiceType is the mDNS service type advertised by the Directory.
	ServiceType = "_mediamtx-directory._tcp.local."

	mdnsIPv4Addr    = "224.0.0.251:5353"
	announcePeriod  = 10 * time.Second
	defaultTTL      = 120 // seconds; goodbye uses 0
)

// BroadcasterConfig parameterises a Broadcaster.
type BroadcasterConfig struct {
	// InstanceName is the mDNS service instance name, e.g. "kaivue-directory".
	// Defaults to the system hostname if empty.
	InstanceName string

	// Port is the TCP port the Directory's management API listens on.
	// Embedded in the mDNS SRV record. Required.
	Port int

	// TXTRecords are additional key=value pairs embedded in the DNS-SD TXT
	// record, e.g. {"version": "1", "site": "building-a"}.
	// Callers may use these for out-of-band hints; Recorders ignore unknown keys.
	TXTRecords map[string]string

	// Logger receives structured log output. Nil = slog.Default().
	Logger *slog.Logger
}

// Broadcaster periodically announces a DNS-SD PTR+SRV+TXT record set over
// multicast DNS for the Directory service. It is safe for concurrent use.
type Broadcaster struct {
	cfg      BroadcasterConfig
	instance string
	conn     *net.UDPConn
	log      *slog.Logger
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewBroadcaster creates and starts a Broadcaster. Call Stop to shut it down.
func NewBroadcaster(cfg BroadcasterConfig) (*Broadcaster, error) {
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("mdns/broadcaster: Port is required")
	}
	instance := cfg.InstanceName
	if instance == "" {
		h, err := hostName()
		if err != nil {
			return nil, fmt.Errorf("mdns/broadcaster: resolve hostname: %w", err)
		}
		instance = h
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	addr, err := net.ResolveUDPAddr("udp4", mdnsIPv4Addr)
	if err != nil {
		return nil, fmt.Errorf("mdns/broadcaster: resolve multicast addr: %w", err)
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("mdns/broadcaster: dial multicast: %w", err)
	}

	b := &Broadcaster{
		cfg:      cfg,
		instance: instance,
		conn:     conn,
		log:      log,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}

	// Send an immediate announcement then schedule repeats.
	if err := b.sendAnnouncement(defaultTTL); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("mdns/broadcaster: initial announce: %w", err)
	}

	go b.loop()
	return b, nil
}

// Stop sends a mDNS goodbye packet (TTL=0) and shuts down the Broadcaster.
// It is safe to call multiple times.
func (b *Broadcaster) Stop() {
	b.stopOnce.Do(func() {
		close(b.stopCh)
		<-b.doneCh
	})
}

func (b *Broadcaster) loop() {
	defer close(b.doneCh)
	ticker := time.NewTicker(announcePeriod)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			if err := b.sendAnnouncement(0); err != nil {
				b.log.Warn("mdns/broadcaster: goodbye send failed", "error", err)
			}
			_ = b.conn.Close()
			return
		case <-ticker.C:
			if err := b.sendAnnouncement(defaultTTL); err != nil {
				b.log.Warn("mdns/broadcaster: announce failed", "error", err)
			}
		}
	}
}

// sendAnnouncement writes a minimal DNS-SD message containing PTR, SRV, and
// TXT records for the Directory service. ttl=0 is a goodbye packet.
func (b *Broadcaster) sendAnnouncement(ttl uint32) error {
	pkt := buildDNSSDPacket(b.instance, b.cfg.Port, b.cfg.TXTRecords, ttl)
	_, err := b.conn.Write(pkt)
	if err != nil {
		return fmt.Errorf("write multicast: %w", err)
	}
	b.log.Debug("mdns/broadcaster: announced",
		"instance", b.instance,
		"port", b.cfg.Port,
		"ttl", ttl,
	)
	return nil
}

// buildDNSSDPacket constructs a minimal DNS message carrying three resource
// records for RFC 6762 / RFC 6763 DNS-SD:
//
//	PTR  ServiceType → <instance>.ServiceType
//	SRV  <instance>.ServiceType → <hostname>:<port>
//	TXT  <instance>.ServiceType → key=value pairs
//
// This is intentionally minimal — no EDNS0, no NSEC. Enough for the
// Recorder's listener to parse the hostname and port.
func buildDNSSDPacket(instance string, port int, txt map[string]string, ttl uint32) []byte {
	// Fully-qualified service instance name.
	fqInstance := instance + "." + ServiceType

	// --- DNS header (12 bytes) ---
	// QR=1 (response), AA=1 (authoritative), Opcode=0, ANCOUNT=3
	header := []byte{
		0x00, 0x00, // ID = 0 (mDNS)
		0x84, 0x00, // QR=1, Opcode=0, AA=1, TC=0, RD=0
		0x00, 0x00, // QDCOUNT = 0
		0x00, 0x03, // ANCOUNT = 3 (PTR + SRV + TXT)
		0x00, 0x00, // NSCOUNT = 0
		0x00, 0x00, // ARCOUNT = 0
	}

	// --- PTR record ---
	// NAME = ServiceType
	// TYPE = PTR (12), CLASS = IN (1), TTL, RDLENGTH, RDATA = fqInstance label
	ptr := buildRR(encodeDNSName(ServiceType), 12, 1, ttl, encodeDNSName(fqInstance))

	// --- SRV record ---
	// RDATA = priority(2) + weight(2) + port(2) + target
	hostname := localHostname(instance)
	srvRdata := make([]byte, 6+len(encodeDNSName(hostname)))
	binary.BigEndian.PutUint16(srvRdata[0:2], 0)              // priority
	binary.BigEndian.PutUint16(srvRdata[2:4], 0)              // weight
	binary.BigEndian.PutUint16(srvRdata[4:6], uint16(port))   //nolint:gosec // port ≤ 65535
	copy(srvRdata[6:], encodeDNSName(hostname))
	srv := buildRR(encodeDNSName(fqInstance), 33, 1, ttl, srvRdata)

	// --- TXT record ---
	var txtParts []string
	for k, v := range txt {
		txtParts = append(txtParts, k+"="+v)
	}
	// Always include the service version so listeners can gate on it.
	txtParts = append(txtParts, "v=1")
	txtData := encodeTXT(txtParts)
	txtRR := buildRR(encodeDNSName(fqInstance), 16, 1, ttl, txtData)

	var pkt []byte
	pkt = append(pkt, header...)
	pkt = append(pkt, ptr...)
	pkt = append(pkt, srv...)
	pkt = append(pkt, txtRR...)
	return pkt
}

// buildRR constructs a DNS resource record.
func buildRR(name []byte, rrtype, class uint16, ttl uint32, rdata []byte) []byte {
	rr := make([]byte, len(name)+10+len(rdata))
	copy(rr, name)
	off := len(name)
	binary.BigEndian.PutUint16(rr[off:], rrtype)
	binary.BigEndian.PutUint16(rr[off+2:], class)
	binary.BigEndian.PutUint32(rr[off+4:], ttl)
	binary.BigEndian.PutUint16(rr[off+8:], uint16(len(rdata))) //nolint:gosec
	copy(rr[off+10:], rdata)
	return rr
}

// encodeDNSName encodes a dot-separated DNS name into label-length-prefixed
// wire format. Trailing dot is consumed. Maximum label length is 63 bytes;
// names longer than 255 bytes are silently truncated (should not occur in
// practice for mDNS service names).
func encodeDNSName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	labels := strings.Split(name, ".")
	var buf []byte
	for _, l := range labels {
		if len(l) == 0 {
			continue
		}
		if len(l) > 63 {
			l = l[:63]
		}
		buf = append(buf, byte(len(l))) //nolint:gosec
		buf = append(buf, []byte(l)...)
	}
	buf = append(buf, 0x00) // root label
	return buf
}

// encodeTXT encodes a slice of "key=value" strings into DNS TXT RDATA.
// Each string is length-prefixed (1 byte).
func encodeTXT(parts []string) []byte {
	var buf []byte
	for _, p := range parts {
		if len(p) > 255 {
			p = p[:255]
		}
		buf = append(buf, byte(len(p))) //nolint:gosec
		buf = append(buf, []byte(p)...)
	}
	if len(buf) == 0 {
		buf = []byte{0x00}
	}
	return buf
}

// localHostname returns a .local mDNS hostname for the given instance label,
// e.g. "kaivue-directory.local.".
func localHostname(instance string) string {
	return instance + ".local."
}

// hostName returns the OS hostname, used as the default instance name.
func hostName() (string, error) {
	h, err := net.LookupAddr("127.0.0.1")
	if err == nil && len(h) > 0 {
		return strings.TrimSuffix(h[0], "."), nil
	}
	// Fallback to the machine hostname.
	ifaces, err2 := net.Interfaces()
	if err2 != nil {
		return "kaivue-directory", nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		return iface.Name, nil
	}
	return "kaivue-directory", nil
}
