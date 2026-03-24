package playback

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDurationMP4ToGo(t *testing.T) {
	tests := []struct {
		name      string
		d         uint64
		timeScale uint32
		want      time.Duration
	}{
		{
			name:      "zero duration",
			d:         0,
			timeScale: 90000,
			want:      0,
		},
		{
			name:      "exactly one second at 90kHz",
			d:         90000,
			timeScale: 90000,
			want:      time.Second,
		},
		{
			name:      "half second at 90kHz",
			d:         45000,
			timeScale: 90000,
			want:      500 * time.Millisecond,
		},
		{
			name:      "one second at 48kHz",
			d:         48000,
			timeScale: 48000,
			want:      time.Second,
		},
		{
			name:      "10 seconds at 90kHz",
			d:         900000,
			timeScale: 90000,
			want:      10 * time.Second,
		},
		{
			name:      "large value 1 hour at 90kHz",
			d:         90000 * 3600,
			timeScale: 90000,
			want:      time.Hour,
		},
		{
			name:      "fractional at 1000 timescale",
			d:         1500,
			timeScale: 1000,
			want:      1500 * time.Millisecond,
		},
		{
			name:      "zero timescale returns zero",
			d:         12345,
			timeScale: 0,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DurationMP4ToGo(tt.d, tt.timeScale)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDurationGoToMP4(t *testing.T) {
	tests := []struct {
		name      string
		d         time.Duration
		timeScale uint32
		want      uint64
	}{
		{
			name:      "zero",
			d:         0,
			timeScale: 90000,
			want:      0,
		},
		{
			name:      "one second at 90kHz",
			d:         time.Second,
			timeScale: 90000,
			want:      90000,
		},
		{
			name:      "half second at 90kHz",
			d:         500 * time.Millisecond,
			timeScale: 90000,
			want:      45000,
		},
		{
			name:      "10 seconds at 48kHz",
			d:         10 * time.Second,
			timeScale: 48000,
			want:      480000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DurationGoToMP4(tt.d, tt.timeScale)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestDurationRoundTrip(t *testing.T) {
	// Verify that converting Go -> MP4 -> Go preserves the value
	// for durations that are exact multiples.
	timeScale := uint32(90000)
	original := 5 * time.Second
	mp4val := DurationGoToMP4(original, timeScale)
	got := DurationMP4ToGo(mp4val, timeScale)
	require.Equal(t, original, got)
}

func TestTracksCompatible(t *testing.T) {
	tracksA := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}

	tracksB := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}

	tracksC := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "hev1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}

	tracksD := []TrackInfo{
		{ID: 1, TimeScale: 90000, Codec: "avc1"},
	}

	tracksE := []TrackInfo{
		{ID: 1, TimeScale: 44100, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}

	tracksF := []TrackInfo{
		{ID: 3, TimeScale: 90000, Codec: "avc1"},
		{ID: 2, TimeScale: 48000, Codec: "mp4a"},
	}

	tests := []struct {
		name string
		a, b []TrackInfo
		want bool
	}{
		{"identical tracks", tracksA, tracksB, true},
		{"self-compatible", tracksA, tracksA, true},
		{"different codec", tracksA, tracksC, false},
		{"different length", tracksA, tracksD, false},
		{"different timescale", tracksA, tracksE, false},
		{"different track ID", tracksA, tracksF, false},
		{"both empty", nil, nil, true},
		{"one empty", tracksA, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TracksCompatible(tt.a, tt.b)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestKeyframeFlag(t *testing.T) {
	// Verify the keyframe flag constant matches the expected bit pattern.
	// Bit 16 = 0x10000. When this bit is set, sample is NOT a sync sample.

	t.Run("sync sample has flag clear", func(t *testing.T) {
		flags := uint32(0x00000000)
		isSync := (flags & sampleFlagIsNonSyncSample) == 0
		require.True(t, isSync)
	})

	t.Run("non-sync sample has flag set", func(t *testing.T) {
		flags := uint32(0x00010000)
		isSync := (flags & sampleFlagIsNonSyncSample) == 0
		require.False(t, isSync)
	})

	t.Run("non-sync with other flags", func(t *testing.T) {
		flags := uint32(0x01010000) // bit 16 set plus others
		isSync := (flags & sampleFlagIsNonSyncSample) == 0
		require.False(t, isSync)
	})

	t.Run("sync with other flags", func(t *testing.T) {
		flags := uint32(0x02000200) // various bits but NOT bit 16
		isSync := (flags & sampleFlagIsNonSyncSample) == 0
		require.True(t, isSync)
	})
}

func TestTrackInfoEquality(t *testing.T) {
	a := TrackInfo{ID: 1, TimeScale: 90000, Codec: "avc1"}
	b := TrackInfo{ID: 1, TimeScale: 90000, Codec: "avc1"}
	c := TrackInfo{ID: 2, TimeScale: 90000, Codec: "avc1"}

	require.Equal(t, a, b)
	require.NotEqual(t, a, c)
}

func TestReadSegmentHeader_MissingFile(t *testing.T) {
	_, err := ReadSegmentHeader("/nonexistent/path/to/file.mp4")
	require.Error(t, err)
	require.Contains(t, err.Error(), "open segment")
}

func TestReadSegmentSamples_MissingFile(t *testing.T) {
	err := ReadSegmentSamples("/nonexistent/path/to/file.mp4", func(s Sample) error {
		return nil
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "open segment")
}

func TestFindKeyframeBefore_MissingFile(t *testing.T) {
	_, err := FindKeyframeBefore("/nonexistent/path/to/file.mp4", 0)
	require.Error(t, err)
}

func TestReadSegmentHeader_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp("", "fmp4-test-empty-*.mp4")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.Close()

	// An empty file has no boxes; ReadBoxStructure returns no error,
	// but the header will have zero tracks and zero duration.
	hdr, err := ReadSegmentHeader(f.Name())
	require.NoError(t, err)
	require.Empty(t, hdr.Tracks)
	require.Equal(t, time.Duration(0), hdr.Duration)
}

func TestReadSegmentSamples_EmptyFile(t *testing.T) {
	f, err := os.CreateTemp("", "fmp4-test-empty-*.mp4")
	require.NoError(t, err)
	defer os.Remove(f.Name())
	f.Close()

	// An empty file has no moof boxes; callback is never called, no error.
	called := false
	err = ReadSegmentSamples(f.Name(), func(s Sample) error {
		called = true
		return nil
	})
	require.NoError(t, err)
	require.False(t, called)
}

func TestSampleStruct(t *testing.T) {
	// Verify Sample struct fields can be populated.
	s := Sample{
		DTS:       12345,
		PTSOffset: 100,
		Duration:  3000,
		IsSync:    true,
		Data:      []byte{0x00, 0x01, 0x02},
		TrackID:   1,
	}
	require.Equal(t, uint64(12345), s.DTS)
	require.Equal(t, int32(100), s.PTSOffset)
	require.Equal(t, uint32(3000), s.Duration)
	require.True(t, s.IsSync)
	require.Len(t, s.Data, 3)
	require.Equal(t, uint32(1), s.TrackID)
}

func TestSegmentHeaderStruct(t *testing.T) {
	h := SegmentHeader{
		Tracks: []TrackInfo{
			{ID: 1, TimeScale: 90000, Codec: "avc1"},
			{ID: 2, TimeScale: 48000, Codec: "mp4a"},
		},
		Duration: 10 * time.Second,
	}
	require.Len(t, h.Tracks, 2)
	require.Equal(t, 10*time.Second, h.Duration)
}
