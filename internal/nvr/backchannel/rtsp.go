package backchannel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Sentinel errors for RTSP connection state.
var (
	ErrNotConnected     = errors.New("RTSP backchannel not connected")
	ErrAlreadyConnected = errors.New("RTSP backchannel already connected")
)

// rtspState represents the connection lifecycle state.
type rtspState int

const (
	rtspStateDisconnected rtspState = iota
	rtspStateConnecting
	rtspStateConnected
	rtspStateClosed
)

// RTSPConn manages an RTSP session for sending audio to a camera over interleaved TCP.
type RTSPConn struct {
	uri             string
	username        string
	password        string
	conn            net.Conn
	packer          *RTPPacker
	state           rtspState
	mu              sync.Mutex
	writeMu         sync.Mutex
	keepAliveCancel context.CancelFunc
}

// NewRTSPConn creates a new RTSPConn in the disconnected state.
func NewRTSPConn(uri, username, password string) *RTSPConn {
	return &RTSPConn{
		uri:      uri,
		username: username,
		password: password,
		state:    rtspStateDisconnected,
	}
}

// State returns the current connection state, protected by a mutex.
func (c *RTSPConn) State() rtspState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Connect dials the camera's RTSP TCP endpoint and starts a keep-alive goroutine.
// The DESCRIBE/SETUP/PLAY handshake will be added when tested against real cameras.
func (c *RTSPConn) Connect(ctx context.Context, codec string, sampleRate int) error {
	c.mu.Lock()
	if c.state != rtspStateDisconnected {
		c.mu.Unlock()
		return ErrAlreadyConnected
	}
	c.state = rtspStateConnecting
	c.mu.Unlock()

	host, err := hostFromRTSPURI(c.uri)
	if err != nil {
		c.mu.Lock()
		c.state = rtspStateDisconnected
		c.mu.Unlock()
		return fmt.Errorf("parse RTSP URI: %w", err)
	}

	dialer := net.Dialer{}
	tcpConn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		c.mu.Lock()
		c.state = rtspStateDisconnected
		c.mu.Unlock()
		return fmt.Errorf("dial RTSP TCP: %w", err)
	}

	packer := NewRTPPacker(codec, sampleRate)

	keepAliveCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	c.conn = tcpConn
	c.packer = packer
	c.keepAliveCancel = cancel
	c.state = rtspStateConnected
	c.mu.Unlock()

	go c.keepAlive(keepAliveCtx)

	return nil
}

// SendAudio packs audio data into an RTP packet and writes it as an interleaved TCP frame.
// Returns ErrNotConnected if the connection is not in the connected state.
func (c *RTSPConn) SendAudio(audioData []byte) error {
	c.mu.Lock()
	if c.state != rtspStateConnected {
		c.mu.Unlock()
		return ErrNotConnected
	}
	conn := c.conn
	packer := c.packer
	c.mu.Unlock()

	c.writeMu.Lock()
	pkt := packer.Pack(audioData)
	raw := pkt.Marshal()
	err := writeInterleavedFrame(conn, 0, raw)
	c.writeMu.Unlock()

	return err
}

// Close cancels the keep-alive goroutine and shuts down the TCP connection.
func (c *RTSPConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == rtspStateClosed || c.state == rtspStateDisconnected {
		return nil
	}

	if c.keepAliveCancel != nil {
		c.keepAliveCancel()
		c.keepAliveCancel = nil
	}

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.state = rtspStateClosed
	return nil
}

// keepAlive sends RTSP OPTIONS over the raw TCP connection every 30 seconds to
// prevent session timeout. The OPTIONS request is written as plain RTSP text;
// the response is not parsed.
func (c *RTSPConn) keepAlive(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	cseq := 1

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			uri := c.uri
			state := c.state
			c.mu.Unlock()

			if state != rtspStateConnected || conn == nil {
				return
			}

			cseq++
			msg := fmt.Sprintf("OPTIONS %s RTSP/1.0\r\nCSeq: %d\r\n\r\n", uri, cseq)
			c.writeMu.Lock()
			_, err := fmt.Fprint(conn, msg)
			c.writeMu.Unlock()
			if err != nil {
				log.Printf("backchannel: keep-alive OPTIONS failed: %v", err)
			}
		}
	}
}

// writeInterleavedFrame writes an RTSP interleaved binary frame to conn as a single write.
// Format: '$' (0x24) + channel (1 byte) + length (2 bytes big-endian) + payload.
func writeInterleavedFrame(conn net.Conn, channel byte, payload []byte) error {
	frame := make([]byte, 4+len(payload))
	frame[0] = 0x24
	frame[1] = channel
	binary.BigEndian.PutUint16(frame[2:4], uint16(len(payload)))
	copy(frame[4:], payload)
	_, err := conn.Write(frame)
	return err
}

// hostFromRTSPURI extracts the host:port from an RTSP URI.
// If no port is present, port 554 is assumed.
func hostFromRTSPURI(uri string) (string, error) {
	// Minimal parse: strip scheme, credentials, path.
	// Expected format: rtsp://[user:pass@]host[:port][/path]
	rest := uri
	if len(rest) < 7 || rest[:7] != "rtsp://" {
		return "", fmt.Errorf("URI must start with rtsp://")
	}
	rest = rest[7:]

	// Strip path.
	if idx := strings.IndexByte(rest, '/'); idx >= 0 {
		rest = rest[:idx]
	}

	// Strip credentials.
	if idx := strings.IndexByte(rest, '@'); idx >= 0 {
		rest = rest[idx+1:]
	}

	// If no port, append default 554.
	if _, _, err := net.SplitHostPort(rest); err != nil {
		rest = rest + ":554"
	}

	return rest, nil
}

