package recordstore

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
)

// ReadNTPFromFile opens an fMP4 file and extracts the NTP timestamp from
// the mtxi custom box in the init header. Returns time.Time{} (zero) if
// the mtxi box is not present (legacy recordings).
func ReadNTPFromFile(filePath string) (time.Time, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	return ReadNTP(f)
}

// ReadNTP extracts the NTP timestamp from an fMP4 stream's mtxi box.
func ReadNTP(r io.ReadSeeker) (time.Time, error) {
	buf := make([]byte, 8)

	// Read ftyp box header.
	if _, err := io.ReadFull(r, buf); err != nil {
		return time.Time{}, fmt.Errorf("reading ftyp header: %w", err)
	}
	if !bytes.Equal(buf[4:], []byte{'f', 't', 'y', 'p'}) {
		return time.Time{}, fmt.Errorf("expected ftyp box, got %q", string(buf[4:]))
	}
	ftypSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	// Seek past ftyp to moov.
	if _, err := r.Seek(int64(ftypSize), io.SeekStart); err != nil {
		return time.Time{}, err
	}

	// Read moov box header.
	if _, err := io.ReadFull(r, buf); err != nil {
		return time.Time{}, fmt.Errorf("reading moov header: %w", err)
	}
	if !bytes.Equal(buf[4:], []byte{'m', 'o', 'o', 'v'}) {
		return time.Time{}, fmt.Errorf("expected moov box, got %q", string(buf[4:]))
	}
	moovSize := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])

	// Read ftyp+moov bytes and parse as fmp4 init.
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return time.Time{}, err
	}
	initBuf := make([]byte, uint64(ftypSize+moovSize))
	if _, err := io.ReadFull(r, initBuf); err != nil {
		return time.Time{}, fmt.Errorf("reading init: %w", err)
	}

	var init fmp4.Init
	if err := init.Unmarshal(bytes.NewReader(initBuf)); err != nil {
		return time.Time{}, fmt.Errorf("parsing init: %w", err)
	}

	// Find the mtxi box in user data.
	for _, box := range init.UserData {
		if mtxi, ok := box.(*Mtxi); ok {
			if mtxi.NTP == 0 {
				return time.Time{}, fmt.Errorf("mtxi box has zero NTP")
			}
			return time.Unix(0, mtxi.NTP), nil
		}
	}

	return time.Time{}, fmt.Errorf("mtxi box not found")
}
