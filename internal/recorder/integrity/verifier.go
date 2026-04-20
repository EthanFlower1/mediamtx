// Package integrity provides recording segment integrity verification.
package integrity

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	StatusOK          = "ok"
	StatusCorrupted   = "corrupted"
	StatusQuarantined = "quarantined"
	StatusUnverified  = "unverified"
)

// RecordingInfo contains the DB-side metadata needed for verification.
type RecordingInfo struct {
	FilePath      string
	FileSize      int64
	InitSize      int64
	FragmentCount int
	DurationMs    int64
}

// VerificationResult holds the outcome of verifying a single segment.
type VerificationResult struct {
	Status string // StatusOK or StatusCorrupted
	Detail string // empty if ok, failure reason if corrupted
}

// VerifySegment runs structural and metadata consistency checks on a recording segment.
// Checks are run in order and short-circuit on first failure:
// 1. File existence
// 2. File size match (vs DB)
// 3. ftyp box validation
// 4. moov box validation
// 5. Init size consistency (ftyp+moov vs DB init_size)
// 6. Fragment walk (iterate moof+mdat pairs, validate sizes)
// 7. Fragment count match (vs DB)
// 8. Duration consistency (basic check - 0 fragments with positive duration)
// 9. File completeness (no trailing garbage)
func VerifySegment(rec RecordingInfo) VerificationResult {
	// 1. File existence
	info, err := os.Stat(rec.FilePath)
	if err != nil {
		return VerificationResult{Status: StatusCorrupted, Detail: "file missing"}
	}

	// 2. File size match
	if rec.FileSize > 0 && info.Size() != rec.FileSize {
		return VerificationResult{
			Status: StatusCorrupted,
			Detail: fmt.Sprintf("size mismatch: db=%d file=%d", rec.FileSize, info.Size()),
		}
	}

	f, err := os.Open(rec.FilePath)
	if err != nil {
		return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot open file: %v", err)}
	}
	defer f.Close()

	fileSize := info.Size()

	// 3. ftyp box
	ftypSize, ftypType, err := readBoxHeader(f)
	if err != nil {
		return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read ftyp: %v", err)}
	}
	if ftypType != "ftyp" {
		return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("invalid ftyp box: got %q", ftypType)}
	}

	// 4. moov box
	if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
		return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("seek to moov failed: %v", err)}
	}
	moovSize, moovType, err := readBoxHeader(f)
	if err != nil {
		return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read moov: %v", err)}
	}
	if moovType != "moov" {
		return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("invalid/missing moov box: got %q", moovType)}
	}

	initSize := ftypSize + moovSize

	// 5. Init size consistency
	if rec.InitSize > 0 && initSize != rec.InitSize {
		return VerificationResult{
			Status: StatusCorrupted,
			Detail: fmt.Sprintf("init size mismatch: db=%d file=%d", rec.InitSize, initSize),
		}
	}

	// 6. Fragment walk
	pos := initSize
	fragmentCount := 0
	for pos < fileSize {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("seek failed at offset %d: %v", pos, err)}
		}

		moofSize, moofType, err := readBoxHeader(f)
		if err != nil {
			if pos == fileSize {
				break
			}
			return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read box at offset %d: %v", pos, err)}
		}

		// Skip non-moof boxes (e.g., Mtxi, free)
		if moofType != "moof" {
			if moofSize == 0 {
				break
			}
			pos += moofSize
			continue
		}

		// Read mdat after moof
		mdatPos := pos + moofSize
		if mdatPos >= fileSize {
			return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("truncated: moof at offset %d has no mdat", pos)}
		}
		if _, err := f.Seek(mdatPos, io.SeekStart); err != nil {
			return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("seek to mdat failed at offset %d: %v", mdatPos, err)}
		}
		mdatSize, mdatType, err := readBoxHeader(f)
		if err != nil {
			return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read mdat at offset %d: %v", mdatPos, err)}
		}
		if mdatType != "mdat" {
			return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("expected mdat at offset %d, got %q", mdatPos, mdatType)}
		}

		if mdatSize == 0 {
			mdatSize = fileSize - mdatPos
		}

		if mdatPos+mdatSize > fileSize {
			return VerificationResult{
				Status: StatusCorrupted,
				Detail: fmt.Sprintf("truncated mdat at offset %d: declares %d bytes but only %d remain", mdatPos, mdatSize, fileSize-mdatPos),
			}
		}

		fragmentCount++
		pos = mdatPos + mdatSize
	}

	// 7. Fragment count match
	if rec.FragmentCount > 0 && fragmentCount != rec.FragmentCount {
		return VerificationResult{
			Status: StatusCorrupted,
			Detail: fmt.Sprintf("fragment count mismatch: db=%d file=%d", rec.FragmentCount, fragmentCount),
		}
	}

	// 8. Duration consistency
	if rec.DurationMs > 0 && fragmentCount == 0 {
		return VerificationResult{
			Status: StatusCorrupted,
			Detail: fmt.Sprintf("duration mismatch: db=%dms but file has 0 fragments", rec.DurationMs),
		}
	}

	// 9. File completeness
	if pos != fileSize {
		trailing := fileSize - pos
		return VerificationResult{
			Status: StatusCorrupted,
			Detail: fmt.Sprintf("trailing garbage: %d bytes after last mdat", trailing),
		}
	}

	return VerificationResult{Status: StatusOK}
}

// readBoxHeader reads an MP4 box header (size + type).
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
	}

	return size, boxType, nil
}

// durationDriftExceeds checks if the drift between two duration values exceeds a threshold percentage.
func durationDriftExceeds(dbMs, fileMs int64, thresholdPct float64) bool {
	if dbMs == 0 {
		return false
	}
	drift := math.Abs(float64(dbMs-fileMs)) / float64(dbMs) * 100
	return drift > thresholdPct
}
