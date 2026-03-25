package api

import (
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// HLSHandler implements HTTP endpoints for HLS VOD playback of fMP4 recordings.
type HLSHandler struct {
	DB             *db.DB
	RecordingsPath string // base path for recordings, e.g. "./recordings"
}

// fragmentInfo describes a single moof+mdat pair inside an fMP4 file.
type fragmentInfo struct {
	offset int64
	size   int64
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
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
		return
	}

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

	// Extract JWT token from the request so we can pass it to segment URLs.
	token := c.Query("token")
	if token == "" {
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Build the m3u8 playlist.
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:7\n")
	b.WriteString("#EXT-X-TARGETDURATION:2\n")
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")

	first := true
	for _, rec := range recordings {
		initSize, fragments, scanErr := scanFragments(rec.FilePath)
		if scanErr != nil {
			// Skip files we cannot parse; they may be truncated or corrupt.
			continue
		}
		if len(fragments) == 0 {
			continue
		}

		// Build the segment URL from the file path.
		segmentURL := segmentURLFromFilePath(rec.FilePath, h.RecordingsPath, token)

		if !first {
			b.WriteString("#EXT-X-DISCONTINUITY\n")
		}
		first = false

		// EXT-X-MAP points at the init segment (ftyp + moov).
		b.WriteString(fmt.Sprintf("#EXT-X-MAP:URI=\"%s\",BYTERANGE=\"%d@0\"\n", segmentURL, initSize))

		// One entry per moof+mdat fragment.
		for _, frag := range fragments {
			b.WriteString(fmt.Sprintf("#EXTINF:1.0,\n"))
			b.WriteString(fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d\n", frag.size, frag.offset))
			b.WriteString(segmentURL + "\n")
		}
	}

	b.WriteString("#EXT-X-ENDLIST\n")

	c.Header("Content-Type", "application/vnd.apple.mpegurl")
	c.Header("Cache-Control", "no-cache")
	c.String(http.StatusOK, b.String())
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

// scanFragments reads an fMP4 file and returns the init segment size and a
// list of fragment (moof+mdat) offsets and sizes. It only reads box headers,
// never the payload data, so it is efficient even for large files.
func scanFragments(filePath string) (initSize int64, fragments []fragmentInfo, err error) {
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

	// Read ftyp box.
	ftypSize, ftypType, err := readBoxHeader(f)
	if err != nil {
		return 0, nil, fmt.Errorf("reading ftyp header: %w", err)
	}
	if ftypType != "ftyp" {
		return 0, nil, fmt.Errorf("expected ftyp box, got %q", ftypType)
	}

	// Seek past ftyp body.
	if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
		return 0, nil, err
	}

	// Read moov box.
	moovSize, moovType, err := readBoxHeader(f)
	if err != nil {
		return 0, nil, fmt.Errorf("reading moov header: %w", err)
	}
	if moovType != "moov" {
		return 0, nil, fmt.Errorf("expected moov box, got %q", moovType)
	}

	initSize = ftypSize + moovSize

	// Scan moof+mdat pairs.
	pos := initSize
	for pos < fileSize {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return 0, nil, err
		}

		moofSize, moofType, err := readBoxHeader(f)
		if err != nil {
			// Reached a truncated box at end of file; stop gracefully.
			break
		}
		if moofType != "moof" {
			// Unexpected box type; skip it and try to continue.
			if moofSize == 0 {
				break
			}
			pos += moofSize
			continue
		}

		// Read the mdat box header that follows moof.
		if _, err := f.Seek(pos+moofSize, io.SeekStart); err != nil {
			break
		}
		mdatSize, mdatType, err := readBoxHeader(f)
		if err != nil {
			break
		}
		if mdatType != "mdat" {
			// If the box after moof isn't mdat, something is off; skip the moof.
			pos += moofSize
			continue
		}

		// Handle mdat with size 0 (extends to end of file).
		if mdatSize == 0 {
			mdatSize = fileSize - (pos + moofSize)
		}

		fragments = append(fragments, fragmentInfo{
			offset: pos,
			size:   moofSize + mdatSize,
		})

		pos += moofSize + mdatSize
	}

	return initSize, fragments, nil
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
