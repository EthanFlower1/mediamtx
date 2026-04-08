// Package recordercontrol implements the server-side of the Directory → Recorder
// push channel defined in internal/shared/proto/v1/recorder_control.proto.
//
// The public surface is:
//
//   - EventBus      — in-process pub/sub; one fan-out queue per recorder.
//   - Handler       — the Connect-Go / http.Handler that serves StreamAssignments.
//   - CameraStore   — interface the Handler queries to load the initial Snapshot.
//   - RecorderStore — interface the Handler calls to mark DEGRADED / OFFLINE state.
//
// KAI-252: server side only. The Recorder client (KAI-253, Wave 2) is a
// sibling ticket that depends on this proto + server. Do NOT touch
// internal/recorder/*.
package recordercontrol

import (
	"sync"
	"time"
)

// EventKind is the discriminant for AssignmentEvent payloads carried over
// the bus. Mirrors the oneof variants in AssignmentEvent proto.
type EventKind int

const (
	EventKindCameraAdded   EventKind = iota + 1
	EventKindCameraUpdated           // camera config changed
	EventKindCameraRemoved           // camera removed / unassigned
	EventKindForceResync             // server signals the stream must resync on reconnect
)

// CameraPayload is the in-process representation of a camera record passed
// through the event bus. The credential_ref is an opaque blob ref — the
// Recorder fetches the actual secret from the cryptostore (KAI-251).
//
// TODO(KAI-249): replace with the generated proto Camera type once
// buf generate is wired up (KAI-310). For now this mirrors the Camera
// message in cameras.proto without the protobuf dependency.
type CameraPayload struct {
	ID           string
	TenantID     string // Must match the Recorder's tenant — checked on publish.
	RecorderID   string
	Name         string
	CredentialRef string // opaque ref, NEVER a plaintext secret
	ConfigJSON   string // serialized CameraConfig; Recorder applies atomically
	ConfigVersion int64
}

// RemovalPayload carries the minimal data needed for a CameraRemoved event.
type RemovalPayload struct {
	CameraID        string
	TenantID        string
	PurgeRecordings bool
	Reason          string
}

// BusEvent is a single event queued per-recorder. Exactly one of the
// payload fields is non-zero, selected by Kind.
type BusEvent struct {
	Kind    EventKind
	Camera  *CameraPayload // set for Added / Updated
	Removal *RemovalPayload // set for Removed
	// Version is a monotonically increasing counter stamped by the bus on
	// publish. Subscribers receive events in version order within a stream.
	Version   int64
	EmittedAt time.Time
}

// subscriber is one active StreamAssignments stream. It holds a bounded
// channel draining to the HTTP streaming handler.
//
// queueSize is the per-recorder bounded queue depth mandated by the design
// (§8.3 back-pressure). Exceeding it forces a full resync on reconnect.
const queueSize = 256

type subscriber struct {
	recorderID string
	tenantID   string
	ch         chan BusEvent
	closed     bool
}

// EventBus is the in-process pub/sub hub for recorder control events.
// It is goroutine-safe. One EventBus instance is shared by the entire
// server process (instantiate in main / server setup and pass to Handler).
//
// Fan-out model: Publish delivers to all subscribers registered for a
// (tenantID, recorderID) pair. If a subscriber's queue is full the oldest
// event is dropped and a ForceResync sentinel is queued so the Recorder
// knows it missed events and must resync on the next reconnect.
type EventBus struct {
	mu      sync.Mutex
	subs    map[string][]*subscriber // key: tenantID+"/"+recorderID
	version int64
}

// NewEventBus constructs a ready-to-use EventBus.
func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[string][]*subscriber)}
}

// Subscribe registers a new subscriber for (tenantID, recorderID) events.
// The returned channel receives all future events for this recorder.
// The caller MUST call Unsubscribe when the stream closes to avoid leaking
// the entry in the map.
func (b *EventBus) Subscribe(tenantID, recorderID string) (<-chan BusEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &subscriber{
		recorderID: recorderID,
		tenantID:   tenantID,
		ch:         make(chan BusEvent, queueSize),
	}
	key := busKey(tenantID, recorderID)
	b.subs[key] = append(b.subs[key], sub)

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if sub.closed {
			return
		}
		sub.closed = true
		close(sub.ch)
		list := b.subs[key]
		updated := list[:0]
		for _, s := range list {
			if s != sub {
				updated = append(updated, s)
			}
		}
		if len(updated) == 0 {
			delete(b.subs, key)
		} else {
			b.subs[key] = updated
		}
	}
	return sub.ch, cancel
}

// Publish fans out ev to every subscriber of (tenantID, recorderID).
//
// Multi-tenant safety: tenantID MUST be derived from the authenticated
// session when the change originated in an API handler — the bus does not
// re-derive it. This is the same seam as the DB helpers: callers are
// responsible for naming the correct tenant.
//
// Back-pressure: if a subscriber's channel is full the oldest event is
// discarded and a ForceResync sentinel replaces it. The Recorder will
// re-open the stream and receive a fresh Snapshot.
func (b *EventBus) Publish(tenantID, recorderID string, ev BusEvent) {
	b.mu.Lock()
	key := busKey(tenantID, recorderID)
	subs := make([]*subscriber, len(b.subs[key]))
	copy(subs, b.subs[key])
	b.version++
	ev.Version = b.version
	if ev.EmittedAt.IsZero() {
		ev.EmittedAt = time.Now().UTC()
	}
	b.mu.Unlock()

	for _, sub := range subs {
		b.deliver(sub, ev)
	}
}

// deliver attempts a non-blocking send. On overflow it drops the oldest
// message from the buffered channel and enqueues a ForceResync sentinel
// so the stream handler knows it must trigger a full resync on reconnect.
func (b *EventBus) deliver(sub *subscriber, ev BusEvent) {
	select {
	case sub.ch <- ev:
		// fast path: channel had space
	default:
		// Channel full — drain one event and push a resync sentinel.
		select {
		case <-sub.ch:
		default:
		}
		resync := BusEvent{
			Kind:      EventKindForceResync,
			EmittedAt: time.Now().UTC(),
		}
		select {
		case sub.ch <- resync:
		default:
			// If even the resync can't fit, the channel is in serious
			// trouble. The handler will time out and the next reconnect
			// will get a Snapshot regardless.
		}
	}
}

// busKey is the fan-out map key. Embedding tenantID prevents cross-tenant
// event delivery even if two tenants have a recorder with the same ID.
func busKey(tenantID, recorderID string) string {
	return tenantID + "/" + recorderID
}
