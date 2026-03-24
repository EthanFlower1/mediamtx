package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	amp4 "github.com/abema/go-mp4"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
)

// ErrNoSegments is returned when no recording segments are found for a camera.
var ErrNoSegments = errors.New("no recording segments found for camera")

// stepDuration is the amount of time to advance/rewind per step command.
const stepDuration = time.Second / 30 // ~one frame at 30fps

// positionEmitInterval is how often position events are emitted during playback.
const positionEmitInterval = 500 * time.Millisecond

// fragmentDuration is the target wall-clock duration of each playback fragment.
const fragmentDuration = time.Second

// CameraMuxer holds the muxer and segment state for a single camera in a playback session.
type CameraMuxer struct {
	CameraID string
	Path     string // MediaMTX path name
	Muxer    *SpliceMuxer
	// Current segment state
	segments []*recordstore.Segment
	segIndex int
}

// PlaybackSession is the state machine that owns per-camera muxers, handles
// playback commands (play, pause, seek, speed, step), and manages a playback
// goroutine that reads segments and writes fragments.
type PlaybackSession struct {
	mu sync.Mutex

	id           string
	state        SessionState
	speed        float64
	positionSecs float64   // seconds since midnight of dateStart
	dateStart    time.Time // midnight of selected date

	cameras    map[string]*CameraMuxer
	recordPath string // pattern like "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"

	ctx    context.Context
	cancel context.CancelFunc

	onEvent      func(Event)
	lastActivity time.Time
	createdAt    time.Time

	// playWake is used to signal the playback goroutine when state changes
	// from non-playing to playing.
	playWake chan struct{}
}

// NewPlaybackSession creates a new session starting paused at startPos.
func NewPlaybackSession(
	id string,
	dateStart time.Time,
	startPos float64,
	recordPath string,
	onEvent func(Event),
) *PlaybackSession {
	ctx, cancel := context.WithCancel(context.Background())
	now := time.Now()

	s := &PlaybackSession{
		id:           id,
		state:        StatePaused,
		speed:        1.0,
		positionSecs: startPos,
		dateStart:    dateStart,
		cameras:      make(map[string]*CameraMuxer),
		recordPath:   recordPath,
		ctx:          ctx,
		cancel:       cancel,
		onEvent:      onEvent,
		lastActivity: now,
		createdAt:    now,
		playWake:     make(chan struct{}, 1),
	}

	go s.playbackLoop()

	return s
}

// ID returns the session identifier.
func (s *PlaybackSession) ID() string {
	return s.id
}

// State returns the current session state.
func (s *PlaybackSession) State() SessionState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Position returns the current playback position in seconds since midnight.
func (s *PlaybackSession) Position() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.positionSecs
}

// Speed returns the current playback speed.
func (s *PlaybackSession) Speed() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.speed
}

// LastActivity returns the time of the last command received.
func (s *PlaybackSession) LastActivity() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastActivity
}

// CreatedAt returns the session creation time.
func (s *PlaybackSession) CreatedAt() time.Time {
	return s.createdAt
}

// IsPlaying returns true when the session is in the playing state.
func (s *PlaybackSession) IsPlaying() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == StatePlaying
}

// SetEventCallback replaces the event callback, allowing a new WebSocket
// connection to receive events for an existing session.
func (s *PlaybackSession) SetEventCallback(fn func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onEvent = fn
}

// IsIdle returns true when the session has had no activity for longer than
// the given grace period.
func (s *PlaybackSession) IsIdle(grace time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastActivity) > grace
}

// AddCamera finds recording segments for the given camera and adds it to the session.
func (s *PlaybackSession) AddCamera(cameraID, mediamtxPath, recordPathPattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return errors.New("session is disposed")
	}

	pathConf := &conf.Path{
		RecordPath:   recordPathPattern,
		RecordFormat: conf.RecordFormatFMP4,
	}

	segments, err := recordstore.FindSegments(pathConf, mediamtxPath, nil, nil)
	if err != nil {
		return fmt.Errorf("find segments for %s: %w", cameraID, err)
	}
	if len(segments) == 0 {
		return ErrNoSegments
	}

	header, err := ReadSegmentHeader(segments[0].Fpath)
	if err != nil {
		return fmt.Errorf("read segment header for %s: %w", cameraID, err)
	}

	muxer := NewSpliceMuxer(header.Tracks)

	// Read the init segment (ftyp+moov) from the first segment file.
	initData, err := readInitSegment(segments[0].Fpath)
	if err != nil {
		return fmt.Errorf("read init segment for %s: %w", cameraID, err)
	}
	muxer.WriteInit(initData)

	// Set audio drop based on current speed.
	muxer.SetDropAudio(s.speed != 1.0)

	s.cameras[cameraID] = &CameraMuxer{
		CameraID: cameraID,
		Path:     mediamtxPath,
		Muxer:    muxer,
		segments: segments,
	}

	s.lastActivity = time.Now()
	return nil
}

// RemoveCamera removes a camera from the session and closes its output channel.
func (s *PlaybackSession) RemoveCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cam, ok := s.cameras[cameraID]
	if !ok {
		return
	}

	close(cam.Muxer.Out)
	delete(s.cameras, cameraID)
	s.lastActivity = time.Now()
}

// StreamChannel returns the output channel for a camera's muxed fMP4 data,
// or nil if the camera is not in this session.
func (s *PlaybackSession) StreamChannel(cameraID string) <-chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()

	cam, ok := s.cameras[cameraID]
	if !ok {
		return nil
	}
	return cam.Muxer.Out
}

// Play transitions the session to the playing state.
func (s *PlaybackSession) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return
	}

	s.state = StatePlaying
	s.lastActivity = time.Now()

	// Wake the playback goroutine.
	select {
	case s.playWake <- struct{}{}:
	default:
	}

	playing := true
	speed := s.speed
	pos := s.positionSecs
	s.emitEvent(Event{
		EventType: "state",
		Playing:   &playing,
		Speed:     &speed,
		Position:  &pos,
	})
}

// Pause transitions the session to the paused state.
func (s *PlaybackSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return
	}

	s.state = StatePaused
	s.lastActivity = time.Now()

	playing := false
	speed := s.speed
	pos := s.positionSecs
	s.emitEvent(Event{
		EventType: "state",
		Playing:   &playing,
		Speed:     &speed,
		Position:  &pos,
	})
}

// Seek updates the playback position and splices all camera muxers.
func (s *PlaybackSession) Seek(positionSecs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return
	}

	prevState := s.state
	s.state = StateSeeking
	s.positionSecs = positionSecs
	s.lastActivity = time.Now()

	// Splice all camera muxers to prepare for the new position.
	for _, cam := range s.cameras {
		cam.Muxer.Splice()
	}

	// Restore previous state (or paused if was seeking).
	if prevState == StatePlaying {
		s.state = StatePlaying
		// Wake the playback goroutine.
		select {
		case s.playWake <- struct{}{}:
		default:
		}
	} else {
		s.state = StatePaused
	}

	pos := s.positionSecs
	s.emitEvent(Event{
		EventType: "position",
		Position:  &pos,
	})
}

// SetSpeed changes the playback speed and updates audio filtering on all muxers.
// At speed != 1.0, audio is dropped.
func (s *PlaybackSession) SetSpeed(rate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return
	}

	s.speed = rate
	s.lastActivity = time.Now()

	dropAudio := rate != 1.0
	for _, cam := range s.cameras {
		cam.Muxer.SetDropAudio(dropAudio)
	}

	speed := s.speed
	pos := s.positionSecs
	playing := s.state == StatePlaying
	s.emitEvent(Event{
		EventType: "state",
		Speed:     &speed,
		Position:  &pos,
		Playing:   &playing,
	})
}

// Step advances or rewinds by one frame-duration and pauses.
// direction > 0 steps forward, direction < 0 steps backward.
func (s *PlaybackSession) Step(direction int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return
	}

	s.state = StateStepping
	s.lastActivity = time.Now()

	// Drop audio during step.
	for _, cam := range s.cameras {
		cam.Muxer.SetDropAudio(true)
	}

	if direction > 0 {
		s.positionSecs += stepDuration.Seconds()
	} else if direction < 0 {
		s.positionSecs -= stepDuration.Seconds()
		if s.positionSecs < 0 {
			s.positionSecs = 0
		}
	}

	// Splice muxers for the new position.
	for _, cam := range s.cameras {
		cam.Muxer.Splice()
	}

	// Restore audio drop based on speed.
	dropAudio := s.speed != 1.0
	for _, cam := range s.cameras {
		cam.Muxer.SetDropAudio(dropAudio)
	}

	s.state = StatePaused

	pos := s.positionSecs
	s.emitEvent(Event{
		EventType: "position",
		Position:  &pos,
	})
}

// Dispose cancels the session, closes all muxer channels, and transitions to disposed.
func (s *PlaybackSession) Dispose() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateDisposed {
		return
	}

	s.state = StateDisposed
	s.cancel()

	for _, cam := range s.cameras {
		close(cam.Muxer.Out)
	}
}

// emitEvent sends an event via the callback. Must NOT hold the lock when the
// callback might block, but we accept this since onEvent should be non-blocking.
// Caller holds s.mu.
func (s *PlaybackSession) emitEvent(e Event) {
	if s.onEvent != nil {
		s.onEvent(e)
	}
}

// playbackLoop runs in its own goroutine, reading segments and writing fragments
// while the session is in the playing state.
func (s *PlaybackSession) playbackLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.playWake:
			s.runPlayback()
		}
	}
}

// runPlayback drives playback while the session is in the playing state.
// It reads segment samples and advances the position.
func (s *PlaybackSession) runPlayback() {
	posTicker := time.NewTicker(positionEmitInterval)
	defer posTicker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		s.mu.Lock()
		if s.state != StatePlaying {
			s.mu.Unlock()
			return
		}

		speed := s.speed
		pos := s.positionSecs
		s.mu.Unlock()

		// Advance playback for each camera.
		s.advancePlayback()

		// Calculate sleep duration based on speed.
		absSpeed := speed
		if absSpeed < 0 {
			absSpeed = -absSpeed
		}
		if absSpeed == 0 {
			absSpeed = 1.0
		}
		sleepDur := time.Duration(float64(fragmentDuration) / absSpeed)
		if sleepDur < 10*time.Millisecond {
			sleepDur = 10 * time.Millisecond
		}

		// Advance position.
		s.mu.Lock()
		if s.state != StatePlaying {
			s.mu.Unlock()
			return
		}
		s.positionSecs += speed * fragmentDuration.Seconds()
		if s.positionSecs < 0 {
			s.positionSecs = 0
			s.state = StatePaused
			playing := false
			s.emitEvent(Event{EventType: "state", Playing: &playing})
			s.mu.Unlock()
			return
		}
		pos = s.positionSecs
		s.mu.Unlock()

		// Emit position.
		s.onEvent(Event{EventType: "position", Position: &pos})

		// Sleep to pace playback.
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(sleepDur):
		}
	}
}

// advancePlayback reads the next fragment worth of samples from each camera's
// current segment and writes them to the muxer.
func (s *PlaybackSession) advancePlayback() {
	s.mu.Lock()
	pos := s.positionSecs
	cameras := make([]*CameraMuxer, 0, len(s.cameras))
	for _, cam := range s.cameras {
		cameras = append(cameras, cam)
	}
	dateStart := s.dateStart
	s.mu.Unlock()

	targetTime := dateStart.Add(time.Duration(pos * float64(time.Second)))

	for _, cam := range cameras {
		s.advanceCameraPlayback(cam, targetTime)
	}
}

// advanceCameraPlayback reads samples from the current segment for one camera.
func (s *PlaybackSession) advanceCameraPlayback(cam *CameraMuxer, targetTime time.Time) {
	if len(cam.segments) == 0 {
		return
	}

	// Find the segment that covers targetTime.
	segIdx := cam.segIndex
	if segIdx >= len(cam.segments) {
		// Past all segments; nothing to do.
		return
	}

	// Advance segIndex if targetTime is past the current segment.
	for segIdx < len(cam.segments)-1 && targetTime.After(cam.segments[segIdx+1].Start) {
		segIdx++
	}

	// Check for gap: if targetTime is before the current segment's start.
	if targetTime.Before(cam.segments[segIdx].Start) {
		// If we're before the first segment, check if there's a gap.
		if segIdx == 0 || targetTime.After(cam.segments[segIdx-1].Start) {
			gapStart := s.positionSecs
			nextStart := cam.segments[segIdx].Start.Sub(s.dateStart).Seconds()
			s.onEvent(Event{
				EventType: "segment_gap",
				CameraID:  &cam.CameraID,
				GapStart:  &gapStart,
				NextStart: &nextStart,
			})
		}
		cam.segIndex = segIdx
		return
	}

	cam.segIndex = segIdx
	seg := cam.segments[segIdx]

	// Calculate the offset within the segment.
	offsetInSeg := targetTime.Sub(seg.Start)
	if offsetInSeg < 0 {
		offsetInSeg = 0
	}

	// Read samples from the segment for approximately 1 second of content.
	targetDTS := DurationGoToMP4(offsetInSeg, 90000) // assume 90kHz for video
	endDTS := targetDTS + 90000                       // 1 second of samples

	err := ReadSegmentSamples(seg.Fpath, func(sample Sample) error {
		if sample.DTS < targetDTS {
			return nil // skip samples before our target
		}
		if sample.DTS >= endDTS {
			return errDone // stop reading
		}
		cam.Muxer.WriteSample(sample)
		return nil
	})
	if err != nil {
		// Log would go here; for now just return.
		return
	}

	cam.Muxer.FlushFragment()
}

// readInitSegment reads the ftyp and moov boxes from an fMP4 segment file
// and returns their raw bytes concatenated.
func readInitSegment(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open segment for init: %w", err)
	}
	defer f.Close()

	return readInitSegmentFromReader(f)
}

// readInitSegmentFromReader reads ftyp and moov boxes from an io.ReadSeeker
// and returns their raw bytes.
func readInitSegmentFromReader(r io.ReadSeeker) ([]byte, error) {
	type boxRange struct {
		offset int64
		size   int64
	}

	var boxes []boxRange

	_, err := amp4.ReadBoxStructure(r, func(h *amp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case amp4.BoxTypeFtyp(), amp4.BoxTypeMoov():
			offset := int64(h.BoxInfo.Offset)
			size := int64(h.BoxInfo.Size)
			boxes = append(boxes, boxRange{offset: offset, size: size})

			// For moov, we need to read it but not expand.
			if h.BoxInfo.Type == amp4.BoxTypeMoov() {
				// We have both boxes we need; stop.
				return nil, errDone
			}
			return nil, nil

		case amp4.BoxTypeMoof():
			// We've reached the fragments; stop scanning.
			return nil, errDone

		default:
			return nil, nil
		}
	})
	if err != nil && !errors.Is(err, errDone) {
		return nil, fmt.Errorf("scan init boxes: %w", err)
	}

	if len(boxes) == 0 {
		return nil, fmt.Errorf("no ftyp or moov boxes found")
	}

	// Read the raw bytes for each box.
	var result []byte
	for _, b := range boxes {
		data := make([]byte, b.size)
		if _, err := r.Seek(b.offset, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek to box at offset %d: %w", b.offset, err)
		}
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("read box at offset %d: %w", b.offset, err)
		}
		result = append(result, data...)
	}

	return result, nil
}
