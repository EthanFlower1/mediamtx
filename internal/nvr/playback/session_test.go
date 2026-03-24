package playback

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// collectEvents returns a callback that appends events to a thread-safe slice.
func collectEvents() (func(Event), func() []Event) {
	var mu sync.Mutex
	var events []Event
	cb := func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	}
	getter := func() []Event {
		mu.Lock()
		defer mu.Unlock()
		out := make([]Event, len(events))
		copy(out, events)
		return out
	}
	return cb, getter
}

func TestNewPlaybackSession(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-1", dateStart, 3600.0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	require.Equal(t, "sess-1", s.ID())
	require.Equal(t, StatePaused, s.State())
	require.InDelta(t, 3600.0, s.Position(), 0.001)
	require.InDelta(t, 1.0, s.Speed(), 0.001)
}

func TestPlayPauseTransitions(t *testing.T) {
	cb, getEvents := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-2", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	// Start paused.
	require.Equal(t, StatePaused, s.State())

	// Play.
	s.Play()
	require.Equal(t, StatePlaying, s.State())

	// Emits a state event for play.
	events := getEvents()
	require.GreaterOrEqual(t, len(events), 1)
	found := false
	for _, e := range events {
		if e.EventType == "state" && e.Playing != nil && *e.Playing {
			found = true
			break
		}
	}
	require.True(t, found, "expected a state event with playing=true")

	// Pause.
	s.Pause()
	require.Equal(t, StatePaused, s.State())

	events = getEvents()
	foundPause := false
	for _, e := range events {
		if e.EventType == "state" && e.Playing != nil && !*e.Playing {
			foundPause = true
			break
		}
	}
	require.True(t, foundPause, "expected a state event with playing=false")
}

func TestSeekUpdatesPosition(t *testing.T) {
	cb, getEvents := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-3", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	// Add a mock camera with a real SpliceMuxer to verify Splice is called.
	tracks := []TrackInfo{{ID: 1, TimeScale: 90000, Codec: "avc1"}}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	// Write a sample and flush so the muxer has a non-zero lastDTS.
	muxer.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	muxer.FlushFragment()
	drainOne(t, muxer.Out)

	// Seek to 1800 seconds (30 minutes).
	s.Seek(1800.0)

	require.InDelta(t, 1800.0, s.Position(), 0.001)

	// Verify seek event was emitted.
	events := getEvents()
	foundSeek := false
	for _, e := range events {
		if e.EventType == "position" && e.Position != nil {
			if *e.Position == 1800.0 {
				foundSeek = true
				break
			}
		}
	}
	require.True(t, foundSeek, "expected a position event at 1800")
}

func TestSeekCallsSpliceOnMuxers(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-4", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	tracks := []TrackInfo{{ID: 1, TimeScale: 90000, Codec: "avc1"}}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	// Write samples to the muxer buffer.
	muxer.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	muxer.WriteSample(Sample{DTS: 3000, Duration: 3000, IsSync: false, Data: []byte{0x02}, TrackID: 1})

	// Seek should call Splice which discards buffered samples.
	s.Seek(500.0)

	// Flush should produce nothing since splice discarded the buffer.
	muxer.FlushFragment()
	select {
	case <-muxer.Out:
		t.Fatal("expected no data after seek/splice discarded buffered samples")
	default:
		// Good.
	}
}

func TestSetSpeedUpdatesAudioDrop(t *testing.T) {
	cb, getEvents := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-5", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	tracks := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	// At default speed 1.0, audio should not be dropped.
	// Write audio + video, flush, and check audio is present.
	muxer.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	muxer.WriteSample(Sample{DTS: 0, Duration: 1024, IsSync: true, Data: []byte{0xAA}, TrackID: 2})
	muxer.FlushFragment()
	frag := drainOne(t, muxer.Out)
	trackIDs := extractTrackIDs(t, frag)
	require.Contains(t, trackIDs, uint32(2), "at 1.0x audio should be present")

	// Set speed to 2.0x — audio should be dropped.
	s.SetSpeed(2.0)
	require.InDelta(t, 2.0, s.Speed(), 0.001)

	// Verify audio is now dropped.
	muxer.WriteSample(Sample{DTS: 3000, Duration: 3000, IsSync: true, Data: []byte{0x03}, TrackID: 1})
	muxer.WriteSample(Sample{DTS: 1024, Duration: 1024, IsSync: true, Data: []byte{0xBB}, TrackID: 2})
	muxer.FlushFragment()
	frag2 := drainOne(t, muxer.Out)
	trackIDs2 := extractTrackIDs(t, frag2)
	require.Contains(t, trackIDs2, uint32(1))
	require.NotContains(t, trackIDs2, uint32(2), "at 2.0x audio should be dropped")

	// Set speed back to 1.0 — audio should come back.
	s.SetSpeed(1.0)

	muxer.WriteSample(Sample{DTS: 6000, Duration: 3000, IsSync: true, Data: []byte{0x05}, TrackID: 1})
	muxer.WriteSample(Sample{DTS: 2048, Duration: 1024, IsSync: true, Data: []byte{0xCC}, TrackID: 2})
	muxer.FlushFragment()
	frag3 := drainOne(t, muxer.Out)
	trackIDs3 := extractTrackIDs(t, frag3)
	require.Contains(t, trackIDs3, uint32(2), "at 1.0x audio should be present again")

	// Verify speed event was emitted.
	events := getEvents()
	foundSpeed := false
	for _, e := range events {
		if e.EventType == "state" && e.Speed != nil && *e.Speed == 2.0 {
			foundSpeed = true
			break
		}
	}
	require.True(t, foundSpeed, "expected a state event with speed=2.0")
}

func TestDisposeTransition(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-6", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)

	tracks := []TrackInfo{{ID: 1, TimeScale: 90000, Codec: "avc1"}}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	s.Dispose()
	require.Equal(t, StateDisposed, s.State())

	// After dispose, the muxer's Out channel should be closed.
	_, ok := <-muxer.Out
	require.False(t, ok, "muxer Out channel should be closed after Dispose")
}

func TestDisposeIdempotent(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-7", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)

	// Double dispose should not panic.
	s.Dispose()
	s.Dispose()
	require.Equal(t, StateDisposed, s.State())
}

func TestPlayAfterDispose(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-8", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	s.Dispose()

	// Play after dispose should be a no-op.
	s.Play()
	require.Equal(t, StateDisposed, s.State())
}

func TestStepSetsState(t *testing.T) {
	cb, getEvents := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-9", dateStart, 100.0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	// Step forward.
	s.Step(1)

	// After step, session should go back to paused.
	require.Equal(t, StatePaused, s.State())

	// Check that a position event was emitted.
	events := getEvents()
	foundPos := false
	for _, e := range events {
		if e.EventType == "position" && e.Position != nil {
			foundPos = true
			break
		}
	}
	require.True(t, foundPos, "expected a position event after step")
}

func TestStreamChannelReturnsNilForUnknownCamera(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-10", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	ch := s.StreamChannel("nonexistent")
	require.Nil(t, ch)
}

func TestStreamChannelReturnsMuxerOut(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-11", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	tracks := []TrackInfo{{ID: 1, TimeScale: 90000, Codec: "avc1"}}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	ch := s.StreamChannel("cam1")
	require.NotNil(t, ch)
	require.Equal(t, (<-chan []byte)(muxer.Out), ch)
}

func TestRemoveCamera(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-12", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	tracks := []TrackInfo{{ID: 1, TimeScale: 90000, Codec: "avc1"}}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	s.RemoveCamera("cam1")

	// Channel should be closed.
	_, ok := <-muxer.Out
	require.False(t, ok, "muxer Out channel should be closed after RemoveCamera")

	// StreamChannel should return nil.
	ch := s.StreamChannel("cam1")
	require.Nil(t, ch)
}

func TestLastActivityUpdatesOnCommands(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-13", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	before := s.LastActivity()
	time.Sleep(1 * time.Millisecond)

	s.Play()
	afterPlay := s.LastActivity()
	require.True(t, afterPlay.After(before), "lastActivity should advance after Play")

	time.Sleep(1 * time.Millisecond)
	s.Seek(500.0)
	afterSeek := s.LastActivity()
	require.True(t, afterSeek.After(afterPlay), "lastActivity should advance after Seek")
}

func TestSetSpeedNegativeDropsAudio(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-14", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	tracks := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}
	muxer := NewSpliceMuxer(tracks)

	s.mu.Lock()
	s.cameras["cam1"] = &CameraMuxer{
		CameraID: "cam1",
		Path:     "cam1",
		Muxer:    muxer,
	}
	s.mu.Unlock()

	// Set negative speed (reverse).
	s.SetSpeed(-1.0)

	muxer.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	muxer.WriteSample(Sample{DTS: 0, Duration: 1024, IsSync: true, Data: []byte{0xAA}, TrackID: 2})
	muxer.FlushFragment()
	frag := drainOne(t, muxer.Out)
	trackIDs := extractTrackIDs(t, frag)
	require.Contains(t, trackIDs, uint32(1))
	require.NotContains(t, trackIDs, uint32(2), "at -1.0x audio should be dropped")
}

func TestConcurrentAccess(t *testing.T) {
	cb, _ := collectEvents()
	dateStart := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	s := NewPlaybackSession("sess-15", dateStart, 0, "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f", cb)
	defer s.Dispose()

	// Verify no races when calling methods concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Play()
			s.Pause()
			s.Seek(float64(i) * 100)
			s.SetSpeed(2.0)
			s.SetSpeed(1.0)
			_ = s.State()
			_ = s.Position()
			_ = s.Speed()
			_ = s.LastActivity()
			s.Step(1)
		}()
	}
	wg.Wait()
}
