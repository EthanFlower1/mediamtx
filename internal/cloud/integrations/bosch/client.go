package bosch

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

var (
	ErrNotConnected   = errors.New("bosch: not connected to panel")
	ErrAuthFailed     = errors.New("bosch: panel authentication failed")
	ErrPanelRejected  = errors.New("bosch: panel rejected command")
)

// ClientConfig holds the parameters for connecting to a Bosch panel.
type ClientConfig struct {
	Host     string
	Port     int
	AuthCode string
	Series   PanelSeries

	// Timeouts
	ConnectTimeout   time.Duration // default 10s
	ReadTimeout      time.Duration // default 30s
	HeartbeatInterval time.Duration // default 15s
	ReconnectDelay   time.Duration // default 5s
	MaxReconnects    int           // 0 = unlimited
}

func (c *ClientConfig) defaults() {
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 10 * time.Second
	}
	if c.ReadTimeout == 0 {
		c.ReadTimeout = 30 * time.Second
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 15 * time.Second
	}
	if c.ReconnectDelay == 0 {
		c.ReconnectDelay = 5 * time.Second
	}
}

// FrameHandler is called for each frame received from the panel.
type FrameHandler func(frame *Frame)

// Conn abstracts a network connection for testing.
type Conn interface {
	Read(b []byte) (n int, err error)
	Write(b []byte) (n int, err error)
	Close() error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

// Dialer abstracts TCP dialing for testing.
type Dialer func(ctx context.Context, network, address string) (Conn, error)

// Client manages a TCP connection to a Bosch B/G-Series alarm panel using
// the Mode2 automation protocol. It handles authentication, heartbeat,
// automatic reconnection, and frame dispatch.
type Client struct {
	cfg    ClientConfig
	dialer Dialer

	mu       sync.Mutex
	conn     Conn
	state    ConnectionState
	handler  FrameHandler

	ctx       context.Context
	ctxCancel context.CancelFunc
	wg        sync.WaitGroup

	// Metrics
	reconnects    int
	framesRecvd   int64
	framesSent    int64
	lastHeartbeat time.Time
}

// NewClient creates a panel client. Call Start() to begin connection.
func NewClient(cfg ClientConfig, handler FrameHandler) *Client {
	cfg.defaults()
	return &Client{
		cfg:     cfg,
		handler: handler,
		state:   StateDisconnected,
		dialer:  defaultDialer,
	}
}

func defaultDialer(ctx context.Context, network, address string) (Conn, error) {
	d := &net.Dialer{}
	c, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// SetDialer overrides the default TCP dialer (used for testing).
func (c *Client) SetDialer(d Dialer) {
	c.dialer = d
}

// State returns the current connection state.
func (c *Client) State() ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) setState(s ConnectionState) {
	c.mu.Lock()
	c.state = s
	c.mu.Unlock()
}

// Start initiates the connection to the panel and begins the read loop.
func (c *Client) Start(ctx context.Context) {
	c.ctx, c.ctxCancel = context.WithCancel(ctx)

	c.wg.Add(1)
	go c.connectLoop()

	log.Printf("[bosch] client started for %s:%d (%s-Series)",
		c.cfg.Host, c.cfg.Port, c.cfg.Series)
}

// Stop gracefully shuts down the client.
func (c *Client) Stop() {
	if c.ctxCancel != nil {
		c.ctxCancel()
	}
	c.wg.Wait()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.state = StateDisconnected
	c.mu.Unlock()

	log.Printf("[bosch] client stopped for %s:%d", c.cfg.Host, c.cfg.Port)
}

// SendFrame sends a Mode2 frame to the panel.
func (c *Client) SendFrame(f *Frame) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return ErrNotConnected
	}

	data, err := f.MarshalBinary()
	if err != nil {
		return fmt.Errorf("bosch: marshal frame: %w", err)
	}

	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("bosch: set write deadline: %w", err)
	}

	_, err = conn.Write(data)
	if err != nil {
		return fmt.Errorf("bosch: write frame: %w", err)
	}

	c.mu.Lock()
	c.framesSent++
	c.mu.Unlock()
	return nil
}

func (c *Client) connectLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.connect(); err != nil {
			log.Printf("[bosch] connection failed: %v", err)
			c.setState(StateReconnecting)

			c.mu.Lock()
			c.reconnects++
			reconnects := c.reconnects
			c.mu.Unlock()

			if c.cfg.MaxReconnects > 0 && reconnects > c.cfg.MaxReconnects {
				log.Printf("[bosch] max reconnects (%d) exceeded, giving up", c.cfg.MaxReconnects)
				c.setState(StateDisconnected)
				return
			}

			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.cfg.ReconnectDelay):
				continue
			}
		}

		// Connected and authenticated — run the read loop.
		c.readLoop()

		// readLoop exited — reconnect unless context is cancelled.
		select {
		case <-c.ctx.Done():
			return
		default:
			c.setState(StateReconnecting)
			c.mu.Lock()
			c.reconnects++
			c.mu.Unlock()
			log.Printf("[bosch] connection lost, reconnecting in %v", c.cfg.ReconnectDelay)
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(c.cfg.ReconnectDelay):
			}
		}
	}
}

func (c *Client) connect() error {
	c.setState(StateConnecting)

	addr := fmt.Sprintf("%s:%d", c.cfg.Host, c.cfg.Port)
	dialCtx, cancel := context.WithTimeout(c.ctx, c.cfg.ConnectTimeout)
	defer cancel()

	conn, err := c.dialer(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial %s: %w", addr, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	// Authenticate.
	c.setState(StateAuthenticating)
	if err := c.authenticate(conn); err != nil {
		conn.Close()
		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()
		return fmt.Errorf("auth: %w", err)
	}

	c.setState(StateConnected)
	log.Printf("[bosch] connected and authenticated to %s", addr)
	return nil
}

func (c *Client) authenticate(conn Conn) error {
	// Wait for auth challenge from panel.
	if err := conn.SetReadDeadline(time.Now().Add(c.cfg.ConnectTimeout)); err != nil {
		return err
	}

	frame, err := c.readFrame(conn)
	if err != nil {
		return fmt.Errorf("read auth challenge: %w", err)
	}
	if frame.Command != cmdAuthChallenge {
		return fmt.Errorf("expected auth challenge (0x%02X), got 0x%02X",
			cmdAuthChallenge, frame.Command)
	}

	// Send auth reply with passcode.
	reply := BuildAuthReply(c.cfg.AuthCode)
	data, err := reply.MarshalBinary()
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("write auth reply: %w", err)
	}

	// Wait for auth result.
	if err := conn.SetReadDeadline(time.Now().Add(c.cfg.ConnectTimeout)); err != nil {
		return err
	}
	result, err := c.readFrame(conn)
	if err != nil {
		return fmt.Errorf("read auth result: %w", err)
	}
	if result.Command != cmdAuthResult {
		return fmt.Errorf("expected auth result (0x%02X), got 0x%02X",
			cmdAuthResult, result.Command)
	}
	if len(result.Payload) > 0 && result.Payload[0] != 0x01 {
		return ErrAuthFailed
	}

	return nil
}

func (c *Client) readLoop() {
	conn := c.conn

	// Start heartbeat sender.
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(c.cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				ping := BuildPing()
				if err := c.SendFrame(ping); err != nil {
					log.Printf("[bosch] heartbeat send failed: %v", err)
					return
				}
				c.mu.Lock()
				c.lastHeartbeat = time.Now()
				c.mu.Unlock()
			}
		}
	}()

	for {
		select {
		case <-c.ctx.Done():
			<-heartbeatDone
			return
		default:
		}

		if err := conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout)); err != nil {
			log.Printf("[bosch] set read deadline: %v", err)
			<-heartbeatDone
			return
		}

		frame, err := c.readFrame(conn)
		if err != nil {
			if c.ctx.Err() != nil {
				<-heartbeatDone
				return
			}
			log.Printf("[bosch] read error: %v", err)
			<-heartbeatDone
			return
		}

		c.mu.Lock()
		c.framesRecvd++
		c.mu.Unlock()

		// ACK the frame if it's an event report.
		if frame.Command == cmdEventReport && len(frame.Payload) > 0 {
			ack := BuildAck(frame.Payload[0])
			if err := c.SendFrame(ack); err != nil {
				log.Printf("[bosch] failed to ACK event: %v", err)
			}
		}

		// Dispatch to handler.
		if c.handler != nil {
			c.handler(frame)
		}
	}
}

// readFrame reads one complete Mode2 frame from the connection.
func (c *Client) readFrame(conn Conn) (*Frame, error) {
	// Read STX.
	stx := make([]byte, 1)
	if _, err := io.ReadFull(conn, stx); err != nil {
		return nil, err
	}
	if stx[0] != frameSTX {
		return nil, ErrFrameNoSTX
	}

	// Read LEN (2 bytes).
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return nil, err
	}
	frameLen := int(binary.BigEndian.Uint16(lenBuf))
	if frameLen < 2 || frameLen > maxPayloadSize+2 {
		return nil, fmt.Errorf("bosch: invalid frame length %d", frameLen)
	}

	// Read CMD + payload + CHKSUM + ETX.
	rest := make([]byte, frameLen+1) // frameLen bytes + ETX
	if _, err := io.ReadFull(conn, rest); err != nil {
		return nil, err
	}

	// Reassemble full frame for UnmarshalFrame.
	full := make([]byte, 0, 3+frameLen+1)
	full = append(full, frameSTX)
	full = append(full, lenBuf...)
	full = append(full, rest...)

	return UnmarshalFrame(full)
}
