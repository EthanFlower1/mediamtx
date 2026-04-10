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
	EventKindForceResync             // server signals the stream must do a full resync
)

// BusEvent is a single event queued per-recorder. Exactly one of the
// payload fields is non-zero, selected by Kind.
type BusEvent struct {
	Kind    EventKind
	Camera  *CameraRow      // set for Added / Updated
	Removal *RemovalPayload // set for Removed
	// Version is a monotonically increasing counter stamped by the bus on
	// publish. Subscribers receive events in version order within a stream.
	Version   int64
	EmittedAt time.Time
}

// RemovalPayload carries the minimal data needed for a CameraRemoved event.
type RemovalPayload struct {
	CameraID        string
	PurgeRecordings bool
	Reason          string
}

// subscriber is one active StreamAssignments stream. It holds a bounded
// channel draining to the HTTP streaming handler.
//
// queueSize is the per-recorder bounded queue depth. Exceeding it forces
// a full resync on reconnect.
const queueSize = 256

type subscriber struct {
	recorderID string
	ch         chan BusEvent
	closed     bool
}

// EventBus is the in-process pub/sub hub for recorder control events on the
// on-prem Directory. It is goroutine-safe. One EventBus instance is shared
// by the entire Directory process.
//
// Unlike the cloud-plane EventBus (which keys on tenantID+recorderID), the
// on-prem EventBus keys only on recorderID because the on-prem Directory
// operates in a single-tenant context.
//
// Fan-out model: Publish delivers to all subscribers registered for a
// recorderID. If a subscriber's queue is full the oldest event is dropped
// and a ForceResync sentinel is queued.
type EventBus struct {
	mu      sync.Mutex
	subs    map[string][]*subscriber // key: recorderID
	version int64
}

// NewEventBus constructs a ready-to-use EventBus.
func NewEventBus() *EventBus {
	return &EventBus{subs: make(map[string][]*subscriber)}
}

// Subscribe registers a new subscriber for recorderID events.
// The returned channel receives all future events for this recorder.
// The caller MUST call the returned unsubscribe function when the stream
// closes to avoid leaking the entry in the map.
func (b *EventBus) Subscribe(recorderID string) (<-chan BusEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &subscriber{
		recorderID: recorderID,
		ch:         make(chan BusEvent, queueSize),
	}
	b.subs[recorderID] = append(b.subs[recorderID], sub)

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if sub.closed {
			return
		}
		sub.closed = true
		close(sub.ch)
		list := b.subs[recorderID]
		updated := list[:0]
		for _, s := range list {
			if s != sub {
				updated = append(updated, s)
			}
		}
		if len(updated) == 0 {
			delete(b.subs, recorderID)
		} else {
			b.subs[recorderID] = updated
		}
	}
	return sub.ch, cancel
}

// Publish fans out ev to every subscriber of recorderID.
//
// Back-pressure: if a subscriber's channel is full the oldest event is
// discarded and a ForceResync sentinel replaces it. The Recorder will
// re-open the stream and receive a fresh Snapshot.
func (b *EventBus) Publish(recorderID string, ev BusEvent) {
	b.mu.Lock()
	subs := make([]*subscriber, len(b.subs[recorderID]))
	copy(subs, b.subs[recorderID])
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

// CurrentVersion returns the latest published version counter. Used when
// constructing Snapshot events so the version is consistent with what
// the bus last emitted.
func (b *EventBus) CurrentVersion() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.version
}

// deliver attempts a non-blocking send. On overflow it drops the oldest
// message and enqueues a ForceResync sentinel.
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
			// If even the resync can't fit, the handler will time out
			// and the next reconnect will get a Snapshot.
		}
	}
}
