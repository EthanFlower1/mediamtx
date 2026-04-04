package playback

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/stretchr/testify/require"
)

func TestOnSeek(t *testing.T) {
	for _, ca := range []struct {
		name              string
		timestamp         time.Time
		expectKeyframeOff time.Duration // offset from segment start
		expectPreroll     float64
	}{
		{
			name:              "seek to exact keyframe at 0s",
			timestamp:         time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local),
			expectKeyframeOff: 0,
			expectPreroll:     0,
		},
		{
			name:              "seek to 1.5s (should snap to keyframe at 0s)",
			timestamp:         time.Date(2008, 11, 7, 11, 22, 1, 500000000, time.Local),
			expectKeyframeOff: 0,
			expectPreroll:     1.5,
		},
		{
			name:              "seek to exact keyframe at 3s",
			timestamp:         time.Date(2008, 11, 7, 11, 22, 3, 0, time.Local),
			expectKeyframeOff: 3 * time.Second,
			expectPreroll:     0,
		},
		{
			name:              "seek to 4.5s (should snap to keyframe at 3s)",
			timestamp:         time.Date(2008, 11, 7, 11, 22, 4, 500000000, time.Local),
			expectKeyframeOff: 3 * time.Second,
			expectPreroll:     1.5,
		},
		{
			name:              "seek to 7s (should snap to keyframe at 6s)",
			timestamp:         time.Date(2008, 11, 7, 11, 22, 7, 0, time.Local),
			expectKeyframeOff: 6 * time.Second,
			expectPreroll:     1.0,
		},
	} {
		t.Run(ca.name, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mediamtx-seek-test")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
			require.NoError(t, err)

			writeTestSegmentWithKeyframes(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-000000.mp4"))

			s := &Server{
				Address:      "127.0.0.1:9996",
				ReadTimeout:  conf.Duration(10 * time.Second),
				WriteTimeout: conf.Duration(10 * time.Second),
				PathConfs: map[string]*conf.Path{
					"mypath": {
						Name:         "mypath",
						RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
						RecordFormat: conf.RecordFormatFMP4,
					},
				},
				AuthManager: test.NilAuthManager,
				Parent:      test.NilLogger,
			}
			err = s.Initialize()
			require.NoError(t, err)
			defer s.Close()

			u, err := url.Parse("http://myuser:mypass@localhost:9996/seek")
			require.NoError(t, err)

			v := url.Values{}
			v.Set("path", "mypath")
			v.Set("timestamp", ca.timestamp.Format(time.RFC3339Nano))
			u.RawQuery = v.Encode()

			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			require.NoError(t, err)

			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer res.Body.Close()

			require.Equal(t, http.StatusOK, res.StatusCode)

			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)

			var resp seekResponse
			err = json.Unmarshal(body, &resp)
			require.NoError(t, err)

			segStart := time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local)
			expectedKFTime := segStart.Add(ca.expectKeyframeOff)

			require.Equal(t, ca.timestamp.UTC(), resp.RequestedTimestamp.UTC())
			require.Equal(t, expectedKFTime.UTC(), resp.KeyframeTimestamp.UTC())
			require.InDelta(t, ca.expectPreroll, resp.PrerollDuration, 0.01)
			require.Equal(t, segStart.UTC(), resp.SegmentStart.UTC())
		})
	}
}

func TestOnSeekNotFound(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-seek-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	s := &Server{
		Address:      "127.0.0.1:9996",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:         "mypath",
				RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: conf.RecordFormatFMP4,
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	u, err := url.Parse("http://myuser:mypass@localhost:9996/seek")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	v.Set("timestamp", time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano))
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestOnGetWithTimestamp(t *testing.T) {
	dir, err := os.MkdirTemp("", "mediamtx-seek-test")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	err = os.Mkdir(filepath.Join(dir, "mypath"), 0o755)
	require.NoError(t, err)

	writeTestSegmentWithKeyframes(t, filepath.Join(dir, "mypath", "2008-11-07_11-22-00-000000.mp4"))

	s := &Server{
		Address:      "127.0.0.1:9996",
		ReadTimeout:  conf.Duration(10 * time.Second),
		WriteTimeout: conf.Duration(10 * time.Second),
		PathConfs: map[string]*conf.Path{
			"mypath": {
				Name:         "mypath",
				RecordPath:   filepath.Join(dir, "%path/%Y-%m-%d_%H-%M-%S-%f"),
				RecordFormat: conf.RecordFormatFMP4,
			},
		},
		AuthManager: test.NilAuthManager,
		Parent:      test.NilLogger,
	}
	err = s.Initialize()
	require.NoError(t, err)
	defer s.Close()

	// Request: seek to 4.5s, wanting 2 seconds of content.
	// The keyframe before 4.5s is at 3s, so we should get content from 3s.
	// The preroll header should indicate the time between the keyframe and the
	// requested timestamp.
	u, err := url.Parse("http://myuser:mypass@localhost:9996/get")
	require.NoError(t, err)

	v := url.Values{}
	v.Set("path", "mypath")
	v.Set("start", time.Date(2008, 11, 7, 11, 22, 0, 0, time.Local).Format(time.RFC3339Nano))
	v.Set("duration", "8")
	v.Set("format", "fmp4")
	v.Set("timestamp", time.Date(2008, 11, 7, 11, 22, 4, 500000000, time.Local).Format(time.RFC3339Nano))
	u.RawQuery = v.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	require.NoError(t, err)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	// Check that preroll headers are set
	prerollHeader := res.Header.Get("X-Playback-Preroll-Duration")
	require.NotEmpty(t, prerollHeader)

	keyframeHeader := res.Header.Get("X-Playback-Keyframe-Timestamp")
	require.NotEmpty(t, keyframeHeader)

	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	// Verify we got valid fmp4 data
	var parts fmp4.Parts
	err = parts.Unmarshal(body)
	require.NoError(t, err)
	require.Greater(t, len(parts), 0)
}
