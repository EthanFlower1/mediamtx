package backchannel

import (
	"crypto/rand"
	"encoding/binary"
	"strings"
)

// RTPHeader represents the fixed 12-byte RTP header per RFC 3550.
type RTPHeader struct {
	Version        uint8
	Padding        bool
	Extension      bool
	Marker         bool
	PayloadType    uint8
	SequenceNumber uint16
	Timestamp      uint32
	SSRC           uint32
}

// RTPPacket holds an RTP header and its payload.
type RTPPacket struct {
	Header  RTPHeader
	Payload []byte
}

// Marshal serializes the RTPPacket into the 12-byte RTP header followed by the payload.
func (p *RTPPacket) Marshal() []byte {
	buf := make([]byte, 12+len(p.Payload))

	var padding, extension uint8
	if p.Header.Padding {
		padding = 1
	}
	if p.Header.Extension {
		extension = 1
	}
	buf[0] = (p.Header.Version << 6) | (padding << 5) | (extension << 4)

	var marker uint8
	if p.Header.Marker {
		marker = 1
	}
	buf[1] = (marker << 7) | (p.Header.PayloadType & 0x7F)

	binary.BigEndian.PutUint16(buf[2:4], p.Header.SequenceNumber)
	binary.BigEndian.PutUint32(buf[4:8], p.Header.Timestamp)
	binary.BigEndian.PutUint32(buf[8:12], p.Header.SSRC)

	copy(buf[12:], p.Payload)
	return buf
}

// RTPPacker builds RTP packets for a specific codec and sample rate.
type RTPPacker struct {
	PayloadType    uint8
	ClockRate      uint32
	sequenceNumber uint16
	timestamp      uint32
	ssrc           uint32
}

// NewRTPPacker creates an RTPPacker for the given codec and sample rate.
// Codec mapping: G711 (mu-law) -> PT 0, G711a (A-law) -> PT 8, AAC -> PT 96, default -> PT 96.
// SSRC is generated randomly via crypto/rand.
func NewRTPPacker(codec string, sampleRate int) *RTPPacker {
	upper := strings.ToUpper(codec)

	var pt uint8
	switch upper {
	case "G711":
		pt = 0
	case "G711A":
		pt = 8
	case "AAC":
		pt = 96
	default:
		pt = 96
	}

	var ssrcBytes [4]byte
	_, _ = rand.Read(ssrcBytes[:])
	ssrc := binary.BigEndian.Uint32(ssrcBytes[:])

	return &RTPPacker{
		PayloadType: pt,
		ClockRate:   uint32(sampleRate),
		ssrc:        ssrc,
	}
}

// Pack wraps audioData in an RTPPacket, incrementing sequence number and advancing the
// timestamp by the number of samples (len(audioData)) after creating the packet.
func (p *RTPPacker) Pack(audioData []byte) *RTPPacket {
	p.sequenceNumber++

	pkt := &RTPPacket{
		Header: RTPHeader{
			Version:        2,
			PayloadType:    p.PayloadType,
			SequenceNumber: p.sequenceNumber,
			Timestamp:      p.timestamp,
			SSRC:           p.ssrc,
		},
		Payload: audioData,
	}

	p.timestamp += uint32(len(audioData))

	return pkt
}
