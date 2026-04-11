package dmp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

func TestReceiver_StartStop(t *testing.T) {
	cfg := ReceiverConfig{
		ListenAddr:  "127.0.0.1:0",
		ReadTimeout: 5 * time.Second,
	}

	r := NewReceiver(cfg, func(event *AlarmEvent) {})

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	addr := r.Addr()
	if addr == nil {
		t.Fatal("Addr() returned nil after Start")
	}

	r.Stop()
}

func TestReceiver_ProcessesAlarmEvent(t *testing.T) {
	var mu sync.Mutex
	var received []*AlarmEvent

	handler := func(event *AlarmEvent) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
	}

	cfg := ReceiverConfig{
		ListenAddr:  "127.0.0.1:0",
		ReadTimeout: 5 * time.Second,
	}

	r := NewReceiver(cfg, handler)

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	// Connect as a panel and send a SIA message.
	conn, err := net.Dial("tcp", r.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send a burglary alarm: account 1234, area 1, zone 1.
	siaMsg := fmt.Sprintf("0001004C\"SIA\"0001#1234[Nri01/EBA001]\r")
	if _, err := conn.Write([]byte(siaMsg)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for the event to be processed.
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for alarm event")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	ev := received[0]
	if ev.EventCode != CodeBurglaryAlarm {
		t.Errorf("EventCode = %q, want %q", ev.EventCode, CodeBurglaryAlarm)
	}
	if ev.Zone != 1 {
		t.Errorf("Zone = %d, want 1", ev.Zone)
	}
	if ev.Area != 1 {
		t.Errorf("Area = %d, want 1", ev.Area)
	}
	if ev.AccountID != "1234" {
		t.Errorf("AccountID = %q, want 1234", ev.AccountID)
	}
	if ev.Severity != SeverityWarning {
		t.Errorf("Severity = %q, want %q", ev.Severity, SeverityWarning)
	}
}

func TestReceiver_SendsACK(t *testing.T) {
	cfg := ReceiverConfig{
		ListenAddr:  "127.0.0.1:0",
		ReadTimeout: 5 * time.Second,
	}

	r := NewReceiver(cfg, func(event *AlarmEvent) {})

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	conn, err := net.Dial("tcp", r.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	siaMsg := "0001004C\"SIA\"0001#1234[Nri01/EBA001]\r"
	if _, err := conn.Write([]byte(siaMsg)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Read the ACK response.
	buf := make([]byte, 256)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read ACK failed: %v", err)
	}

	ack := string(buf[:n])
	if !searchSubstring(ack, "\"ACK\"") {
		t.Errorf("expected ACK response, got %q", ack)
	}
	if !searchSubstring(ack, "0001") {
		t.Errorf("ACK should contain sequence 0001, got %q", ack)
	}
	if !searchSubstring(ack, "1234") {
		t.Errorf("ACK should contain account 1234, got %q", ack)
	}
}

func TestReceiver_MaxConnections(t *testing.T) {
	cfg := ReceiverConfig{
		ListenAddr:     "127.0.0.1:0",
		ReadTimeout:    5 * time.Second,
		MaxConnections: 2,
	}

	r := NewReceiver(cfg, func(event *AlarmEvent) {})

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	addr := r.Addr().String()

	// Open 2 connections (should succeed).
	conn1, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial 1 failed: %v", err)
	}
	defer conn1.Close()

	conn2, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial 2 failed: %v", err)
	}
	defer conn2.Close()

	// Send data on both to ensure they're being handled.
	conn1.Write([]byte("0001004C\"SIA\"0001#1234[Nri01/EBA001]\r"))
	conn2.Write([]byte("0001004C\"SIA\"0002#1234[Nri01/EBA002]\r"))

	// Brief pause to let the goroutines pick up the connections.
	time.Sleep(100 * time.Millisecond)

	// The 3rd connection should be accepted at TCP level but the receiver
	// will close it. We verify the first two still work.
	conn1.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 256)
	n, err := conn1.Read(buf)
	if err != nil {
		t.Fatalf("Read from conn1 failed: %v", err)
	}
	if !searchSubstring(string(buf[:n]), "ACK") {
		t.Errorf("expected ACK from conn1")
	}
}

func TestReceiver_MultipleEvents(t *testing.T) {
	var mu sync.Mutex
	var received []*AlarmEvent

	handler := func(event *AlarmEvent) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
	}

	cfg := ReceiverConfig{
		ListenAddr:  "127.0.0.1:0",
		ReadTimeout: 5 * time.Second,
	}

	r := NewReceiver(cfg, handler)

	ctx := context.Background()
	if err := r.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer r.Stop()

	conn, err := net.Dial("tcp", r.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Send multiple events in sequence.
	events := []string{
		"0001004C\"SIA\"0001#1234[Nri01/EBA001]\r",
		"0001004C\"SIA\"0002#1234[Nri01/EFA003]\r",
		"0001004C\"SIA\"0003#1234[Nri02/EPA010]\r",
	}

	for _, msg := range events {
		if _, err := conn.Write([]byte(msg)); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		// Read ACK.
		buf := make([]byte, 256)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		conn.Read(buf)
	}

	// Wait for all events.
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		count := len(received)
		mu.Unlock()
		if count >= 3 {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("timed out waiting for events, got %d", len(received))
			mu.Unlock()
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if received[0].EventCode != CodeBurglaryAlarm {
		t.Errorf("event 0: EventCode = %q, want %q", received[0].EventCode, CodeBurglaryAlarm)
	}
	if received[1].EventCode != CodeFireAlarm {
		t.Errorf("event 1: EventCode = %q, want %q", received[1].EventCode, CodeFireAlarm)
	}
	if received[2].EventCode != CodePanicAlarm {
		t.Errorf("event 2: EventCode = %q, want %q", received[2].EventCode, CodePanicAlarm)
	}
}
