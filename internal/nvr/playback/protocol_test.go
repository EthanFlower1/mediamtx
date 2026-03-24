package playback

import (
	"encoding/json"
	"testing"
)

func TestCommandParsing(t *testing.T) {
	raw := `{"cmd":"seek","seq":5,"position":36000.5}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Cmd != "seek" {
		t.Fatalf("expected seek, got %s", cmd.Cmd)
	}
	if cmd.Seq != 5 {
		t.Fatalf("expected seq 5, got %d", cmd.Seq)
	}
	if cmd.Position == nil || *cmd.Position != 36000.5 {
		t.Fatalf("expected position 36000.5, got %v", cmd.Position)
	}
}

func TestCommandCreate(t *testing.T) {
	raw := `{"cmd":"create","seq":1,"camera_ids":["cam1","cam2"],"start":"2026-03-24T10:00:00Z"}`
	var cmd Command
	if err := json.Unmarshal([]byte(raw), &cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.Cmd != "create" {
		t.Fatalf("expected create, got %s", cmd.Cmd)
	}
	if len(cmd.CameraIDs) != 2 {
		t.Fatalf("expected 2 camera IDs, got %d", len(cmd.CameraIDs))
	}
	if cmd.Start == nil || *cmd.Start != "2026-03-24T10:00:00Z" {
		t.Fatalf("expected start time, got %v", cmd.Start)
	}
}

func TestEventSerialization(t *testing.T) {
	playing := true
	speed := 2.0
	pos := 36000.5
	seq := 5
	ev := Event{
		EventType: "state",
		AckSeq:    &seq,
		Playing:   &playing,
		Speed:     &speed,
		Position:  &pos,
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["event"] != "state" {
		t.Fatalf("expected state event, got %v", parsed["event"])
	}
	if parsed["ack_seq"].(float64) != 5 {
		t.Fatalf("expected ack_seq 5")
	}
	if parsed["playing"].(bool) != true {
		t.Fatalf("expected playing true")
	}
}

func TestEventCreated(t *testing.T) {
	sid := "abc123"
	seq := 1
	ev := Event{
		EventType: "created",
		AckSeq:    &seq,
		SessionID: &sid,
		Streams:   map[string]string{"cam1": "/api/nvr/playback/stream/abc123/cam1"},
	}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	if parsed["session_id"] != "abc123" {
		t.Fatalf("expected session_id abc123")
	}
	streams := parsed["streams"].(map[string]interface{})
	if streams["cam1"] != "/api/nvr/playback/stream/abc123/cam1" {
		t.Fatalf("unexpected stream URL")
	}
}

func TestSessionStateValues(t *testing.T) {
	if StatePaused != 0 {
		t.Fatal("StatePaused should be 0")
	}
	if StatePlaying != 1 {
		t.Fatal("StatePlaying should be 1")
	}
	if StateSeeking != 2 {
		t.Fatal("StateSeeking should be 2")
	}
	if StateDisposed != 4 {
		t.Fatal("StateDisposed should be 4")
	}
}
