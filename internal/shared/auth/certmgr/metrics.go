package certmgr

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// RenewalResult is the label value for certmgr_renewals_total{result=}.
type RenewalResult string

const (
	RenewalResultOK      RenewalResult = "ok"
	RenewalResultError   RenewalResult = "error"
	RenewalResultSkipped RenewalResult = "skipped"
)

// ReEnrollResult is the label value for certmgr_reenrollments_total{result=}.
type ReEnrollResult string

const (
	ReEnrollResultOK    ReEnrollResult = "ok"
	ReEnrollResultError ReEnrollResult = "error"
)

// Metrics holds Prometheus-exposition-compatible counters and gauges for the
// certmgr package. It emits plain-text Prometheus format (no dependency on
// prometheus/client_golang — consistent with the rest of the codebase until
// that dep is added to go.mod).
//
// All fields are safe for concurrent use.
type Metrics struct {
	// certExpiresAt is the Unix timestamp (seconds) of the current cert's
	// NotAfter. Exposed as certmgr_cert_expires_at gauge.
	certExpiresAtSec atomic.Int64

	// renewalsOK counts successful renewals.
	renewalsOK atomic.Uint64
	// renewalsError counts renewal attempts that returned an error.
	renewalsError atomic.Uint64
	// renewalsSkipped counts tick cycles where the cert was still valid and
	// no renewal was attempted.
	renewalsSkipped atomic.Uint64

	// reenrollmentsOK counts successful re-enrollments.
	reenrollmentsOK atomic.Uint64
	// reenrollmentsError counts re-enrollment attempts that returned an error.
	reenrollmentsError atomic.Uint64
}

// NewMetrics constructs a zeroed Metrics. Pass the returned value to Config.
func NewMetrics() *Metrics { return &Metrics{} }

// SetCertExpiry records the cert expiry time. Called whenever the active cert
// changes (initial load, renewal, re-enrollment).
func (m *Metrics) SetCertExpiry(t time.Time) {
	m.certExpiresAtSec.Store(t.Unix())
}

// RecordRenewal increments the renewal counter for result.
func (m *Metrics) RecordRenewal(r RenewalResult) {
	switch r {
	case RenewalResultOK:
		m.renewalsOK.Add(1)
	case RenewalResultError:
		m.renewalsError.Add(1)
	default:
		m.renewalsSkipped.Add(1)
	}
}

// RecordReEnrollment increments the re-enrollment counter for result.
func (m *Metrics) RecordReEnrollment(r ReEnrollResult) {
	switch r {
	case ReEnrollResultOK:
		m.reenrollmentsOK.Add(1)
	default:
		m.reenrollmentsError.Add(1)
	}
}

// WritePrometheus writes the Prometheus text-exposition lines for all
// certmgr metrics to w. Callers can integrate this into their existing
// /metrics handler.
func (m *Metrics) WritePrometheus(w io.Writer) {
	fmt.Fprintf(w, "# HELP certmgr_cert_expires_at Unix timestamp (seconds) when the active mTLS leaf cert expires.\n")
	fmt.Fprintf(w, "# TYPE certmgr_cert_expires_at gauge\n")
	fmt.Fprintf(w, "certmgr_cert_expires_at %d\n", m.certExpiresAtSec.Load())

	fmt.Fprintf(w, "# HELP certmgr_renewals_total Number of cert renewal attempts, labelled by result.\n")
	fmt.Fprintf(w, "# TYPE certmgr_renewals_total counter\n")
	fmt.Fprintf(w, "certmgr_renewals_total{result=\"ok\"} %d\n", m.renewalsOK.Load())
	fmt.Fprintf(w, "certmgr_renewals_total{result=\"error\"} %d\n", m.renewalsError.Load())
	fmt.Fprintf(w, "certmgr_renewals_total{result=\"skipped\"} %d\n", m.renewalsSkipped.Load())

	fmt.Fprintf(w, "# HELP certmgr_reenrollments_total Number of re-enrollment attempts (fallback path), labelled by result.\n")
	fmt.Fprintf(w, "# TYPE certmgr_reenrollments_total counter\n")
	fmt.Fprintf(w, "certmgr_reenrollments_total{result=\"ok\"} %d\n", m.reenrollmentsOK.Load())
	fmt.Fprintf(w, "certmgr_reenrollments_total{result=\"error\"} %d\n", m.reenrollmentsError.Load())
}
