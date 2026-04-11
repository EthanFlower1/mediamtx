// Package forensic implements multi-faceted forensic search combining CLIP
// semantic search, object detection events, license plate reads, time windows,
// and camera filters into a composable query DSL.
//
// KAI-290: Feature 10 — Forensic multi-faceted search.
package forensic

import (
	"fmt"
	"strings"
	"time"
)

// Operator defines how child clauses are combined.
type Operator string

const (
	OpAND Operator = "AND"
	OpOR  Operator = "OR"
	OpNOT Operator = "NOT"
)

// ClauseType identifies what kind of search a leaf clause performs.
type ClauseType string

const (
	// ClauseCLIP performs a CLIP text-similarity search.
	ClauseCLIP ClauseType = "clip"

	// ClauseObject matches by object detection class name.
	ClauseObject ClauseType = "object"

	// ClauseLPR matches by license plate text (substring or exact).
	ClauseLPR ClauseType = "lpr"

	// ClauseTime restricts results to a time window.
	ClauseTime ClauseType = "time"

	// ClauseCamera restricts results to specific cameras.
	ClauseCamera ClauseType = "camera"

	// ClauseTimeOfDay restricts results to a recurring time-of-day window
	// (e.g., 18:00-08:00 every day).
	ClauseTimeOfDay ClauseType = "time_of_day"

	// ClauseDayOfWeek restricts results to specific days of the week.
	ClauseDayOfWeek ClauseType = "day_of_week"

	// ClauseConfidence sets a minimum confidence threshold.
	ClauseConfidence ClauseType = "confidence"
)

// Query represents a forensic search query tree. It can be either a leaf
// clause (when Type is set) or a composite node (when Op and Children are set).
type Query struct {
	// Op is the logical operator combining Children. Empty for leaf clauses.
	Op Operator `json:"op,omitempty"`

	// Children are sub-queries combined by Op.
	Children []*Query `json:"children,omitempty"`

	// --- Leaf clause fields (used when Op is empty) ---

	// Type identifies the leaf clause kind.
	Type ClauseType `json:"type,omitempty"`

	// CLIPText is the natural language search text (for ClauseCLIP).
	CLIPText string `json:"clip_text,omitempty"`

	// ObjectClass is the detection class to match (for ClauseObject).
	// Supports comma-separated list, e.g. "car,truck".
	ObjectClass string `json:"object_class,omitempty"`

	// PlateText is the license plate text to search (for ClauseLPR).
	// Supports wildcard matching with * (e.g. "ABC*").
	PlateText string `json:"plate_text,omitempty"`

	// Start and End define the absolute time range (for ClauseTime).
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`

	// CameraIDs lists specific cameras to include (for ClauseCamera).
	CameraIDs []string `json:"camera_ids,omitempty"`

	// TimeOfDayStart and TimeOfDayEnd define a recurring daily window in
	// "HH:MM" format (for ClauseTimeOfDay). Wraps past midnight when
	// Start > End (e.g., "22:00"-"06:00").
	TimeOfDayStart string `json:"time_of_day_start,omitempty"`
	TimeOfDayEnd   string `json:"time_of_day_end,omitempty"`

	// DaysOfWeek lists ISO day numbers 1=Mon..7=Sun (for ClauseDayOfWeek).
	DaysOfWeek []int `json:"days_of_week,omitempty"`

	// MinConfidence is the minimum detection confidence (for ClauseConfidence).
	MinConfidence float64 `json:"min_confidence,omitempty"`
}

// IsLeaf returns true if this is a leaf clause (not a composite).
func (q *Query) IsLeaf() bool {
	return q.Op == ""
}

// Validate checks the query tree for structural correctness.
func (q *Query) Validate() error {
	if q == nil {
		return fmt.Errorf("query is nil")
	}
	if q.IsLeaf() {
		return q.validateLeaf()
	}
	return q.validateComposite()
}

func (q *Query) validateLeaf() error {
	switch q.Type {
	case ClauseCLIP:
		if strings.TrimSpace(q.CLIPText) == "" {
			return fmt.Errorf("clip clause requires non-empty clip_text")
		}
	case ClauseObject:
		if strings.TrimSpace(q.ObjectClass) == "" {
			return fmt.Errorf("object clause requires non-empty object_class")
		}
	case ClauseLPR:
		if strings.TrimSpace(q.PlateText) == "" {
			return fmt.Errorf("lpr clause requires non-empty plate_text")
		}
	case ClauseTime:
		if q.Start == nil && q.End == nil {
			return fmt.Errorf("time clause requires at least one of start or end")
		}
	case ClauseCamera:
		if len(q.CameraIDs) == 0 {
			return fmt.Errorf("camera clause requires at least one camera_id")
		}
	case ClauseTimeOfDay:
		if q.TimeOfDayStart == "" || q.TimeOfDayEnd == "" {
			return fmt.Errorf("time_of_day clause requires both start and end times")
		}
	case ClauseDayOfWeek:
		if len(q.DaysOfWeek) == 0 {
			return fmt.Errorf("day_of_week clause requires at least one day")
		}
		for _, d := range q.DaysOfWeek {
			if d < 1 || d > 7 {
				return fmt.Errorf("day_of_week values must be 1-7, got %d", d)
			}
		}
	case ClauseConfidence:
		if q.MinConfidence <= 0 || q.MinConfidence > 1 {
			return fmt.Errorf("confidence must be between 0 and 1")
		}
	default:
		return fmt.Errorf("unknown clause type: %q", q.Type)
	}
	return nil
}

func (q *Query) validateComposite() error {
	switch q.Op {
	case OpAND, OpOR:
		if len(q.Children) < 2 {
			return fmt.Errorf("%s requires at least 2 children, got %d", q.Op, len(q.Children))
		}
	case OpNOT:
		if len(q.Children) != 1 {
			return fmt.Errorf("NOT requires exactly 1 child, got %d", len(q.Children))
		}
	default:
		return fmt.Errorf("unknown operator: %q", q.Op)
	}
	for i, child := range q.Children {
		if err := child.Validate(); err != nil {
			return fmt.Errorf("child[%d]: %w", i, err)
		}
	}
	return nil
}

// String returns a human-readable representation of the query tree.
func (q *Query) String() string {
	if q.IsLeaf() {
		return q.leafString()
	}
	parts := make([]string, len(q.Children))
	for i, child := range q.Children {
		parts[i] = child.String()
	}
	if q.Op == OpNOT {
		return fmt.Sprintf("NOT(%s)", parts[0])
	}
	return fmt.Sprintf("(%s)", strings.Join(parts, " "+string(q.Op)+" "))
}

func (q *Query) leafString() string {
	switch q.Type {
	case ClauseCLIP:
		return fmt.Sprintf("clip(%q)", q.CLIPText)
	case ClauseObject:
		return fmt.Sprintf("object(%s)", q.ObjectClass)
	case ClauseLPR:
		return fmt.Sprintf("lpr(%s)", q.PlateText)
	case ClauseTime:
		s, e := "...", "..."
		if q.Start != nil {
			s = q.Start.Format(time.RFC3339)
		}
		if q.End != nil {
			e = q.End.Format(time.RFC3339)
		}
		return fmt.Sprintf("time(%s..%s)", s, e)
	case ClauseCamera:
		return fmt.Sprintf("camera(%s)", strings.Join(q.CameraIDs, ","))
	case ClauseTimeOfDay:
		return fmt.Sprintf("time_of_day(%s-%s)", q.TimeOfDayStart, q.TimeOfDayEnd)
	case ClauseDayOfWeek:
		days := make([]string, len(q.DaysOfWeek))
		for i, d := range q.DaysOfWeek {
			days[i] = fmt.Sprintf("%d", d)
		}
		return fmt.Sprintf("day_of_week(%s)", strings.Join(days, ","))
	case ClauseConfidence:
		return fmt.Sprintf("confidence(>=%.2f)", q.MinConfidence)
	default:
		return fmt.Sprintf("?(%s)", q.Type)
	}
}
