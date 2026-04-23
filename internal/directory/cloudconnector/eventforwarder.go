package cloudconnector

import "sync/atomic"

// EventSender is a callback that transmits an event to the cloud.
type EventSender func(EventPayload) error

// EventForwarder buffers local events and sends them to the cloud via the
// provided sender. Forward never blocks; events are dropped when the buffer
// is full.
type EventForwarder struct {
	sender  EventSender
	ch      chan EventPayload
	dropped atomic.Int64
}

// NewEventForwarder returns a forwarder with the given channel buffer size.
func NewEventForwarder(sender EventSender, bufferSize int) *EventForwarder {
	return &EventForwarder{
		sender: sender,
		ch:     make(chan EventPayload, bufferSize),
	}
}

// Forward enqueues an event without blocking. If the buffer is full the event
// is silently dropped and the drop counter is incremented.
func (f *EventForwarder) Forward(e EventPayload) {
	select {
	case f.ch <- e:
	default:
		f.dropped.Add(1)
	}
}

// Flush drains all buffered events synchronously, calling the sender for each.
func (f *EventForwarder) Flush() {
	for {
		select {
		case e := <-f.ch:
			_ = f.sender(e)
		default:
			return
		}
	}
}

// Dropped returns the number of events that were dropped due to a full buffer.
func (f *EventForwarder) Dropped() int64 {
	return f.dropped.Load()
}
