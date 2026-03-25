package api

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type playlistCacheEntry struct {
	playlist string
	cachedAt time.Time
}

const playlistCacheTTL = 30 * time.Second

// HLSHandler implements HTTP endpoints for HLS VOD playback of fMP4 recordings.
type HLSHandler struct {
	DB             *db.DB
	RecordingsPath string // base path for recordings, e.g. "./recordings"

	mu    sync.Mutex
	cache map[string]*playlistCacheEntry // key: "cameraId:date"
}

// FragmentInfo describes a single moof+mdat pair inside an fMP4 file.
type FragmentInfo struct {
	Offset     int64
	Size       int64
	DurationMs float64 // actual duration in milliseconds, from trun/tfhd
}

// ServePlaylist generates an HLS VOD playlist covering all recordings for a
// camera on a given date. The playlist uses byte-range addressing into the
// original fMP4 files so no transcoding or remuxing is needed.
//
// GET /vod/:cameraId/playlist.m3u8?date=YYYY-MM-DD
func (h *HLSHandler) ServePlaylist(c *gin.Context) {
	cameraID := c.Param("cameraId")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cameraId is required"})
		return
	}

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	dateStr := c.Query("date")
	date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
		return
	}

	cacheKey := cameraID + ":" + dateStr

	h.mu.Lock()
	if h.cache == nil {
		h.cache = make(map[string]*playlistCacheEntry)
	}
	if entry, ok := h.cache[cacheKey]; ok {
		isToday := dateStr == time.Now().Format("2006-01-02")
		if !isToday || time.Since(entry.cachedAt) < playlistCacheTTL {
			h.mu.Unlock()
			c.Header("Content-Type", "application/vnd.apple.mpegurl")
			c.Header("Cache-Control", "no-cache")
			c.String(http.StatusOK, entry.playlist)
			return
		}
	}
	h.mu.Unlock()

	start := date
	end := date.Add(24 * time.Hour)

	recordings, err := h.DB.QueryRecordings(cameraID, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
		return
	}

	if len(recordings) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no recordings found for this date"})
		return
	}

	token := c.Query("token")
	if token == "" {
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Two-pass: collect segment data first to compute maxDuration,
	// then write complete playlist with correct TARGETDURATION header.
	type playlistEntry struct {
		line string
	}
	var entries []playlistEntry
	var maxDuration float64
	first := true

	for _, rec := range recordings {
		// Try DB-backed fragments first.
		fragments, dbErr := h.DB.GetFragments(rec.ID)
		if dbErr != nil || len(fragments) == 0 {
			// Fallback: scan file directly (un-indexed recording).
			initSize, scannedFrags, scanErr := ScanFragments(rec.FilePath)
			if scanErr != nil || len(scannedFrags) == 0 {
				continue
			}

			segmentURL := segmentURLFromFilePath(rec.FilePath, h.RecordingsPath, token)
			if !first {
				entries = append(entries, playlistEntry{"#EXT-X-DISCONTINUITY\n"})
			}
			first = false
			entries = append(entries, playlistEntry{fmt.Sprintf("#EXT-X-MAP:URI=\"%s\",BYTERANGE=\"%d@0\"\n", segmentURL, initSize)})
			for _, frag := range scannedFrags {
				durSec := frag.DurationMs / 1000.0
				if durSec > maxDuration {
					maxDuration = durSec
				}
				entries = append(entries,
					playlistEntry{fmt.Sprintf("#EXTINF:%.6f,\n", durSec)},
					playlistEntry{fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d\n", frag.Size, frag.Offset)},
					playlistEntry{segmentURL + "\n"},
				)
			}
			continue
		}

		// DB-backed path: use pre-computed fragment metadata.
		segmentURL := segmentURLFromFilePath(rec.FilePath, h.RecordingsPath, token)
		if !first {
			entries = append(entries, playlistEntry{"#EXT-X-DISCONTINUITY\n"})
		}
		first = false

		initSize := rec.InitSize
		if initSize == 0 {
			initSize = 1024
		}
		entries = append(entries, playlistEntry{fmt.Sprintf("#EXT-X-MAP:URI=\"%s\",BYTERANGE=\"%d@0\"\n", segmentURL, initSize)})

		for _, frag := range fragments {
			durSec := frag.DurationMs / 1000.0
			if durSec > maxDuration {
				maxDuration = durSec
			}
			entries = append(entries,
				playlistEntry{fmt.Sprintf("#EXTINF:%.6f,\n", durSec)},
				playlistEntry{fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d\n", frag.Size, frag.ByteOffset)},
				playlistEntry{segmentURL + "\n"},
			)
		}
	}

	// Write complete playlist with correct header.
	targetDur := int(maxDuration) + 1
	if targetDur < 1 {
		targetDur = 2
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:7\n")
	b.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDur))
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	for _, e := range entries {
		b.WriteString(e.line)
	}
	b.WriteString("#EXT-X-ENDLIST\n")

	playlist := b.String()

	h.mu.Lock()
	h.cache[cacheKey] = &playlistCacheEntry{
		playlist: playlist,
		cachedAt: time.Now(),
	}
	h.mu.Unlock()

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-cache")
	c.String(http.StatusOK, playlist)
}

// InvalidateCache clears the playlist cache for a camera+date combo.
func (h *HLSHandler) InvalidateCache(cameraID, date string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cache != nil {
		delete(h.cache, cameraID+":"+date)
	}
}

// ServeThumbnail extracts a single JPEG frame from the nearest keyframe to the
// requested timestamp. Uses ffmpeg to decode the fragment.
//
// GET /vod/thumbnail?camera_id=X&time=RFC3339
func (h *HLSHandler) ServeThumbnail(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	timeStr := c.Query("time")
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time, expected RFC3339"})
		return
	}

	// Find the recording that contains this timestamp.
	recs, err := h.DB.QueryRecordings(cameraID, t.Add(-1*time.Second), t.Add(1*time.Second))
	if err != nil || len(recs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no recording at this time"})
		return
	}

	rec := recs[0]

	// Use ffmpeg to extract a single frame.
	startTime, _ := time.Parse("2006-01-02T15:04:05.000Z", rec.StartTime)
	offset := t.Sub(startTime)

	cmd := exec.CommandContext(c.Request.Context(),
		"ffmpeg",
		"-ss", fmt.Sprintf("%.3f", offset.Seconds()),
		"-i", rec.FilePath,
		"-frames:v", "1",
		"-f", "mjpeg",
		"-q:v", "5",
		"pipe:1",
	)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to extract thumbnail", err)
		return
	}

	c.Header("Content-Type", "image/jpeg")
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, "image/jpeg", stdout.Bytes())
}

// ServeSegment serves raw recording files with HTTP Range support for
// byte-range requests. The path is validated to prevent directory traversal.
//
// GET /vod/segments/*filepath
func (h *HLSHandler) ServeSegment(c *gin.Context) {
	reqPath := c.Param("filepath")
	if reqPath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filepath is required"})
		return
	}

	// Build full path and validate it doesn't escape the recordings directory.
	fullPath := filepath.Join(h.RecordingsPath, reqPath)
	absRecordings, err := filepath.Abs(h.RecordingsPath)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to resolve recordings path", err)
		return
	}
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to resolve file path", err)
		return
	}

	if !strings.HasPrefix(absPath, absRecordings+string(filepath.Separator)) && absPath != absRecordings {
		c.JSON(http.StatusForbidden, gin.H{"error": "path traversal not allowed"})
		return
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "segment not found"})
		return
	}

	http.ServeFile(c.Writer, c.Request, absPath)
}

// segmentURLFromFilePath converts a recording file path into a segment URL.
// It strips the recordings base prefix and builds:
//
//	/api/nvr/vod/segments/RELATIVE_PATH?jwt=TOKEN
func segmentURLFromFilePath(filePath, recordingsBase, token string) string {
	// The FilePath in the DB is stored as e.g. "./recordings/nvr/ad410/file.mp4".
	// Strip the leading "./recordings/" (or the recordingsBase) to get the
	// relative path for the URL.
	rel := filePath

	// Try stripping "./recordings/" prefix first (common storage pattern).
	if strings.HasPrefix(rel, "./recordings/") {
		rel = strings.TrimPrefix(rel, "./recordings/")
	} else if strings.HasPrefix(rel, "recordings/") {
		rel = strings.TrimPrefix(rel, "recordings/")
	} else {
		// Fall back: strip the configured base path.
		absBase, err1 := filepath.Abs(recordingsBase)
		absFile, err2 := filepath.Abs(filePath)
		if err1 == nil && err2 == nil {
			if r, err := filepath.Rel(absBase, absFile); err == nil {
				rel = r
			}
		}
	}

	url := "/api/nvr/vod/segments/" + rel
	if token != "" {
		url += "?jwt=" + token
	}
	return url
}

// ScanFragments reads an fMP4 file and returns the init segment size and a
// list of fragment (moof+mdat) offsets, sizes, and real durations. It reads
// box headers and trun/tfhd timing data to produce accurate fragment durations.
func ScanFragments(filePath string) (initSize int64, fragments []FragmentInfo, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}
	fileSize := info.Size()

	ftypSize, ftypType, err := readBoxHeader(f)
	if err != nil {
		return 0, nil, fmt.Errorf("reading ftyp header: %w", err)
	}
	if ftypType != "ftyp" {
		return 0, nil, fmt.Errorf("expected ftyp box, got %q", ftypType)
	}

	if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
		return 0, nil, err
	}

	moovSize, moovType, err := readBoxHeader(f)
	if err != nil {
		return 0, nil, fmt.Errorf("reading moov header: %w", err)
	}
	if moovType != "moov" {
		return 0, nil, fmt.Errorf("expected moov box, got %q", moovType)
	}

	initSize = ftypSize + moovSize

	// Extract timescale from mvhd inside moov.
	timescale, err := readTimescale(f, ftypSize, moovSize)
	if err != nil {
		timescale = 1000
	}

	pos := initSize
	for pos < fileSize {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, nil, err
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

		fragments = append(fragments, FragmentInfo{
			Offset:     pos,
			Size:       moofSize + mdatSize,
			DurationMs: durationMs,
		})

		pos += moofSize + mdatSize
	}

	return initSize, fragments, nil
}

// readTimescale reads the timescale from the mvhd box inside moov.
func readTimescale(f io.ReadSeeker, moovStart, moovSize int64) (uint32, error) {
	pos := moovStart + 8 // skip moov container header to scan children
	end := moovStart + moovSize
	for pos < end {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, err
		}
		boxSize, boxType, err := readBoxHeader(f)
		if err != nil {
			return 0, err
		}
		if boxType == "mvhd" {
			// mvhd: version(1) + flags(3) + creation(4/8) + modification(4/8) + timescale(4)
			var ver [1]byte
			if _, err := io.ReadFull(f, ver[:]); err != nil {
				return 0, err
			}
			// Skip flags (3 bytes)
			if _, err := f.Seek(3, io.SeekCurrent); err != nil {
				return 0, err
			}
			if ver[0] == 0 {
				// Skip creation_time(4) + modification_time(4)
				if _, err := f.Seek(8, io.SeekCurrent); err != nil {
					return 0, err
				}
			} else {
				// Skip creation_time(8) + modification_time(8)
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
		if boxSize == 0 {
			break
		}
		pos += boxSize
	}
	return 0, fmt.Errorf("mvhd not found in moov")
}

// readFragmentDuration reads the total sample duration from a moof box.
func readFragmentDuration(f io.ReadSeeker, moofStart, moofSize int64, timescale uint32) (float64, error) {
	pos := moofStart + 8 // skip moof header
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
			// Parse ONLY the first traf (video track). Multi-track moofs have
			// interleaved video+audio trafs with different timescales; summing
			// both produces incorrect durations.
			trafEnd := pos + boxSize
			childPos := pos + 8 // skip traf header
			for childPos < trafEnd {
				if _, err := f.Seek(childPos, io.SeekStart); err != nil {
					return 0, err
				}
				childSize, childType, err := readBoxHeader(f)
				if err != nil {
					break
				}

				if childType == "tfhd" {
					// version(1) + flags(3)
					var vf [4]byte
					if _, err := io.ReadFull(f, vf[:]); err != nil {
						break
					}
					flags := uint32(vf[1])<<16 | uint32(vf[2])<<8 | uint32(vf[3])
					// Skip track_id (4 bytes)
					if _, err := f.Seek(4, io.SeekCurrent); err != nil {
						break
					}
					if flags&0x000001 != 0 { // base-data-offset-present
						if _, err := f.Seek(8, io.SeekCurrent); err != nil {
							break
						}
					}
					if flags&0x000002 != 0 { // sample-description-index-present
						if _, err := f.Seek(4, io.SeekCurrent); err != nil {
							break
						}
					}
					if flags&0x000008 != 0 { // default-sample-duration-present
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

					if flags&0x000001 != 0 { // data-offset-present
						if _, err := f.Seek(4, io.SeekCurrent); err != nil {
							break
						}
					}
					if flags&0x000004 != 0 { // first-sample-flags-present
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
			// Only use the first traf — break out of the moof child loop.
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

	durationMs := float64(totalDuration) / float64(timescale) * 1000.0
	return durationMs, nil
}

// readBoxHeader reads an fMP4/ISO BMFF box header from the current position
// of r and returns the total box size (including header) and the 4-character
// box type. It handles both normal (32-bit) and extended (64-bit) sizes.
func readBoxHeader(r io.ReadSeeker) (size int64, boxType string, err error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, "", err
	}

	size = int64(binary.BigEndian.Uint32(hdr[0:4]))
	boxType = string(hdr[4:8])

	if size == 1 {
		// Extended size: next 8 bytes are the real size (uint64).
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return 0, "", err
		}
		size = int64(binary.BigEndian.Uint64(ext[:]))
	}
	// size == 0 means the box extends to end of file; caller handles this.

	return size, boxType, nil
}
