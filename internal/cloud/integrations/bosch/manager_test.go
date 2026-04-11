package bosch

import (
	"context"
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"
)

// mockConn implements the Conn interface for testing.
type mockConn struct {
	mu       sync.Mutex
	readBuf  []byte
	readPos  int
	written  []byte
	closed   bool
	readErr  error
	writeErr error
}

func newMockConn(data []byte) *mockConn {
	return &mockConn{readBuf: data}
}

func (m *mockConn) Read(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.readPos >= len(m.readBuf) {
		return 0, io.EOF
	}
	n := copy(b, m.readBuf[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.written = append(m.written, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func (m *mockConn) appendData(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readBuf = append(m.readBuf, data...)
}

// buildAuthSequence builds wire data for: auth challenge + auth result (success).
func buildAuthSequence() []byte {
	challenge := &Frame{Command: cmdAuthChallenge, Payload: []byte{0x01}}
	challengeData, _ := challenge.MarshalBinary()

	result := &Frame{Command: cmdAuthResult, Payload: []byte{0x01}} // success
	resultData, _ := result.MarshalBinary()

	return append(challengeData, resultData...)
}

func TestManager_AddAndRemovePanel(t *testing.T) {
	dispatcher := &mockDispatcher{}
	mgr := NewManager(dispatcher)

	// Use a mock dialer that returns a conn with auth sequence + immediate EOF.
	cfg := PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		Port:     7700,
		AuthCode: "1234",
		Series:   PanelSeriesB,
		Enabled:  true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Override the client to use our mock dialer after AddPanel.
	// We need to test the Manager logic, not actual TCP.
	// For this test, just verify the panel management API.
	err := mgr.AddPanel(ctx, cfg)
	if err != nil {
		// Client will fail to connect since there's no real panel,
		// but the manager should still register it.
		t.Fatalf("AddPanel: %v", err)
	}

	// Verify panel is listed.
	panels := mgr.ListPanels()
	if _, ok := panels["panel-1"]; !ok {
		t.Error("panel-1 not found in ListPanels")
	}

	// Duplicate add should fail.
	err = mgr.AddPanel(ctx, cfg)
	if err == nil {
		t.Error("expected error for duplicate panel add")
	}

	// Remove.
	err = mgr.RemovePanel("panel-1")
	if err == nil {
		panels = mgr.ListPanels()
		if len(panels) != 0 {
			t.Errorf("panels after remove: got %d want 0", len(panels))
		}
	}

	// Remove non-existent.
	err = mgr.RemovePanel("panel-999")
	if err == nil {
		t.Error("expected error removing non-existent panel")
	}
}

func TestManager_SetZoneMappings(t *testing.T) {
	dispatcher := &mockDispatcher{}
	mgr := NewManager(dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := PanelConfig{
		ID:       "panel-1",
		TenantID: "tenant-1",
		Host:     "192.168.1.100",
		Port:     7700,
		AuthCode: "1234",
		Series:   PanelSeriesB,
		Enabled:  true,
	}

	_ = mgr.AddPanel(ctx, cfg)

	mappings := []ZoneCameraMapping{
		{
			ID:         "m1",
			TenantID:   "tenant-1",
			PanelID:    "panel-1",
			ZoneNumber: 1,
			CameraIDs:  []string{"cam-a"},
			Actions:    []Action{{Type: ActionRecord, Duration: 30}},
			Enabled:    true,
		},
	}

	err := mgr.SetZoneMappings("panel-1", mappings)
	if err != nil {
		t.Fatalf("SetZoneMappings: %v", err)
	}

	// Set mappings for non-existent panel.
	err = mgr.SetZoneMappings("panel-999", mappings)
	if err == nil {
		t.Error("expected error for non-existent panel")
	}
}

func TestManager_StopAll(t *testing.T) {
	dispatcher := &mockDispatcher{}
	mgr := NewManager(dispatcher)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 3; i++ {
		cfg := PanelConfig{
			ID:       "panel-" + string(rune('a'+i)),
			TenantID: "tenant-1",
			Host:     "192.168.1.100",
			Port:     7700 + i,
			AuthCode: "1234",
			Series:   PanelSeriesB,
			Enabled:  true,
		}
		_ = mgr.AddPanel(ctx, cfg)
	}

	mgr.StopAll()

	panels := mgr.ListPanels()
	if len(panels) != 0 {
		t.Errorf("panels after StopAll: got %d want 0", len(panels))
	}
}

func TestClient_NewClientDefaults(t *testing.T) {
	cfg := ClientConfig{
		Host:     "192.168.1.1",
		AuthCode: "1234",
		Series:   PanelSeriesB,
	}

	client := NewClient(cfg, nil)

	if client.cfg.Port != DefaultPort {
		t.Errorf("default port: got %d want %d", client.cfg.Port, DefaultPort)
	}
	if client.cfg.ConnectTimeout != 10*time.Second {
		t.Errorf("default connect timeout: got %v want 10s", client.cfg.ConnectTimeout)
	}
	if client.cfg.ReadTimeout != 30*time.Second {
		t.Errorf("default read timeout: got %v want 30s", client.cfg.ReadTimeout)
	}
	if client.cfg.HeartbeatInterval != 15*time.Second {
		t.Errorf("default heartbeat interval: got %v want 15s", client.cfg.HeartbeatInterval)
	}
	if client.State() != StateDisconnected {
		t.Errorf("initial state: got %q want %q", client.State(), StateDisconnected)
	}
}

func TestClient_AuthSuccess(t *testing.T) {
	authData := buildAuthSequence()
	conn := newMockConn(authData)

	handler := func(frame *Frame) {}
	client := NewClient(ClientConfig{
		Host:           "test",
		Port:           7700,
		AuthCode:       "5678",
		Series:         PanelSeriesG,
		MaxReconnects:  1,
		ReconnectDelay: 10 * time.Millisecond,
	}, handler)

	client.SetDialer(func(ctx context.Context, network, address string) (Conn, error) {
		return conn, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	client.Start(ctx)

	// Give time for connect + auth + read loop to start and fail on EOF.
	time.Sleep(100 * time.Millisecond)
	cancel()
	client.Stop()

	// Verify auth reply was sent.
	conn.mu.Lock()
	defer conn.mu.Unlock()
	if len(conn.written) == 0 {
		t.Error("no data written to connection (expected auth reply)")
	}
}

func TestClient_AuthFailed(t *testing.T) {
	// Auth challenge + auth result with failure byte.
	client := NewClient(ClientConfig{
		Host:           "test",
		Port:           7700,
		AuthCode:       "wrong",
		Series:         PanelSeriesB,
		MaxReconnects:  1,
		ReconnectDelay: 10 * time.Millisecond,
		ConnectTimeout: 1 * time.Second,
	}, nil)

	dialCount := 0
	client.SetDialer(func(ctx context.Context, network, address string) (Conn, error) {
		dialCount++
		// Return fresh conn each time.
		challengeD, _ := (&Frame{Command: cmdAuthChallenge, Payload: []byte{0x01}}).MarshalBinary()
		resultD, _ := (&Frame{Command: cmdAuthResult, Payload: []byte{0x00}}).MarshalBinary()
		return newMockConn(append(challengeD, resultD...)), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	client.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	client.Stop()

	// Should have tried at least once.
	if dialCount < 1 {
		t.Errorf("dial count: got %d, expected at least 1", dialCount)
	}
}

func TestClient_EventDispatchToHandler(t *testing.T) {
	authData := buildAuthSequence()

	// Build an event report frame to append after auth.
	payload := make([]byte, 9)
	payload[0] = 0x01
	binary.BigEndian.PutUint16(payload[1:3], 0x0130) // burglary
	binary.BigEndian.PutUint16(payload[3:5], 5)       // zone 5
	payload[5] = 0x01
	binary.BigEndian.PutUint16(payload[6:8], 0)
	payload[8] = 3

	eventFrame := &Frame{Command: cmdEventReport, Payload: payload}
	eventData, _ := eventFrame.MarshalBinary()

	connData := append(authData, eventData...)

	var mu sync.Mutex
	var received []*Frame
	handler := func(frame *Frame) {
		mu.Lock()
		received = append(received, frame)
		mu.Unlock()
	}

	client := NewClient(ClientConfig{
		Host:           "test",
		Port:           7700,
		AuthCode:       "1234",
		Series:         PanelSeriesB,
		MaxReconnects:  1,
		ReconnectDelay: 10 * time.Millisecond,
	}, handler)

	client.SetDialer(func(ctx context.Context, network, address string) (Conn, error) {
		return newMockConn(connData), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	client.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	client.Stop()

	mu.Lock()
	defer mu.Unlock()

	if len(received) == 0 {
		t.Fatal("expected at least one frame dispatched to handler")
	}

	gotEvent := false
	for _, f := range received {
		if f.Command == cmdEventReport {
			gotEvent = true
		}
	}
	if !gotEvent {
		t.Error("no event report frame received by handler")
	}
}
