package legacydb

import "time"

// StreamBitrateInfo holds observed bitrate data for a stream.
type StreamBitrateInfo struct {
	StreamID   string  `json:"stream_id"`
	StreamName string  `json:"stream_name"`
	BitrateBps float64 `json:"bitrate_bps"`
	Source     string  `json:"bitrate_source"`
}

// GetStreamBitrates returns the average bitrate for each stream of a camera,
// calculated from recordings in the last 7 days.
func (d *DB) GetStreamBitrates(cameraID string) ([]StreamBitrateInfo, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(timeFormat)

	observed := make(map[string]StreamBitrateInfo)
	rows, err := d.Query(`
		SELECT r.stream_id, COALESCE(cs.name, ''), SUM(r.file_size), SUM(r.duration_ms)
		FROM recordings r
		LEFT JOIN camera_streams cs ON r.stream_id = cs.id
		WHERE r.camera_id = ? AND r.start_time > ? AND r.duration_ms > 0
		GROUP BY r.stream_id`, cameraID, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var streamID, name string
		var totalBytes, totalMs int64
		if err := rows.Scan(&streamID, &name, &totalBytes, &totalMs); err != nil {
			return nil, err
		}
		if totalMs > 0 {
			bitrate := float64(totalBytes) * 8 * 1000 / float64(totalMs)
			observed[streamID] = StreamBitrateInfo{
				StreamID:   streamID,
				StreamName: name,
				BitrateBps: bitrate,
				Source:     "observed",
			}
		}
	}

	streams, err := d.ListCameraStreams(cameraID)
	if err != nil {
		return nil, err
	}

	var results []StreamBitrateInfo
	for _, s := range streams {
		if info, ok := observed[s.ID]; ok {
			results = append(results, info)
		} else {
			results = append(results, StreamBitrateInfo{
				StreamID:   s.ID,
				StreamName: s.Name,
				BitrateBps: estimateBitrate(s),
				Source:     "estimated",
			})
		}
	}
	return results, nil
}

func estimateBitrate(s *CameraStream) float64 {
	pixels := s.Width * s.Height
	if pixels <= 0 {
		pixels = 1920 * 1080
	}
	baseBps := float64(pixels) / float64(1920*1080) * 4_000_000
	codec := s.VideoCodec
	if codec == "H265" || codec == "h265" || codec == "HEVC" || codec == "hevc" {
		baseBps *= 0.6
	}
	return baseBps
}

// GetEventFrequency returns average events/day and average event duration
// for a camera over the last 7 days.
func (d *DB) GetEventFrequency(cameraID string) (eventsPerDay float64, avgDurationSec float64, source string, err error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).UTC().Format(timeFormat)

	var count int64
	var totalDurationSec float64
	err = d.QueryRow(`
		SELECT COUNT(*),
			COALESCE(SUM((julianday(ended_at) - julianday(started_at)) * 86400), 0)
		FROM motion_events
		WHERE camera_id = ? AND started_at > ? AND ended_at IS NOT NULL`,
		cameraID, cutoff,
	).Scan(&count, &totalDurationSec)
	if err != nil {
		return 0, 0, "", err
	}

	if count == 0 {
		return 1.0, 3600, "default", nil
	}

	days := 7.0
	eventsPerDay = float64(count) / days
	avgDurationSec = totalDurationSec / float64(count)
	return eventsPerDay, avgDurationSec, "historical", nil
}
