package integrity

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// makeBox creates an MP4 box with the given 4-char type and payload.
func makeBox(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// makeMoov creates a minimal moov box with an mvhd child.
func makeMoov() []byte {
	mvhdPayload := make([]byte, 100)
	binary.BigEndian.PutUint32(mvhdPayload[8:12], 90000) // timescale
	mvhd := makeBox("mvhd", mvhdPayload)
	return makeBox("moov", mvhd)
}

// makeMoofMdat creates a moof+mdat pair with dummy data of given size.
func makeMoofMdat(mdatPayloadSize int) []byte {
	moof := makeBox("moof", []byte{0, 0, 0, 0})
	mdatPayload := make([]byte, mdatPayloadSize)
	mdat := makeBox("mdat", mdatPayload)
	result := make([]byte, 0, len(moof)+len(mdat))
	result = append(result, moof...)
	result = append(result, mdat...)
	return result
}

// writeValidFMP4 writes a minimal valid fMP4 file and returns the path.
func writeValidFMP4(t *testing.T, dir string) string {
	t.Helper()
	ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := makeMoov()
	frag1 := makeMoofMdat(100)
	frag2 := makeMoofMdat(200)

	data := make([]byte, 0, len(ftyp)+len(moov)+len(frag1)+len(frag2))
	data = append(data, ftyp...)
	data = append(data, moov...)
	data = append(data, frag1...)
	data = append(data, frag2...)

	path := filepath.Join(dir, "valid.mp4")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestVerifySegment_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := writeValidFMP4(t, dir)

	info, _ := os.Stat(path)
	ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := makeMoov()
	initSize := int64(len(ftyp) + len(moov))

	rec := RecordingInfo{
		FilePath:      path,
		FileSize:      info.Size(),
		InitSize:      initSize,
		FragmentCount: 2,
		DurationMs:    0,
	}

	result := VerifySegment(rec)
	if result.Status != StatusOK {
		t.Errorf("expected status ok, got %s: %s", result.Status, result.Detail)
	}
}

func TestVerifySegment_MissingFile(t *testing.T) {
	rec := RecordingInfo{
		FilePath: "/nonexistent/file.mp4",
		FileSize: 1000,
	}
	result := VerifySegment(rec)
	if result.Status != StatusCorrupted {
		t.Errorf("expected corrupted, got %s", result.Status)
	}
	if result.Detail != "file missing" {
		t.Errorf("unexpected detail: %s", result.Detail)
	}
}

func TestVerifySegment_SizeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeValidFMP4(t, dir)

	rec := RecordingInfo{
		FilePath: path,
		FileSize: 999999,
	}
	result := VerifySegment(rec)
	if result.Status != StatusCorrupted {
		t.Errorf("expected corrupted, got %s", result.Status)
	}
}

func TestVerifySegment_InvalidFtyp(t *testing.T) {
	dir := t.TempDir()
	data := makeBox("free", []byte("notftyp!"))
	path := filepath.Join(dir, "bad.mp4")
	os.WriteFile(path, data, 0o644)

	info, _ := os.Stat(path)
	rec := RecordingInfo{
		FilePath: path,
		FileSize: info.Size(),
	}
	result := VerifySegment(rec)
	if result.Status != StatusCorrupted {
		t.Errorf("expected corrupted, got %s", result.Status)
	}
}

func TestVerifySegment_TruncatedFile(t *testing.T) {
	dir := t.TempDir()
	ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := makeMoov()
	frag := makeMoofMdat(100)

	data := make([]byte, 0, len(ftyp)+len(moov)+len(frag)/2)
	data = append(data, ftyp...)
	data = append(data, moov...)
	data = append(data, frag[:len(frag)/2]...)

	path := filepath.Join(dir, "truncated.mp4")
	os.WriteFile(path, data, 0o644)

	info, _ := os.Stat(path)
	rec := RecordingInfo{
		FilePath: path,
		FileSize: info.Size(),
		InitSize: int64(len(ftyp) + len(moov)),
	}
	result := VerifySegment(rec)
	if result.Status != StatusCorrupted {
		t.Errorf("expected corrupted, got %s", result.Status)
	}
}

func TestVerifySegment_TrailingGarbage(t *testing.T) {
	dir := t.TempDir()
	ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := makeMoov()
	frag := makeMoofMdat(100)

	data := make([]byte, 0, len(ftyp)+len(moov)+len(frag)+10)
	data = append(data, ftyp...)
	data = append(data, moov...)
	data = append(data, frag...)
	data = append(data, []byte("extratrash")...)

	path := filepath.Join(dir, "trailing.mp4")
	os.WriteFile(path, data, 0o644)

	info, _ := os.Stat(path)
	rec := RecordingInfo{
		FilePath:      path,
		FileSize:      info.Size(),
		InitSize:      int64(len(ftyp) + len(moov)),
		FragmentCount: 1,
	}
	result := VerifySegment(rec)
	if result.Status != StatusCorrupted {
		t.Errorf("expected corrupted, got %s", result.Status)
	}
}

func TestVerifySegment_FragmentCountMismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeValidFMP4(t, dir)

	info, _ := os.Stat(path)
	ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := makeMoov()

	rec := RecordingInfo{
		FilePath:      path,
		FileSize:      info.Size(),
		InitSize:      int64(len(ftyp) + len(moov)),
		FragmentCount: 5,
	}
	result := VerifySegment(rec)
	if result.Status != StatusCorrupted {
		t.Errorf("expected corrupted, got %s", result.Status)
	}
}
