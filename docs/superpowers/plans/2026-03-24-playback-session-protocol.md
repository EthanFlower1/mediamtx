# Playback Session Protocol Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the stateless HTTP-per-seek playback model with a WebSocket-controlled session that splices fMP4 fragments into persistent chunked HTTP streams, enabling instant seeking, trick play, and synchronized multi-camera playback.

**Architecture:** A `SessionManager` in `internal/nvr/playback/` owns playback sessions. Each session has per-camera fMP4 muxers that read segments from disk via `recordstore` and write fragments into HTTP chunked responses via Go channels. A WebSocket handler dispatches commands (play, pause, seek, speed, step) to sessions. The Flutter client's `PlaybackController` switches from HTTP-per-seek to WebSocket commands — its public API is unchanged.

**Tech Stack:** Go (gorilla/websocket, abema/go-mp4, gin, recordstore), Flutter (web_socket_channel, media_kit)

**Spec:** `docs/superpowers/specs/2026-03-24-playback-session-protocol-design.md`

### Critical Architecture Notes

**`recordstore.FindSegments()` requires `*conf.Path`:** The first parameter is a full `*conf.Path` struct (from `internal/conf`), not a string. It reads `RecordPath` (pattern like `./recordings/%path/%Y-%m-%d_%H-%M-%S-%f`) and `RecordFormat` from it. The `SessionManager` must construct a `*conf.Path` with the correct `RecordPath` pattern. The NVR's `RecordingsPath` is just a directory — the full pattern must be passed from MediaMTX config via the `RouterConfig`.

**fMP4 sample data reading:** The `ReadSegmentSamples` function must actually read sample bytes from `mdat`, not just parse metadata. Use the pattern from `internal/playback/segment_fmp4.go:474` — `moofOffset + uint64(trun.DataOffset)` gives the mdat data position, then read `SampleSize` bytes per sample.

**Channel sends under lock:** `SpliceMuxer.WriteInit()` and `FlushFragment()` must NOT send on channels while holding the mutex — use a local buffer pattern (build bytes under lock, send after unlock) to avoid deadlocks.

**Stream endpoint auth:** Register `/api/nvr/playback/stream/:session/:camera` outside the JWT middleware group. Session ID (UUID v4) acts as bearer token. The WebSocket endpoint stays in the protected group.

**Audio during trick play:** At all non-1x speeds, reverse, and frame stepping — drop audio track samples, only emit video. This is standard NVR behavior.

---

## File Structure

### New Go files:

| File                                         | Responsibility                                                                                                                                                                                                    |
| -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/nvr/playback/fmp4_reader.go`       | Read fMP4 segment files: parse moov for track info, iterate moof+mdat for samples. Reimplements MediaMTX's unexported `segmentFMP4*` functions using `go-mp4`.                                                    |
| `internal/nvr/playback/fmp4_reader_test.go`  | Unit tests for fMP4 reader                                                                                                                                                                                        |
| `internal/nvr/playback/splice_muxer.go`      | fMP4 writer that maintains sequence number and DTS continuity across splices. Writes init segment once, then fragments. Supports seek (splice), speed change, keyframe decimation, frame step, reverse.           |
| `internal/nvr/playback/splice_muxer_test.go` | Unit tests for splice muxer                                                                                                                                                                                       |
| `internal/nvr/playback/session.go`           | `PlaybackSession` — owns per-camera muxers, handles commands, tracks position, manages playback goroutines                                                                                                        |
| `internal/nvr/playback/session_test.go`      | Unit tests for session state machine                                                                                                                                                                              |
| `internal/nvr/playback/manager.go`           | `SessionManager` — creates/resumes/disposes sessions, handles timeouts, resolves camera IDs to paths. Constructs `*conf.Path` for `recordstore.FindSegments()` using the RecordPath pattern from MediaMTX config. |
| `internal/nvr/playback/ws.go`                | WebSocket handler — command parsing, JSON envelope with seq/ack_seq, event broadcasting                                                                                                                           |
| `internal/nvr/playback/stream.go`            | HTTP handler for `/api/nvr/playback/stream/:session/:camera` — chunked response writer reading from a channel, keep-alive during pause                                                                            |
| `internal/nvr/playback/protocol.go`          | Shared types: Command, Event, SessionState enums, JSON message structs                                                                                                                                            |

### Modified files:

| File                                                            | Changes                                                        |
| --------------------------------------------------------------- | -------------------------------------------------------------- |
| `internal/nvr/api/router.go`                                    | Register WebSocket and stream endpoints                        |
| `internal/nvr/nvr.go`                                           | Create SessionManager with DB + recordingsPath, pass to router |
| `clients/flutter/lib/screens/playback/playback_controller.dart` | Rewrite internals to use WebSocket commands                    |
| `clients/flutter/lib/services/playback_service.dart`            | Add WebSocket URL construction                                 |

---

## Task 1: Protocol types and message structs

**Files:**

- Create: `internal/nvr/playback/protocol.go`
- Test: `internal/nvr/playback/protocol_test.go`

Define all shared types: commands, events, JSON messages.

- [ ] **Step 1: Write test for command/event JSON round-trip**

Create `internal/nvr/playback/protocol_test.go`:

```go
package playback

import (
	"encoding/json"
	"testing"
)

func TestCommandParsing(t *testing.T) {
	raw := `{"cmd":"seek","seq":5,"position":36000.5}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Cmd != "seek" {
		t.Fatalf("expected seek, got %s", cmd.Cmd)
	}
	if cmd.Seq != 5 {
		t.Fatalf("expected seq 5, got %d", cmd.Seq)
	}
	if cmd.Position == nil || *cmd.Position != 36000.5 {
		t.Fatalf("expected position 36000.5, got %v", cmd.Position)
	}
}

func TestEventSerialization(t *testing.T) {
	ev := Event{
		EventType: "state",
		AckSeq:    intPtr(5),
		Playing:   boolPtr(true),
		Speed:     float64Ptr(2.0),
		Position:  float64Ptr(36000.5),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["event"] != "state" {
		t.Fatalf("expected state event, got %v", parsed["event"])
	}
	if parsed["ack_seq"].(float64) != 5 {
		t.Fatalf("expected ack_seq 5")
	}
}

func intPtr(i int) *int          { return &i }
func boolPtr(b bool) *bool       { return &b }
func float64Ptr(f float64) *float64 { return &f }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/playback && go test -run TestCommand -v`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement protocol types**

Create `internal/nvr/playback/protocol.go`:

```go
package playback

// Command is a message from client to server.
type Command struct {
	Cmd       string   `json:"cmd"`
	Seq       int      `json:"seq"`
	// create
	CameraIDs []string `json:"camera_ids,omitempty"`
	Start     *string  `json:"start,omitempty"`
	// resume
	SessionID *string  `json:"session_id,omitempty"`
	// seek
	Position  *float64 `json:"position,omitempty"`
	// speed
	Rate      *float64 `json:"rate,omitempty"`
	// step
	Direction *int     `json:"direction,omitempty"`
	// add_camera / remove_camera
	CameraID  *string  `json:"camera_id,omitempty"`
}

// Event is a message from server to client.
type Event struct {
	EventType string             `json:"event"`
	AckSeq    *int               `json:"ack_seq,omitempty"`
	// created
	SessionID *string            `json:"session_id,omitempty"`
	Streams   map[string]string  `json:"streams,omitempty"`
	// position / state
	Position  *float64           `json:"position,omitempty"`
	Playing   *bool              `json:"playing,omitempty"`
	Speed     *float64           `json:"speed,omitempty"`
	Time      *string            `json:"time,omitempty"`
	// buffering
	CameraID  *string            `json:"camera_id,omitempty"`
	Buffering *bool              `json:"buffering,omitempty"`
	// segment_gap
	GapStart    *float64         `json:"gap_start,omitempty"`
	NextStart   *float64         `json:"next_start,omitempty"`
	// stream_restart / stream_added
	NewURL    *string            `json:"new_url,omitempty"`
	URL       *string            `json:"url,omitempty"`
	// error
	Message   *string            `json:"message,omitempty"`
}

// SessionState represents the playback state machine.
type SessionState int

const (
	StatePaused  SessionState = iota
	StatePlaying
	StateSeeking
	StateStepping
	StateDisposed
)
```

- [ ] **Step 4: Run tests**

Run: `cd internal/nvr/playback && go test -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/playback/protocol.go internal/nvr/playback/protocol_test.go
git commit -m "feat(playback): add protocol types for session commands and events"
```

---

## Task 2: fMP4 segment reader

**Files:**

- Create: `internal/nvr/playback/fmp4_reader.go`
- Create: `internal/nvr/playback/fmp4_reader_test.go`

Reimplements the fMP4 reading that MediaMTX does in its unexported `segmentFMP4*` functions. Uses `github.com/abema/go-mp4` directly.

- [ ] **Step 1: Write test for reading fMP4 header**

Create `internal/nvr/playback/fmp4_reader_test.go`. The test will need a real fMP4 test fixture. For now, test the public interface contract:

```go
package playback

import (
	"testing"
)

func TestTrackInfoEquality(t *testing.T) {
	a := TrackInfo{ID: 1, TimeScale: 90000, Codec: "avc1"}
	b := TrackInfo{ID: 1, TimeScale: 90000, Codec: "avc1"}
	c := TrackInfo{ID: 1, TimeScale: 48000, Codec: "mp4a"}

	if !a.Equal(b) {
		t.Fatal("identical tracks should be equal")
	}
	if a.Equal(c) {
		t.Fatal("different tracks should not be equal")
	}
}

func TestSampleIsKeyframe(t *testing.T) {
	s := Sample{
		DTS:      90000,
		PTSOffset: 0,
		Duration: 3000,
		IsSync:   true,
		Data:     []byte{0, 0, 0, 1},
		TrackID:  1,
	}
	if !s.IsSync {
		t.Fatal("expected keyframe")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/playback && go test -run TestTrackInfo -v`
Expected: FAIL

- [ ] **Step 3: Implement fMP4 reader**

Create `internal/nvr/playback/fmp4_reader.go`:

```go
package playback

import (
	"fmt"
	"io"
	"os"
	"time"

	amp4 "github.com/abema/go-mp4"
)

// TrackInfo describes a track's codec and timing.
type TrackInfo struct {
	ID        uint32
	TimeScale uint32
	Codec     string // "avc1", "hev1", "mp4a", "Opus", etc.
}

func (t TrackInfo) Equal(other TrackInfo) bool {
	return t.ID == other.ID && t.TimeScale == other.TimeScale && t.Codec == other.Codec
}

// Sample is a single decoded sample from an fMP4 segment.
type Sample struct {
	DTS       uint64
	PTSOffset int32
	Duration  uint32
	IsSync    bool
	Data      []byte
	TrackID   uint32
}

// SegmentHeader holds the init information from an fMP4 file.
type SegmentHeader struct {
	Tracks   []TrackInfo
	Duration time.Duration
}

// ReadSegmentHeader reads the ftyp+moov boxes from an fMP4 file,
// returning track info and total duration.
func ReadSegmentHeader(path string) (*SegmentHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var header SegmentHeader

	_, err = amp4.ReadBoxStructure(f, func(h *amp4.ReadHandle) (interface{}, error) {
		switch h.BoxInfo.Type {
		case amp4.BoxTypeMoov():
			return h.Expand()

		case amp4.BoxTypeTrak():
			return h.Expand()

		case amp4.BoxTypeMdia():
			return h.Expand()

		case amp4.BoxTypeMdhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd := box.(*amp4.Mdhd)
			// Store timeScale temporarily — will be matched with stsd codec
			if len(header.Tracks) > 0 {
				header.Tracks[len(header.Tracks)-1].TimeScale = mdhd.Timescale
			}
			return nil, nil

		case amp4.BoxTypeTkhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tkhd := box.(*amp4.Tkhd)
			header.Tracks = append(header.Tracks, TrackInfo{ID: tkhd.TrackID})
			return nil, nil

		case amp4.BoxTypeStbl():
			return h.Expand()

		case amp4.BoxTypeStsd():
			return h.Expand()

		case amp4.BoxTypeAvc1():
			if len(header.Tracks) > 0 {
				header.Tracks[len(header.Tracks)-1].Codec = "avc1"
			}
			return nil, nil

		case amp4.BoxTypeHev1():
			if len(header.Tracks) > 0 {
				header.Tracks[len(header.Tracks)-1].Codec = "hev1"
			}
			return nil, nil

		case amp4.BoxTypeMp4a():
			if len(header.Tracks) > 0 {
				header.Tracks[len(header.Tracks)-1].Codec = "mp4a"
			}
			return nil, nil

		case amp4.BoxTypeMinf():
			return h.Expand()
		}

		return nil, nil
	})

	return &header, err
}

// SampleCallback is called for each sample read from a segment.
// Return false to stop reading.
type SampleCallback func(sample Sample) bool

// ReadSegmentSamples reads all moof+mdat pairs from an fMP4 file
// and calls the callback for each sample.
func ReadSegmentSamples(path string, cb SampleCallback) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var currentTrackID uint32
	var baseDecodeTime uint64
	var defaultSampleDuration uint32
	var defaultSampleSize uint32
	var defaultSampleFlags uint32
	stopped := false

	_, err = amp4.ReadBoxStructure(f, func(h *amp4.ReadHandle) (interface{}, error) {
		if stopped {
			return nil, nil
		}

		switch h.BoxInfo.Type {
		case amp4.BoxTypeMoof():
			return h.Expand()

		case amp4.BoxTypeTraf():
			return h.Expand()

		case amp4.BoxTypeTfhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfhd := box.(*amp4.Tfhd)
			currentTrackID = tfhd.TrackID
			defaultSampleDuration = tfhd.DefaultSampleDuration
			defaultSampleSize = tfhd.DefaultSampleSize
			defaultSampleFlags = tfhd.DefaultSampleFlags
			return nil, nil

		case amp4.BoxTypeTfdt():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt := box.(*amp4.Tfdt)
			baseDecodeTime = tfdt.BaseMediaDecodeTimeV1
			return nil, nil

		case amp4.BoxTypeTrun():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*amp4.Trun)

			// Calculate data offset
			dataOffset := h.BoxInfo.Offset + h.BoxInfo.Size
			if trun.DataOffset != 0 {
				// DataOffset is relative to moof start
				// We need to find mdat position
			}

			dts := baseDecodeTime
			for i := range trun.Entries {
				entry := trun.Entries[i]

				dur := defaultSampleDuration
				if entry.SampleDuration != 0 {
					dur = entry.SampleDuration
				}

				size := defaultSampleSize
				if entry.SampleSize != 0 {
					size = entry.SampleSize
				}

				flags := defaultSampleFlags
				if i == 0 && trun.FirstSampleFlags != 0 {
					flags = trun.FirstSampleFlags
				} else if entry.SampleFlags != 0 {
					flags = entry.SampleFlags
				}

				isSync := (flags>>24)&0x1 == 0 && (flags>>16)&0x1 == 0

				sample := Sample{
					DTS:       dts,
					PTSOffset: int32(entry.SampleCompositionTimeOffsetV1),
					Duration:  dur,
					IsSync:    isSync,
					Data:      nil, // Data read separately from mdat
					TrackID:   currentTrackID,
				}

				_ = size // Size used for mdat reading

				if !cb(sample) {
					stopped = true
					return nil, nil
				}

				dts += uint64(dur)
			}
			return nil, nil
		}

		return nil, nil
	})

	if stopped {
		return nil
	}
	return err
}

// DurationMP4ToGo converts MP4 timescale duration to Go duration.
func DurationMP4ToGo(d uint64, timeScale uint32) time.Duration {
	if timeScale == 0 {
		return 0
	}
	return time.Duration(float64(d) / float64(timeScale) * float64(time.Second))
}

// DurationGoToMP4 converts Go duration to MP4 timescale units.
func DurationGoToMP4(d time.Duration, timeScale uint32) uint64 {
	return uint64(float64(d.Nanoseconds()) / float64(time.Second.Nanoseconds()) * float64(timeScale))
}

// TracksCompatible checks if two track lists can be spliced.
func TracksCompatible(a, b []TrackInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

// FindKeyframeBefore scans a segment file and returns the DTS of the last
// keyframe at or before the target time (relative to segment start).
func FindKeyframeBefore(path string, targetDTS uint64) (uint64, error) {
	var lastKeyDTS uint64
	err := ReadSegmentSamples(path, func(s Sample) bool {
		if s.IsSync && s.DTS <= targetDTS {
			lastKeyDTS = s.DTS
		}
		return s.DTS <= targetDTS
	})
	return lastKeyDTS, err
}
```

**Note:** The sample data reading from mdat is complex and depends on the trun data_offset + moof position. The implementer should reference `internal/playback/segment_fmp4.go:418-532` for the exact offset calculation pattern. The core pattern is:

1. Record moof box offset
2. Use trun.DataOffset (relative to moof start) to find mdat sample position
3. Read sample data of `SampleSize` bytes from that position

- [ ] **Step 4: Run tests**

Run: `cd internal/nvr/playback && go test -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/playback/fmp4_reader.go internal/nvr/playback/fmp4_reader_test.go
git commit -m "feat(playback): add fMP4 segment reader using go-mp4"
```

---

## Task 3: Splice muxer

**Files:**

- Create: `internal/nvr/playback/splice_muxer.go`
- Create: `internal/nvr/playback/splice_muxer_test.go`

The muxer writes fMP4 fragments into a byte channel. It maintains sequence number and DTS continuity across splices. This is the core technical piece.

- [ ] **Step 1: Write test for splice muxer**

Create `internal/nvr/playback/splice_muxer_test.go`:

```go
package playback

import (
	"testing"
)

func TestSpliceMuxerSequenceContinuity(t *testing.T) {
	m := NewSpliceMuxer([]TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
	})

	// Write 3 fragments
	for i := 0; i < 3; i++ {
		m.WriteSample(Sample{
			DTS:      uint64(i) * 90000,
			Duration: 90000,
			IsSync:   true,
			Data:     []byte{0},
			TrackID:  1,
		})
		m.FlushFragment()
	}

	if m.NextSequenceNumber() != 4 { // init=1, then 3 fragments
		t.Fatalf("expected seq 4, got %d", m.NextSequenceNumber())
	}

	// Splice to new position
	m.Splice()

	// Write more fragments — sequence should continue
	m.WriteSample(Sample{
		DTS:      500 * 90000, // totally different source time
		Duration: 90000,
		IsSync:   true,
		Data:     []byte{0},
		TrackID:  1,
	})
	m.FlushFragment()

	if m.NextSequenceNumber() != 5 {
		t.Fatalf("expected seq 5 after splice, got %d", m.NextSequenceNumber())
	}
}

func TestSpliceMuxerDTSContinuity(t *testing.T) {
	m := NewSpliceMuxer([]TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
	})

	// Write a fragment ending at DTS 270000 (3 seconds)
	m.WriteSample(Sample{
		DTS: 0, Duration: 90000, IsSync: true, Data: []byte{0}, TrackID: 1,
	})
	m.WriteSample(Sample{
		DTS: 90000, Duration: 90000, IsSync: false, Data: []byte{0}, TrackID: 1,
	})
	m.WriteSample(Sample{
		DTS: 180000, Duration: 90000, IsSync: false, Data: []byte{0}, TrackID: 1,
	})
	m.FlushFragment()

	lastDTS := m.LastOutputDTS()

	// Splice to a completely different source position
	m.Splice()

	// The next fragment's baseDecodeTime should continue from lastDTS
	m.WriteSample(Sample{
		DTS: 999 * 90000, // source DTS is irrelevant after splice
		Duration: 90000, IsSync: true, Data: []byte{0}, TrackID: 1,
	})
	m.FlushFragment()

	// Verify output DTS continues from where we left off
	if m.LastOutputDTS() <= lastDTS {
		t.Fatalf("output DTS should advance after splice: last=%d, now=%d", lastDTS, m.LastOutputDTS())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/playback && go test -run TestSpliceMuxer -v`
Expected: FAIL

- [ ] **Step 3: Implement splice muxer**

Create `internal/nvr/playback/splice_muxer.go`:

```go
package playback

import (
	"bytes"
	"encoding/binary"
	"sync"
)

// SpliceMuxer writes fMP4 init + fragments with splice support.
// After a Splice() call, the next fragment continues sequence numbers
// and DTS from where the previous fragment ended, regardless of the
// source sample's original DTS.
type SpliceMuxer struct {
	mu sync.Mutex

	tracks    []TrackInfo
	seqNum    uint32 // next moof sequence number
	lastDTS   uint64 // last output DTS (in track 1 timescale)
	dtsOffset int64  // added to source DTS to get output DTS
	spliced   bool   // true after Splice() until first fragment written

	// Current fragment being built
	samples []Sample

	// Output channel — consumer reads init + fragments
	Out chan []byte

	initWritten bool
}

// NewSpliceMuxer creates a muxer for the given tracks.
func NewSpliceMuxer(tracks []TrackInfo) *SpliceMuxer {
	return &SpliceMuxer{
		tracks: tracks,
		seqNum: 1,
		Out:    make(chan []byte, 16),
	}
}

// NextSequenceNumber returns the next moof sequence number.
func (m *SpliceMuxer) NextSequenceNumber() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seqNum
}

// LastOutputDTS returns the last DTS written to output.
func (m *SpliceMuxer) LastOutputDTS() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastDTS
}

// WriteInit writes the fMP4 init segment (ftyp + moov).
// Called once when the stream starts.
func (m *SpliceMuxer) WriteInit(initData []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initWritten {
		m.Out <- initData
		m.initWritten = true
	}
}

// WriteSample adds a sample to the current fragment.
func (m *SpliceMuxer) WriteSample(s Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = append(m.samples, s)
}

// FlushFragment marshals accumulated samples into a moof+mdat pair
// and sends it to the Out channel. Maintains sequence and DTS continuity.
func (m *SpliceMuxer) FlushFragment() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.samples) == 0 {
		return
	}

	// If we just spliced, recalculate DTS offset so output is continuous
	if m.spliced {
		firstSourceDTS := m.samples[0].DTS
		m.dtsOffset = int64(m.lastDTS) - int64(firstSourceDTS) + int64(m.samples[0].Duration)
		m.spliced = false
	}

	// Build moof+mdat
	// Group samples by track
	trackSamples := make(map[uint32][]Sample)
	for _, s := range m.samples {
		trackSamples[s.TrackID] = append(trackSamples[s.TrackID], s)
	}

	var buf bytes.Buffer
	moofData := m.buildMoof(trackSamples)
	mdatData := m.buildMdat(m.samples)

	buf.Write(moofData)
	buf.Write(mdatData)

	// Update state
	lastSample := m.samples[len(m.samples)-1]
	outputDTS := uint64(int64(lastSample.DTS) + m.dtsOffset)
	m.lastDTS = outputDTS + uint64(lastSample.Duration)
	m.seqNum++
	m.samples = m.samples[:0]

	m.Out <- buf.Bytes()
}

// Splice prepares for a position jump. The next FlushFragment will
// adjust DTS offset so output timestamps are continuous.
func (m *SpliceMuxer) Splice() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.spliced = true
	m.samples = m.samples[:0] // discard any buffered samples
}

// buildMoof constructs a minimal moof box.
// This is a simplified implementation — the real version must write
// proper traf/tfhd/tfdt/trun boxes per ISO 14496-12.
func (m *SpliceMuxer) buildMoof(trackSamples map[uint32][]Sample) []byte {
	var buf bytes.Buffer

	// For each track, build traf with tfhd + tfdt + trun
	var trafs bytes.Buffer
	for trackID, samples := range trackSamples {
		if len(samples) == 0 {
			continue
		}
		traf := m.buildTraf(trackID, samples)
		trafs.Write(traf)
	}

	// mfhd box (movie fragment header)
	mfhd := make([]byte, 16)
	binary.BigEndian.PutUint32(mfhd[0:4], 16)                 // box size
	copy(mfhd[4:8], []byte("mfhd"))                           // box type
	binary.BigEndian.PutUint32(mfhd[8:12], 0)                 // version + flags
	binary.BigEndian.PutUint32(mfhd[12:16], m.seqNum)         // sequence number

	// moof box wrapping mfhd + trafs
	moofSize := uint32(8 + len(mfhd) + trafs.Len())
	moofHeader := make([]byte, 8)
	binary.BigEndian.PutUint32(moofHeader[0:4], moofSize)
	copy(moofHeader[4:8], []byte("moof"))

	buf.Write(moofHeader)
	buf.Write(mfhd)
	buf.Write(trafs.Bytes())

	return buf.Bytes()
}

// buildTraf constructs a traf box for one track.
func (m *SpliceMuxer) buildTraf(trackID uint32, samples []Sample) []byte {
	// This is a skeleton — the real implementation must write:
	// 1. tfhd with trackID and default flags
	// 2. tfdt with baseMediaDecodeTime (adjusted by dtsOffset)
	// 3. trun with per-sample duration, size, flags, composition offset
	//
	// The implementer should reference:
	// - internal/playback/muxer_fmp4.go:133-200 for the marshal pattern
	// - github.com/abema/go-mp4 WriteBoxStructure API
	//
	// Key requirement: baseDecodeTime = samples[0].DTS + m.dtsOffset
	// This ensures DTS continuity across splices.

	var buf bytes.Buffer
	// Placeholder — real implementation uses go-mp4 WriteBoxStructure
	_ = trackID
	_ = samples
	return buf.Bytes()
}

// buildMdat constructs an mdat box containing all sample data.
func (m *SpliceMuxer) buildMdat(samples []Sample) []byte {
	var dataSize int
	for _, s := range samples {
		dataSize += len(s.Data)
	}

	buf := make([]byte, 8+dataSize)
	binary.BigEndian.PutUint32(buf[0:4], uint32(8+dataSize))
	copy(buf[4:8], []byte("mdat"))

	offset := 8
	for _, s := range samples {
		copy(buf[offset:], s.Data)
		offset += len(s.Data)
	}

	return buf
}
```

**Implementation note:** The `buildMoof`/`buildTraf` methods above are simplified skeletons. The real implementation must use `amp4.WriteBoxStructure` to properly marshal tfhd, tfdt (version 1 for 64-bit baseMediaDecodeTime), and trun boxes with all required flags. The implementer should closely reference `internal/playback/muxer_fmp4.go:133-200` for the correct box structure and `github.com/abema/go-mp4` API patterns.

- [ ] **Step 4: Run tests**

Run: `cd internal/nvr/playback && go test -run TestSpliceMuxer -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/playback/splice_muxer.go internal/nvr/playback/splice_muxer_test.go
git commit -m "feat(playback): add splice muxer with DTS/sequence continuity"
```

---

## Task 4: Session state machine

**Files:**

- Create: `internal/nvr/playback/session.go`
- Create: `internal/nvr/playback/session_test.go`

The session owns per-camera muxers and manages playback state.

- [ ] **Step 1: Write test for session state transitions**

Create `internal/nvr/playback/session_test.go`:

```go
package playback

import (
	"testing"
)

func TestSessionStateTransitions(t *testing.T) {
	// Test that state transitions are valid
	s := &PlaybackSession{state: StatePaused}

	if s.state != StatePaused {
		t.Fatal("should start paused")
	}

	s.state = StatePlaying
	if s.state != StatePlaying {
		t.Fatal("should be playing")
	}

	s.state = StateSeeking
	if s.state != StateSeeking {
		t.Fatal("should be seeking")
	}

	s.state = StatePaused
	if s.state != StatePaused {
		t.Fatal("should be paused after seek")
	}
}

func TestSessionPositionSeconds(t *testing.T) {
	s := &PlaybackSession{
		positionSecs: 36000.5, // 10:00:00.5
	}
	if s.positionSecs != 36000.5 {
		t.Fatalf("expected 36000.5, got %f", s.positionSecs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/playback && go test -run TestSession -v`

- [ ] **Step 3: Implement session**

Create `internal/nvr/playback/session.go`:

```go
package playback

import (
	"context"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/recordstore"
)

// CameraMuxer holds a camera's muxer and output channel.
type CameraMuxer struct {
	CameraID string
	Path     string // MediaMTX path
	Muxer    *SpliceMuxer
}

// PlaybackSession manages playback for one or more cameras.
type PlaybackSession struct {
	mu sync.Mutex

	id           string
	state        SessionState
	speed        float64
	positionSecs float64 // seconds since midnight on selected date
	dateStart    time.Time // midnight of selected date

	cameras      map[string]*CameraMuxer
	recordPath   string // recordstore path pattern

	// Playback goroutine control
	ctx    context.Context
	cancel context.CancelFunc

	// Event callback — session sends events to the WS handler
	onEvent func(Event)

	// Timestamps for cleanup
	lastActivity time.Time
	createdAt    time.Time
}

// NewPlaybackSession creates a session in paused state.
func NewPlaybackSession(
	id string,
	dateStart time.Time,
	startPositionSecs float64,
	recordPath string,
	onEvent func(Event),
) *PlaybackSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &PlaybackSession{
		id:           id,
		state:        StatePaused,
		speed:        1.0,
		positionSecs: startPositionSecs,
		dateStart:    dateStart,
		cameras:      make(map[string]*CameraMuxer),
		recordPath:   recordPath,
		ctx:          ctx,
		cancel:       cancel,
		onEvent:      onEvent,
		lastActivity: time.Now(),
		createdAt:    time.Now(),
	}
}

// ID returns the session ID.
func (s *PlaybackSession) ID() string { return s.id }

// AddCamera adds a camera to the session.
func (s *PlaybackSession) AddCamera(cameraID, mediamtxPath, recordPathPattern string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Construct a *conf.Path for recordstore.FindSegments().
	// Only RecordPath and RecordFormat are needed.
	pathConf := &conf.Path{
		RecordPath:   recordPathPattern,
		RecordFormat: conf.RecordFormatFMP4,
	}

	segments, err := recordstore.FindSegments(
		pathConf, mediamtxPath, nil, nil,
	)
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		return ErrNoSegments
	}

	header, err := ReadSegmentHeader(segments[0].Fpath)
	if err != nil {
		return err
	}

	muxer := NewSpliceMuxer(header.Tracks)

	s.cameras[cameraID] = &CameraMuxer{
		CameraID: cameraID,
		Path:     mediamtxPath,
		Muxer:    muxer,
	}

	return nil
}

// RemoveCamera removes a camera from the session.
func (s *PlaybackSession) RemoveCamera(cameraID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cameras, cameraID)
}

// StreamChannel returns the muxer output channel for a camera.
func (s *PlaybackSession) StreamChannel(cameraID string) <-chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	cm, ok := s.cameras[cameraID]
	if !ok {
		return nil
	}
	return cm.Muxer.Out
}

// Play starts writing fragments.
func (s *PlaybackSession) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StatePlaying {
		return
	}
	s.state = StatePlaying
	s.lastActivity = time.Now()

	// Start playback goroutine for each camera
	go s.playbackLoop()
}

// Pause stops writing fragments.
func (s *PlaybackSession) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StatePaused
	s.lastActivity = time.Now()
}

// Seek splices all camera muxers to a new position.
func (s *PlaybackSession) Seek(positionSecs float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.positionSecs = positionSecs
	s.state = StateSeeking
	s.lastActivity = time.Now()

	for _, cm := range s.cameras {
		cm.Muxer.Splice()
	}

	// The playback loop will pick up the new position
	s.state = StatePaused
	s.onEvent(Event{
		EventType: "state",
		Playing:   boolPtr(false),
		Position:  &positionSecs,
		Speed:     &s.speed,
	})
}

// SetSpeed changes playback rate.
func (s *PlaybackSession) SetSpeed(rate float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.speed = rate
	s.lastActivity = time.Now()
}

// Step writes a single frame and stays paused.
func (s *PlaybackSession) Step(direction int) {
	s.mu.Lock()
	s.lastActivity = time.Now()
	s.mu.Unlock()
	// Implementation: read next/previous keyframe, write to muxer, update position
	// Detailed in spec section 4
}

// Dispose tears down the session.
func (s *PlaybackSession) Dispose() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateDisposed
	s.cancel()
	for _, cm := range s.cameras {
		close(cm.Muxer.Out)
	}
}

// playbackLoop reads segments and writes fragments at the appropriate rate.
func (s *PlaybackSession) playbackLoop() {
	ticker := time.NewTicker(time.Second) // ~1 fragment per second at 1x
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.state != StatePlaying {
				s.mu.Unlock()
				continue
			}

			// Read next fragment from disk for each camera,
			// write to muxer, advance position
			s.readAndWriteNextFragment()

			// Adjust tick rate for speed
			interval := time.Duration(float64(time.Second) / s.speed)
			ticker.Reset(interval)

			// Send position event
			pos := s.positionSecs
			s.mu.Unlock()

			s.onEvent(Event{
				EventType: "position",
				Position:  &pos,
			})
		}
	}
}

// readAndWriteNextFragment reads the next ~1s of samples from each camera's
// segments and writes them to the muxer. Must be called with mu held.
func (s *PlaybackSession) readAndWriteNextFragment() {
	targetTime := s.dateStart.Add(time.Duration(s.positionSecs * float64(time.Second)))

	for _, cm := range s.cameras {
		// Find segment containing targetTime
		pathConf := &conf.Path{
			RecordPath:   s.recordPath,
			RecordFormat: conf.RecordFormatFMP4,
		}
		segments, err := recordstore.FindSegments(
			pathConf, cm.Path,
			&targetTime, nil,
		)
		if err != nil || len(segments) == 0 {
			continue
		}

		// Read samples from segment, write to muxer
		// Advance position by fragment duration
		ReadSegmentSamples(segments[0].Fpath, func(sample Sample) bool {
			cm.Muxer.WriteSample(sample)
			return false // one fragment worth
		})
		cm.Muxer.FlushFragment()
	}

	// Advance position by ~1 second (adjusted for speed)
	s.positionSecs += 1.0
}

var ErrNoSegments = &PlaybackError{Message: "no segments found"}

type PlaybackError struct {
	Message string
}

func (e *PlaybackError) Error() string { return e.Message }
```

**Implementation note:** The `playbackLoop` and `readAndWriteNextFragment` methods are simplified. The real implementation needs to:

1. Track which segment file is currently being read (avoid re-scanning recordstore every second)
2. Keep a file reader open across fragments for the same segment
3. Handle segment boundaries (transition to next segment file)
4. Handle recording gaps (emit `segment_gap` event, skip to next segment)
5. Handle end of recordings (emit `end` event, pause)
6. For speeds > 4x, skip non-keyframe samples
7. For reverse, read GOPs backward

- [ ] **Step 4: Run tests**

Run: `cd internal/nvr/playback && go test -run TestSession -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/playback/session.go internal/nvr/playback/session_test.go
git commit -m "feat(playback): add PlaybackSession with state machine and camera muxers"
```

---

## Task 5: Session manager

**Files:**

- Create: `internal/nvr/playback/manager.go`

- [ ] **Step 1: Implement session manager**

Create `internal/nvr/playback/manager.go`:

```go
package playback

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SessionManager creates, tracks, and cleans up playback sessions.
type SessionManager struct {
	mu sync.Mutex

	db         *db.DB
	recordPath string // Full RecordPath pattern (e.g., "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f")

	sessions map[string]*PlaybackSession

	// Cleanup
	gracePeriod time.Duration
	idleTimeout time.Duration
}

// NewSessionManager creates a manager.
// recordPath is the full MediaMTX RecordPath pattern (NOT just the directory).
func NewSessionManager(database *db.DB, recordPath string) *SessionManager {
	m := &SessionManager{
		db:          database,
		recordPath:  recordPath,
		sessions:    make(map[string]*PlaybackSession),
		gracePeriod: 30 * time.Second,
		idleTimeout: 10 * time.Minute,
	}

	// Start cleanup goroutine
	go m.cleanupLoop()

	return m
}

// CreateSession creates a new playback session.
func (m *SessionManager) CreateSession(
	cameraIDs []string,
	startTime time.Time,
	startPositionSecs float64,
	onEvent func(Event),
) (*PlaybackSession, error) {
	sessionID := uuid.New().String()

	dayStart := time.Date(
		startTime.Year(), startTime.Month(), startTime.Day(),
		0, 0, 0, 0, startTime.Location(),
	)

	session := NewPlaybackSession(
		sessionID, dayStart, startPositionSecs, m.recordPath, onEvent,
	)

	// Resolve camera IDs to MediaMTX paths and add to session.
	// Construct a *conf.Path for recordstore.FindSegments().
	for _, camID := range cameraIDs {
		cam, err := m.db.GetCamera(camID)
		if err != nil {
			session.Dispose()
			return nil, fmt.Errorf("camera %s not found: %w", camID, err)
		}
		if err := session.AddCamera(camID, cam.MediaMTXPath, m.recordPath); err != nil {
			// Non-fatal — camera may have no recordings yet
			continue
		}
	}

	m.mu.Lock()
	m.sessions[sessionID] = session
	m.mu.Unlock()

	return session, nil
}

// GetSession returns a session by ID.
func (m *SessionManager) GetSession(id string) *PlaybackSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// DisposeSession removes and disposes a session.
func (m *SessionManager) DisposeSession(id string) {
	m.mu.Lock()
	session, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if session != nil {
		session.Dispose()
	}
}

// cleanupLoop periodically removes expired sessions.
func (m *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		var toRemove []string
		for id, s := range m.sessions {
			s.mu.Lock()
			idle := now.Sub(s.lastActivity) > m.idleTimeout
			s.mu.Unlock()
			if idle {
				toRemove = append(toRemove, id)
			}
		}
		m.mu.Unlock()

		for _, id := range toRemove {
			m.DisposeSession(id)
		}
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/nvr/playback/manager.go
git commit -m "feat(playback): add SessionManager with UUID sessions and cleanup"
```

---

## Task 6: WebSocket handler

**Files:**

- Create: `internal/nvr/playback/ws.go`

- [ ] **Step 1: Implement WebSocket handler**

Create `internal/nvr/playback/ws.go`:

```go
package playback

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWebSocket upgrades to WebSocket and processes commands.
func HandleWebSocket(manager *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var session *PlaybackSession
		var sessionID string

		// Event sender — writes events to the WebSocket
		eventCh := make(chan Event, 32)
		onEvent := func(ev Event) {
			select {
			case eventCh <- ev:
			default: // drop if channel full
			}
		}

		// Writer goroutine
		done := make(chan struct{})
		go func() {
			defer close(done)
			for ev := range eventCh {
				data, _ := json.Marshal(ev)
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return
				}
			}
		}()

		// Reader loop — process commands
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var cmd Command
			if err := json.Unmarshal(message, &cmd); err != nil {
				onEvent(Event{EventType: "error", Message: strPtr("invalid JSON")})
				continue
			}

			switch cmd.Cmd {
			case "create":
				if cmd.Start == nil || len(cmd.CameraIDs) == 0 {
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: strPtr("missing camera_ids or start")})
					continue
				}
				startTime, err := time.Parse(time.RFC3339, *cmd.Start)
				if err != nil {
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: strPtr("invalid start time")})
					continue
				}

				session, err = manager.CreateSession(cmd.CameraIDs, startTime, 0, onEvent)
				if err != nil {
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: strPtr(err.Error())})
					continue
				}
				sessionID = session.ID()

				// Build stream URLs
				streams := make(map[string]string)
				for _, camID := range cmd.CameraIDs {
					streams[camID] = "/api/nvr/playback/stream/" + sessionID + "/" + camID
				}

				onEvent(Event{
					EventType: "created",
					AckSeq:    &cmd.Seq,
					SessionID: &sessionID,
					Streams:   streams,
				})

			case "resume":
				if cmd.SessionID == nil {
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: strPtr("missing session_id")})
					continue
				}
				session = manager.GetSession(*cmd.SessionID)
				if session == nil {
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: strPtr("session not found")})
					continue
				}
				sessionID = session.ID()
				session.onEvent = onEvent // rebind event callback to new WS

				pos := session.positionSecs
				playing := session.state == StatePlaying
				speed := session.speed
				onEvent(Event{
					EventType: "state",
					AckSeq:    &cmd.Seq,
					Playing:   &playing,
					Speed:     &speed,
					Position:  &pos,
				})

			case "play":
				if session == nil {
					continue
				}
				session.Play()
				playing := true
				pos := session.positionSecs
				onEvent(Event{EventType: "state", AckSeq: &cmd.Seq, Playing: &playing, Position: &pos})

			case "pause":
				if session == nil {
					continue
				}
				session.Pause()
				playing := false
				pos := session.positionSecs
				onEvent(Event{EventType: "state", AckSeq: &cmd.Seq, Playing: &playing, Position: &pos})

			case "seek":
				if session == nil || cmd.Position == nil {
					continue
				}
				session.Seek(*cmd.Position)

			case "speed":
				if session == nil || cmd.Rate == nil {
					continue
				}
				session.SetSpeed(*cmd.Rate)
				onEvent(Event{EventType: "state", AckSeq: &cmd.Seq, Speed: cmd.Rate})

			case "step":
				if session == nil || cmd.Direction == nil {
					continue
				}
				session.Step(*cmd.Direction)

			case "add_camera":
				if session == nil || cmd.CameraID == nil {
					continue
				}
				cam, err := manager.db.GetCamera(*cmd.CameraID)
				if err != nil {
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: strPtr("camera not found")})
					continue
				}
				session.AddCamera(*cmd.CameraID, cam.MediaMTXPath)
				url := "/api/nvr/playback/stream/" + sessionID + "/" + *cmd.CameraID
				onEvent(Event{EventType: "stream_added", AckSeq: &cmd.Seq, CameraID: cmd.CameraID, URL: &url})

			case "remove_camera":
				if session == nil || cmd.CameraID == nil {
					continue
				}
				session.RemoveCamera(*cmd.CameraID)
				onEvent(Event{EventType: "stream_removed", AckSeq: &cmd.Seq, CameraID: cmd.CameraID})

			case "close":
				if session != nil {
					manager.DisposeSession(sessionID)
					session = nil
				}
				onEvent(Event{EventType: "state", AckSeq: &cmd.Seq})
			}
		}

		// WebSocket disconnected — start grace period
		close(eventCh)
		<-done

		if session != nil {
			session.Pause()
			// Grace period — session stays alive for reconnect
			go func() {
				time.Sleep(manager.gracePeriod)
				s := manager.GetSession(sessionID)
				if s != nil {
					s.mu.Lock()
					idle := time.Since(s.lastActivity) > manager.gracePeriod
					s.mu.Unlock()
					if idle {
						manager.DisposeSession(sessionID)
					}
				}
			}()
		}
	}
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 2: Commit**

```bash
git add internal/nvr/playback/ws.go
git commit -m "feat(playback): add WebSocket handler for session commands"
```

---

## Task 7: HTTP stream handler

**Files:**

- Create: `internal/nvr/playback/stream.go`

- [ ] **Step 1: Implement HTTP stream handler**

Create `internal/nvr/playback/stream.go`:

```go
package playback

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HandleStream serves a persistent fMP4 chunked stream for a camera.
func HandleStream(manager *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID := c.Param("session")
		cameraID := c.Param("camera")

		session := manager.GetSession(sessionID)
		if session == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		ch := session.StreamChannel(cameraID)
		if ch == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not in session"})
			return
		}

		// Set headers for chunked fMP4 stream
		c.Header("Content-Type", "video/mp4")
		c.Header("Transfer-Encoding", "chunked")
		c.Header("Cache-Control", "no-cache, no-store")
		c.Header("Connection", "keep-alive")
		c.Header("Accept-Ranges", "none")
		c.Status(http.StatusOK)

		// Flush headers
		c.Writer.Flush()

		keepAlive := time.NewTicker(10 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case data, ok := <-ch:
				if !ok {
					// Channel closed — session disposed
					return
				}
				if _, err := c.Writer.Write(data); err != nil {
					return // client disconnected
				}
				c.Writer.Flush()

			case <-keepAlive.C:
				// Send empty flush to prevent HTTP timeout during pause
				c.Writer.Flush()

			case <-c.Request.Context().Done():
				return // client disconnected
			}
		}
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/nvr/playback/stream.go
git commit -m "feat(playback): add HTTP chunked stream handler with keep-alive"
```

---

## Task 8: Register routes and wire to NVR

**Files:**

- Modify: `internal/nvr/api/router.go`
- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Add SessionManager to NVR initialization**

In `internal/nvr/nvr.go`, add a `playbackManager` field and a `RecordPathPattern` config field:

```go
// Add to NVR struct (after existing fields):
playbackManager   *playback.SessionManager
RecordPathPattern string // e.g. "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"

// Add to Initialize() after database initialization:
// RecordPathPattern comes from MediaMTX config (paths -> recordPath).
// Default: "./recordings/%path/%Y-%m-%d_%H-%M-%S-%f"
if n.RecordPathPattern == "" {
    n.RecordPathPattern = n.RecordingsPath + "/%path/%Y-%m-%d_%H-%M-%S-%f"
}
n.playbackManager = playback.NewSessionManager(n.database, n.RecordPathPattern)
```

Note: The `RecordPathPattern` should ideally be read from mediamtx.yml's path config. If it's not directly available to the NVR subsystem, use the default pattern which matches the default MediaMTX config.

- [ ] **Step 2: Pass manager to router config**

In `internal/nvr/nvr.go` `RegisterRoutes()`, add `PlaybackManager` to the `RouterConfig`:

```go
// Add field to RouterConfig in router.go:
PlaybackManager *playback.SessionManager

// Pass in nvr.go RegisterRoutes():
PlaybackManager: n.playbackManager,
```

- [ ] **Step 3: Register playback endpoints in router**

In `internal/nvr/api/router.go`:

```go
// Stream endpoint — NO JWT auth, session ID is the bearer token.
// Register OUTSIDE the protected group, on the base nvr group.
if cfg.PlaybackManager != nil {
    nvr.GET("/playback/stream/:session/:camera", playback.HandleStream(cfg.PlaybackManager))
}

// Inside the protected (JWT auth) group:
if cfg.PlaybackManager != nil {
    api.GET("/playback/ws", playback.HandleWebSocket(cfg.PlaybackManager))
}
```

The WebSocket endpoint requires JWT auth for the initial connection. The stream endpoint authenticates by validating the session ID (UUID v4, unguessable) — `media_kit` cannot send JWT headers on bare HTTP GETs.

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/nvr.go internal/nvr/api/router.go
git commit -m "feat(playback): wire SessionManager to NVR and register routes"
```

---

## Task 9: Flutter PlaybackController rewrite

**Files:**

- Modify: `clients/flutter/lib/screens/playback/playback_controller.dart`
- Modify: `clients/flutter/lib/services/playback_service.dart`

- [ ] **Step 1: Add WebSocket URL to PlaybackService**

In `clients/flutter/lib/services/playback_service.dart`, add:

```dart
String playbackWsUrl() {
  final uri = Uri.parse(serverUrl);
  return 'ws://${uri.host}:${uri.port}/api/nvr/playback/ws';
}

String streamBaseUrl() {
  final uri = Uri.parse(serverUrl);
  return '${uri.scheme}://${uri.host}:${uri.port}';
}
```

- [ ] **Step 2: Rewrite PlaybackController**

Replace the internals of `clients/flutter/lib/screens/playback/playback_controller.dart`. The public API stays the same — only the implementation changes from HTTP-per-seek to WebSocket commands.

Key changes:

- Add `WebSocketChannel` connection
- `seek()` sends `{"cmd": "seek", "position": X}` instead of opening new HTTP streams
- `play()`/`pause()` send WebSocket commands
- Position comes from server `position` events, not player stream
- Players opened once from `created` event stream URLs
- `_streamOrigin` and stale position rejection removed — server is authoritative

```dart
// New imports needed:
import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';

// Key new fields:
WebSocketChannel? _ws;
int _seq = 0;
String? _sessionId;

// seek() becomes:
Future<void> seek(Duration target) async {
  final secs = target.inMilliseconds / 1000.0;
  _position = target;
  _sendCommand({'cmd': 'seek', 'position': secs});
  notifyListeners();
}

// play() becomes:
void play() {
  _isPlaying = true;
  _sendCommand({'cmd': 'play'});
  notifyListeners();
}

// Position tracking from WS events:
void _handleEvent(Map<String, dynamic> event) {
  switch (event['event']) {
    case 'position':
      final secs = (event['position'] as num).toDouble();
      _position = Duration(milliseconds: (secs * 1000).round());
      notifyListeners();
    case 'state':
      if (event['playing'] != null) _isPlaying = event['playing'];
      if (event['speed'] != null) _speed = event['speed'];
      if (event['position'] != null) {
        _position = Duration(milliseconds: ((event['position'] as num).toDouble() * 1000).round());
      }
      notifyListeners();
    case 'created':
      _sessionId = event['session_id'];
      _openStreams(event['streams'] as Map<String, dynamic>);
    // ... handle other events
  }
}
```

The full rewrite should preserve all existing public methods and their signatures. The `static` helper methods (findContainingSegment, etc.) remain unchanged — they're still used by the timeline for gap/event visualization.

- [ ] **Step 3: Run tests**

Run: `cd clients/flutter && flutter test`
Expected: All 22 tests pass (static helpers unchanged)

- [ ] **Step 4: Run analysis**

Run: `cd clients/flutter && flutter analyze lib/screens/playback/`
Expected: No issues

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_controller.dart clients/flutter/lib/services/playback_service.dart
git commit -m "feat(flutter): rewrite PlaybackController to use WebSocket session protocol"
```

---

## Task 10: Integration smoke test

**Files:**

- Various — fix any issues found

- [ ] **Step 1: Build Go backend**

Run: `go build ./...`
Fix any compilation errors.

- [ ] **Step 2: Run Go tests**

Run: `go test ./internal/nvr/playback/...`
All tests should pass.

- [ ] **Step 3: Run Flutter tests**

Run: `cd clients/flutter && flutter test`
All tests should pass.

- [ ] **Step 4: Run Flutter analysis**

Run: `cd clients/flutter && flutter analyze`
No issues.

- [ ] **Step 5: Fix any issues and commit**

```bash
git add -A
git commit -m "fix: resolve integration issues for playback session protocol"
```
