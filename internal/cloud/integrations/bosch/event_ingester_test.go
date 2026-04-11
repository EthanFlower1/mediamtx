package bosch

import (
	"encoding/binary"
	"sync"
	"testing"
)

func TestEventIngester_HandleFrame_DecodesEvent(t *testing.T) {
	ingester := NewEventIngester("panel-1", "tenant-1")

	var got *AlarmEvent
	var mu sync.Mutex
	ingester.OnEvent(func(event *AlarmEvent) {
		mu.Lock()
		defer mu.Unlock()
		got = event
	})

	// Build an event report frame payload:
	// [SEQ][EVENT_HI][EVENT_LO][ZONE_HI][ZONE_LO][AREA][USER_HI][USER_LO][PRIORITY]
	payload := make([]byte, 9)
	payload[0] = 0x01                                  // seq
	binary.BigEndian.PutUint16(payload[1:3], 0x0130)   // burglary alarm
	binary.BigEndian.PutUint16(payload[3:5], 5)        // zone 5
	payload[5] = 0x01                                  // area 1
	binary.BigEndian.PutUint16(payload[6:8], 0)        // no user
	payload[8] = 3                                     // priority 3

	frame := &Frame{
		Command: cmdEventReport,
		Payload: payload,
	}

	ingester.HandleFrame(frame)

	mu.Lock()
	defer mu.Unlock()

	if got == nil {
		t.Fatal("expected event, got nil")
	}
	if got.PanelID != "panel-1" {
		t.Errorf("PanelID: got %q want %q", got.PanelID, "panel-1")
	}
	if got.TenantID != "tenant-1" {
		t.Errorf("TenantID: got %q want %q", got.TenantID, "tenant-1")
	}
	if got.EventType != EventBurglary {
		t.Errorf("EventType: got %q want %q", got.EventType, EventBurglary)
	}
	if got.ZoneNumber != 5 {
		t.Errorf("ZoneNumber: got %d want %d", got.ZoneNumber, 5)
	}
	if got.AreaNumber != 1 {
		t.Errorf("AreaNumber: got %d want %d", got.AreaNumber, 1)
	}
	if got.Priority != 3 {
		t.Errorf("Priority: got %d want %d", got.Priority, 3)
	}
	if got.RawCode != 0x0130 {
		t.Errorf("RawCode: got 0x%04X want 0x0130", got.RawCode)
	}
}

func TestEventIngester_IgnoresNonEventFrames(t *testing.T) {
	ingester := NewEventIngester("panel-1", "tenant-1")

	called := false
	ingester.OnEvent(func(event *AlarmEvent) {
		called = true
	})

	// Heartbeat frame should be ignored.
	ingester.HandleFrame(&Frame{Command: cmdHeartbeat})
	// Zone status should be ignored.
	ingester.HandleFrame(&Frame{Command: cmdZoneStatus, Payload: []byte{0x01}})

	if called {
		t.Error("handler should not be called for non-event frames")
	}
}

func TestEventIngester_DropsShortPayload(t *testing.T) {
	ingester := NewEventIngester("panel-1", "tenant-1")

	called := false
	ingester.OnEvent(func(event *AlarmEvent) {
		called = true
	})

	// Payload too short (< 9 bytes).
	ingester.HandleFrame(&Frame{
		Command: cmdEventReport,
		Payload: []byte{0x01, 0x02, 0x03},
	})

	if called {
		t.Error("handler should not be called for short payload")
	}

	processed, dropped := ingester.Stats()
	if processed != 0 {
		t.Errorf("processed: got %d want 0", processed)
	}
	if dropped != 1 {
		t.Errorf("dropped: got %d want 1", dropped)
	}
}

func TestEventIngester_MultipleHandlers(t *testing.T) {
	ingester := NewEventIngester("panel-1", "tenant-1")

	var mu sync.Mutex
	callCount := 0
	handler := func(event *AlarmEvent) {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	ingester.OnEvent(handler)
	ingester.OnEvent(handler)
	ingester.OnEvent(handler)

	payload := make([]byte, 9)
	payload[0] = 0x01
	binary.BigEndian.PutUint16(payload[1:3], 0x0110) // fire alarm
	binary.BigEndian.PutUint16(payload[3:5], 1)
	payload[5] = 0x01
	binary.BigEndian.PutUint16(payload[6:8], 0)
	payload[8] = 5

	ingester.HandleFrame(&Frame{
		Command: cmdEventReport,
		Payload: payload,
	})

	mu.Lock()
	defer mu.Unlock()
	if callCount != 3 {
		t.Errorf("callCount: got %d want 3", callCount)
	}
}

func TestClassifyEvent(t *testing.T) {
	tests := []struct {
		code     uint16
		wantType EventType
	}{
		{0x0130, EventBurglary},
		{0x0135, EventBurglary},
		{0x0110, EventFire},
		{0x0115, EventFire},
		{0x0120, EventPanic},
		{0x0125, EventPanic},
		{0x0300, EventTrouble},
		{0x0370, EventZoneFault},
		{0x0570, EventZoneRestore},
		{0x0400, EventArmDisarm},
		{0x0440, EventArmDisarm},
		{0x0200, EventSupervisory},
		{0xFFFF, EventUnknown},
	}

	for _, tc := range tests {
		eventType, msg := classifyEvent(tc.code)
		if eventType != tc.wantType {
			t.Errorf("code 0x%04X: got %q want %q", tc.code, eventType, tc.wantType)
		}
		if msg == "" {
			t.Errorf("code 0x%04X: empty message", tc.code)
		}
	}
}
