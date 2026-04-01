package integrity

import (
	"context"
	"fmt"
	"os"
	"time"
)

// ScanItem pairs a recording ID with the info needed for verification.
type ScanItem struct {
	RecordingID int64
	CameraID    string
	Info        RecordingInfo
}

// Scanner runs periodic background integrity verification.
type Scanner struct {
	Interval  time.Duration
	BatchSize int
	FetchFunc func(cutoff time.Time, batchSize int) ([]ScanItem, error)
	OnResult  func(recordingID int64, result VerificationResult)
}

// Run starts the scanner loop. It blocks until ctx is cancelled.
func (s *Scanner) Run(ctx context.Context) {
	// Run immediately on start, then on interval.
	s.scan()

	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scan()
		}
	}
}

func (s *Scanner) scan() {
	cutoff := time.Now().Add(-24 * time.Hour)
	items, err := s.FetchFunc(cutoff, s.BatchSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NVR: integrity scanner fetch failed: %v\n", err)
		return
	}

	for _, item := range items {
		result := VerifySegment(item.Info)
		s.OnResult(item.RecordingID, result)
	}
}
