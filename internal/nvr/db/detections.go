package db

import "time"

// Detection represents an object detection within a motion event.
type Detection struct {
	ID            int64   `json:"id"`
	MotionEventID int64   `json:"motion_event_id"`
	FrameTime     string  `json:"frame_time"`
	Class         string  `json:"class"`
	Confidence    float64 `json:"confidence"`
	BoxX          float64 `json:"box_x"`
	BoxY          float64 `json:"box_y"`
	BoxW          float64 `json:"box_w"`
	BoxH          float64 `json:"box_h"`
	Embedding     []byte  `json:"-"`
	Attributes    string  `json:"attributes,omitempty"`
}

// InsertDetection inserts a new detection into the database.
func (d *DB) InsertDetection(det *Detection) error {
	res, err := d.Exec(`
		INSERT INTO detections (motion_event_id, frame_time, class, confidence,
			box_x, box_y, box_w, box_h, embedding, attributes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		det.MotionEventID, det.FrameTime, det.Class, det.Confidence,
		det.BoxX, det.BoxY, det.BoxW, det.BoxH, det.Embedding, det.Attributes,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	det.ID = id
	return nil
}

// ListDetectionsByEvent returns all detections for a given motion event ID.
func (d *DB) ListDetectionsByEvent(eventID int64) ([]*Detection, error) {
	rows, err := d.Query(`
		SELECT id, motion_event_id, frame_time, class, confidence,
			box_x, box_y, box_w, box_h, embedding, attributes
		FROM detections WHERE motion_event_id = ?
		ORDER BY frame_time`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detections []*Detection
	for rows.Next() {
		det := &Detection{}
		if err := rows.Scan(
			&det.ID, &det.MotionEventID, &det.FrameTime, &det.Class,
			&det.Confidence, &det.BoxX, &det.BoxY, &det.BoxW, &det.BoxH,
			&det.Embedding, &det.Attributes,
		); err != nil {
			return nil, err
		}
		detections = append(detections, det)
	}
	return detections, rows.Err()
}

// ListDetectionsWithEmbeddings returns detections that have non-null embeddings,
// optionally filtered by camera ID and time range. It joins with motion_events
// to get the camera_id. When cameraID is empty, all cameras are included.
func (d *DB) ListDetectionsWithEmbeddings(cameraID string, start, end time.Time) ([]*Detection, error) {
	query := `
		SELECT d.id, d.motion_event_id, d.frame_time, d.class, d.confidence,
			d.box_x, d.box_y, d.box_w, d.box_h, d.embedding, COALESCE(d.attributes, '')
		FROM detections d
		JOIN motion_events me ON d.motion_event_id = me.id
		WHERE d.embedding IS NOT NULL
		  AND d.frame_time >= ?
		  AND d.frame_time <= ?`

	args := []interface{}{
		start.UTC().Format(timeFormat),
		end.UTC().Format(timeFormat),
	}

	if cameraID != "" {
		query += ` AND me.camera_id = ?`
		args = append(args, cameraID)
	}

	query += ` ORDER BY d.frame_time DESC`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detections []*Detection
	for rows.Next() {
		det := &Detection{}
		if err := rows.Scan(
			&det.ID, &det.MotionEventID, &det.FrameTime, &det.Class,
			&det.Confidence, &det.BoxX, &det.BoxY, &det.BoxW, &det.BoxH,
			&det.Embedding, &det.Attributes,
		); err != nil {
			return nil, err
		}
		detections = append(detections, det)
	}
	return detections, rows.Err()
}

// DetectionWithEvent is a Detection enriched with motion event metadata for
// search results.
type DetectionWithEvent struct {
	Detection
	CameraID      string `json:"camera_id"`
	CameraName    string `json:"camera_name"`
	ThumbnailPath string `json:"thumbnail_path,omitempty"`
}

// ListDetectionsWithEvents returns detections with their associated motion event
// and camera information, filtered by camera ID and time range. Only detections
// with non-null embeddings are returned.
func (d *DB) ListDetectionsWithEvents(cameraID string, start, end time.Time) ([]*DetectionWithEvent, error) {
	query := `
		SELECT d.id, d.motion_event_id, d.frame_time, d.class, d.confidence,
			d.box_x, d.box_y, d.box_w, d.box_h, d.embedding, COALESCE(d.attributes, ''),
			me.camera_id, COALESCE(c.name, ''), COALESCE(me.thumbnail_path, '')
		FROM detections d
		JOIN motion_events me ON d.motion_event_id = me.id
		LEFT JOIN cameras c ON me.camera_id = c.id
		WHERE d.embedding IS NOT NULL
		  AND d.frame_time >= ?
		  AND d.frame_time <= ?`

	args := []interface{}{
		start.UTC().Format(timeFormat),
		end.UTC().Format(timeFormat),
	}

	if cameraID != "" {
		query += ` AND me.camera_id = ?`
		args = append(args, cameraID)
	}

	query += ` ORDER BY d.frame_time DESC LIMIT 500`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*DetectionWithEvent
	for rows.Next() {
		r := &DetectionWithEvent{}
		if err := rows.Scan(
			&r.ID, &r.MotionEventID, &r.FrameTime, &r.Class,
			&r.Confidence, &r.BoxX, &r.BoxY, &r.BoxW, &r.BoxH,
			&r.Embedding, &r.Attributes,
			&r.CameraID, &r.CameraName, &r.ThumbnailPath,
		); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// GetRecentDetections returns detections for the given camera that occurred
// after the given time. It joins with motion_events to filter by camera_id.
func (d *DB) GetRecentDetections(cameraID string, since time.Time) ([]*Detection, error) {
	rows, err := d.Query(`
		SELECT d.id, d.motion_event_id, d.frame_time, d.class, d.confidence,
			d.box_x, d.box_y, d.box_w, d.box_h, COALESCE(d.attributes, '')
		FROM detections d
		JOIN motion_events me ON d.motion_event_id = me.id
		WHERE me.camera_id = ? AND d.frame_time > ?
		ORDER BY d.frame_time DESC`,
		cameraID, since.UTC().Format(timeFormat),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detections []*Detection
	for rows.Next() {
		det := &Detection{}
		if err := rows.Scan(
			&det.ID, &det.MotionEventID, &det.FrameTime, &det.Class,
			&det.Confidence, &det.BoxX, &det.BoxY, &det.BoxW, &det.BoxH,
			&det.Attributes,
		); err != nil {
			return nil, err
		}
		detections = append(detections, det)
	}
	return detections, rows.Err()
}
