package forensic

import (
	"testing"
	"time"
)

func TestValidate_LeafClauses(t *testing.T) {
	tests := []struct {
		name    string
		query   *Query
		wantErr bool
	}{
		{
			name:    "valid clip",
			query:   &Query{Type: ClauseCLIP, CLIPText: "red truck"},
			wantErr: false,
		},
		{
			name:    "empty clip text",
			query:   &Query{Type: ClauseCLIP, CLIPText: ""},
			wantErr: true,
		},
		{
			name:    "valid object",
			query:   &Query{Type: ClauseObject, ObjectClass: "car,truck"},
			wantErr: false,
		},
		{
			name:    "empty object class",
			query:   &Query{Type: ClauseObject, ObjectClass: ""},
			wantErr: true,
		},
		{
			name:    "valid lpr",
			query:   &Query{Type: ClauseLPR, PlateText: "ABC123"},
			wantErr: false,
		},
		{
			name:    "valid time range",
			query:   timeRangeQuery(time.Now().Add(-time.Hour), time.Now()),
			wantErr: false,
		},
		{
			name:    "time range no bounds",
			query:   &Query{Type: ClauseTime},
			wantErr: true,
		},
		{
			name:    "valid camera",
			query:   &Query{Type: ClauseCamera, CameraIDs: []string{"cam1"}},
			wantErr: false,
		},
		{
			name:    "empty camera list",
			query:   &Query{Type: ClauseCamera, CameraIDs: nil},
			wantErr: true,
		},
		{
			name:    "valid time_of_day",
			query:   &Query{Type: ClauseTimeOfDay, TimeOfDayStart: "18:00", TimeOfDayEnd: "08:00"},
			wantErr: false,
		},
		{
			name:    "valid day_of_week",
			query:   &Query{Type: ClauseDayOfWeek, DaysOfWeek: []int{1, 2, 3}},
			wantErr: false,
		},
		{
			name:    "invalid day_of_week",
			query:   &Query{Type: ClauseDayOfWeek, DaysOfWeek: []int{0, 8}},
			wantErr: true,
		},
		{
			name:    "valid confidence",
			query:   &Query{Type: ClauseConfidence, MinConfidence: 0.85},
			wantErr: false,
		},
		{
			name:    "invalid confidence",
			query:   &Query{Type: ClauseConfidence, MinConfidence: 1.5},
			wantErr: true,
		},
		{
			name:    "unknown type",
			query:   &Query{Type: "bogus"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate_Composite(t *testing.T) {
	tests := []struct {
		name    string
		query   *Query
		wantErr bool
	}{
		{
			name: "valid AND",
			query: And(
				CLIP("red truck"),
				Object("car"),
			),
			wantErr: false,
		},
		{
			name: "AND with one child",
			query: &Query{
				Op:       OpAND,
				Children: []*Query{CLIP("red truck")},
			},
			wantErr: true,
		},
		{
			name: "valid OR",
			query: Or(
				Object("car"),
				Object("truck"),
			),
			wantErr: false,
		},
		{
			name:    "valid NOT",
			query:   Not(Object("truck")),
			wantErr: false,
		},
		{
			name: "NOT with two children",
			query: &Query{
				Op:       OpNOT,
				Children: []*Query{Object("car"), Object("truck")},
			},
			wantErr: true,
		},
		{
			name: "nested valid",
			query: And(
				CLIP("red truck"),
				Or(
					Object("car"),
					LPR("ABC*"),
				),
				TimeOfDay("18:00", "08:00"),
			),
			wantErr: false,
		},
		{
			name: "nested invalid child",
			query: And(
				CLIP("red truck"),
				&Query{Type: ClauseLPR, PlateText: ""},
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.query.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQuery_String(t *testing.T) {
	q := And(
		CLIP("red truck"),
		Not(Object("bicycle")),
		Camera("cam1", "cam2"),
	)
	s := q.String()
	if s == "" {
		t.Error("String() returned empty")
	}
	// Should contain key elements.
	for _, want := range []string{"clip", "red truck", "NOT", "bicycle", "cam1", "cam2"} {
		if !containsStr(s, want) {
			t.Errorf("String() = %q, missing %q", s, want)
		}
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		(haystack == needle || len(haystack) > len(needle) &&
			findSubstring(haystack, needle))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func timeRangeQuery(start, end time.Time) *Query {
	return &Query{Type: ClauseTime, Start: &start, End: &end}
}
