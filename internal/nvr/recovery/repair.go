package recovery

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// RepairResult describes the outcome of repairing an fMP4 segment.
type RepairResult struct {
	Repaired           bool   // File was truncated to recover data
	AlreadyComplete    bool   // File was already structurally complete
	Unrecoverable      bool   // No complete fragments; file cannot be repaired
	OriginalSize       int64  // File size before repair
	NewSize            int64  // File size after repair (== OriginalSize if not repaired)
	FragmentsRecovered int    // Number of complete moof+mdat pairs found
	Detail             string // Human-readable description of what happened
}

// RepairSegment inspects an fMP4 file and truncates it to the last complete
// moof+mdat pair if the file is incomplete. This recovers data from segments
// that were being written when the process was killed.
func RepairSegment(path string) (RepairResult, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return RepairResult{}, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return RepairResult{}, fmt.Errorf("stat file: %w", err)
	}
	fileSize := info.Size()

	if fileSize < 8 {
		return RepairResult{}, fmt.Errorf("file too small (%d bytes)", fileSize)
	}

	// Validate ftyp box.
	ftypSize, ftypType, err := readBoxHeader(f)
	if err != nil {
		return RepairResult{}, fmt.Errorf("reading ftyp: %w", err)
	}
	if ftypType != "ftyp" {
		return RepairResult{}, fmt.Errorf("not an fMP4 file: first box is %q, expected ftyp", ftypType)
	}

	// Skip to moov box.
	if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
		return RepairResult{}, err
	}

	moovSize, moovType, err := readBoxHeader(f)
	if err != nil {
		return RepairResult{Unrecoverable: true, OriginalSize: fileSize,
			Detail: "truncated before moov box"}, nil
	}
	if moovType != "moov" {
		return RepairResult{Unrecoverable: true, OriginalSize: fileSize,
			Detail: fmt.Sprintf("expected moov box, got %q", moovType)}, nil
	}

	initEnd := ftypSize + moovSize
	if initEnd > fileSize {
		return RepairResult{Unrecoverable: true, OriginalSize: fileSize,
			Detail: "moov box extends beyond file"}, nil
	}

	// Walk moof+mdat pairs.
	pos := initEnd
	lastCompleteEnd := initEnd // end of last complete moof+mdat pair
	fragments := 0

	for pos < fileSize {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			break
		}

		moofSize, moofType, err := readBoxHeader(f)
		if err != nil {
			break // truncated box header — stop walking
		}
		if moofType != "moof" {
			break // unexpected box type — stop walking
		}
		if pos+moofSize > fileSize {
			break // moof extends beyond file
		}

		// Read mdat header after moof.
		if _, err := f.Seek(pos+moofSize, io.SeekStart); err != nil {
			break
		}
		mdatSize, mdatType, err := readBoxHeader(f)
		if err != nil {
			break // truncated mdat header
		}
		if mdatType != "mdat" {
			break // expected mdat after moof
		}

		// Handle mdat with size 0 (extends to EOF).
		if mdatSize == 0 {
			mdatSize = fileSize - (pos + moofSize)
		}

		pairEnd := pos + moofSize + mdatSize
		if pairEnd > fileSize {
			break // mdat extends beyond file — incomplete pair
		}

		fragments++
		lastCompleteEnd = pairEnd
		pos = pairEnd
	}

	result := RepairResult{
		OriginalSize:       fileSize,
		FragmentsRecovered: fragments,
	}

	if fragments == 0 {
		result.Unrecoverable = true
		result.Detail = "no complete moof+mdat pairs after moov"
		return result, nil
	}

	if lastCompleteEnd == fileSize {
		result.AlreadyComplete = true
		result.NewSize = fileSize
		result.Detail = fmt.Sprintf("file complete with %d fragments", fragments)
		return result, nil
	}

	// Truncate the file to the last complete pair.
	if err := f.Truncate(lastCompleteEnd); err != nil {
		return RepairResult{}, fmt.Errorf("truncate file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return RepairResult{}, fmt.Errorf("sync file: %w", err)
	}

	result.Repaired = true
	result.NewSize = lastCompleteEnd
	result.Detail = fmt.Sprintf("truncated from %d to %d bytes, recovered %d fragments",
		fileSize, lastCompleteEnd, fragments)
	return result, nil
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
