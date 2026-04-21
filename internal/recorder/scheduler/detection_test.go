package scheduler

import (
	"testing"
	"time"

	db "github.com/bluenviron/mediamtx/internal/recorder/db"
)

func makeDetSched(day int, start, end string, enabled bool) *db.DetectionSchedule {
	return &db.DetectionSchedule{
		ID:        "ds-1",
		CameraID:  "cam-1",
		DayOfWeek: day,
		StartTime: start,
		EndTime:   end,
		Enabled:   enabled,
	}
}

func TestEvaluateDetectionSchedules_NoSchedules(t *testing.T) {
	active, id := EvaluateDetectionSchedules(nil, time.Now())
	if active {
		t.Fatal("expected inactive with no schedules")
	}
	if id != "" {
		t.Fatalf("expected empty ID, got %s", id)
	}
}

func TestEvaluateDetectionSchedules_MatchingEntry(t *testing.T) {
	// Wednesday 10:00
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC) // Wednesday = 3
	scheds := []*db.DetectionSchedule{
		makeDetSched(3, "08:00", "18:00", true),
	}
	active, id := EvaluateDetectionSchedules(scheds, now)
	if !active {
		t.Fatal("expected active")
	}
	if id != "ds-1" {
		t.Fatalf("expected ds-1, got %s", id)
	}
}

func TestEvaluateDetectionSchedules_WrongDay(t *testing.T) {
	// Wednesday 10:00 but schedule is for Monday
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	scheds := []*db.DetectionSchedule{
		makeDetSched(1, "08:00", "18:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if active {
		t.Fatal("expected inactive on wrong day")
	}
}

func TestEvaluateDetectionSchedules_Disabled(t *testing.T) {
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC) // Wednesday
	scheds := []*db.DetectionSchedule{
		makeDetSched(3, "08:00", "18:00", false),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if active {
		t.Fatal("expected inactive when disabled")
	}
}

func TestEvaluateDetectionSchedules_BeforeStart(t *testing.T) {
	now := time.Date(2026, 3, 18, 7, 59, 0, 0, time.UTC) // Wednesday
	scheds := []*db.DetectionSchedule{
		makeDetSched(3, "08:00", "18:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if active {
		t.Fatal("expected inactive before start")
	}
}

func TestEvaluateDetectionSchedules_AtExactEnd(t *testing.T) {
	now := time.Date(2026, 3, 18, 18, 0, 0, 0, time.UTC) // Wednesday
	scheds := []*db.DetectionSchedule{
		makeDetSched(3, "08:00", "18:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if active {
		t.Fatal("expected inactive at exact end time (exclusive)")
	}
}

func TestEvaluateDetectionSchedules_CrossMidnight_Evening(t *testing.T) {
	// Friday 23:00, schedule 22:00-06:00 on Friday (5)
	now := time.Date(2026, 3, 20, 23, 0, 0, 0, time.UTC) // Friday = 5
	scheds := []*db.DetectionSchedule{
		makeDetSched(5, "22:00", "06:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if !active {
		t.Fatal("expected active in evening portion of cross-midnight")
	}
}

func TestEvaluateDetectionSchedules_CrossMidnight_Morning(t *testing.T) {
	// Saturday 03:00, schedule 22:00-06:00 on Friday (5)
	now := time.Date(2026, 3, 21, 3, 0, 0, 0, time.UTC) // Saturday = 6
	scheds := []*db.DetectionSchedule{
		makeDetSched(5, "22:00", "06:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if !active {
		t.Fatal("expected active in morning portion of cross-midnight")
	}
}

func TestEvaluateDetectionSchedules_CrossMidnight_WrongTime(t *testing.T) {
	// Friday 10:00, schedule 22:00-06:00 on Friday (5)
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	scheds := []*db.DetectionSchedule{
		makeDetSched(5, "22:00", "06:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if active {
		t.Fatal("expected inactive outside cross-midnight window")
	}
}

func TestEvaluateDetectionSchedules_24Hour(t *testing.T) {
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC) // Wednesday = 3
	scheds := []*db.DetectionSchedule{
		makeDetSched(3, "00:00", "00:00", true),
	}
	active, _ := EvaluateDetectionSchedules(scheds, now)
	if !active {
		t.Fatal("expected active for 24-hour coverage")
	}
}

func TestDetectionScheduleTemplates_AllPresent(t *testing.T) {
	templates := DetectionScheduleTemplates()
	if len(templates) != 5 {
		t.Fatalf("expected 5 templates, got %d", len(templates))
	}

	ids := map[string]bool{}
	for _, tmpl := range templates {
		ids[tmpl.ID] = true
	}

	for _, expected := range []string{"always", "never", "business_hours", "after_hours", "weekends"} {
		if !ids[expected] {
			t.Errorf("missing template: %s", expected)
		}
	}
}

func TestDetectionScheduleTemplates_BusinessHours(t *testing.T) {
	templates := DetectionScheduleTemplates()
	var bh DetectionScheduleTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "business_hours" {
			bh = tmpl
			break
		}
	}

	if len(bh.Entries) != 5 {
		t.Fatalf("expected 5 entries for business_hours, got %d", len(bh.Entries))
	}

	// Monday 10:00 should match
	mon := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC) // Monday
	scheds := entriesToSchedules(bh.Entries)
	active, _ := EvaluateDetectionSchedules(scheds, mon)
	if !active {
		t.Fatal("expected business_hours active on Monday 10:00")
	}

	// Saturday 10:00 should not match
	sat := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	active, _ = EvaluateDetectionSchedules(scheds, sat)
	if active {
		t.Fatal("expected business_hours inactive on Saturday")
	}
}

func TestDetectionScheduleTemplates_AfterHours(t *testing.T) {
	templates := DetectionScheduleTemplates()
	var ah DetectionScheduleTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "after_hours" {
			ah = tmpl
			break
		}
	}

	scheds := entriesToSchedules(ah.Entries)

	// Monday 20:00 should be active (evening after hours)
	active, _ := EvaluateDetectionSchedules(scheds, time.Date(2026, 3, 16, 20, 0, 0, 0, time.UTC))
	if !active {
		t.Fatal("expected after_hours active on Monday 20:00")
	}

	// Monday 10:00 should be inactive (business hours)
	active, _ = EvaluateDetectionSchedules(scheds, time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC))
	if active {
		t.Fatal("expected after_hours inactive on Monday 10:00")
	}

	// Saturday 14:00 should be active (weekend)
	active, _ = EvaluateDetectionSchedules(scheds, time.Date(2026, 3, 21, 14, 0, 0, 0, time.UTC))
	if !active {
		t.Fatal("expected after_hours active on Saturday")
	}
}

func TestDetectionScheduleTemplates_Never(t *testing.T) {
	templates := DetectionScheduleTemplates()
	var never DetectionScheduleTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "never" {
			never = tmpl
			break
		}
	}

	if len(never.Entries) != 0 {
		t.Fatalf("expected 0 entries for never template, got %d", len(never.Entries))
	}

	active, _ := EvaluateDetectionSchedules(nil, time.Now())
	if active {
		t.Fatal("expected never template to be inactive")
	}
}

// entriesToSchedules converts template entries to DetectionSchedule slice for
// testing.
func entriesToSchedules(entries []DetectionScheduleEntry) []*db.DetectionSchedule {
	scheds := make([]*db.DetectionSchedule, len(entries))
	for i, e := range entries {
		scheds[i] = &db.DetectionSchedule{
			ID:        "tmpl-entry",
			CameraID:  "cam-test",
			DayOfWeek: e.DayOfWeek,
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			Enabled:   true,
		}
	}
	return scheds
}
