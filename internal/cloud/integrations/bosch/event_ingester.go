package bosch

import (
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"
)

// EventHandler is called for each decoded alarm event.
type EventHandler func(event *AlarmEvent)

// EventIngester decodes raw Mode2 event frames from a Bosch panel into
// normalized AlarmEvent structs and dispatches them to registered handlers.
type EventIngester struct {
	panelID  string
	tenantID string

	mu       sync.RWMutex
	handlers []EventHandler

	// Stats
	eventsProcessed int64
	eventsDropped   int64
}

// NewEventIngester creates an ingester for a specific panel.
func NewEventIngester(panelID, tenantID string) *EventIngester {
	return &EventIngester{
		panelID:  panelID,
		tenantID: tenantID,
	}
}

// OnEvent registers an event handler. Multiple handlers are supported.
func (e *EventIngester) OnEvent(handler EventHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers = append(e.handlers, handler)
}

// HandleFrame is the FrameHandler callback for the Client. It filters for
// event report frames, decodes them, and dispatches to registered handlers.
func (e *EventIngester) HandleFrame(frame *Frame) {
	if frame.Command != cmdEventReport {
		return
	}

	event, err := e.decodeEvent(frame.Payload)
	if err != nil {
		log.Printf("[bosch] [ingester] failed to decode event: %v", err)
		e.mu.Lock()
		e.eventsDropped++
		e.mu.Unlock()
		return
	}

	e.mu.Lock()
	e.eventsProcessed++
	e.mu.Unlock()

	e.dispatch(event)
}

// Stats returns ingestion statistics.
func (e *EventIngester) Stats() (processed, dropped int64) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.eventsProcessed, e.eventsDropped
}

func (e *EventIngester) dispatch(event *AlarmEvent) {
	e.mu.RLock()
	handlers := make([]EventHandler, len(e.handlers))
	copy(handlers, e.handlers)
	e.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
}

// decodeEvent parses a Mode2 event report payload into an AlarmEvent.
//
// Event report payload format (B/G-Series):
//   [SEQ][EVENT_HI][EVENT_LO][ZONE_HI][ZONE_LO][AREA][USER_HI][USER_LO][PRIORITY]
//
// Minimum payload size: 9 bytes.
func (e *EventIngester) decodeEvent(payload []byte) (*AlarmEvent, error) {
	if len(payload) < 9 {
		return nil, ErrFrameTooShort
	}

	eventCode := binary.BigEndian.Uint16(payload[1:3])
	zoneNum := int(binary.BigEndian.Uint16(payload[3:5]))
	areaNum := int(payload[5])
	userNum := int(binary.BigEndian.Uint16(payload[6:8]))
	priority := int(payload[8])

	eventType, message := classifyEvent(eventCode)

	return &AlarmEvent{
		PanelID:    e.panelID,
		TenantID:   e.tenantID,
		EventType:  eventType,
		ZoneNumber: zoneNum,
		AreaNumber: areaNum,
		UserNumber: userNum,
		Priority:   priority,
		RawCode:    eventCode,
		Message:    message,
		Timestamp:  time.Now().UTC(),
	}, nil
}

// classifyEvent maps a Bosch SIA/CID event code to an EventType and
// human-readable message. Codes follow the SIA DC-05 standard as
// implemented by Bosch B/G-Series panels.
func classifyEvent(code uint16) (EventType, string) {
	switch {
	// Fire alarms: E110-E119
	case code >= 0x0110 && code <= 0x0119:
		return EventFire, "Fire alarm"

	// Panic alarms: E120-E129
	case code >= 0x0120 && code <= 0x0129:
		return EventPanic, "Panic alarm"

	// Burglary alarms: E130-E139
	case code >= 0x0130 && code <= 0x0139:
		return EventBurglary, "Burglary alarm"

	// Supervisory: E200-E299
	case code >= 0x0200 && code <= 0x0299:
		return EventSupervisory, "Supervisory event"

	// Zone faults: E370-E379 (checked before broader trouble range)
	case code >= 0x0370 && code <= 0x0379:
		return EventZoneFault, "Zone fault"

	// Trouble conditions: E300-E399 (excluding zone faults above)
	case code >= 0x0300 && code <= 0x0399:
		return EventTrouble, "System trouble"

	// Arm/Disarm: E400-E409 (arm), E440-E449 (disarm)
	case code >= 0x0400 && code <= 0x0409:
		return EventArmDisarm, "Area armed"
	case code >= 0x0440 && code <= 0x0449:
		return EventArmDisarm, "Area disarmed"

	// Zone restores: E570-E579
	case code >= 0x0570 && code <= 0x0579:
		return EventZoneRestore, "Zone restore"

	default:
		return EventUnknown, fmt.Sprintf("Unknown event code 0x%04X", code)
	}
}

