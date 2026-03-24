package playback

import (
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
)

// audioCodecs lists codec strings that identify audio tracks.
var audioCodecs = map[string]bool{
	"mp4a": true,
	"Opus": true,
	"ac-3": true,
	"ec-3": true,
}

// SpliceMuxer writes fMP4 init segments and fragments to an output channel.
// After a Splice() call, the next fragment continues moof sequence numbers
// and DTS from where the previous fragment ended, enabling seamless seeking
// within a persistent HTTP chunked stream.
type SpliceMuxer struct {
	mu        sync.Mutex
	tracks    []TrackInfo
	seqNum    uint32   // next moof sequence number (1-based)
	lastDTS   uint64   // last output DTS (in first video track's timescale)
	dtsOffset int64    // added to source DTS to get output DTS
	spliced   bool     // true after Splice() until next flush recalculates offset
	dropAudio bool     // filter out audio samples
	samples   []Sample // current fragment being built
	Out       chan []byte
}

// NewSpliceMuxer creates a new SpliceMuxer for the given tracks.
func NewSpliceMuxer(tracks []TrackInfo) *SpliceMuxer {
	return &SpliceMuxer{
		tracks: tracks,
		seqNum: 1,
		Out:    make(chan []byte, 16),
	}
}

// NextSequenceNumber returns the next moof sequence number that will be used.
func (m *SpliceMuxer) NextSequenceNumber() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seqNum
}

// LastOutputDTS returns the last output DTS value written.
func (m *SpliceMuxer) LastOutputDTS() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastDTS
}

// SetDropAudio controls whether audio samples are filtered out.
func (m *SpliceMuxer) SetDropAudio(drop bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropAudio = drop
}

// WriteInit sends an init segment (ftyp+moov bytes) to the output channel.
func (m *SpliceMuxer) WriteInit(initData []byte) {
	// No lock needed for the channel send itself; initData is already built.
	m.Out <- initData
}

// WriteSample adds a sample to the current fragment buffer.
// If dropAudio is true and the sample's track is an audio codec, it is skipped.
func (m *SpliceMuxer) WriteSample(s Sample) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.dropAudio && m.isAudioTrack(s.TrackID) {
		return
	}

	m.samples = append(m.samples, s)
}

// FlushFragment marshals all buffered samples into an fMP4 moof+mdat fragment
// and sends it to the Out channel. If spliced is true, the DTS offset is
// recalculated so output DTS continues from where the last fragment ended.
func (m *SpliceMuxer) FlushFragment() {
	m.mu.Lock()

	if len(m.samples) == 0 {
		m.mu.Unlock()
		return
	}

	// If we just spliced, recalculate the DTS offset based on the first
	// sample in this new batch so that output DTS continues seamlessly.
	if m.spliced {
		// Find the first sample's source DTS to compute the new offset.
		firstSrcDTS := m.samples[0].DTS
		// offset = lastDTS - firstSrcDTS  (so output = src + offset = lastDTS)
		m.dtsOffset = int64(m.lastDTS) - int64(firstSrcDTS)
		m.spliced = false
	}

	// Group samples by track and compute output DTS values.
	type trackAccum struct {
		id       int
		baseTime uint64
		samples  []*fmp4.Sample
		lastDTS  uint64
	}
	trackMap := make(map[uint32]*trackAccum)

	for i := range m.samples {
		s := &m.samples[i]
		outputDTS := uint64(int64(s.DTS) + m.dtsOffset)

		ta, ok := trackMap[s.TrackID]
		if !ok {
			ta = &trackAccum{
				id:       int(s.TrackID),
				baseTime: outputDTS,
				lastDTS:  outputDTS,
			}
			trackMap[s.TrackID] = ta
		}

		// Set the duration of the previous sample in this track.
		if len(ta.samples) > 0 {
			prev := ta.samples[len(ta.samples)-1]
			if outputDTS > ta.lastDTS {
				prev.Duration = uint32(outputDTS - ta.lastDTS)
			}
		}

		ta.samples = append(ta.samples, &fmp4.Sample{
			Duration:        s.Duration, // default; overridden by next sample's DTS delta
			PTSOffset:       s.PTSOffset,
			IsNonSyncSample: !s.IsSync,
			Payload:         s.Data,
		})
		ta.lastDTS = outputDTS

		// Track the maximum output DTS + duration for lastDTS bookkeeping.
		endDTS := outputDTS + uint64(s.Duration)
		if endDTS > m.lastDTS {
			m.lastDTS = endDTS
		}
	}

	// Build the fmp4.Part.
	part := fmp4.Part{
		SequenceNumber: m.seqNum,
	}
	m.seqNum++

	// Add tracks in a deterministic order (by track info order).
	for _, ti := range m.tracks {
		ta, ok := trackMap[ti.ID]
		if !ok {
			continue
		}
		part.Tracks = append(part.Tracks, &fmp4.PartTrack{
			ID:       ta.id,
			BaseTime: ta.baseTime,
			Samples:  ta.samples,
		})
	}

	// Clear the sample buffer.
	m.samples = m.samples[:0]

	// Marshal the fragment bytes under the lock, then release before sending.
	var buf seekablebuffer.Buffer
	err := part.Marshal(&buf)
	m.mu.Unlock()

	if err != nil {
		// In production this would be logged; for now silently skip.
		return
	}

	m.Out <- buf.Bytes()
}

// Splice prepares for a position jump. It discards any buffered samples
// and marks the muxer so that the next FlushFragment recalculates the DTS
// offset for seamless continuity. Sequence numbers are NOT reset.
func (m *SpliceMuxer) Splice() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samples = m.samples[:0]
	m.spliced = true
}

// isAudioTrack returns true if the given track ID corresponds to an audio codec.
func (m *SpliceMuxer) isAudioTrack(trackID uint32) bool {
	for _, t := range m.tracks {
		if t.ID == trackID {
			return audioCodecs[t.Codec]
		}
	}
	return false
}
