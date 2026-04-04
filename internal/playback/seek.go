package playback

import (
	"bytes"
	"fmt"
	"io"
	"time"

	amp4 "github.com/abema/go-mp4"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mcodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

// KeyframeInfo contains information about a keyframe found during seeking.
type KeyframeInfo struct {
	// TrackID is the ID of the track containing the keyframe.
	TrackID int
	// DTS is the decode timestamp in timescale units.
	DTS int64
	// DTSGo is the decode timestamp as a Go duration from the segment start.
	DTSGo time.Duration
	// MoofOffset is the byte offset of the moof box containing this keyframe.
	MoofOffset int64
}

// segmentFMP4FindKeyframesManual scans an fmp4 segment to find all keyframe
// positions for the given video track. It parses through moof/traf/trun boxes
// to identify sync samples (IDR frames).
func segmentFMP4FindKeyframesManual(
	r readSeekerAt,
	videoTrackID int,
	timeScale uint32,
) ([]KeyframeInfo, error) {
	_, err := r.(io.Seeker).Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 8)

	// skip ftyp
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return nil, fmt.Errorf("ftyp box not found")
	}
	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	_, err = r.(io.Seeker).Seek(int64(ftypSize), io.SeekStart)
	if err != nil {
		return nil, err
	}

	// skip moov
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return nil, fmt.Errorf("moov box not found")
	}
	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	_, err = r.(io.Seeker).Seek(int64(ftypSize)+int64(moovSize), io.SeekStart)
	if err != nil {
		return nil, err
	}

	var keyframes []KeyframeInfo

	// iterate over moof+mdat pairs
	for {
		moofPos, err := r.(io.Seeker).Seek(0, io.SeekCurrent)
		if err != nil {
			return keyframes, nil
		}

		_, err = io.ReadFull(r, buf)
		if err != nil {
			return keyframes, nil // EOF
		}

		if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'f'}) {
			return keyframes, nil
		}

		moofSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		moofEnd := moofPos + int64(moofSize)

		// skip mfhd
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return keyframes, nil
		}
		if !bytes.Equal(buf[4:], []byte{'m', 'f', 'h', 'd'}) {
			return keyframes, nil
		}
		mfhdSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		_, err = r.(io.Seeker).Seek(int64(mfhdSize)-8, io.SeekCurrent)
		if err != nil {
			return keyframes, nil
		}

		// iterate over traf boxes within this moof
		for {
			curPos, err := r.(io.Seeker).Seek(0, io.SeekCurrent)
			if err != nil || curPos >= moofEnd {
				break
			}

			_, err = io.ReadFull(r, buf)
			if err != nil {
				break
			}

			boxSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

			if bytes.Equal(buf[4:], []byte{'t', 'r', 'a', 'f'}) {
				kfs, err := parseTrafForKeyframes(r, videoTrackID, timeScale, moofPos)
				if err != nil {
					break
				}
				keyframes = append(keyframes, kfs...)
			} else {
				// skip non-traf box
				_, err = r.(io.Seeker).Seek(int64(boxSize)-8, io.SeekCurrent)
				if err != nil {
					break
				}
			}
		}

		// seek to end of moof
		_, err = r.(io.Seeker).Seek(moofEnd, io.SeekStart)
		if err != nil {
			return keyframes, nil
		}

		// skip mdat
		_, err = io.ReadFull(r, buf)
		if err != nil {
			return keyframes, nil
		}
		if !bytes.Equal(buf[4:], []byte{'m', 'd', 'a', 't'}) {
			return keyframes, nil
		}
		mdatSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
		_, err = r.(io.Seeker).Seek(int64(mdatSize)-8, io.SeekCurrent)
		if err != nil {
			return keyframes, nil
		}
	}
}

// parseTrafForKeyframes parses a traf box and returns keyframe info for the target track.
func parseTrafForKeyframes(
	r io.Reader,
	videoTrackID int,
	timeScale uint32,
	moofOffset int64,
) ([]KeyframeInfo, error) {
	buf := make([]byte, 8)

	// parse tfhd
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(buf[4:], []byte{'t', 'f', 'h', 'd'}) {
		return nil, fmt.Errorf("tfhd box not found")
	}
	tfhdSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	buf2 := make([]byte, tfhdSize-8)
	_, err = io.ReadFull(r, buf2)
	if err != nil {
		return nil, err
	}

	var tfhd amp4.Tfhd
	_, err = amp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfhd, amp4.Context{})
	if err != nil {
		return nil, fmt.Errorf("invalid tfhd: %w", err)
	}

	if int(tfhd.TrackID) != videoTrackID {
		return nil, nil // not our track, skip
	}

	// parse tfdt
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(buf[4:], []byte{'t', 'f', 'd', 't'}) {
		return nil, fmt.Errorf("tfdt box not found")
	}
	tfdtSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	buf2 = make([]byte, tfdtSize-8)
	_, err = io.ReadFull(r, buf2)
	if err != nil {
		return nil, err
	}

	var tfdt amp4.Tfdt
	_, err = amp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &tfdt, amp4.Context{})
	if err != nil {
		return nil, fmt.Errorf("invalid tfdt: %w", err)
	}

	// parse trun
	_, err = io.ReadFull(r, buf)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(buf[4:], []byte{'t', 'r', 'u', 'n'}) {
		return nil, fmt.Errorf("trun box not found")
	}
	trunSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	buf2 = make([]byte, trunSize-8)
	_, err = io.ReadFull(r, buf2)
	if err != nil {
		return nil, err
	}

	var trun amp4.Trun
	_, err = amp4.Unmarshal(bytes.NewReader(buf2), uint64(len(buf2)), &trun, amp4.Context{})
	if err != nil {
		return nil, fmt.Errorf("invalid trun: %w", err)
	}

	var keyframes []KeyframeInfo
	dts := int64(tfdt.BaseMediaDecodeTimeV1)

	for _, entry := range trun.Entries {
		if (entry.SampleFlags & sampleFlagIsNonSyncSample) == 0 {
			keyframes = append(keyframes, KeyframeInfo{
				TrackID:    videoTrackID,
				DTS:        dts,
				DTSGo:      durationMp4ToGo(dts, timeScale),
				MoofOffset: moofOffset,
			})
		}
		dts += int64(entry.SampleDuration)
	}

	return keyframes, nil
}

// findNearestKeyframeBefore returns the keyframe at or before the given offset
// from the segment start. If no keyframe is found at or before the offset,
// it returns the first keyframe.
func findNearestKeyframeBefore(keyframes []KeyframeInfo, offset time.Duration) (KeyframeInfo, bool) {
	if len(keyframes) == 0 {
		return KeyframeInfo{}, false
	}

	// Find the last keyframe with DTSGo <= offset
	best := keyframes[0]
	found := false

	for _, kf := range keyframes {
		if kf.DTSGo <= offset {
			best = kf
			found = true
		} else {
			break // keyframes are in order, so we can stop
		}
	}

	if !found {
		// All keyframes are after the offset; return the first one
		return keyframes[0], true
	}

	return best, true
}

// findVideoTrackID returns the track ID of the first video track in the init.
func findVideoTrackID(init *fmp4.Init) int {
	for _, track := range init.Tracks {
		if isVideoCodec(track.Codec) {
			return track.ID
		}
	}
	return 0
}

// isVideoCodec returns true if the codec is a video codec.
func isVideoCodec(codec fmp4.Codec) bool {
	switch codec.(type) {
	case *mcodecs.H264, *mcodecs.H265, *mcodecs.VP9, *mcodecs.AV1:
		return true
	default:
		return false
	}
}
