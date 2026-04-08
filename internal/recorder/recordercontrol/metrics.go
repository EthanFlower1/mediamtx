package recordercontrol

import "sync/atomic"

// clientMetrics holds lightweight in-process counters. We use atomic int64s
// rather than the prometheus client library because prometheus/client_golang
// is not yet in go.mod and adding it is a separate dependency decision.
//
// When the project adopts prometheus/client_golang these fields can be
// replaced by prometheus.Gauge / prometheus.Counter with zero behavior
// change to callers.
//
// Metric names follow the spec:
//   recordercontrol_client_connected{recorder_id=}            — gauge (0/1)
//   recordercontrol_client_reconnects_total{reason=}          — counter
//   recordercontrol_client_events_applied_total{event_type=}  — counter
//   recordercontrol_client_reconcile_runs_total{result=}      — counter
type clientMetrics struct {
	// connected is 1 when the stream is up, 0 when not.
	connected int64

	// reconnects by reason string.
	reconnectStreamDrop  int64
	reconnectForceResync int64

	// events applied by type.
	eventsSnapshot int64
	eventsAdded    int64
	eventsUpdated  int64
	eventsRemoved  int64
	eventsHeartbeat int64

	// reconcile runs by result.
	reconcileOK      int64
	reconcilePartial int64
	reconcileError   int64
}

func (m *clientMetrics) setConnected(v bool) {
	if v {
		atomic.StoreInt64(&m.connected, 1)
	} else {
		atomic.StoreInt64(&m.connected, 0)
	}
}

func (m *clientMetrics) incReconnect(reason string) {
	switch reason {
	case "stream_drop":
		atomic.AddInt64(&m.reconnectStreamDrop, 1)
	case "force_resync":
		atomic.AddInt64(&m.reconnectForceResync, 1)
	}
}

func (m *clientMetrics) incEvent(kind string) {
	switch kind {
	case kindSnapshot:
		atomic.AddInt64(&m.eventsSnapshot, 1)
	case kindCameraAdded:
		atomic.AddInt64(&m.eventsAdded, 1)
	case kindCameraUpdated:
		atomic.AddInt64(&m.eventsUpdated, 1)
	case kindCameraRemoved:
		atomic.AddInt64(&m.eventsRemoved, 1)
	case kindHeartbeat:
		atomic.AddInt64(&m.eventsHeartbeat, 1)
	}
}

func (m *clientMetrics) incReconcile(result string) {
	switch result {
	case reconcileResultOK:
		atomic.AddInt64(&m.reconcileOK, 1)
	case reconcileResultPartial:
		atomic.AddInt64(&m.reconcilePartial, 1)
	case reconcileResultError:
		atomic.AddInt64(&m.reconcileError, 1)
	}
}

// Snapshot returns a point-in-time copy of all counters for tests / health
// probes without holding any lock.
func (m *clientMetrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Connected:            atomic.LoadInt64(&m.connected),
		ReconnectStreamDrop:  atomic.LoadInt64(&m.reconnectStreamDrop),
		ReconnectForceResync: atomic.LoadInt64(&m.reconnectForceResync),
		EventsSnapshot:       atomic.LoadInt64(&m.eventsSnapshot),
		EventsAdded:          atomic.LoadInt64(&m.eventsAdded),
		EventsUpdated:        atomic.LoadInt64(&m.eventsUpdated),
		EventsRemoved:        atomic.LoadInt64(&m.eventsRemoved),
		EventsHeartbeat:      atomic.LoadInt64(&m.eventsHeartbeat),
		ReconcileOK:          atomic.LoadInt64(&m.reconcileOK),
		ReconcilePartial:     atomic.LoadInt64(&m.reconcilePartial),
		ReconcileError:       atomic.LoadInt64(&m.reconcileError),
	}
}

// MetricsSnapshot is a copyable view of clientMetrics at a point in time.
type MetricsSnapshot struct {
	Connected int64

	ReconnectStreamDrop  int64
	ReconnectForceResync int64

	EventsSnapshot  int64
	EventsAdded     int64
	EventsUpdated   int64
	EventsRemoved   int64
	EventsHeartbeat int64

	ReconcileOK      int64
	ReconcilePartial int64
	ReconcileError   int64
}
