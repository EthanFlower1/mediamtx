package playback

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	amp4 "github.com/abema/go-mp4"
)

const (
	// sampleFlagIsNonSyncSample is bit 16 of the sample_flags field.
	// When set, the sample is NOT a sync (key) frame.
	sampleFlagIsNonSyncSample = 1 << 16
)

var errDone = errors.New("done")

// TrackInfo describes a single track in an fMP4 segment.
type TrackInfo struct {
	ID        uint32
	TimeScale uint32
	Codec     string // "avc1", "hev1", "mp4a", "Opus", etc.
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

// SegmentHeader contains the metadata extracted from moov.
type SegmentHeader struct {
	Tracks   []TrackInfo
	Duration time.Duration
}

// SampleCallback is invoked for each sample during ReadSegmentSamples.
// Return a non-nil error to stop iteration early.
type SampleCallback func(s Sample) error

// DurationMP4ToGo converts an MP4 duration (in timescale units) to time.Duration
// using integer arithmetic to avoid floating-point precision loss.
func DurationMP4ToGo(d uint64, timeScale uint32) time.Duration {
	ts := uint64(timeScale)
	if ts == 0 {
		return 0
	}
	secs := d / ts
	remainder := d % ts
	return time.Duration(secs)*time.Second + time.Duration(remainder)*time.Second/time.Duration(ts)
}

// DurationGoToMP4 converts a time.Duration to MP4 duration in timescale units
// using integer arithmetic.
func DurationGoToMP4(d time.Duration, timeScale uint32) uint64 {
	ts := uint64(timeScale)
	secs := uint64(d / time.Second)
	remainder := uint64(d % time.Second)
	return secs*ts + remainder*ts/uint64(time.Second)
}

// TracksCompatible returns true if slices a and b have the same length,
// and for each index the TrackID, TimeScale, and Codec name match.
func TracksCompatible(a, b []TrackInfo) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID ||
			a[i].TimeScale != b[i].TimeScale ||
			a[i].Codec != b[i].Codec {
			return false
		}
	}
	return true
}

// ReadSegmentHeader opens an fMP4 file and reads track info and duration from
// the moov box using go-mp4's ReadBoxStructure.
func ReadSegmentHeader(path string) (*SegmentHeader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open segment: %w", err)
	}
	defer f.Close()

	return readSegmentHeaderFromReader(f)
}

// readSegmentHeaderFromReader reads track info from an io.ReadSeeker.
// It walks the box tree: moov > trak > (tkhd for track ID, mdia > mdhd for timescale, mdia > minf > stbl > stsd for codec).
func readSegmentHeaderFromReader(r io.ReadSeeker) (*SegmentHeader, error) {
	var header SegmentHeader

	// State carried across box callbacks via params.
	// We track current trak's trackID, timescale, and codec per-trak.
	type trakState struct {
		trackID   uint32
		timeScale uint32
		codec     string
	}

	var currentTrak *trakState
	var duration time.Duration
	var mvhdTimescale uint32

	_, err := amp4.ReadBoxStructure(r, func(h *amp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case amp4.BoxTypeFtyp():
			// skip ftyp
			return nil, nil

		case amp4.BoxTypeMoov():
			return h.Expand()

		case amp4.BoxTypeMvhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mvhd := box.(*amp4.Mvhd)
			mvhdTimescale = mvhd.Timescale
			if mvhd.FullBox.Version == 0 {
				duration = DurationMP4ToGo(uint64(mvhd.DurationV0), mvhdTimescale)
			} else {
				duration = DurationMP4ToGo(mvhd.DurationV1, mvhdTimescale)
			}
			return nil, nil

		case amp4.BoxTypeTrak():
			currentTrak = &trakState{}
			_, err := h.Expand()
			if err != nil {
				return nil, err
			}
			// After expanding the trak, record the track info
			if currentTrak.trackID != 0 && currentTrak.timeScale != 0 {
				header.Tracks = append(header.Tracks, TrackInfo{
					ID:        currentTrak.trackID,
					TimeScale: currentTrak.timeScale,
					Codec:     currentTrak.codec,
				})
			}
			currentTrak = nil
			return nil, nil

		case amp4.BoxTypeTkhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tkhd := box.(*amp4.Tkhd)
			if currentTrak != nil {
				currentTrak.trackID = tkhd.TrackID
			}
			return nil, nil

		case amp4.BoxTypeMdia():
			return h.Expand()

		case amp4.BoxTypeMdhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			mdhd := box.(*amp4.Mdhd)
			if currentTrak != nil {
				currentTrak.timeScale = mdhd.Timescale
			}
			return nil, nil

		case amp4.BoxTypeMinf():
			return h.Expand()

		case amp4.BoxTypeStbl():
			return h.Expand()

		case amp4.BoxTypeStsd():
			// We need to expand stsd to see its children (codec boxes).
			return h.Expand()

		case amp4.BoxTypeAvc1():
			if currentTrak != nil {
				currentTrak.codec = "avc1"
			}
			return nil, nil

		case amp4.BoxTypeHev1():
			if currentTrak != nil {
				currentTrak.codec = "hev1"
			}
			return nil, nil

		case amp4.BoxTypeMp4a():
			if currentTrak != nil {
				currentTrak.codec = "mp4a"
			}
			return nil, nil

		case amp4.BoxTypeOpus():
			if currentTrak != nil {
				currentTrak.codec = "Opus"
			}
			return nil, nil

		case amp4.BoxTypeAv01():
			if currentTrak != nil {
				currentTrak.codec = "av01"
			}
			return nil, nil

		case amp4.BoxTypeVp09():
			if currentTrak != nil {
				currentTrak.codec = "vp09"
			}
			return nil, nil

		default:
			// For codec boxes we don't explicitly handle, try detecting by name.
			// Common ones: hvc1, etc.
			typStr := h.BoxInfo.Type.String()
			if currentTrak != nil && currentTrak.codec == "" {
				// If we're inside stsd and this is a recognized codec string
				switch typStr {
				case "hvc1":
					currentTrak.codec = "hvc1"
				case "vp08":
					currentTrak.codec = "vp08"
				case "ac-3":
					currentTrak.codec = "ac-3"
				case "ec-3":
					currentTrak.codec = "ec-3"
				}
			}
			return nil, nil
		}
	})
	if err != nil {
		return nil, fmt.Errorf("parse moov: %w", err)
	}

	header.Duration = duration
	return &header, nil
}

// ReadSegmentSamples opens an fMP4 file and iterates over every moof+mdat pair,
// calling cb for each sample with its actual data bytes read from mdat.
func ReadSegmentSamples(path string, cb SampleCallback) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open segment: %w", err)
	}
	defer f.Close()

	return readSegmentSamplesFromReader(f, cb)
}

// readSeekerAt combines io.ReadSeeker and io.ReaderAt.
type readSeekerAt interface {
	io.ReadSeeker
	io.ReaderAt
}

// readSegmentSamplesFromReader walks moof+mdat pairs via ReadBoxStructure.
func readSegmentSamplesFromReader(r readSeekerAt, cb SampleCallback) error {
	var moofOffset uint64
	var tfhd *amp4.Tfhd
	var tfdt *amp4.Tfdt

	_, err := amp4.ReadBoxStructure(r, func(h *amp4.ReadHandle) (any, error) {
		switch h.BoxInfo.Type {
		case amp4.BoxTypeMoof():
			moofOffset = h.BoxInfo.Offset
			return h.Expand()

		case amp4.BoxTypeTraf():
			return h.Expand()

		case amp4.BoxTypeTfhd():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfhd = box.(*amp4.Tfhd)
			return nil, nil

		case amp4.BoxTypeTfdt():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			tfdt = box.(*amp4.Tfdt)
			return nil, nil

		case amp4.BoxTypeTrun():
			box, _, err := h.ReadPayload()
			if err != nil {
				return nil, err
			}
			trun := box.(*amp4.Trun)

			if tfhd == nil || tfdt == nil {
				return nil, fmt.Errorf("trun without preceding tfhd/tfdt")
			}

			// The data offset in trun is relative to the moof box start.
			dataOffset := moofOffset + uint64(trun.DataOffset)

			// Determine base decode time with version awareness.
			var baseDecodeTime uint64
			if tfdt.FullBox.Version == 0 {
				baseDecodeTime = uint64(tfdt.BaseMediaDecodeTimeV0)
			} else {
				baseDecodeTime = tfdt.BaseMediaDecodeTimeV1
			}

			dts := baseDecodeTime
			trackID := tfhd.TrackID

			for _, e := range trun.Entries {
				// Read sample data from the file.
				payload := make([]byte, e.SampleSize)
				n, err := r.ReadAt(payload, int64(dataOffset))
				if err != nil {
					return nil, fmt.Errorf("read sample data at offset %d: %w", dataOffset, err)
				}
				if n != int(e.SampleSize) {
					return nil, fmt.Errorf("partial sample read: got %d, want %d", n, e.SampleSize)
				}

				// Keyframe detection: if the non-sync flag is NOT set, it's a sync frame.
				isSync := (e.SampleFlags & sampleFlagIsNonSyncSample) == 0

				sample := Sample{
					DTS:       dts,
					PTSOffset: e.SampleCompositionTimeOffsetV1,
					Duration:  e.SampleDuration,
					IsSync:    isSync,
					Data:      payload,
					TrackID:   trackID,
				}

				if err := cb(sample); err != nil {
					if errors.Is(err, errDone) {
						return nil, errDone
					}
					return nil, err
				}

				dataOffset += uint64(e.SampleSize)
				dts += uint64(e.SampleDuration)
			}
			return nil, nil

		default:
			return nil, nil
		}
	})
	if err != nil && !errors.Is(err, errDone) {
		return fmt.Errorf("read samples: %w", err)
	}
	return nil
}

// FindKeyframeBefore finds the DTS of the last keyframe at or before targetDTS
// across all tracks in the segment. Returns 0 if no keyframe is found before
// the target, or an error if the file cannot be read.
func FindKeyframeBefore(path string, targetDTS uint64) (uint64, error) {
	var bestDTS uint64
	found := false

	err := ReadSegmentSamples(path, func(s Sample) error {
		if s.IsSync && s.DTS <= targetDTS {
			if !found || s.DTS > bestDTS {
				bestDTS = s.DTS
				found = true
			}
		}
		// If we've passed the target and already have a keyframe, we could
		// keep going (there might be later tracks with earlier keyframes)
		// but for single-track video this is fine.
		return nil
	})
	if err != nil {
		return 0, err
	}

	if !found {
		return 0, fmt.Errorf("no keyframe found at or before DTS %d", targetDTS)
	}

	return bestDTS, nil
}
