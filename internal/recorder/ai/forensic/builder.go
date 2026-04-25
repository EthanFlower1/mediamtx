package forensic

import "time"

// Builder provides a fluent API for constructing forensic query trees.
type Builder struct {
	root *Query
}

// NewBuilder creates a new query builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// And combines the given queries with AND.
func And(queries ...*Query) *Query {
	return &Query{Op: OpAND, Children: queries}
}

// Or combines the given queries with OR.
func Or(queries ...*Query) *Query {
	return &Query{Op: OpOR, Children: queries}
}

// Not negates the given query.
func Not(q *Query) *Query {
	return &Query{Op: OpNOT, Children: []*Query{q}}
}

// CLIP creates a CLIP text similarity search clause.
func CLIP(text string) *Query {
	return &Query{Type: ClauseCLIP, CLIPText: text}
}

// Object creates an object class match clause.
func Object(classes ...string) *Query {
	return &Query{Type: ClauseObject, ObjectClass: joinClasses(classes)}
}

// LPR creates a license plate text match clause.
func LPR(plateText string) *Query {
	return &Query{Type: ClauseLPR, PlateText: plateText}
}

// TimeRange creates a time range clause.
func TimeRange(start, end time.Time) *Query {
	s, e := start, end
	return &Query{Type: ClauseTime, Start: &s, End: &e}
}

// Camera creates a camera filter clause.
func Camera(ids ...string) *Query {
	return &Query{Type: ClauseCamera, CameraIDs: ids}
}

// TimeOfDay creates a recurring daily time window clause (HH:MM format).
func TimeOfDay(start, end string) *Query {
	return &Query{Type: ClauseTimeOfDay, TimeOfDayStart: start, TimeOfDayEnd: end}
}

// DayOfWeek creates a day-of-week filter clause (ISO: 1=Mon..7=Sun).
func DayOfWeek(days ...int) *Query {
	return &Query{Type: ClauseDayOfWeek, DaysOfWeek: days}
}

// Confidence creates a minimum confidence threshold clause.
func Confidence(min float64) *Query {
	return &Query{Type: ClauseConfidence, MinConfidence: min}
}

func joinClasses(classes []string) string {
	if len(classes) == 0 {
		return ""
	}
	result := classes[0]
	for _, c := range classes[1:] {
		result += "," + c
	}
	return result
}

// Build constructs the final query from the builder.
func (b *Builder) CLIP(text string) *Builder {
	b.addChild(CLIP(text))
	return b
}

// Object adds an object class filter.
func (b *Builder) Object(classes ...string) *Builder {
	b.addChild(Object(classes...))
	return b
}

// LPR adds a license plate search.
func (b *Builder) LPR(plateText string) *Builder {
	b.addChild(LPR(plateText))
	return b
}

// TimeRange adds an absolute time range.
func (b *Builder) TimeRange(start, end time.Time) *Builder {
	b.addChild(TimeRange(start, end))
	return b
}

// Camera adds a camera filter.
func (b *Builder) Camera(ids ...string) *Builder {
	b.addChild(Camera(ids...))
	return b
}

// TimeOfDay adds a recurring daily time window.
func (b *Builder) TimeOfDay(start, end string) *Builder {
	b.addChild(TimeOfDay(start, end))
	return b
}

// DayOfWeek adds a day-of-week filter.
func (b *Builder) DayOfWeek(days ...int) *Builder {
	b.addChild(DayOfWeek(days...))
	return b
}

// Confidence adds a minimum confidence threshold.
func (b *Builder) Confidence(min float64) *Builder {
	b.addChild(Confidence(min))
	return b
}

// Build returns the composed query. If multiple clauses were added,
// they are combined with AND.
func (b *Builder) Build() *Query {
	if b.root == nil {
		return &Query{Type: ClauseObject, ObjectClass: "*"}
	}
	if b.root.Op == OpAND && len(b.root.Children) == 1 {
		return b.root.Children[0]
	}
	return b.root
}

func (b *Builder) addChild(q *Query) {
	if b.root == nil {
		b.root = &Query{Op: OpAND, Children: []*Query{q}}
	} else {
		b.root.Children = append(b.root.Children, q)
	}
}
