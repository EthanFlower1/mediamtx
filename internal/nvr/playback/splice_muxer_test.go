package playback

import (
	"io"
	"testing"

	amp4 "github.com/abema/go-mp4"
	"github.com/stretchr/testify/require"
)

var testTracks = []TrackInfo{
	{ID: 1, TimeScale: 90000, Codec: "avc1"},
	{ID: 2, TimeScale: 48000, Codec: "mp4a"},
}

func drainOne(t *testing.T, ch chan []byte) []byte {
	t.Helper()
	select {
	case data := <-ch:
		return data
	default:
		t.Fatal("expected data on Out channel, got none")
		return nil
	}
}

func TestSpliceMuxerSequenceContinuity(t *testing.T) {
	m := NewSpliceMuxer(testTracks)

	// Verify initial sequence number.
	require.Equal(t, uint32(1), m.NextSequenceNumber())

	// Write and flush first fragment.
	m.WriteSample(Sample{
		DTS: 0, Duration: 3000, IsSync: true,
		Data: []byte{0x01}, TrackID: 1,
	})
	m.FlushFragment()
	drainOne(t, m.Out)
	require.Equal(t, uint32(2), m.NextSequenceNumber())

	// Splice (simulating a seek).
	m.Splice()

	// Write and flush second fragment after splice.
	m.WriteSample(Sample{
		DTS: 900000, Duration: 3000, IsSync: true,
		Data: []byte{0x02}, TrackID: 1,
	})
	m.FlushFragment()
	drainOne(t, m.Out)
	require.Equal(t, uint32(3), m.NextSequenceNumber())

	// Splice again and flush a third fragment.
	m.Splice()
	m.WriteSample(Sample{
		DTS: 1800000, Duration: 3000, IsSync: true,
		Data: []byte{0x03}, TrackID: 1,
	})
	m.FlushFragment()
	drainOne(t, m.Out)
	require.Equal(t, uint32(4), m.NextSequenceNumber())
}

func TestSpliceMuxerDTSContinuity(t *testing.T) {
	tracks := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
	}
	m := NewSpliceMuxer(tracks)

	// First fragment: samples at DTS 0, 3000, 6000.
	m.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	m.WriteSample(Sample{DTS: 3000, Duration: 3000, IsSync: false, Data: []byte{0x02}, TrackID: 1})
	m.WriteSample(Sample{DTS: 6000, Duration: 3000, IsSync: false, Data: []byte{0x03}, TrackID: 1})
	m.FlushFragment()

	// After flush, lastDTS should be 6000 + 3000 = 9000.
	frag1 := drainOne(t, m.Out)
	require.NotEmpty(t, frag1)
	require.Equal(t, uint64(9000), m.LastOutputDTS())

	// Verify base time of first fragment is 0.
	bt1 := extractBaseTime(t, frag1, 1)
	require.Equal(t, uint64(0), bt1)

	// Now splice and write samples from a completely different source position.
	m.Splice()

	// Source DTS jumps to 500000 (a different segment).
	m.WriteSample(Sample{DTS: 500000, Duration: 3000, IsSync: true, Data: []byte{0x04}, TrackID: 1})
	m.WriteSample(Sample{DTS: 503000, Duration: 3000, IsSync: false, Data: []byte{0x05}, TrackID: 1})
	m.FlushFragment()

	frag2 := drainOne(t, m.Out)
	require.NotEmpty(t, frag2)

	// After splice, the output DTS should continue from 9000.
	// dtsOffset = 9000 - 500000 = -491000
	// First sample output DTS = 500000 + (-491000) = 9000
	// Second sample output DTS = 503000 + (-491000) = 12000
	// lastDTS = max(12000 + 3000) = 15000
	require.Equal(t, uint64(15000), m.LastOutputDTS())

	// Verify the fragment baseTime is 9000 (continuous from first fragment).
	bt2 := extractBaseTime(t, frag2, 1)
	require.Equal(t, uint64(9000), bt2)
}

func TestSpliceMuxerAudioFiltering(t *testing.T) {
	// Test that with dropAudio on from the start, no audio appears.
	m := NewSpliceMuxer(testTracks)
	m.SetDropAudio(true)

	m.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	m.WriteSample(Sample{DTS: 0, Duration: 1024, IsSync: true, Data: []byte{0xAA}, TrackID: 2})
	m.WriteSample(Sample{DTS: 3000, Duration: 3000, IsSync: false, Data: []byte{0x02}, TrackID: 1})

	m.FlushFragment()
	frag := drainOne(t, m.Out)
	require.NotEmpty(t, frag)

	trackIDs := extractTrackIDs(t, frag)
	require.Contains(t, trackIDs, uint32(1))
	require.NotContains(t, trackIDs, uint32(2), "audio track should be filtered out")

	// Test that without dropAudio, audio is included.
	m2 := NewSpliceMuxer(testTracks)

	m2.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	m2.WriteSample(Sample{DTS: 0, Duration: 1024, IsSync: true, Data: []byte{0xAA}, TrackID: 2})

	m2.FlushFragment()
	frag2 := drainOne(t, m2.Out)
	require.NotEmpty(t, frag2)

	trackIDs2 := extractTrackIDs(t, frag2)
	require.Contains(t, trackIDs2, uint32(1))
	require.Contains(t, trackIDs2, uint32(2), "audio track should be present when not filtering")
}

func TestSpliceMuxerSpliceDiscardsBuffered(t *testing.T) {
	m := NewSpliceMuxer(testTracks)

	// Buffer some samples.
	m.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	m.WriteSample(Sample{DTS: 3000, Duration: 3000, IsSync: false, Data: []byte{0x02}, TrackID: 1})

	// Splice discards them.
	m.Splice()

	// Flushing should produce nothing since samples were discarded.
	m.FlushFragment()

	select {
	case <-m.Out:
		t.Fatal("expected no data after splice with no new samples")
	default:
		// Good -- no data sent.
	}

	// Verify sequence number was NOT incremented (no fragment was written).
	require.Equal(t, uint32(1), m.NextSequenceNumber())
}

func TestSpliceMuxerEmptyFlush(t *testing.T) {
	m := NewSpliceMuxer(testTracks)

	// Flushing with no samples should not send anything or panic.
	m.FlushFragment()

	select {
	case <-m.Out:
		t.Fatal("expected no data from empty flush")
	default:
		// Good.
	}
}

func TestSpliceMuxerWriteInit(t *testing.T) {
	m := NewSpliceMuxer(testTracks)

	// WriteInit just forwards raw bytes to the Out channel.
	// Use arbitrary bytes to verify pass-through behavior.
	initData := []byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70} // fake ftyp header
	m.WriteInit(initData)

	got := drainOne(t, m.Out)
	require.Equal(t, initData, got)
}

func TestSpliceMuxerMultipleTracksInFragment(t *testing.T) {
	tracks := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}
	m := NewSpliceMuxer(tracks)

	// Write interleaved video and audio samples.
	m.WriteSample(Sample{DTS: 0, Duration: 3000, IsSync: true, Data: []byte{0x01}, TrackID: 1})
	m.WriteSample(Sample{DTS: 0, Duration: 1024, IsSync: true, Data: []byte{0xAA}, TrackID: 2})
	m.WriteSample(Sample{DTS: 3000, Duration: 3000, IsSync: false, Data: []byte{0x02}, TrackID: 1})
	m.WriteSample(Sample{DTS: 1024, Duration: 1024, IsSync: true, Data: []byte{0xBB}, TrackID: 2})

	m.FlushFragment()
	frag := drainOne(t, m.Out)
	require.NotEmpty(t, frag)

	trackIDs := extractTrackIDs(t, frag)
	require.Contains(t, trackIDs, uint32(1))
	require.Contains(t, trackIDs, uint32(2))
}

// -- helpers for parsing fMP4 fragments in tests --

// byteReadSeekerAt wraps a byte slice to provide io.ReadSeeker + io.ReaderAt.
type byteReadSeekerAt struct {
	data []byte
	pos  int64
}

func (b *byteReadSeekerAt) Read(p []byte) (int, error) {
	if b.pos >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += int64(n)
	return n, nil
}

func (b *byteReadSeekerAt) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		b.pos = offset
	case io.SeekCurrent:
		b.pos += offset
	case io.SeekEnd:
		b.pos = int64(len(b.data)) + offset
	}
	if b.pos < 0 {
		b.pos = 0
	}
	return b.pos, nil
}

func (b *byteReadSeekerAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) {
		return 0, io.EOF
	}
	n := copy(p, b.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// extractBaseTime parses an fMP4 moof+mdat fragment and returns the
// baseMediaDecodeTime for the given trackID.
func extractBaseTime(t *testing.T, data []byte, trackID uint32) uint64 {
	t.Helper()
	r := &byteReadSeekerAt{data: data}

	var currentTfhdTrackID uint32
	var result uint64
	found := false

	_, err := amp4.ReadBoxStructure(r, func(h *amp4.ReadHandle) (any, error) {
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
			currentTfhdTrackID = tfhd.TrackID
			return nil, nil
		case amp4.BoxTypeTfdt():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt := box.(*amp4.Tfdt)
			if currentTfhdTrackID == trackID {
				if tfdt.FullBox.Version == 0 {
					result = uint64(tfdt.BaseMediaDecodeTimeV0)
				} else {
					result = tfdt.BaseMediaDecodeTimeV1
				}
				found = true
			}
			return nil, nil
		default:
			return nil, nil
		}
	})
	require.NoError(t, err)
	require.True(t, found, "track %d not found in fragment", trackID)
	return result
}

// extractTrackIDs parses an fMP4 moof+mdat fragment and returns all track IDs.
func extractTrackIDs(t *testing.T, data []byte) []uint32 {
	t.Helper()
	r := &byteReadSeekerAt{data: data}

	var trackIDs []uint32

	_, err := amp4.ReadBoxStructure(r, func(h *amp4.ReadHandle) (any, error) {
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
			trackIDs = append(trackIDs, tfhd.TrackID)
			return nil, nil
		default:
			return nil, nil
		}
	})
	require.NoError(t, err)
	return trackIDs
}
