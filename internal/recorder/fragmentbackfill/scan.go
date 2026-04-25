// Package fragmentbackfill ports the legacy NVR fragment-backfill goroutine.
// It scans recordings that lack fragment metadata and writes indexed
// fragment rows to the database so playback can use byte-range requests.
package fragmentbackfill

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// FragmentInfo describes a single moof+mdat pair within an fMP4 file.
type FragmentInfo struct {
	Offset     int64
	Size       int64
	DurationMs float64
}

// scanFile opens filePath, parses its fMP4 box structure, and returns the
// init-segment size (ftyp+moov bytes) and a slice of fragment descriptors.
// Ported from internal/nvr/api/hls.go (deleted in commit 86569ce37).
func scanFile(filePath string) (ScanResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return ScanResult{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return ScanResult{}, err
	}
	fileSize := info.Size()

	ftypSize, ftypType, err := readBoxHeader(f)
	if err != nil {
		return ScanResult{}, fmt.Errorf("reading ftyp header: %w", err)
	}
	if ftypType != "ftyp" {
		return ScanResult{}, fmt.Errorf("expected ftyp box, got %q", ftypType)
	}

	if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
		return ScanResult{}, err
	}

	moovSize, moovType, err := readBoxHeader(f)
	if err != nil {
		return ScanResult{}, fmt.Errorf("reading moov header: %w", err)
	}
	if moovType != "moov" {
		return ScanResult{}, fmt.Errorf("expected moov box, got %q", moovType)
	}

	initSize := ftypSize + moovSize

	// Extract timescale from mdhd inside moov.
	timescale, err := readTimescale(f, ftypSize, moovSize)
	if err != nil {
		timescale = 1000
	}

	var fragments []FragmentInfo
	pos := initSize

	for pos < fileSize {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return ScanResult{}, err
		}

		moofSize, moofType, err := readBoxHeader(f)
		if err != nil {
			break
		}
		if moofType != "moof" {
			if moofSize == 0 {
				break
			}
			pos += moofSize
			continue
		}

		durationMs, durErr := readFragmentDuration(f, pos, moofSize, timescale)
		if durErr != nil {
			durationMs = 1000.0
		}

		if _, err := f.Seek(pos+moofSize, io.SeekStart); err != nil {
			break
		}
		mdatSize, mdatType, err := readBoxHeader(f)
		if err != nil {
			break
		}
		if mdatType != "mdat" {
			pos += moofSize
			continue
		}

		if mdatSize == 0 {
			mdatSize = fileSize - (pos + moofSize)
		}

		totalPair := moofSize + mdatSize
		if totalPair < 0 || pos+totalPair > fileSize {
			break
		}

		fragments = append(fragments, FragmentInfo{
			Offset:     pos,
			Size:       totalPair,
			DurationMs: durationMs,
		})

		pos += totalPair
	}

	return ScanResult{InitSize: initSize, Fragments: fragments}, nil
}

// readTimescale reads the video track's timescale from mdhd inside
// moov → trak → mdia → mdhd.
func readTimescale(f io.ReadSeeker, moovStart, moovSize int64) (uint32, error) {
	pos := moovStart + 8
	end := moovStart + moovSize
	for pos < end {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, err
		}
		boxSize, boxType, err := readBoxHeader(f)
		if err != nil || boxSize == 0 {
			break
		}
		if boxType == "trak" {
			ts, err := readMdhdTimescale(f, pos, boxSize)
			if err == nil {
				return ts, nil
			}
		}
		pos += boxSize
	}
	return 0, fmt.Errorf("mdhd not found in moov")
}

// readMdhdTimescale reads the timescale from mdhd inside a trak box.
func readMdhdTimescale(f io.ReadSeeker, trakStart, trakSize int64) (uint32, error) {
	pos := trakStart + 8
	end := trakStart + trakSize
	for pos < end {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, err
		}
		boxSize, boxType, err := readBoxHeader(f)
		if err != nil || boxSize == 0 {
			break
		}
		if boxType == "mdia" {
			mPos := pos + 8
			mEnd := pos + boxSize
			for mPos < mEnd {
				if _, err := f.Seek(mPos, io.SeekStart); err != nil {
					return 0, err
				}
				mBoxSize, mBoxType, err := readBoxHeader(f)
				if err != nil || mBoxSize == 0 {
					break
				}
				if mBoxType == "mdhd" {
					var ver [1]byte
					if _, err := io.ReadFull(f, ver[:]); err != nil {
						return 0, err
					}
					if _, err := f.Seek(3, io.SeekCurrent); err != nil {
						return 0, err
					}
					if ver[0] == 0 {
						if _, err := f.Seek(8, io.SeekCurrent); err != nil {
							return 0, err
						}
					} else {
						if _, err := f.Seek(16, io.SeekCurrent); err != nil {
							return 0, err
						}
					}
					var ts [4]byte
					if _, err := io.ReadFull(f, ts[:]); err != nil {
						return 0, err
					}
					return binary.BigEndian.Uint32(ts[:]), nil
				}
				mPos += mBoxSize
			}
		}
		pos += boxSize
	}
	return 0, fmt.Errorf("mdhd not found in trak")
}

// readFragmentDuration reads total sample duration from the first traf in a moof box.
func readFragmentDuration(f io.ReadSeeker, moofStart, moofSize int64, timescale uint32) (float64, error) {
	pos := moofStart + 8
	end := moofStart + moofSize

	var defaultSampleDuration uint32
	var totalDuration uint64

	for pos < end {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, err
		}
		boxSize, boxType, err := readBoxHeader(f)
		if err != nil {
			return 0, err
		}

		if boxType == "traf" {
			trafEnd := pos + boxSize
			childPos := pos + 8
			for childPos < trafEnd {
				if _, err := f.Seek(childPos, io.SeekStart); err != nil {
					return 0, err
				}
				childSize, childType, err := readBoxHeader(f)
				if err != nil {
					break
				}

				if childType == "tfhd" {
					var vf [4]byte
					if _, err := io.ReadFull(f, vf[:]); err != nil {
						break
					}
					flags := uint32(vf[1])<<16 | uint32(vf[2])<<8 | uint32(vf[3])
					if _, err := f.Seek(4, io.SeekCurrent); err != nil {
						break
					}
					if flags&0x000001 != 0 {
						if _, err := f.Seek(8, io.SeekCurrent); err != nil {
							break
						}
					}
					if flags&0x000002 != 0 {
						if _, err := f.Seek(4, io.SeekCurrent); err != nil {
							break
						}
					}
					if flags&0x000008 != 0 {
						var dur [4]byte
						if _, err := io.ReadFull(f, dur[:]); err != nil {
							break
						}
						defaultSampleDuration = binary.BigEndian.Uint32(dur[:])
					}
				}

				if childType == "trun" {
					var vf [4]byte
					if _, err := io.ReadFull(f, vf[:]); err != nil {
						break
					}
					flags := uint32(vf[1])<<16 | uint32(vf[2])<<8 | uint32(vf[3])
					var sc [4]byte
					if _, err := io.ReadFull(f, sc[:]); err != nil {
						break
					}
					sampleCount := binary.BigEndian.Uint32(sc[:])

					if flags&0x000001 != 0 {
						if _, err := f.Seek(4, io.SeekCurrent); err != nil {
							break
						}
					}
					if flags&0x000004 != 0 {
						if _, err := f.Seek(4, io.SeekCurrent); err != nil {
							break
						}
					}

					hasDuration := flags&0x000100 != 0
					hasSize := flags&0x000200 != 0
					hasFlags := flags&0x000400 != 0
					hasCTO := flags&0x000800 != 0

					for i := uint32(0); i < sampleCount; i++ {
						if hasDuration {
							var d [4]byte
							if _, err := io.ReadFull(f, d[:]); err != nil {
								break
							}
							totalDuration += uint64(binary.BigEndian.Uint32(d[:]))
						} else {
							totalDuration += uint64(defaultSampleDuration)
						}
						if hasSize {
							if _, err := f.Seek(4, io.SeekCurrent); err != nil {
								break
							}
						}
						if hasFlags {
							if _, err := f.Seek(4, io.SeekCurrent); err != nil {
								break
							}
						}
						if hasCTO {
							if _, err := f.Seek(4, io.SeekCurrent); err != nil {
								break
							}
						}
					}
				}

				if childSize == 0 {
					break
				}
				childPos += childSize
			}
			// Only parse the first traf (video track).
			break
		}

		if boxSize == 0 {
			break
		}
		pos += boxSize
	}

	if timescale == 0 {
		return 0, fmt.Errorf("timescale is zero")
	}
	return float64(totalDuration) / float64(timescale) * 1000.0, nil
}

// readBoxHeader reads an ISO BMFF box header (size + type). Handles both
// 32-bit and 64-bit extended sizes.
func readBoxHeader(r io.ReadSeeker) (size int64, boxType string, err error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, "", err
	}

	size = int64(binary.BigEndian.Uint32(hdr[0:4]))
	boxType = string(hdr[4:8])

	if size == 1 {
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, "", err
		}
		size = int64(binary.BigEndian.Uint64(ext[:]))
		if size < 0 {
			return 0, "", fmt.Errorf("box size overflow")
		}
	}

	return size, boxType, nil
}
