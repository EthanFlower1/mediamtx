package db

import "time"

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
