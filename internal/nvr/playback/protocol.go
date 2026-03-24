package playback

// Command is a message from client to server.
type Command struct {
	Cmd string `json:"cmd"`
	Seq int    `json:"seq"`
	// create
	CameraIDs []string `json:"camera_ids,omitempty"`
	Start     *string  `json:"start,omitempty"`
	// resume
	SessionID *string `json:"session_id,omitempty"`
	// seek
	Position *float64 `json:"position,omitempty"`
	// speed
	Rate *float64 `json:"rate,omitempty"`
	// step
	Direction *int `json:"direction,omitempty"`
	// add_camera / remove_camera
	CameraID *string `json:"camera_id,omitempty"`
}

// Event is a message from server to client.
type Event struct {
	EventType string `json:"event"`
	AckSeq    *int   `json:"ack_seq,omitempty"`
	// created
	SessionID *string           `json:"session_id,omitempty"`
	Streams   map[string]string `json:"streams,omitempty"`
	// position / state
	Position *float64 `json:"position,omitempty"`
	Playing  *bool    `json:"playing,omitempty"`
	Speed    *float64 `json:"speed,omitempty"`
	Time     *string  `json:"time,omitempty"`
	// buffering
	CameraID  *string `json:"camera_id,omitempty"`
	Buffering *bool   `json:"buffering,omitempty"`
	// segment_gap
	GapStart  *float64 `json:"gap_start,omitempty"`
	NextStart *float64 `json:"next_start,omitempty"`
	// stream_restart / stream_added
	NewURL *string `json:"new_url,omitempty"`
	URL    *string `json:"url,omitempty"`
	// error
	Message *string `json:"message,omitempty"`
}

// SessionState represents the playback state machine.
type SessionState int

const (
	StatePaused   SessionState = iota
	StatePlaying
	StateSeeking
	StateStepping
	StateDisposed
)
