package forensic

import (
	"testing"
	"time"
)

// TestEvaluateLeaf tests the evaluate function on leaf clauses using a mock
// approach — we build results directly and evaluate queries against them.
func TestEvaluateLeaf(t *testing.T) {
	e := &Executor{}

	// A result representing a car detection on Tuesday at 19:30.
	ts := time.Date(2026, 4, 7, 19, 30, 0, 0, time.UTC) // Tuesday
	result := &Result{
		ID:             "det-1",
		CameraID:       "loading-dock",
		CameraName:     "Loading Dock",
		Timestamp:      ts,
		MatchedClasses: []string{"car"},
		Confidence:     0.92,
		CLIPSimilarity: 0.75,
	}

	tests := []struct {
		name      string
		query     *Query
		wantMatch bool
		wantScore float64
	}{
		{
			name:      "object match",
			query:     Object("car"),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "object no match",
			query:     Object("truck"),
			wantMatch: false,
		},
		{
			name:      "object multi-class match",
			query:     Object("truck", "car"),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "camera match",
			query:     Camera("loading-dock"),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "camera no match",
			query:     Camera("parking-lot"),
			wantMatch: false,
		},
		{
			name:      "time range match",
			query:     TimeRange(ts.Add(-time.Hour), ts.Add(time.Hour)),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "time range too early",
			query:     TimeRange(ts.Add(time.Hour), ts.Add(2*time.Hour)),
			wantMatch: false,
		},
		{
			name:      "time of day match evening",
			query:     TimeOfDay("18:00", "22:00"),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "time of day no match morning",
			query:     TimeOfDay("08:00", "12:00"),
			wantMatch: false,
		},
		{
			name:      "time of day overnight match",
			query:     TimeOfDay("18:00", "08:00"),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "day of week Tuesday (ISO 2)",
			query:     DayOfWeek(2, 3, 4),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "day of week Monday only",
			query:     DayOfWeek(1),
			wantMatch: false,
		},
		{
			name:      "confidence above threshold",
			query:     Confidence(0.80),
			wantMatch: true,
			wantScore: 1.0,
		},
		{
			name:      "confidence below threshold",
			query:     Confidence(0.95),
			wantMatch: false,
		},
		{
			name:      "clip with similarity score",
			query:     CLIP("red car"),
			wantMatch: true,
			wantScore: 0.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, ok := e.evaluate(tt.query, result)
			if ok != tt.wantMatch {
				t.Errorf("evaluate() matched = %v, want %v", ok, tt.wantMatch)
			}
			if tt.wantMatch && score != tt.wantScore {
				t.Errorf("evaluate() score = %v, want %v", score, tt.wantScore)
			}
		})
	}
}

// TestEvaluateComposite tests AND/OR/NOT composition.
func TestEvaluateComposite(t *testing.T) {
	e := &Executor{}

	ts := time.Date(2026, 4, 7, 19, 30, 0, 0, time.UTC) // Tuesday
	result := &Result{
		ID:             "det-2",
		CameraID:       "loading-dock",
		Timestamp:      ts,
		MatchedClasses: []string{"car"},
		Confidence:     0.92,
		CLIPSimilarity: 0.75,
	}

	tests := []struct {
		name      string
		query     *Query
		wantMatch bool
	}{
		{
			name: "AND all match",
			query: And(
				Object("car"),
				Camera("loading-dock"),
				DayOfWeek(2),
			),
			wantMatch: true,
		},
		{
			name: "AND one fails",
			query: And(
				Object("car"),
				Camera("parking-lot"),
			),
			wantMatch: false,
		},
		{
			name: "OR one matches",
			query: Or(
				Object("truck"),
				Object("car"),
			),
			wantMatch: true,
		},
		{
			name: "OR none match",
			query: Or(
				Object("truck"),
				Object("bicycle"),
			),
			wantMatch: false,
		},
		{
			name:      "NOT excludes match",
			query:     Not(Object("car")),
			wantMatch: false,
		},
		{
			name:      "NOT includes non-match",
			query:     Not(Object("truck")),
			wantMatch: true,
		},
		{
			name: "complex: red truck at loading dock 6pm-8am Tue-Thu",
			query: And(
				CLIP("red truck"),
				TimeOfDay("18:00", "08:00"),
				DayOfWeek(2, 3, 4),
				Camera("loading-dock"),
			),
			wantMatch: true, // CLIP matches via similarity, time/day/camera all match
		},
		{
			name: "complex: NOT bicycle AND car",
			query: And(
				Object("car"),
				Not(Object("bicycle")),
				Confidence(0.80),
			),
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := e.evaluate(tt.query, result)
			if ok != tt.wantMatch {
				t.Errorf("evaluate() matched = %v, want %v", ok, tt.wantMatch)
			}
		})
	}
}

// TestDedup verifies result deduplication.
func TestDedup(t *testing.T) {
	results := []Result{
		{ID: "a", Score: 0.9},
		{ID: "b", Score: 0.8},
		{ID: "a", Score: 0.7}, // duplicate, lower score
		{ID: "c", Score: 0.6},
		{ID: "b", Score: 0.95}, // duplicate, higher score
	}

	deduped := dedup(results)
	if len(deduped) != 3 {
		t.Fatalf("expected 3 results after dedup, got %d", len(deduped))
	}

	// Check that highest scores were kept.
	scoreMap := make(map[string]float64)
	for _, r := range deduped {
		scoreMap[r.ID] = r.Score
	}
	if scoreMap["a"] != 0.9 {
		t.Errorf("expected a=0.9, got %v", scoreMap["a"])
	}
	if scoreMap["b"] != 0.95 {
		t.Errorf("expected b=0.95, got %v", scoreMap["b"])
	}
	if scoreMap["c"] != 0.6 {
		t.Errorf("expected c=0.6, got %v", scoreMap["c"])
	}
}

// TestBuilder tests the fluent query builder.
func TestBuilder(t *testing.T) {
	now := time.Now()
	q := NewBuilder().
		CLIP("red truck").
		Object("car", "truck").
		Camera("cam1").
		TimeRange(now.Add(-24*time.Hour), now).
		TimeOfDay("18:00", "08:00").
		DayOfWeek(2, 3, 4).
		Confidence(0.80).
		Build()

	if err := q.Validate(); err != nil {
		t.Fatalf("builder query invalid: %v", err)
	}

	if q.Op != OpAND {
		t.Errorf("expected AND root, got %v", q.Op)
	}
	if len(q.Children) != 7 {
		t.Errorf("expected 7 children, got %d", len(q.Children))
	}
}

// TestBuilderSingle tests builder with a single clause.
func TestBuilderSingle(t *testing.T) {
	q := NewBuilder().CLIP("red truck").Build()
	if err := q.Validate(); err != nil {
		t.Fatalf("single clause invalid: %v", err)
	}
	if q.Type != ClauseCLIP {
		t.Errorf("expected CLIP leaf, got %v", q.Type)
	}
}

// TestAddSnippet verifies snippet generation around a match.
func TestAddSnippet(t *testing.T) {
	e := &Executor{SnippetPadding: 10 * time.Second}
	ts := time.Date(2026, 4, 7, 19, 30, 0, 0, time.UTC)
	r := &Result{Timestamp: ts}
	e.addSnippet(r)

	if r.SnippetStart == nil || r.SnippetEnd == nil {
		t.Fatal("snippet times should not be nil")
	}
	if r.SnippetStart.After(ts) {
		t.Error("snippet start should be before timestamp")
	}
	if r.SnippetEnd.Before(ts) {
		t.Error("snippet end should be after timestamp")
	}
	if r.SnippetEnd.Sub(*r.SnippetStart) != 20*time.Second {
		t.Errorf("expected 20s window, got %v", r.SnippetEnd.Sub(*r.SnippetStart))
	}
}

// TestLPRWildcard tests LPR plate matching with wildcards.
func TestLPRWildcard(t *testing.T) {
	e := &Executor{}

	result := &Result{
		MatchedPlate: "ABC123",
	}

	tests := []struct {
		name      string
		plate     string
		wantMatch bool
	}{
		{"exact match", "ABC123", true},
		{"wildcard prefix", "ABC*", true},
		{"wildcard no match", "XYZ*", false},
		{"substring match", "BC12", true},
		{"no match", "ZZZ999", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := LPR(tt.plate)
			_, ok := e.evaluate(q, result)
			if ok != tt.wantMatch {
				t.Errorf("LPR(%q) matched = %v, want %v", tt.plate, ok, tt.wantMatch)
			}
		})
	}
}

// TestSampleComplexQueries validates 5+ sample complex queries that demonstrate
// the forensic search DSL. These are the acceptance criteria queries.
func TestSampleComplexQueries(t *testing.T) {
	queries := []*Query{
		// 1. "Find all videos where a red truck appeared between Tuesday and
		//     Thursday at the loading dock between 6pm and 8am"
		And(
			CLIP("red truck"),
			DayOfWeek(2, 3, 4),
			Camera("loading-dock"),
			TimeOfDay("18:00", "08:00"),
		),

		// 2. "Find any person or vehicle with plate ABC* in the parking lot"
		And(
			Or(
				Object("person"),
				LPR("ABC*"),
			),
			Camera("parking-lot"),
		),

		// 3. "Find cars but not trucks last Tuesday with high confidence"
		And(
			Object("car"),
			Not(Object("truck")),
			DayOfWeek(2),
			Confidence(0.85),
		),

		// 4. "Find delivery vans at entrances during business hours"
		And(
			CLIP("delivery van package"),
			TimeOfDay("08:00", "18:00"),
			Camera("entrance-1", "entrance-2"),
		),

		// 5. "Find vehicles with specific plate OR matching white sedan"
		Or(
			LPR("XYZ789"),
			And(
				CLIP("white sedan"),
				Object("car"),
			),
		),

		// 6. "Find all people on weekends with at least 90% confidence at night"
		And(
			Object("person"),
			DayOfWeek(6, 7),
			Confidence(0.90),
			TimeOfDay("22:00", "06:00"),
		),
	}

	for i, q := range queries {
		t.Run("sample_query_"+string(rune('1'+i)), func(t *testing.T) {
			if err := q.Validate(); err != nil {
				t.Fatalf("sample query %d invalid: %v", i+1, err)
			}
			s := q.String()
			if s == "" {
				t.Errorf("sample query %d has empty string representation", i+1)
			}
			t.Logf("Query %d: %s", i+1, s)
		})
	}
}
