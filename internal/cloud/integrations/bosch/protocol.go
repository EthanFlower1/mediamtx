package bosch

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Mode2 protocol constants for Bosch B/G-Series panels.
// The Mode2 (automation) protocol uses a simple binary frame format over TCP.
const (
	// Frame markers
	frameSTX byte = 0x02 // start of frame
	frameETX byte = 0x03 // end of frame

	// Command types (panel -> host)
	cmdEventReport   byte = 0x01
	cmdZoneStatus    byte = 0x02
	cmdAreaStatus    byte = 0x03
	cmdOutputStatus  byte = 0x04
	cmdPanelStatus   byte = 0x05
	cmdHeartbeat     byte = 0x06
	cmdAuthChallenge byte = 0x10
	cmdAuthResult    byte = 0x11

	// Command types (host -> panel)
	cmdAck         byte = 0x80
	cmdNack        byte = 0x81
	cmdAuthReply   byte = 0x90
	cmdArmArea     byte = 0xA0
	cmdDisarmArea  byte = 0xA1
	cmdSetOutput   byte = 0xA2
	cmdRequestSync byte = 0xA3
	cmdPing        byte = 0xA4

	// Max frame payload size (protocol limit).
	maxPayloadSize = 512
)

var (
	ErrFrameTooShort  = errors.New("bosch: frame too short")
	ErrFrameNoSTX     = errors.New("bosch: missing STX marker")
	ErrFrameNoETX     = errors.New("bosch: missing ETX marker")
	ErrFrameChecksum  = errors.New("bosch: checksum mismatch")
	ErrPayloadTooLong = errors.New("bosch: payload exceeds max size")
)

// Frame represents a single Mode2 protocol frame.
//
// Wire format:
//   [STX][LEN_HI][LEN_LO][CMD][PAYLOAD...][CHECKSUM][ETX]
//
// LEN is the total byte count from CMD through CHECKSUM (inclusive).
// CHECKSUM is the XOR of all bytes from LEN_HI through the last payload byte.
type Frame struct {
	Command byte
	Payload []byte
}

// MarshalBinary encodes a Frame into the Mode2 wire format.
func (f *Frame) MarshalBinary() ([]byte, error) {
	if len(f.Payload) > maxPayloadSize {
		return nil, ErrPayloadTooLong
	}

	// LEN = 1 (cmd) + len(payload) + 1 (checksum)
	frameLen := uint16(1 + len(f.Payload) + 1)

	buf := make([]byte, 0, 4+len(f.Payload)+2) // STX + LEN(2) + CMD + payload + CHKSUM + ETX
	buf = append(buf, frameSTX)
	buf = append(buf, byte(frameLen>>8), byte(frameLen&0xFF))
	buf = append(buf, f.Command)
	buf = append(buf, f.Payload...)

	// Checksum: XOR of LEN_HI, LEN_LO, CMD, and all payload bytes.
	var chk byte
	for _, b := range buf[1:] { // skip STX
		chk ^= b
	}
	buf = append(buf, chk)
	buf = append(buf, frameETX)

	return buf, nil
}

// UnmarshalFrame parses a Mode2 wire frame from raw bytes.
// The input must be a complete frame from STX to ETX inclusive.
func UnmarshalFrame(data []byte) (*Frame, error) {
	if len(data) < 6 { // STX + LEN(2) + CMD + CHKSUM + ETX minimum
		return nil, ErrFrameTooShort
	}
	if data[0] != frameSTX {
		return nil, ErrFrameNoSTX
	}
	if data[len(data)-1] != frameETX {
		return nil, ErrFrameNoETX
	}

	frameLen := int(binary.BigEndian.Uint16(data[1:3]))
	// frameLen covers CMD + payload + CHKSUM
	expectedTotal := 3 + frameLen + 1 // STX + LEN(2) + frameLen + ETX
	if len(data) < expectedTotal {
		return nil, ErrFrameTooShort
	}

	// Verify checksum.
	var chk byte
	for _, b := range data[1 : 3+frameLen-1] { // LEN bytes through last payload byte
		chk ^= b
	}
	gotChk := data[3+frameLen-1]
	if chk != gotChk {
		return nil, fmt.Errorf("%w: expected 0x%02X got 0x%02X", ErrFrameChecksum, chk, gotChk)
	}

	f := &Frame{
		Command: data[3],
	}
	if frameLen > 2 { // has payload bytes beyond CMD and CHKSUM
		f.Payload = make([]byte, frameLen-2)
		copy(f.Payload, data[4:4+frameLen-2])
	}
	return f, nil
}

// BuildAck constructs an acknowledgment frame for a received command.
func BuildAck(seqByte byte) *Frame {
	return &Frame{
		Command: cmdAck,
		Payload: []byte{seqByte},
	}
}

// BuildAuthReply constructs an authentication response frame.
func BuildAuthReply(passcode string) *Frame {
	payload := []byte(passcode)
	return &Frame{
		Command: cmdAuthReply,
		Payload: payload,
	}
}

// BuildPing constructs a keepalive ping frame.
func BuildPing() *Frame {
	return &Frame{
		Command: cmdPing,
	}
}
