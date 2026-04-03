package backchannel

import "testing"

func TestRTPPackerG711(t *testing.T) {
	p := NewRTPPacker("G711", 8000)
	if p.PayloadType != 0 {
		t.Fatalf("expected PT 0 for G711 mu-law, got %d", p.PayloadType)
	}
	audio := make([]byte, 160) // 20ms of G.711 at 8kHz
	for i := range audio {
		audio[i] = byte(i)
	}
	pkt := p.Pack(audio)
	if pkt.Header.Version != 2 {
		t.Fatalf("expected RTP v2, got %d", pkt.Header.Version)
	}
	if pkt.Header.PayloadType != 0 {
		t.Fatalf("expected PT 0, got %d", pkt.Header.PayloadType)
	}
	if pkt.Header.SequenceNumber != 1 {
		t.Fatalf("expected seq 1, got %d", pkt.Header.SequenceNumber)
	}
	if len(pkt.Payload) != 160 {
		t.Fatalf("expected 160 bytes, got %d", len(pkt.Payload))
	}
}

func TestRTPPackerAAC(t *testing.T) {
	p := NewRTPPacker("AAC", 16000)
	if p.PayloadType != 96 {
		t.Fatalf("expected PT 96 for AAC, got %d", p.PayloadType)
	}
	pkt := p.Pack(make([]byte, 256))
	if pkt.Header.PayloadType != 96 {
		t.Fatalf("expected PT 96, got %d", pkt.Header.PayloadType)
	}
}

func TestRTPPackerSequenceIncrement(t *testing.T) {
	p := NewRTPPacker("G711", 8000)
	audio := make([]byte, 160)
	pkt1 := p.Pack(audio)
	pkt2 := p.Pack(audio)
	pkt3 := p.Pack(audio)
	if pkt1.Header.SequenceNumber != 1 {
		t.Fatalf("expected 1, got %d", pkt1.Header.SequenceNumber)
	}
	if pkt2.Header.SequenceNumber != 2 {
		t.Fatalf("expected 2, got %d", pkt2.Header.SequenceNumber)
	}
	if pkt3.Header.SequenceNumber != 3 {
		t.Fatalf("expected 3, got %d", pkt3.Header.SequenceNumber)
	}
}

func TestRTPPackerTimestampIncrement(t *testing.T) {
	p := NewRTPPacker("G711", 8000)
	audio := make([]byte, 160)
	pkt1 := p.Pack(audio)
	pkt2 := p.Pack(audio)
	if pkt1.Header.Timestamp != 0 {
		t.Fatalf("expected ts 0, got %d", pkt1.Header.Timestamp)
	}
	if pkt2.Header.Timestamp != 160 {
		t.Fatalf("expected ts 160, got %d", pkt2.Header.Timestamp)
	}
}

func TestRTPPackerG711ALaw(t *testing.T) {
	p := NewRTPPacker("G711a", 8000)
	if p.PayloadType != 8 {
		t.Fatalf("expected PT 8 for A-law, got %d", p.PayloadType)
	}
}

func TestRTPPackerAACTimestamp(t *testing.T) {
	p := NewRTPPacker("AAC", 16000)
	pkt1 := p.Pack(make([]byte, 200)) // compressed size doesn't matter
	pkt2 := p.Pack(make([]byte, 150))
	if pkt1.Header.Timestamp != 0 {
		t.Fatalf("expected 0, got %d", pkt1.Header.Timestamp)
	}
	if pkt2.Header.Timestamp != 1024 {
		t.Fatalf("expected 1024, got %d", pkt2.Header.Timestamp)
	}
}
