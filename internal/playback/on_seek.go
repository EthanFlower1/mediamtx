package playback

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recordstore"
	"github.com/gin-gonic/gin"
)

type seekResponse struct {
	// KeyframeTimestamp is the wall-clock time of the nearest keyframe.
	KeyframeTimestamp time.Time `json:"keyframeTimestamp"`
	// RequestedTimestamp is the originally requested wall-clock time.
	RequestedTimestamp time.Time `json:"requestedTimestamp"`
	// PrerollDuration is the duration between the keyframe and the requested timestamp.
	// A player should skip this amount to reach the exact requested time.
	PrerollDuration float64 `json:"prerollDuration"`
	// SegmentStart is the wall-clock start time of the containing segment.
	SegmentStart time.Time `json:"segmentStart"`
}

func (s *Server) onSeek(ctx *gin.Context) {
	pathName := ctx.Query("path")

	if !s.doAuth(ctx, pathName) {
		return
	}

	timestamp, err := time.Parse(time.RFC3339Nano, ctx.Query("timestamp"))
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("invalid timestamp: %w", err))
		return
	}

	pathConf, err := s.safeFindPathConf(pathName)
	if err != nil {
		s.writeError(ctx, http.StatusBadRequest, err)
		return
	}

	if pathConf.RecordFormat != conf.RecordFormatFMP4 {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("frame-accurate seek is only supported for fMP4 format"))
		return
	}

	// Find segments around the requested timestamp.
	// Use a window to ensure we find the segment containing the timestamp.
	searchStart := timestamp.Add(-1 * time.Hour)
	searchEnd := timestamp.Add(1 * time.Second)
	segments, err := recordstore.FindSegments(pathConf, pathName, &searchStart, &searchEnd)
	if err != nil {
		if errors.Is(err, recordstore.ErrNoSegmentsFound) {
			s.writeError(ctx, http.StatusNotFound, err)
		} else {
			s.writeError(ctx, http.StatusBadRequest, err)
		}
		return
	}

	// Find the segment that contains the requested timestamp.
	// The segment whose start is at or before the timestamp and whose
	// end (start + duration) is after the timestamp.
	var targetSegment *recordstore.Segment
	for i := len(segments) - 1; i >= 0; i-- {
		if !segments[i].Start.After(timestamp) {
			targetSegment = segments[i]
			break
		}
	}

	if targetSegment == nil {
		s.writeError(ctx, http.StatusNotFound, recordstore.ErrNoSegmentsFound)
		return
	}

	// Open the segment and find keyframes.
	f, err := os.Open(targetSegment.Fpath)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, fmt.Errorf("failed to open segment: %w", err))
		return
	}
	defer f.Close()

	init, _, err := segmentFMP4ReadHeader(f)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, fmt.Errorf("failed to read segment header: %w", err))
		return
	}

	videoTrackID := findVideoTrackID(init)
	if videoTrackID == 0 {
		s.writeError(ctx, http.StatusBadRequest, fmt.Errorf("no video track found in segment"))
		return
	}

	keyframes, err := segmentFMP4FindKeyframesManual(f, videoTrackID, findInitTrack(init.Tracks, videoTrackID).TimeScale)
	if err != nil {
		s.writeError(ctx, http.StatusInternalServerError, fmt.Errorf("failed to scan keyframes: %w", err))
		return
	}

	if len(keyframes) == 0 {
		s.writeError(ctx, http.StatusNotFound, fmt.Errorf("no keyframes found in segment"))
		return
	}

	// Compute the offset from segment start to the requested timestamp.
	offsetFromSegStart := timestamp.Sub(targetSegment.Start)

	kf, _ := findNearestKeyframeBefore(keyframes, offsetFromSegStart)

	keyframeTimestamp := targetSegment.Start.Add(kf.DTSGo)
	prerollDuration := timestamp.Sub(keyframeTimestamp)
	if prerollDuration < 0 {
		prerollDuration = 0
	}

	resp := seekResponse{
		KeyframeTimestamp:  keyframeTimestamp,
		RequestedTimestamp: timestamp,
		PrerollDuration:    prerollDuration.Seconds(),
		SegmentStart:       targetSegment.Start,
	}

	ctx.Header("Content-Type", "application/json")
	ctx.Status(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(resp) //nolint:errcheck
}
