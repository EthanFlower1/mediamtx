package playback

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

type writerWrapper struct {
	ctx     *gin.Context
	written bool
}

func (w *writerWrapper) Write(p []byte) (int, error) {
	if !w.written {
		w.written = true
		w.ctx.Header("Accept-Ranges", "none")
		w.ctx.Header("Content-Type", "video/mp4")
	}
	return w.ctx.Writer.Write(p)
}

func parseDuration(raw string) (time.Duration, error) {
	// seconds
	if secs, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), nil
	}

	// deprecated, golang format
	return time.ParseDuration(raw)
}

func seekAndMux(
	recordFormat conf.RecordFormat,
	segments []*recordstore.Segment,
	start time.Time,
	duration time.Duration,
	m muxer,
) error {
	if recordFormat == conf.RecordFormatFMP4 {
		f, err := os.Open(segments[0].Fpath)
		if err != nil {
			return err
		}
		defer f.Close()

		firstInit, _, err := segmentFMP4ReadHeader(f)
		if err != nil {
			return err
		}

		m.writeInit(&fmp4.Init{
			Tracks: firstInit.Tracks,
		})

		firstMtxi := findMtxi(firstInit.UserData)
		startOffset := segments[0].Start.Sub(start) // this is negative
		dts := startOffset
		prevInit := firstInit

		segmentDuration, err := segmentFMP4MuxParts(f, dts, duration, firstInit.Tracks, m)
		if err != nil {
			return err
		}

		segmentEnd := segments[0].Start.Add(segmentDuration)

		for _, seg := range segments[1:] {
			f, err = os.Open(seg.Fpath)
			if err != nil {
				return err
			}
			defer f.Close()

			var init *fmp4.Init
			init, _, err = segmentFMP4ReadHeader(f)
			if err != nil {
				return err
			}

			if !segmentFMP4CanBeConcatenated(prevInit, segmentEnd, init, seg.Start) {
				break
			}

			if firstMtxi != nil {
				mtxi := findMtxi(init.UserData)
				dts = time.Duration(mtxi.DTS-firstMtxi.DTS) + startOffset
			} else { // legacy method
				dts = seg.Start.Sub(start) // this is positive
			}

			segmentDuration, err = segmentFMP4MuxParts(f, dts, duration, firstInit.Tracks, m)
			if err != nil {
				return err
			}

			segmentEnd = seg.Start.Add(segmentDuration)
			prevInit = init
		}

		err = m.flush()
		if err != nil {
			return err
		}

		return nil
	}

	return fmt.Errorf("MPEG-TS format is not supported yet")
}

func (s *Server) onGet(ctx *gin.Context) {
	pathName := ctx.Query("path")

	if !s.doAuth(ctx, pathName) {
		return
	}

	start, err := time.Parse(time.RFC3339Nano, ctx.Query("start"))
	if err != nil {
		// Fall back to RFC3339 (without nanos) for backward compatibility.
		start, err = time.Parse(time.RFC3339, ctx.Query("start"))
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid start: %w", err))
			return
		}
	}

	duration, err := parseDuration(ctx.Query("duration"))
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid duration: %w", err))
		return
	}

	ww := &writerWrapper{ctx: ctx}
	var m muxer

	format := ctx.Query("format")
	switch format {
	case "", "fmp4":
		m = &muxerFMP4{w: ww}

	case "mp4":
		m = &muxerMP4{w: ww}

	default:
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid format: %s", format))
		return
	}

	pathConf, err := s.safeFindPathConf(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	// Frame-accurate seek: if a precise timestamp is specified, adjust the
	// start time to the nearest preceding keyframe and report preroll.
	rawTimestamp := ctx.Query("timestamp")
	if rawTimestamp != "" && pathConf.RecordFormat == conf.RecordFormatFMP4 {
		var timestamp time.Time
		timestamp, err = time.Parse(time.RFC3339Nano, rawTimestamp)
		if err != nil {
			s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid timestamp: %w", err))
			return
		}

		start, duration, err = s.adjustStartToKeyframe(pathConf, pathName, timestamp, duration)
		if err != nil {
			if errors.Is(err, recordstore.ErrNoSegmentsFound) {
				s.writeError(ctx, http.StatusNotFound, err)
			} else {
				s.writeError(ctx, http.StatusBadRequest, err)
			}
			return
		}

		// Set headers to inform the client about the preroll.
		preroll := timestamp.Sub(start)
		if preroll < 0 {
			preroll = 0
		}
		ctx.Header("X-Playback-Preroll-Duration", strconv.FormatFloat(preroll.Seconds(), 'f', -1, 64))
		ctx.Header("X-Playback-Keyframe-Timestamp", start.Format(time.RFC3339Nano))
	}

	end := start.Add(duration)
	segments, err := recordstore.FindSegments(pathConf, pathName, &start, &end)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	err = seekAndMux(pathConf.RecordFormat, segments, start, duration, m)
	if err != nil {
		// user aborted the download
		var neterr *net.OpError
		if errors.As(err, &neterr) {
			return
		}

		// nothing has been written yet; send back JSON
		if !ww.written {
			if errors.Is(err, recordstore.ErrNoSegmentsFound) {
				s.writeError(ctx, http.StatusNotFound, err)
			} else {
				s.writeError(ctx, http.StatusBadRequest, err)
			}
			return
		}

		// something has already been written: abort and write logs only
		s.Log(logger.Error, err.Error())
		return
	}
}

// adjustStartToKeyframe finds the nearest keyframe at or before the given timestamp
// and adjusts the start time and duration accordingly. The returned start time
// is the wall-clock time of the keyframe, and the duration is extended to
// cover the original requested range.
func (s *Server) adjustStartToKeyframe(
	pathConf *conf.Path,
	pathName string,
	timestamp time.Time,
	originalDuration time.Duration,
) (time.Time, time.Duration, error) {
	// Search for segments around the timestamp.
	searchStart := timestamp.Add(-1 * time.Hour)
	searchEnd := timestamp.Add(1 * time.Second)
	segments, err := recordstore.FindSegments(pathConf, pathName, &searchStart, &searchEnd)
	if err != nil {
		return time.Time{}, 0, err
	}

	// Find the segment containing the timestamp.
	var targetSegment *recordstore.Segment
	for i := len(segments) - 1; i >= 0; i-- {
		if !segments[i].Start.After(timestamp) {
			targetSegment = segments[i]
			break
		}
	}

	if targetSegment == nil {
		return time.Time{}, 0, recordstore.ErrNoSegmentsFound
	}

	f, err := os.Open(targetSegment.Fpath)
	if err != nil {
		return time.Time{}, 0, err
	}
	defer f.Close()

	init, _, err := segmentFMP4ReadHeader(f)
	if err != nil {
		return time.Time{}, 0, err
	}

	videoTrackID := findVideoTrackID(init)
	if videoTrackID == 0 {
		// No video track; fall back to using the timestamp as-is.
		return timestamp, originalDuration, nil
	}

	track := findInitTrack(init.Tracks, videoTrackID)
	keyframes, err := segmentFMP4FindKeyframesManual(f, videoTrackID, track.TimeScale)
	if err != nil {
		return time.Time{}, 0, err
	}

	if len(keyframes) == 0 {
		return timestamp, originalDuration, nil
	}

	offsetFromSegStart := timestamp.Sub(targetSegment.Start)
	kf, _ := findNearestKeyframeBefore(keyframes, offsetFromSegStart)

	keyframeTime := targetSegment.Start.Add(kf.DTSGo)
	// Extend duration to cover from keyframe to the end of the original request.
	newDuration := originalDuration + timestamp.Sub(keyframeTime)

	return keyframeTime, newDuration, nil
}
