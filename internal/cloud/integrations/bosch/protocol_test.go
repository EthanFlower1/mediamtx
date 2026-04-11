package bosch

import (
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		frame   Frame
	}{
		{
			name:  "ack with seq byte",
			frame: *BuildAck(0x42),
		},
		{
			name:  "ping empty payload",
			frame: *BuildPing(),
		},
		{
			name: "auth reply with passcode",
			frame: *BuildAuthReply("1234"),
		},
		{
			name: "event report payload",
			frame: Frame{
				Command: cmdEventReport,
				Payload: []byte{0x01, 0x01, 0x30, 0x00, 0x05, 0x01, 0x00, 0x00, 0x03},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.frame.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary: %v", err)
			}

			got, err := UnmarshalFrame(data)
			if err != nil {
				t.Fatalf("UnmarshalFrame: %v", err)
			}

			if got.Command != tc.frame.Command {
				t.Errorf("command: got 0x%02X want 0x%02X", got.Command, tc.frame.Command)
			}

			if len(got.Payload) != len(tc.frame.Payload) {
				t.Fatalf("payload length: got %d want %d", len(got.Payload), len(tc.frame.Payload))
			}

			for i := range got.Payload {
				if got.Payload[i] != tc.frame.Payload[i] {
					t.Errorf("payload[%d]: got 0x%02X want 0x%02X", i, got.Payload[i], tc.frame.Payload[i])
				}
			}
		})
	}
}

func TestUnmarshalFrame_Errors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{
			name: "too short",
			data: []byte{0x02, 0x00},
			want: ErrFrameTooShort,
		},
		{
			name: "missing STX",
			data: []byte{0xFF, 0x00, 0x02, 0xA4, 0xA6, 0x03},
			want: ErrFrameNoSTX,
		},
		{
			name: "missing ETX",
			data: []byte{0x02, 0x00, 0x02, 0xA4, 0xA6, 0xFF},
			want: ErrFrameNoETX,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UnmarshalFrame(tc.data)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestFramePayloadTooLong(t *testing.T) {
	f := &Frame{
		Command: cmdEventReport,
		Payload: make([]byte, maxPayloadSize+1),
	}
	_, err := f.MarshalBinary()
	if err != ErrPayloadTooLong {
		t.Errorf("expected ErrPayloadTooLong, got %v", err)
	}
}

func TestBuildAck(t *testing.T) {
	f := BuildAck(0x07)
	if f.Command != cmdAck {
		t.Errorf("command: got 0x%02X want 0x%02X", f.Command, cmdAck)
	}
	if len(f.Payload) != 1 || f.Payload[0] != 0x07 {
		t.Errorf("payload: got %v want [0x07]", f.Payload)
	}
}

func TestBuildPing(t *testing.T) {
	f := BuildPing()
	if f.Command != cmdPing {
		t.Errorf("command: got 0x%02X want 0x%02X", f.Command, cmdPing)
	}
	if len(f.Payload) != 0 {
		t.Errorf("payload: got %v want empty", f.Payload)
	}
}
