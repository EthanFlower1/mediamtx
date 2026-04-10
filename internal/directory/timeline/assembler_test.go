package timeline

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStore struct {
	segments []Segment
	err      error
}

func (f *fakeStore) QuerySegments(_ []string, _, _ time.Time) ([]Segment, error) {
	return f.segments, f.err
}

var (
	t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = t0.Add(1 * time.Hour)
	t2 = t0.Add(2 * time.Hour)
	t3 = t0.Add(3 * time.Hour)
	t4 = t0.Add(4 * time.Hour)
	t5 = t0.Add(5 * time.Hour)
	t6 = t0.Add(6 * time.Hour)
)

func TestAssembler_SingleSegment(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t1, End: t3},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t6})
	require.NoError(t, err)

	require.Len(t, resp.Segments, 1)
	assert.Equal(t, "cam-1", resp.Segments[0].CameraID)
	assert.Equal(t, []string{"rec-A"}, resp.Segments[0].RecorderIDs)
	assert.Equal(t, t1, resp.Segments[0].Start)
	assert.Equal(t, t3, resp.Segments[0].End)

	// Gaps: before first segment and after last.
	require.Len(t, resp.Gaps, 2)
	assert.Equal(t, t0, resp.Gaps[0].Start)
	assert.Equal(t, t1, resp.Gaps[0].End)
	assert.Equal(t, t3, resp.Gaps[1].Start)
	assert.Equal(t, t6, resp.Gaps[1].End)
}

func TestAssembler_OverlappingFromDifferentRecorders(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t1, End: t3},
		{CameraID: "cam-1", RecorderID: "rec-B", Start: t2, End: t4},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t1, End: t4})
	require.NoError(t, err)

	// Overlapping segments merge into one.
	require.Len(t, resp.Segments, 1)
	assert.Equal(t, t1, resp.Segments[0].Start)
	assert.Equal(t, t4, resp.Segments[0].End)
	assert.ElementsMatch(t, []string{"rec-A", "rec-B"}, resp.Segments[0].RecorderIDs)
	assert.Empty(t, resp.Gaps)
}

func TestAssembler_GapBetweenSegments(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t0, End: t1},
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t3, End: t4},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t4})
	require.NoError(t, err)

	require.Len(t, resp.Segments, 2)
	require.Len(t, resp.Gaps, 1)
	assert.Equal(t, t1, resp.Gaps[0].Start)
	assert.Equal(t, t3, resp.Gaps[0].End)
}

func TestAssembler_AdjacentSegmentsMerge(t *testing.T) {
	// Segments are adjacent (within 1s threshold).
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t0, End: t1},
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t1, End: t2},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t2})
	require.NoError(t, err)

	require.Len(t, resp.Segments, 1)
	assert.Equal(t, t0, resp.Segments[0].Start)
	assert.Equal(t, t2, resp.Segments[0].End)
	assert.Empty(t, resp.Gaps)
}

func TestAssembler_MultipleCameras(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t0, End: t2},
		{CameraID: "cam-2", RecorderID: "rec-B", Start: t1, End: t3},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1", "cam-2"}, Start: t0, End: t4})
	require.NoError(t, err)

	require.Len(t, resp.Segments, 2)
	// cam-1 segment comes first (earlier start).
	assert.Equal(t, "cam-1", resp.Segments[0].CameraID)
	assert.Equal(t, "cam-2", resp.Segments[1].CameraID)
}

func TestAssembler_NoCoverage(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: nil})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t6})
	require.NoError(t, err)

	assert.Empty(t, resp.Segments)
	require.Len(t, resp.Gaps, 1)
	assert.Equal(t, t0, resp.Gaps[0].Start)
	assert.Equal(t, t6, resp.Gaps[0].End)
}

func TestAssembler_FullCoverage(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t0, End: t6},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t6})
	require.NoError(t, err)

	require.Len(t, resp.Segments, 1)
	assert.Empty(t, resp.Gaps)
}

func TestAssembler_UnsortedInput(t *testing.T) {
	// Feed segments out of order — assembler should sort them.
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t3, End: t4},
		{CameraID: "cam-1", RecorderID: "rec-B", Start: t0, End: t1},
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t1, End: t2},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t5})
	require.NoError(t, err)

	// t0-t1 (rec-B), t1-t2 (rec-A) should merge (adjacent), then gap, then t3-t4.
	require.Len(t, resp.Segments, 2)
	assert.Equal(t, t0, resp.Segments[0].Start)
	assert.Equal(t, t2, resp.Segments[0].End)
	assert.Equal(t, t3, resp.Segments[1].Start)

	require.Len(t, resp.Gaps, 2) // between segments and after last
}

func TestAssembler_DuplicateRecorderNotRepeated(t *testing.T) {
	a := NewAssembler(&fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t0, End: t2},
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t1, End: t3},
	}})

	resp, err := a.Assemble(TimelineRequest{CameraIDs: []string{"cam-1"}, Start: t0, End: t3})
	require.NoError(t, err)

	require.Len(t, resp.Segments, 1)
	assert.Equal(t, []string{"rec-A"}, resp.Segments[0].RecorderIDs)
}
