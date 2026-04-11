package dmp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// ReceiverConfig configures the SIA TCP receiver.
type ReceiverConfig struct {
	// ListenAddr is the TCP address to listen on (e.g., ":12345").
	ListenAddr string `json:"listen_addr"`

	// ReadTimeout is the per-connection read timeout.
	ReadTimeout time.Duration `json:"read_timeout"`

	// MaxConnections limits concurrent panel connections. 0 = unlimited.
	MaxConnections int `json:"max_connections"`
}

// DefaultReceiverConfig returns sensible defaults for the SIA receiver.
func DefaultReceiverConfig() ReceiverConfig {
	return ReceiverConfig{
		ListenAddr:     ":12345",
		ReadTimeout:    30 * time.Second,
		MaxConnections: 64,
	}
}

// EventHandler is called for each parsed alarm event.
type EventHandler func(event *AlarmEvent)

// Receiver is a TCP server that accepts SIA protocol connections from DMP
// XR-Series alarm panels.
type Receiver struct {
	Config  ReceiverConfig
	Handler EventHandler

	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	connSem  chan struct{} // connection semaphore
}

// NewReceiver creates a new SIA receiver.
func NewReceiver(cfg ReceiverConfig, handler EventHandler) *Receiver {
	r := &Receiver{
		Config:  cfg,
		Handler: handler,
	}
	if cfg.MaxConnections > 0 {
		r.connSem = make(chan struct{}, cfg.MaxConnections)
	}
	return r
}

// Start begins listening for SIA connections.
func (r *Receiver) Start(ctx context.Context) error {
	r.ctx, r.cancel = context.WithCancel(ctx)

	var err error
	r.listener, err = net.Listen("tcp", r.Config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", r.Config.ListenAddr, err)
	}

	r.wg.Add(1)
	go r.acceptLoop()

	log.Printf("[DMP] [INFO] SIA receiver listening on %s", r.Config.ListenAddr)
	return nil
}

// Stop shuts down the receiver and waits for connections to drain.
func (r *Receiver) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	if r.listener != nil {
		r.listener.Close()
	}
	r.wg.Wait()
	log.Printf("[DMP] [INFO] SIA receiver stopped")
}

// Addr returns the listener address, useful for tests.
func (r *Receiver) Addr() net.Addr {
	if r.listener != nil {
		return r.listener.Addr()
	}
	return nil
}

func (r *Receiver) acceptLoop() {
	defer r.wg.Done()

	for {
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.ctx.Done():
				return
			default:
				if !isClosedError(err) {
					log.Printf("[DMP] [ERROR] accept: %v", err)
				}
				return
			}
		}

		// Enforce connection limit.
		if r.connSem != nil {
			select {
			case r.connSem <- struct{}{}:
			default:
				log.Printf("[DMP] [WARN] connection limit reached, rejecting %s", conn.RemoteAddr())
				conn.Close()
				continue
			}
		}

		r.wg.Add(1)
		go r.handleConn(conn)
	}
}

func (r *Receiver) handleConn(conn net.Conn) {
	defer r.wg.Done()
	defer conn.Close()
	if r.connSem != nil {
		defer func() { <-r.connSem }()
	}

	remote := conn.RemoteAddr().String()
	log.Printf("[DMP] [DEBUG] panel connected: %s", remote)

	scanner := bufio.NewScanner(conn)
	// SIA messages are delimited by CR (\r). Use a custom split function.
	scanner.Split(scanSIAMessages)
	scanner.Buffer(make([]byte, 4096), 4096)

	for {
		select {
		case <-r.ctx.Done():
			return
		default:
		}

		if r.Config.ReadTimeout > 0 {
			conn.SetReadDeadline(time.Now().Add(r.Config.ReadTimeout))
		}

		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if !isTimeoutError(err) && !isClosedError(err) {
					log.Printf("[DMP] [WARN] read error from %s: %v", remote, err)
				}
			}
			return
		}

		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		r.processMessage(conn, line, remote)
	}
}

func (r *Receiver) processMessage(conn net.Conn, line, remote string) {
	msg, err := ParseSIAMessage(line)
	if err != nil {
		log.Printf("[DMP] [WARN] failed to parse SIA message from %s: %v (raw: %q)", remote, err, line)
		return
	}

	// Send ACK.
	ack := SIAAck(msg.Sequence, msg.AccountID)
	if _, err := conn.Write(ack); err != nil {
		log.Printf("[DMP] [WARN] failed to send ACK to %s: %v", remote, err)
	}

	// Parse event.
	event, err := ParseAlarmEvent(msg)
	if err != nil {
		log.Printf("[DMP] [DEBUG] non-event message from %s account %s: %v", remote, msg.AccountID, err)
		return
	}

	log.Printf("[DMP] [INFO] alarm event: account=%s code=%s%s zone=%d area=%d desc=%q severity=%s",
		event.AccountID, event.EventQualifier, event.EventCode,
		event.Zone, event.Area, event.Description, event.Severity)

	if r.Handler != nil {
		r.Handler(event)
	}
}

// scanSIAMessages is a bufio.SplitFunc that splits on CR (\r) characters,
// which terminate SIA protocol messages.
func scanSIAMessages(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Look for CR as message delimiter.
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' {
			return i + 1, data[:i], nil
		}
	}

	// Also accept LF as delimiter for flexibility.
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' && i > 0 {
			// Only split on LF if there's content before it.
			return i + 1, data[:i], nil
		}
	}

	if atEOF {
		return len(data), data, nil
	}

	// Request more data.
	return 0, nil, nil
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}

func isClosedError(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF {
		return true
	}
	return strings.Contains(err.Error(), "use of closed network connection")
}
