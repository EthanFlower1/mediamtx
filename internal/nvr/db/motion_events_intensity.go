package db

import "time"

// IntensityBucket holds the event count for a time bucket.
type IntensityBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	Count       int       `json:"count"`
}

// GetMotionIntensity returns event counts bucketed by the given interval.
func (d *DB) GetMotionIntensity(cameraID string, start, end time.Time, bucketSeconds int) ([]IntensityBucket, error) {
	rows, err := d.Query(`
        SELECT
            strftime('%s', started_at) / ? * ? as bucket_epoch,
            COUNT(*) as count
        FROM motion_events
        WHERE camera_id = ?
            AND started_at >= ?
            AND started_at < ?
        GROUP BY bucket_epoch
        ORDER BY bucket_epoch`,
		bucketSeconds, bucketSeconds,
		cameraID,
		start.UTC().Format(timeFormat),
		end.UTC().Format(timeFormat),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []IntensityBucket
	for rows.Next() {
		var epochSec int64
		var count int
		if err := rows.Scan(&epochSec, &count); err != nil {
			return nil, err
		}
		buckets = append(buckets, IntensityBucket{
			BucketStart: time.Unix(epochSec, 0).UTC(),
			Count:       count,
		})
	}
	return buckets, rows.Err()
}
