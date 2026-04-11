package dmp

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// TimelineIntegrator injects DMP alarm events into the NVR video timeline
// as motion events with event_type="alarm". This allows operators to see
// alarm events correlated with video footage in the timeline viewer.
type TimelineIntegrator struct {
	DB         *db.DB
	ZoneMapper *ZoneMapper

	// AlarmDuration is how long an alarm event spans on the timeline.
	// Defaults to 30 seconds.
	AlarmDuration time.Duration
}

// alarmMetadata is the JSON metadata stored with alarm timeline events.
type alarmMetadata struct {
	Source        string `json:"source"`
	AccountID     string `json:"account_id"`
	EventCode     string `json:"event_code"`
	EventQualifier string `json:"event_qualifier"`
	Zone          int    `json:"zone"`
	Area          int    `json:"area"`
	Severity      string `json:"severity"`
	Description   string `json:"description"`
	PanelRaw      string `json:"panel_raw,omitempty"`
}

// IngestAlarmEvent processes an alarm event and inserts it into the video
// timeline for the mapped camera. Returns the camera ID it was mapped to,
// or empty string if no mapping was found.
func (ti *TimelineIntegrator) IngestAlarmEvent(event *AlarmEvent) string {
	if ti.DB == nil || ti.ZoneMapper == nil {
		return ""
	}

	cameraID, found := ti.ZoneMapper.Lookup(event.AccountID, event.Zone, event.Area)
	if !found {
		log.Printf("[DMP] [DEBUG] no camera mapping for account=%s zone=%d area=%d",
			event.AccountID, event.Zone, event.Area)
		return ""
	}

	duration := ti.AlarmDuration
	if duration == 0 {
		duration = 30 * time.Second
	}

	now := event.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}
	endTime := now.Add(duration)

	startStr := now.Format(time.RFC3339)
	endStr := endTime.Format(time.RFC3339)

	meta := alarmMetadata{
		Source:         "dmp-xr",
		AccountID:      event.AccountID,
		EventCode:      event.EventCode,
		EventQualifier: event.EventQualifier,
		Zone:           event.Zone,
		Area:           event.Area,
		Severity:       event.Severity,
		Description:    event.Description,
	}

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		log.Printf("[DMP] [ERROR] failed to marshal alarm metadata: %v", err)
		return ""
	}
	metaStr := string(metaJSON)

	motionEvent := &db.MotionEvent{
		CameraID:    cameraID,
		StartedAt:   startStr,
		EndedAt:     &endStr,
		EventType:   "alarm",
		ObjectClass: fmt.Sprintf("dmp_%s", event.EventCode),
		Confidence:  1.0,
		Metadata:    &metaStr,
	}

	if err := ti.DB.InsertMotionEvent(motionEvent); err != nil {
		log.Printf("[DMP] [ERROR] failed to insert alarm timeline event: %v", err)
		return ""
	}

	log.Printf("[DMP] [INFO] alarm event inserted into timeline: camera=%s event=%s%s zone=%d",
		cameraID, event.EventQualifier, event.EventCode, event.Zone)

	return cameraID
}
