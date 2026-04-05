package scheduler

import (
	"fmt"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// DetectionScheduleStatus holds the evaluated detection state for a camera.
type DetectionScheduleStatus struct {
	CameraID       string `json:"camera_id"`
	DetectionActive bool   `json:"detection_active"`
	ActiveScheduleID string `json:"active_schedule_id,omitempty"`
	NextTransition  string `json:"next_transition,omitempty"` // ISO timestamp
}

// EvaluateDetectionSchedules determines whether detection should be active for
// a given camera at the specified time. It returns true and the matching
// schedule ID if any enabled schedule entry covers the current time.
//
// Day-of-week convention: 0=Sunday, 1=Monday ... 6=Saturday (matching Go's
// time.Weekday()).
//
// Cross-midnight schedules (start_time > end_time) are supported: if the
// current time is past start on day D, or before end on day D+1, the schedule
// matches.
func EvaluateDetectionSchedules(schedules []*db.DetectionSchedule, now time.Time) (active bool, matchingID string) {
	for _, s := range schedules {
		if !s.Enabled {
			continue
		}
		if detectionScheduleMatchesTime(s, now) {
			return true, s.ID
		}
	}
	return false, ""
}

// detectionScheduleMatchesTime checks whether a single schedule entry matches
// the given time.
func detectionScheduleMatchesTime(s *db.DetectionSchedule, now time.Time) bool {
	startMin, err := parseHHMM(s.StartTime)
	if err != nil {
		return false
	}
	endMin, err := parseHHMM(s.EndTime)
	if err != nil {
		return false
	}

	nowMin := now.Hour()*60 + now.Minute()
	dow := int(now.Weekday()) // 0=Sunday

	// Same start and end means 24-hour coverage.
	if startMin == endMin {
		return s.DayOfWeek == dow
	}

	if startMin < endMin {
		// Normal range (e.g. 08:00-18:00).
		return s.DayOfWeek == dow && nowMin >= startMin && nowMin < endMin
	}

	// Cross-midnight range (e.g. 22:00-06:00).
	// Evening portion: current day matches and time >= start.
	if s.DayOfWeek == dow && nowMin >= startMin {
		return true
	}
	// Morning portion: yesterday matches and time < end.
	yesterday := (dow + 6) % 7 // (dow - 1 + 7) % 7
	if s.DayOfWeek == yesterday && nowMin < endMin {
		return true
	}

	return false
}

// parseHHMM parses a "HH:MM" string into minutes since midnight.
func parseHHMM(s string) (int, error) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, err
	}
	return h*60 + m, nil
}

// DetectionScheduleTemplate represents a named preset that can be applied to
// create detection schedule entries for a camera.
type DetectionScheduleTemplate struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Entries     []DetectionScheduleEntry     `json:"entries"`
}

// DetectionScheduleEntry is a single day+time entry within a template.
type DetectionScheduleEntry struct {
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// DetectionScheduleTemplates returns the built-in detection schedule templates.
func DetectionScheduleTemplates() []DetectionScheduleTemplate {
	return []DetectionScheduleTemplate{
		{
			ID:          "always",
			Name:        "Always On",
			Description: "Detection active 24/7",
			Entries:     allDaysEntry("00:00", "00:00"),
		},
		{
			ID:          "never",
			Name:        "Never",
			Description: "Detection disabled",
			Entries:     nil,
		},
		{
			ID:          "business_hours",
			Name:        "Business Hours",
			Description: "Monday-Friday 08:00-18:00",
			Entries:     weekdayEntry("08:00", "18:00"),
		},
		{
			ID:          "after_hours",
			Name:        "After Hours",
			Description: "Monday-Friday 18:00-08:00 plus full weekends",
			Entries:     afterHoursEntries(),
		},
		{
			ID:          "weekends",
			Name:        "Weekends Only",
			Description: "Saturday and Sunday all day",
			Entries: []DetectionScheduleEntry{
				{DayOfWeek: 0, StartTime: "00:00", EndTime: "00:00"}, // Sunday
				{DayOfWeek: 6, StartTime: "00:00", EndTime: "00:00"}, // Saturday
			},
		},
	}
}

func allDaysEntry(start, end string) []DetectionScheduleEntry {
	entries := make([]DetectionScheduleEntry, 7)
	for i := 0; i < 7; i++ {
		entries[i] = DetectionScheduleEntry{DayOfWeek: i, StartTime: start, EndTime: end}
	}
	return entries
}

func weekdayEntry(start, end string) []DetectionScheduleEntry {
	entries := make([]DetectionScheduleEntry, 5)
	for i := 0; i < 5; i++ {
		entries[i] = DetectionScheduleEntry{DayOfWeek: i + 1, StartTime: start, EndTime: end} // Mon-Fri = 1-5
	}
	return entries
}

func afterHoursEntries() []DetectionScheduleEntry {
	entries := make([]DetectionScheduleEntry, 0, 7)
	// Weekday evenings: Mon-Fri 18:00-08:00 (cross-midnight)
	for d := 1; d <= 5; d++ {
		entries = append(entries, DetectionScheduleEntry{
			DayOfWeek: d,
			StartTime: "18:00",
			EndTime:   "08:00",
		})
	}
	// Full weekends
	entries = append(entries,
		DetectionScheduleEntry{DayOfWeek: 0, StartTime: "00:00", EndTime: "00:00"}, // Sunday
		DetectionScheduleEntry{DayOfWeek: 6, StartTime: "00:00", EndTime: "00:00"}, // Saturday
	)
	return entries
}
