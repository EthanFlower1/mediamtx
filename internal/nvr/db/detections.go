package db

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
