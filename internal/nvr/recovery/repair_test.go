package recovery

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeBox creates an ISO BMFF box with the given 4-char type and payload.
func makeBox(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// makeFtyp returns a minimal ftyp box.
func makeFtyp() []byte {
	// ftyp payload: major_brand(4) + minor_version(4) + compatible_brands(4)
	payload := make([]byte, 12)
	copy(payload[0:4], "isom")
	return makeBox("ftyp", payload)
}

// makeMoov returns a minimal moov box.
func makeMoov() []byte {
	// Minimal mvhd inside moov. 108 bytes is the standard mvhd v0 size.
	mvhd := makeBox("mvhd", make([]byte, 100))
	return makeBox("moov", mvhd)
}

// makeMoof returns a minimal moof box with a mfhd + trun.
func makeMoof(seqNum uint32) []byte {
	// mfhd: version(1) + flags(3) + sequence_number(4) = 8 bytes
	mfhdPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(mfhdPayload[4:8], seqNum)
	mfhd := makeBox("mfhd", mfhdPayload)

	// trun with 1 sample: version(1) + flags(3) + sample_count(4) + sample_duration(4) + sample_size(4)
	trunPayload := make([]byte, 16)
	// flags: 0x000101 = sample-duration-present + sample-size-present
	trunPayload[3] = 0x01
	trunPayload[2] = 0x01
	binary.BigEndian.PutUint32(trunPayload[4:8], 1)    // sample_count
	binary.BigEndian.PutUint32(trunPayload[8:12], 1000) // sample_duration
	binary.BigEndian.PutUint32(trunPayload[12:16], 100) // sample_size

	tfhd := makeBox("tfhd", make([]byte, 4)) // minimal tfhd
	traf := makeBox("traf", append(tfhd, makeBox("trun", trunPayload)...))

	return makeBox("moof", append(mfhd, traf...))
}

// makeMdat returns an mdat box with the given payload size.
func makeMdat(payloadSize int) []byte {
	return makeBox("mdat", make([]byte, payloadSize))
}

// buildValidFMP4 creates a complete, valid fMP4 file with n fragments.
func buildValidFMP4(numFragments int) []byte {
	var data []byte
	data = append(data, makeFtyp()...)
	data = append(data, makeMoov()...)
	for i := 0; i < numFragments; i++ {
		data = append(data, makeMoof(uint32(i+1))...)
		data = append(data, makeMdat(256)...)
	}
	return data
}

// writeTempFile writes data to a temp file and returns the path.
func writeTempFile(t *testing.T, dir string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, "test.mp4")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func TestRepairCompleteFile(t *testing.T) {
	dir := t.TempDir()
	data := buildValidFMP4(2)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.AlreadyComplete)
	assert.False(t, result.Repaired)
	assert.False(t, result.Unrecoverable)
	assert.Equal(t, int64(len(data)), result.OriginalSize)
	assert.Equal(t, 2, result.FragmentsRecovered)
}

func TestRepairTruncatedMdat(t *testing.T) {
	dir := t.TempDir()
	data := buildValidFMP4(2)
	// Truncate 50 bytes off the end (into the second mdat).
	truncated := data[:len(data)-50]
	path := writeTempFile(t, dir, truncated)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Repaired)
	assert.Equal(t, 1, result.FragmentsRecovered)
	assert.Less(t, result.NewSize, result.OriginalSize)

	// Verify the file was actually truncated on disk.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, result.NewSize, info.Size())
}

func TestRepairTruncatedMoof(t *testing.T) {
	dir := t.TempDir()
	// Build 1 complete fragment, then append a partial moof.
	data := buildValidFMP4(1)
	partialMoof := makeMoof(2)[:10] // only first 10 bytes of moof
	data = append(data, partialMoof...)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Repaired)
	assert.Equal(t, 1, result.FragmentsRecovered)
}

func TestRepairNoFragments(t *testing.T) {
	dir := t.TempDir()
	// ftyp + moov only, no moof/mdat.
	var data []byte
	data = append(data, makeFtyp()...)
	data = append(data, makeMoov()...)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Unrecoverable)
	assert.Equal(t, 0, result.FragmentsRecovered)
}

func TestRepairTruncatedMoov(t *testing.T) {
	dir := t.TempDir()
	ftyp := makeFtyp()
	moov := makeMoov()
	// Truncate moov in half.
	data := append(ftyp, moov[:len(moov)/2]...)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Unrecoverable)
}

func TestRepairEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, []byte{})

	_, err := RepairSegment(path)
	assert.Error(t, err)
}

// makeExtendedBox creates an ISO BMFF box using the 64-bit extended size format.
func makeExtendedBox(boxType string, payload []byte) []byte {
	totalSize := uint64(16 + len(payload)) // 4(size=1) + 4(type) + 8(extended size) + payload
	buf := make([]byte, totalSize)
	binary.BigEndian.PutUint32(buf[0:4], 1) // size=1 signals extended size
	copy(buf[4:8], boxType)
	binary.BigEndian.PutUint64(buf[8:16], totalSize)
	copy(buf[16:], payload)
	return buf
}

func TestRepairExtendedSizeBoxes(t *testing.T) {
	dir := t.TempDir()
	// Build file with standard ftyp+moov, then extended-size moof+mdat.
	var data []byte
	data = append(data, makeFtyp()...)
	data = append(data, makeMoov()...)

	// moof with extended size header
	moofPayload := makeMoof(1)[8:] // strip the standard 8-byte header, keep payload
	data = append(data, makeExtendedBox("moof", moofPayload)...)

	// mdat with extended size header
	mdatPayload := make([]byte, 256)
	data = append(data, makeExtendedBox("mdat", mdatPayload)...)

	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.AlreadyComplete)
	assert.Equal(t, 1, result.FragmentsRecovered)
}

func TestRepairThreeFragmentsTruncatedFourth(t *testing.T) {
	dir := t.TempDir()
	data := buildValidFMP4(3)
	// Append a partial 4th fragment (incomplete mdat).
	data = append(data, makeMoof(4)...)
	data = append(data, makeMdat(256)[:20]...) // truncated mdat
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Repaired)
	assert.Equal(t, 3, result.FragmentsRecovered)
}

func TestRepairNotFMP4(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, []byte("this is not an fmp4 file at all"))

	_, err := RepairSegment(path)
	assert.Error(t, err)
}
