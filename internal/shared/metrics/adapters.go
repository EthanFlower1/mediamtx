package metrics

// DirectoryIngestAdapter implements directoryingest.MetricsProvider backed by
// the shared Prometheus registry.
//
// Usage:
//
//	reg := metrics.New()
//	m := metrics.NewDirectoryIngestMetrics(reg)
//	adapter := metrics.NewDirectoryIngestAdapter(m)
//	// pass adapter to directoryingest.Config.Metrics
type DirectoryIngestAdapter struct {
	m *DirectoryIngestMetrics
}

// NewDirectoryIngestAdapter wraps a DirectoryIngestMetrics into the narrow
// directoryingest.MetricsProvider interface.
func NewDirectoryIngestAdapter(m *DirectoryIngestMetrics) *DirectoryIngestAdapter {
	return &DirectoryIngestAdapter{m: m}
}

// IngestMessagesTotal increments the message counter. Fail-open: panics are
// recovered so metrics never block the ingest path.
func (a *DirectoryIngestAdapter) IngestMessagesTotal(stream, result string) {
	safeInc(func() { a.m.MessagesTotal.WithLabelValues(stream, result).Inc() })
}

// BackpressureDropsTotal increments the drop counter. Fail-open.
func (a *DirectoryIngestAdapter) BackpressureDropsTotal() {
	safeInc(func() { a.m.BackpressureDropsTotal.Inc() })
}

// RecorderControlAdapter implements recordercontrol.MetricsProvider backed by
// the shared Prometheus registry.
type RecorderControlAdapter struct {
	m *RecorderControlMetrics
}

// NewRecorderControlAdapter wraps RecorderControlMetrics into the narrow
// recordercontrol.MetricsProvider interface.
func NewRecorderControlAdapter(m *RecorderControlMetrics) *RecorderControlAdapter {
	return &RecorderControlAdapter{m: m}
}

// SetConnected sets the connected gauge for the given recorder.
func (a *RecorderControlAdapter) SetConnected(recorderID string, connected bool) {
	v := 0.0
	if connected {
		v = 1.0
	}
	safeInc(func() { a.m.Connected.WithLabelValues(recorderID).Set(v) })
}

// IncEventApplied increments the events-applied counter for the given type.
func (a *RecorderControlAdapter) IncEventApplied(eventType string) {
	safeInc(func() { a.m.EventsAppliedTotal.WithLabelValues(eventType).Inc() })
}

// IncReconnect increments the reconnect counter for the given reason.
func (a *RecorderControlAdapter) IncReconnect(reason string) {
	safeInc(func() { a.m.ReconnectsTotal.WithLabelValues(reason).Inc() })
}
