package db

// Detection represents an object detection within a motion event.
// This type is used by the webhook dispatcher and related components.
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
