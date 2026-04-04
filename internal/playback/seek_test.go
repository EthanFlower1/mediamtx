package playback

import (
	"os"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4/seekablebuffer"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func writeTestSegmentWithKeyframes(t *testing.T, fpath string) {
	t.Helper()

	init := fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &mcodecs.H264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				},
			},
			{
				ID:        2,
				TimeScale: 48000,
				Codec: &mcodecs.MPEG4Audio{
					Config: mpeg4audio.AudioSpecificConfig{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	var buf1 seekablebuffer.Buffer
	err := init.Marshal(&buf1)
	require.NoError(t, err)

	// Create a segment with multiple GOPs:
	// GOP1: IDR at 0s, P at 1s, P at 2s
	// GOP2: IDR at 3s, P at 4s, P at 5s
	// GOP3: IDR at 6s, P at 7s
	var buf2 seekablebuffer.Buffer
	parts := fmp4.Parts{
		{
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 0,
					Samples: []*fmp4.Sample{
						{
							Duration:        1 * 90000, // IDR at 0s
							IsNonSyncSample: false,
							Payload:         []byte{1, 2},
						},
						{
							Duration:        1 * 90000, // P at 1s
							IsNonSyncSample: true,
							Payload:         []byte{3, 4},
						},
						{
							Duration:        1 * 90000, // P at 2s
							IsNonSyncSample: true,
							Payload:         []byte{5, 6},
						},
					},
				},
				{
					ID:       2,
					BaseTime: 0,
					Samples: []*fmp4.Sample{
						{
							Duration: 3 * 48000,
							Payload:  []byte{10, 11},
						},
					},
				},
			},
		},
		{
			SequenceNumber: 1,
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 3 * 90000,
					Samples: []*fmp4.Sample{
						{
							Duration:        1 * 90000, // IDR at 3s
							IsNonSyncSample: false,
							Payload:         []byte{7, 8},
						},
						{
							Duration:        1 * 90000, // P at 4s
							IsNonSyncSample: true,
							Payload:         []byte{9, 10},
						},
						{
							Duration:        1 * 90000, // P at 5s
							IsNonSyncSample: true,
							Payload:         []byte{11, 12},
						},
					},
				},
			},
		},
		{
			SequenceNumber: 2,
			Tracks: []*fmp4.PartTrack{
				{
					ID:       1,
					BaseTime: 6 * 90000,
					Samples: []*fmp4.Sample{
						{
							Duration:        1 * 90000, // IDR at 6s
							IsNonSyncSample: false,
							Payload:         []byte{13, 14},
						},
						{
							Duration:        1 * 90000, // P at 7s
							IsNonSyncSample: true,
							Payload:         []byte{15, 16},
						},
					},
				},
			},
		},
	}
	err = parts.Marshal(&buf2)
	require.NoError(t, err)

	err = os.WriteFile(fpath, append(buf1.Bytes(), buf2.Bytes()...), 0o644)
	require.NoError(t, err)
}

func TestSegmentFMP4FindKeyframes(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-seek-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	fpath := dir + "/test_segment.mp4"
	writeTestSegmentWithKeyframes(t, fpath)

	f, err := os.Open(fpath)
	require.NoError(t, err)
	defer f.Close()

	keyframes, err := segmentFMP4FindKeyframesManual(f, 1, 90000)
	require.NoError(t, err)

	// Expect 3 keyframes: at 0s, 3s, and 6s
	require.Len(t, keyframes, 3)

	require.Equal(t, int64(0), keyframes[0].DTS)
	require.Equal(t, time.Duration(0), keyframes[0].DTSGo)
	require.Equal(t, 1, keyframes[0].TrackID)

	require.Equal(t, int64(3*90000), keyframes[1].DTS)
	require.Equal(t, 3*time.Second, keyframes[1].DTSGo)
	require.Equal(t, 1, keyframes[1].TrackID)

	require.Equal(t, int64(6*90000), keyframes[2].DTS)
	require.Equal(t, 6*time.Second, keyframes[2].DTSGo)
	require.Equal(t, 1, keyframes[2].TrackID)
}

func TestSegmentFMP4FindKeyframesAudioOnly(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-seek-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	fpath := dir + "/test_segment.mp4"
	writeTestSegmentWithKeyframes(t, fpath)

	f, err := os.Open(fpath)
	require.NoError(t, err)
	defer f.Close()

	// Try to find keyframes for audio track (track 2) - audio has no sync flags
	keyframes, err := segmentFMP4FindKeyframesManual(f, 2, 48000)
	require.NoError(t, err)

	// Audio samples typically have sync flag set (IsNonSyncSample=false by default)
	// so they should all be reported as keyframes
	require.Greater(t, len(keyframes), 0)
}

func TestFindNearestKeyframeBefore(t *testing.T) {
	keyframes := []KeyframeInfo{
		{TrackID: 1, DTS: 0, DTSGo: 0},
		{TrackID: 1, DTS: 3 * 90000, DTSGo: 3 * time.Second},
		{TrackID: 1, DTS: 6 * 90000, DTSGo: 6 * time.Second},
	}

	for _, tt := range []struct {
		name       string
		offset     time.Duration
		expectedKF KeyframeInfo
		wantOK     bool
	}{
		{
			name:       "exact keyframe match at 0",
			offset:     0,
			expectedKF: keyframes[0],
			wantOK:     true,
		},
		{
			name:       "exact keyframe match at 3s",
			offset:     3 * time.Second,
			expectedKF: keyframes[1],
			wantOK:     true,
		},
		{
			name:       "between keyframes at 1.5s",
			offset:     1500 * time.Millisecond,
			expectedKF: keyframes[0],
			wantOK:     true,
		},
		{
			name:       "between keyframes at 4s",
			offset:     4 * time.Second,
			expectedKF: keyframes[1],
			wantOK:     true,
		},
		{
			name:       "after last keyframe at 7s",
			offset:     7 * time.Second,
			expectedKF: keyframes[2],
			wantOK:     true,
		},
		{
			name:       "before first keyframe (negative)",
			offset:     -1 * time.Second,
			expectedKF: keyframes[0],
			wantOK:     true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kf, ok := findNearestKeyframeBefore(keyframes, tt.offset)
			require.Equal(t, tt.wantOK, ok)
			if ok {
				require.Equal(t, tt.expectedKF.DTS, kf.DTS)
				require.Equal(t, tt.expectedKF.DTSGo, kf.DTSGo)
			}
		})
	}

	// Test empty keyframes
	_, ok := findNearestKeyframeBefore(nil, 0)
	require.False(t, ok)
}

func TestFindVideoTrackID(t *testing.T) {
	init := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        1,
				TimeScale: 90000,
				Codec: &mcodecs.H264{
					SPS: test.FormatH264.SPS,
					PPS: test.FormatH264.PPS,
				},
			},
			{
				ID:        2,
				TimeScale: 48000,
				Codec: &mcodecs.MPEG4Audio{
					Config: mpeg4audio.AudioSpecificConfig{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	require.Equal(t, 1, findVideoTrackID(init))

	// Audio only
	initAudioOnly := &fmp4.Init{
		Tracks: []*fmp4.InitTrack{
			{
				ID:        2,
				TimeScale: 48000,
				Codec: &mcodecs.MPEG4Audio{
					Config: mpeg4audio.AudioSpecificConfig{
						Type:         mpeg4audio.ObjectTypeAACLC,
						SampleRate:   48000,
						ChannelCount: 2,
					},
				},
			},
		},
	}

	require.Equal(t, 0, findVideoTrackID(initAudioOnly))
}

func TestIsVideoCodec(t *testing.T) {
	require.True(t, isVideoCodec(&mcodecs.H264{}))
	require.True(t, isVideoCodec(&mcodecs.H265{}))
	require.True(t, isVideoCodec(&mcodecs.VP9{}))
	require.True(t, isVideoCodec(&mcodecs.AV1{}))
	require.False(t, isVideoCodec(&mcodecs.MPEG4Audio{}))
	require.False(t, isVideoCodec(&mcodecs.Opus{}))
}
