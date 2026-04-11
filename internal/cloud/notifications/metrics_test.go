package notifications_test

import (
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

func TestMetricsCollectorRecordAndSnapshot(t *testing.T) {
	mc := notifications.NewMetricsCollector()

	mc.RecordSent("sendgrid", 5)
	mc.RecordFailed("sendgrid", 2)
	mc.RecordRetried("sendgrid", 3)
	mc.RecordDLQ("sendgrid", 1)

	mc.RecordSent("twilio", 10)
	mc.RecordFailed("twilio", 0)

	snap := mc.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 channel snapshots, got %d", len(snap))
	}

	byChannel := make(map[string]notifications.ChannelSnapshot)
	for _, s := range snap {
		byChannel[s.Channel] = s
	}

	sg := byChannel["sendgrid"]
	if sg.Sent != 5 {
		t.Errorf("sendgrid sent: expected 5, got %d", sg.Sent)
	}
	if sg.Failed != 2 {
		t.Errorf("sendgrid failed: expected 2, got %d", sg.Failed)
	}
	if sg.Retried != 3 {
		t.Errorf("sendgrid retried: expected 3, got %d", sg.Retried)
	}
	if sg.DLQ != 1 {
		t.Errorf("sendgrid dlq: expected 1, got %d", sg.DLQ)
	}

	tw := byChannel["twilio"]
	if tw.Sent != 10 {
		t.Errorf("twilio sent: expected 10, got %d", tw.Sent)
	}
}

func TestMetricsCollectorLatency(t *testing.T) {
	mc := notifications.NewMetricsCollector()
	for i := 0; i < 150; i++ {
		mc.RecordLatency("sendgrid", time.Duration(i)*time.Millisecond)
	}
	// Should not panic; ring buffer caps at 100 samples.
}

func TestMetricsCollectorPrometheusFormat(t *testing.T) {
	mc := notifications.NewMetricsCollector()
	mc.RecordSent("sendgrid", 42)
	mc.RecordFailed("sendgrid", 3)

	text := mc.PrometheusTextFormat()

	if !strings.Contains(text, "kaivue_notifications_sent_total") {
		t.Error("expected kaivue_notifications_sent_total in output")
	}
	if !strings.Contains(text, `channel="sendgrid"`) {
		t.Error("expected channel label sendgrid")
	}
	if !strings.Contains(text, "42") {
		t.Error("expected value 42 for sent")
	}
	if !strings.Contains(text, "kaivue_notifications_failed_total") {
		t.Error("expected kaivue_notifications_failed_total in output")
	}
}

func TestMetricsCollectorConcurrentAccess(t *testing.T) {
	mc := notifications.NewMetricsCollector()
	done := make(chan struct{})

	// Concurrent writers.
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				mc.RecordSent("sendgrid", 1)
				mc.RecordFailed("twilio", 1)
				mc.RecordLatency("sendgrid", time.Millisecond)
			}
		}()
	}

	// Concurrent reader.
	go func() {
		defer func() { done <- struct{}{} }()
		for j := 0; j < 100; j++ {
			_ = mc.Snapshot()
			_ = mc.PrometheusTextFormat()
		}
	}()

	for i := 0; i < 11; i++ {
		<-done
	}

	snap := mc.Snapshot()
	byChannel := make(map[string]notifications.ChannelSnapshot)
	for _, s := range snap {
		byChannel[s.Channel] = s
	}
	if byChannel["sendgrid"].Sent != 1000 {
		t.Errorf("expected 1000 sent, got %d", byChannel["sendgrid"].Sent)
	}
	if byChannel["twilio"].Failed != 1000 {
		t.Errorf("expected 1000 failed, got %d", byChannel["twilio"].Failed)
	}
}
