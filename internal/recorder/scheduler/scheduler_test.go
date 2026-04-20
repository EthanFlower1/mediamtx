package scheduler

import (
	"testing"
	"time"

	db "github.com/bluenviron/mediamtx/internal/shared/legacydb"
)

// helper to build a RecordingRule with sensible defaults.
func makeRule(mode, days, start, end string, enabled bool) *db.RecordingRule {
	return &db.RecordingRule{
		ID:        "rule-1",
		CameraID:  "cam-1",
		Name:      "test rule",
		Mode:      mode,
		Days:      days,
		StartTime: start,
		EndTime:   end,
		Enabled:   enabled,
	}
}

func TestEvaluateRules_NoRules(t *testing.T) {
	mode, ids := EvaluateRules(nil, time.Now())
	if mode != ModeOff {
		t.Fatalf("expected ModeOff, got %s", mode)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no active rule IDs, got %v", ids)
	}
}

func TestEvaluateRules_MatchingAlwaysRule(t *testing.T) {
	// Wednesday 10:00
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC) // Wednesday = 3
	rules := []*db.RecordingRule{
		makeRule("always", "[1,2,3,4,5]", "08:00", "18:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeAlways {
		t.Fatalf("expected ModeAlways, got %s", mode)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 active rule, got %d", len(ids))
	}
}

func TestEvaluateRules_MatchingEventsRule(t *testing.T) {
	// Wednesday 10:00
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	rules := []*db.RecordingRule{
		makeRule("events", "[1,2,3,4,5]", "08:00", "18:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeEvents {
		t.Fatalf("expected ModeEvents, got %s", mode)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 active rule, got %d", len(ids))
	}
}

func TestEvaluateRules_AlwaysWinsOverEvents(t *testing.T) {
	// Wednesday 10:00
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	rules := []*db.RecordingRule{
		{
			ID: "rule-events", CameraID: "cam-1", Name: "events rule",
			Mode: "events", Days: "[1,2,3,4,5]",
			StartTime: "08:00", EndTime: "18:00", Enabled: true,
		},
		{
			ID: "rule-always", CameraID: "cam-1", Name: "always rule",
			Mode: "always", Days: "[1,2,3,4,5]",
			StartTime: "09:00", EndTime: "17:00", Enabled: true,
		},
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeAlways {
		t.Fatalf("expected ModeAlways, got %s", mode)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 active rules, got %d", len(ids))
	}
}

func TestEvaluateRules_WrongDay(t *testing.T) {
	// Wednesday 10:00, but rule only applies on Monday (1) and Tuesday (2)
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC) // Wednesday = 3
	rules := []*db.RecordingRule{
		makeRule("always", "[1,2]", "08:00", "18:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeOff {
		t.Fatalf("expected ModeOff, got %s", mode)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no active rules, got %v", ids)
	}
}

func TestEvaluateRules_DisabledRule(t *testing.T) {
	// Wednesday 10:00, matching day/time but disabled
	now := time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
	rules := []*db.RecordingRule{
		makeRule("always", "[1,2,3,4,5]", "08:00", "18:00", false),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeOff {
		t.Fatalf("expected ModeOff, got %s", mode)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no active rules, got %v", ids)
	}
}

func TestEvaluateRules_CrossMidnight_EveningPortion(t *testing.T) {
	// Friday 23:00, rule is 22:00-06:00 on days [5] (Friday start)
	now := time.Date(2026, 3, 20, 23, 0, 0, 0, time.UTC) // Friday = 5
	rules := []*db.RecordingRule{
		makeRule("always", "[5]", "22:00", "06:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeAlways {
		t.Fatalf("expected ModeAlways, got %s", mode)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 active rule, got %d", len(ids))
	}
}

func TestEvaluateRules_CrossMidnight_MorningPortion(t *testing.T) {
	// Saturday 03:00, rule is 22:00-06:00 on days [5] (Friday start)
	// Yesterday was Friday (5), and current time 03:00 < end 06:00 -> match
	now := time.Date(2026, 3, 21, 3, 0, 0, 0, time.UTC) // Saturday = 6
	rules := []*db.RecordingRule{
		makeRule("always", "[5]", "22:00", "06:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeAlways {
		t.Fatalf("expected ModeAlways, got %s", mode)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 active rule, got %d", len(ids))
	}
}

func TestEvaluateRules_CrossMidnight_WrongTime(t *testing.T) {
	// Friday 10:00, rule is 22:00-06:00 on days [5]
	// Current time 10:00 is neither >= 22:00 nor < 06:00
	now := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC) // Friday = 5
	rules := []*db.RecordingRule{
		makeRule("always", "[5]", "22:00", "06:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeOff {
		t.Fatalf("expected ModeOff, got %s", mode)
	}
	if len(ids) != 0 {
		t.Fatalf("expected no active rules, got %v", ids)
	}
}

func TestEvaluateRules_StartEqualsEnd_24Hours(t *testing.T) {
	// Any time on the matching day should match (24-hour coverage)
	now := time.Date(2026, 3, 18, 15, 30, 0, 0, time.UTC) // Wednesday = 3
	rules := []*db.RecordingRule{
		makeRule("always", "[3]", "00:00", "00:00", true),
	}
	mode, ids := EvaluateRules(rules, now)
	if mode != ModeAlways {
		t.Fatalf("expected ModeAlways, got %s", mode)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 active rule, got %d", len(ids))
	}
}

// --- RuleMatchesTime unit tests ---

func TestRuleMatchesTime_NormalRange(t *testing.T) {
	rule := makeRule("always", "[3]", "09:00", "17:00", true)
	// Wednesday 12:00
	now := time.Date(2026, 3, 18, 12, 0, 0, 0, time.UTC)
	if !RuleMatchesTime(rule, now) {
		t.Fatal("expected rule to match")
	}
}

func TestRuleMatchesTime_BeforeStart(t *testing.T) {
	rule := makeRule("always", "[3]", "09:00", "17:00", true)
	// Wednesday 08:59
	now := time.Date(2026, 3, 18, 8, 59, 0, 0, time.UTC)
	if RuleMatchesTime(rule, now) {
		t.Fatal("expected rule NOT to match")
	}
}

func TestRuleMatchesTime_AtExactEnd(t *testing.T) {
	rule := makeRule("always", "[3]", "09:00", "17:00", true)
	// Wednesday 17:00 exactly - end is exclusive
	now := time.Date(2026, 3, 18, 17, 0, 0, 0, time.UTC)
	if RuleMatchesTime(rule, now) {
		t.Fatal("expected rule NOT to match at exact end time")
	}
}

func TestRuleMatchesTime_CrossMidnight_SundayToMonday(t *testing.T) {
	// Rule on Sunday (0) from 23:00-02:00
	// Monday 01:00 -> yesterday was Sunday (0) -> should match morning portion
	rule := makeRule("always", "[0]", "23:00", "02:00", true)
	now := time.Date(2026, 3, 23, 1, 0, 0, 0, time.UTC) // Monday = 1
	if !RuleMatchesTime(rule, now) {
		t.Fatal("expected cross-midnight rule to match on Monday morning")
	}
}

func TestRuleMatchesTime_AllWeekdays(t *testing.T) {
	rule := makeRule("always", "[1,2,3,4,5]", "00:00", "00:00", true)
	// Wednesday
	now := time.Date(2026, 3, 18, 14, 0, 0, 0, time.UTC)
	if !RuleMatchesTime(rule, now) {
		t.Fatal("expected weekday rule to match on Wednesday")
	}
	// Saturday = 6
	sat := time.Date(2026, 3, 21, 14, 0, 0, 0, time.UTC)
	if RuleMatchesTime(rule, sat) {
		t.Fatal("expected weekday rule NOT to match on Saturday")
	}
}
