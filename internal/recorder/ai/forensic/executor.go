package forensic

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/ai"
	db "github.com/bluenviron/mediamtx/internal/shared/legacydb"
)

// Executor runs forensic queries against the NVR database and CLIP embedder.
type Executor struct {
	DB       *db.DB
	Embedder *ai.Embedder // may be nil

	// SnippetPadding is the amount of time before/after a match for the snippet
	// preview window. Default 15 seconds.
	SnippetPadding time.Duration
}

// ExecuteRequest contains the parameters for a forensic search execution.
type ExecuteRequest struct {
	Query  *Query `json:"query"`
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// Execute runs the forensic query and returns a result set.
func (e *Executor) Execute(req *ExecuteRequest) (*ResultSet, error) {
	start := time.Now()

	if err := req.Query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 200 {
		req.Limit = 200
	}

	// Extract global constraints from the query tree to narrow the DB scan.
	constraints := extractConstraints(req.Query)

	// Load candidate results from the database.
	candidates, err := e.loadCandidates(constraints)
	if err != nil {
		return nil, fmt.Errorf("loading candidates: %w", err)
	}

	// Evaluate each candidate against the full query tree.
	var matched []Result
	for i := range candidates {
		score, ok := e.evaluate(req.Query, &candidates[i])
		if ok && score > 0 {
			candidates[i].Score = score
			e.addSnippet(&candidates[i])
			matched = append(matched, candidates[i])
		}
	}

	// Sort by score descending, then by timestamp descending.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Score != matched[j].Score {
			return matched[i].Score > matched[j].Score
		}
		return matched[i].Timestamp.After(matched[j].Timestamp)
	})

	// Deduplicate by ID.
	matched = dedup(matched)

	totalMatches := len(matched)

	// Apply offset and limit.
	if req.Offset > 0 && req.Offset < len(matched) {
		matched = matched[req.Offset:]
	} else if req.Offset >= len(matched) {
		matched = nil
	}
	if len(matched) > req.Limit {
		matched = matched[:req.Limit]
	}

	elapsed := time.Since(start)

	return &ResultSet{
		Query:           req.Query.String(),
		TotalMatches:    totalMatches,
		Results:         matched,
		ExecutionTimeMs: elapsed.Milliseconds(),
	}, nil
}

// constraints holds the extracted global filters from the query tree.
type constraints struct {
	cameraIDs     []string
	start         time.Time
	end           time.Time
	objectClasses []string
	plateText     string
	clipText      string
	minConfidence float64
}

// extractConstraints walks the query tree and extracts filter constraints
// that can be pushed down to the database layer for efficiency.
func extractConstraints(q *Query) constraints {
	c := constraints{
		start: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		end:   time.Now().UTC().Add(time.Hour),
	}
	collectConstraints(q, &c)
	return c
}

func collectConstraints(q *Query, c *constraints) {
	if q.IsLeaf() {
		switch q.Type {
		case ClauseCamera:
			if len(c.cameraIDs) == 0 {
				c.cameraIDs = q.CameraIDs
			}
		case ClauseTime:
			if q.Start != nil && q.Start.After(c.start) {
				c.start = *q.Start
			}
			if q.End != nil && q.End.Before(c.end) {
				c.end = *q.End
			}
		case ClauseObject:
			c.objectClasses = append(c.objectClasses, strings.Split(q.ObjectClass, ",")...)
		case ClauseLPR:
			c.plateText = q.PlateText
		case ClauseCLIP:
			c.clipText = q.CLIPText
		case ClauseConfidence:
			if q.MinConfidence > c.minConfidence {
				c.minConfidence = q.MinConfidence
			}
		}
		return
	}
	// Only collect from AND nodes (OR/NOT children shouldn't narrow globally).
	if q.Op == OpAND {
		for _, child := range q.Children {
			collectConstraints(child, c)
		}
	}
}

// loadCandidates fetches raw results from the database based on constraints.
func (e *Executor) loadCandidates(c constraints) ([]Result, error) {
	var results []Result

	// Load from detection events.
	for _, camID := range e.cameraIDsOrAll(c.cameraIDs) {
		events, err := e.DB.QueryDetectionEvents(camID, "", c.start, c.end)
		if err != nil {
			return nil, fmt.Errorf("querying detection events for camera %s: %w", camID, err)
		}
		for _, ev := range events {
			ts, _ := time.Parse("2006-01-02T15:04:05.000Z", ev.StartTime)
			if ts.IsZero() {
				ts, _ = time.Parse(time.RFC3339, ev.StartTime)
			}
			results = append(results, Result{
				ID:             fmt.Sprintf("det-event-%d", ev.ID),
				CameraID:       ev.CameraID,
				Timestamp:      ts,
				MatchedClasses: []string{ev.Class},
				Confidence:     ev.PeakConfidence,
				ThumbnailPath:  ev.ThumbnailPath,
			})
		}
	}

	// Load from CLIP search if we have text.
	if c.clipText != "" {
		for _, camID := range e.cameraIDsOrAll(c.cameraIDs) {
			clipResults, err := ai.Search(e.Embedder, e.DB, c.clipText, camID, c.start, c.end, 200)
			if err != nil {
				return nil, fmt.Errorf("CLIP search: %w", err)
			}
			for _, cr := range clipResults {
				ts, _ := time.Parse("2006-01-02T15:04:05.000Z", cr.FrameTime)
				if ts.IsZero() {
					ts, _ = time.Parse(time.RFC3339, cr.FrameTime)
				}
				results = append(results, Result{
					ID:             fmt.Sprintf("clip-%d-%d", cr.DetectionID, cr.EventID),
					CameraID:       cr.CameraID,
					CameraName:     cr.CameraName,
					Timestamp:      ts,
					MatchedClasses: []string{cr.Class},
					CLIPSimilarity: cr.Similarity,
					Confidence:     cr.Confidence,
					ThumbnailPath:  cr.ThumbnailPath,
				})
			}
		}
	}

	// Load LPR-related detections if plate text is specified.
	// LPR reads are stored as detections with class containing plate info
	// in the attributes field, or matched via detection_events with class "license_plate".
	if c.plateText != "" {
		for _, camID := range e.cameraIDsOrAll(c.cameraIDs) {
			events, err := e.DB.QueryDetectionEvents(camID, "license_plate", c.start, c.end)
			if err != nil {
				return nil, fmt.Errorf("querying LPR events for camera %s: %w", camID, err)
			}
			for _, ev := range events {
				ts, _ := time.Parse("2006-01-02T15:04:05.000Z", ev.StartTime)
				if ts.IsZero() {
					ts, _ = time.Parse(time.RFC3339, ev.StartTime)
				}
				results = append(results, Result{
					ID:             fmt.Sprintf("lpr-event-%d", ev.ID),
					CameraID:       ev.CameraID,
					Timestamp:      ts,
					MatchedClasses: []string{"license_plate"},
					MatchedPlate:   ev.ZoneID, // ZoneID may hold plate text for LPR events
					Confidence:     ev.PeakConfidence,
					ThumbnailPath:  ev.ThumbnailPath,
				})
			}
		}
	}

	return results, nil
}

// cameraIDsOrAll returns the given IDs, or a single empty string to match all.
func (e *Executor) cameraIDsOrAll(ids []string) []string {
	if len(ids) == 0 {
		return []string{""}
	}
	return ids
}

// evaluate recursively evaluates a query against a candidate result.
// Returns (score, matched). Score is 0-1 for leaf clauses.
func (e *Executor) evaluate(q *Query, r *Result) (float64, bool) {
	if q.IsLeaf() {
		return e.evaluateLeaf(q, r)
	}

	switch q.Op {
	case OpAND:
		minScore := 1.0
		for _, child := range q.Children {
			score, ok := e.evaluate(child, r)
			if !ok {
				return 0, false
			}
			if score < minScore {
				minScore = score
			}
		}
		return minScore, true

	case OpOR:
		maxScore := 0.0
		anyMatch := false
		for _, child := range q.Children {
			score, ok := e.evaluate(child, r)
			if ok {
				anyMatch = true
				if score > maxScore {
					maxScore = score
				}
			}
		}
		return maxScore, anyMatch

	case OpNOT:
		_, ok := e.evaluate(q.Children[0], r)
		if ok {
			return 0, false // matched the negation, so exclude
		}
		return 1.0, true // did not match negation, so include

	default:
		return 0, false
	}
}

func (e *Executor) evaluateLeaf(q *Query, r *Result) (float64, bool) {
	switch q.Type {
	case ClauseCLIP:
		// CLIP results already have similarity scores loaded.
		if r.CLIPSimilarity > 0 {
			return r.CLIPSimilarity, true
		}
		// Check class name as fallback.
		text := strings.ToLower(q.CLIPText)
		for _, cls := range r.MatchedClasses {
			if strings.Contains(strings.ToLower(cls), text) || strings.Contains(text, strings.ToLower(cls)) {
				return 0.5, true
			}
		}
		return 0, false

	case ClauseObject:
		classes := strings.Split(strings.ToLower(q.ObjectClass), ",")
		for _, want := range classes {
			want = strings.TrimSpace(want)
			for _, have := range r.MatchedClasses {
				if strings.EqualFold(have, want) {
					return 1.0, true
				}
			}
		}
		return 0, false

	case ClauseLPR:
		plate := strings.ToUpper(strings.TrimSpace(q.PlateText))
		if r.MatchedPlate == "" {
			return 0, false
		}
		rPlate := strings.ToUpper(r.MatchedPlate)
		if strings.Contains(plate, "*") {
			// Wildcard matching: "ABC*" matches "ABC123".
			prefix := strings.TrimSuffix(plate, "*")
			if strings.HasPrefix(rPlate, prefix) {
				return 1.0, true
			}
			return 0, false
		}
		if rPlate == plate {
			return 1.0, true
		}
		if strings.Contains(rPlate, plate) {
			return 0.8, true
		}
		return 0, false

	case ClauseTime:
		if q.Start != nil && r.Timestamp.Before(*q.Start) {
			return 0, false
		}
		if q.End != nil && r.Timestamp.After(*q.End) {
			return 0, false
		}
		return 1.0, true

	case ClauseCamera:
		for _, id := range q.CameraIDs {
			if r.CameraID == id {
				return 1.0, true
			}
		}
		return 0, false

	case ClauseTimeOfDay:
		return e.evaluateTimeOfDay(q, r)

	case ClauseDayOfWeek:
		return e.evaluateDayOfWeek(q, r)

	case ClauseConfidence:
		if r.Confidence >= q.MinConfidence {
			return 1.0, true
		}
		return 0, false

	default:
		return 0, false
	}
}

func (e *Executor) evaluateTimeOfDay(q *Query, r *Result) (float64, bool) {
	startH, startM, err1 := parseHHMM(q.TimeOfDayStart)
	endH, endM, err2 := parseHHMM(q.TimeOfDayEnd)
	if err1 != nil || err2 != nil {
		return 0, false
	}

	h, m := r.Timestamp.Hour(), r.Timestamp.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM
	curMinutes := h*60 + m

	if startMinutes <= endMinutes {
		// Normal window (e.g., 09:00-17:00).
		if curMinutes >= startMinutes && curMinutes <= endMinutes {
			return 1.0, true
		}
	} else {
		// Overnight window (e.g., 22:00-06:00).
		if curMinutes >= startMinutes || curMinutes <= endMinutes {
			return 1.0, true
		}
	}
	return 0, false
}

func (e *Executor) evaluateDayOfWeek(q *Query, r *Result) (float64, bool) {
	// Go: Sunday=0, Monday=1..Saturday=6
	// ISO: Monday=1..Sunday=7
	goDay := r.Timestamp.Weekday()
	isoDay := int(goDay)
	if isoDay == 0 {
		isoDay = 7 // Sunday
	}
	for _, d := range q.DaysOfWeek {
		if d == isoDay {
			return 1.0, true
		}
	}
	return 0, false
}

func parseHHMM(s string) (int, int, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time format: %q", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return h, m, nil
}

func (e *Executor) addSnippet(r *Result) {
	pad := e.SnippetPadding
	if pad == 0 {
		pad = 15 * time.Second
	}
	start := r.Timestamp.Add(-pad)
	end := r.Timestamp.Add(pad)
	r.SnippetStart = &start
	r.SnippetEnd = &end
}

// dedup removes duplicate results by ID, keeping the highest-scored instance.
func dedup(results []Result) []Result {
	seen := make(map[string]int) // ID -> index in out
	var out []Result
	for _, r := range results {
		if idx, ok := seen[r.ID]; ok {
			if r.Score > out[idx].Score {
				out[idx] = r
			}
		} else {
			seen[r.ID] = len(out)
			out = append(out, r)
		}
	}
	return out
}
