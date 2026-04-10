package timeline

import (
	"sort"
	"time"
)

const gapThreshold = time.Second

// Assembler merges raw segments from multiple Recorders into a unified timeline.
type Assembler struct {
	store SegmentStore
}

// NewAssembler creates an Assembler backed by the given store.
func NewAssembler(store SegmentStore) *Assembler {
	return &Assembler{store: store}
}

// Assemble queries segments for the given request and returns merged segments
// with gap detection.
func (a *Assembler) Assemble(req TimelineRequest) (*TimelineResponse, error) {
	raw, err := a.store.QuerySegments(req.CameraIDs, req.Start, req.End)
	if err != nil {
		return nil, err
	}

	// Group segments by camera.
	byCam := make(map[string][]Segment)
	for _, seg := range raw {
		byCam[seg.CameraID] = append(byCam[seg.CameraID], seg)
	}

	var allMerged []MergedSegment
	var allGaps []Gap

	for _, camID := range req.CameraIDs {
		segs := byCam[camID]
		merged := mergeSegments(camID, segs)
		gaps := detectGaps(camID, merged, req.Start, req.End)
		allMerged = append(allMerged, merged...)
		allGaps = append(allGaps, gaps...)
	}

	// Sort merged segments by start time globally.
	sort.Slice(allMerged, func(i, j int) bool {
		if allMerged[i].Start.Equal(allMerged[j].Start) {
			return allMerged[i].CameraID < allMerged[j].CameraID
		}
		return allMerged[i].Start.Before(allMerged[j].Start)
	})

	if allMerged == nil {
		allMerged = []MergedSegment{}
	}
	if allGaps == nil {
		allGaps = []Gap{}
	}

	return &TimelineResponse{
		Segments: allMerged,
		Gaps:     allGaps,
	}, nil
}

// mergeSegments takes all segments for a single camera (potentially from
// multiple Recorders) and merges overlapping/adjacent ranges.
func mergeSegments(cameraID string, segments []Segment) []MergedSegment {
	if len(segments) == 0 {
		return nil
	}

	// Sort by start time.
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].Start.Before(segments[j].Start)
	})

	var merged []MergedSegment
	current := MergedSegment{
		CameraID:    cameraID,
		RecorderIDs: []string{segments[0].RecorderID},
		Start:       segments[0].Start,
		End:         segments[0].End,
	}

	for _, seg := range segments[1:] {
		// Check if this segment overlaps or is adjacent (within gapThreshold).
		if seg.Start.Before(current.End.Add(gapThreshold)) || seg.Start.Equal(current.End) {
			// Extend the current merged segment.
			if seg.End.After(current.End) {
				current.End = seg.End
			}
			// Track the recorder if not already present.
			if !containsString(current.RecorderIDs, seg.RecorderID) {
				current.RecorderIDs = append(current.RecorderIDs, seg.RecorderID)
			}
		} else {
			// Gap detected — finalize current and start new.
			merged = append(merged, current)
			current = MergedSegment{
				CameraID:    cameraID,
				RecorderIDs: []string{seg.RecorderID},
				Start:       seg.Start,
				End:         seg.End,
			}
		}
	}
	merged = append(merged, current)
	return merged
}

// detectGaps finds time ranges within [queryStart, queryEnd] not covered by
// any merged segment for a given camera.
func detectGaps(cameraID string, merged []MergedSegment, queryStart, queryEnd time.Time) []Gap {
	if len(merged) == 0 {
		// Entire range is a gap.
		return []Gap{{CameraID: cameraID, Start: queryStart, End: queryEnd}}
	}

	var gaps []Gap

	// Gap before first segment.
	if merged[0].Start.After(queryStart.Add(gapThreshold)) {
		gaps = append(gaps, Gap{
			CameraID: cameraID,
			Start:    queryStart,
			End:      merged[0].Start,
		})
	}

	// Gaps between segments.
	for i := 1; i < len(merged); i++ {
		if merged[i].Start.After(merged[i-1].End.Add(gapThreshold)) {
			gaps = append(gaps, Gap{
				CameraID: cameraID,
				Start:    merged[i-1].End,
				End:      merged[i].Start,
			})
		}
	}

	// Gap after last segment.
	last := merged[len(merged)-1]
	if queryEnd.After(last.End.Add(gapThreshold)) {
		gaps = append(gaps, Gap{
			CameraID: cameraID,
			Start:    last.End,
			End:      queryEnd,
		})
	}

	return gaps
}

func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
