package cloudconnector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMarshalRegisterMessage(t *testing.T) {
	env := Envelope{
		Type: MsgTypeRegister,
		Register: &RegisterPayload{
			SiteID:   "site-abc-123",
			SiteAlias: "My Home",
			Version:  "1.2.0",
			PublicIP: "203.0.113.10",
			LANCIDRs: []string{"192.168.1.0/24", "10.0.0.0/8"},
			Capabilities: Capabilities{
				Streams:  true,
				Playback: true,
				AI:       false,
			},
		},
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded Envelope
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, MsgTypeRegister, decoded.Type)
	require.NotNil(t, decoded.Register)
	require.Equal(t, "site-abc-123", decoded.Register.SiteID)
	require.Equal(t, "My Home", decoded.Register.SiteAlias)
	require.Equal(t, "1.2.0", decoded.Register.Version)
	require.Equal(t, "203.0.113.10", decoded.Register.PublicIP)
	require.Equal(t, []string{"192.168.1.0/24", "10.0.0.0/8"}, decoded.Register.LANCIDRs)
	require.True(t, decoded.Register.Capabilities.Streams)
	require.True(t, decoded.Register.Capabilities.Playback)
	require.False(t, decoded.Register.Capabilities.AI)
}

func TestMarshalHeartbeatMessage(t *testing.T) {
	ts := time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC)
	env := Envelope{
		Type: MsgTypeHeartbeat,
		Heartbeat: &HeartbeatPayload{
			SiteID:        "site-abc-123",
			Timestamp:     ts,
			UptimeSec:     86400,
			CameraCount:   8,
			RecorderCount: 2,
			DiskUsedPct:   72.5,
			PublicIP:      "203.0.113.10",
		},
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded Envelope
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, MsgTypeHeartbeat, decoded.Type)
	require.NotNil(t, decoded.Heartbeat)
	require.Equal(t, "site-abc-123", decoded.Heartbeat.SiteID)
	require.True(t, decoded.Heartbeat.Timestamp.Equal(ts))
	require.Equal(t, int64(86400), decoded.Heartbeat.UptimeSec)
	require.Equal(t, 8, decoded.Heartbeat.CameraCount)
	require.Equal(t, 2, decoded.Heartbeat.RecorderCount)
	require.InDelta(t, 72.5, decoded.Heartbeat.DiskUsedPct, 0.01)
	require.Equal(t, "203.0.113.10", decoded.Heartbeat.PublicIP)
}

func TestMarshalEventMessage(t *testing.T) {
	ts := time.Date(2026, 4, 20, 12, 30, 0, 0, time.UTC)
	eventData := json.RawMessage(`{"severity":"high","description":"motion detected"}`)

	env := Envelope{
		Type: MsgTypeEvent,
		Event: &EventPayload{
			Kind:       "alert",
			CameraID:   "cam-001",
			RecorderID: "rec-01",
			Timestamp:  ts,
			Data:       eventData,
		},
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded Envelope
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, MsgTypeEvent, decoded.Type)
	require.NotNil(t, decoded.Event)
	require.Equal(t, "alert", decoded.Event.Kind)
	require.Equal(t, "cam-001", decoded.Event.CameraID)
	require.Equal(t, "rec-01", decoded.Event.RecorderID)
	require.True(t, decoded.Event.Timestamp.Equal(ts))
	require.JSONEq(t, `{"severity":"high","description":"motion detected"}`, string(decoded.Event.Data))
}

func TestMarshalCommandMessage(t *testing.T) {
	cmdData := json.RawMessage(`{"target_camera":"cam-001","action":"ptz_move","params":{"pan":10}}`)

	env := Envelope{
		Type: MsgTypeCommand,
		Command: &CommandPayload{
			ID:   "cmd-uuid-001",
			Kind: "relay_request",
			Data: cmdData,
		},
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded Envelope
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, MsgTypeCommand, decoded.Type)
	require.NotNil(t, decoded.Command)
	require.Equal(t, "cmd-uuid-001", decoded.Command.ID)
	require.Equal(t, "relay_request", decoded.Command.Kind)
	require.JSONEq(t, `{"target_camera":"cam-001","action":"ptz_move","params":{"pan":10}}`, string(decoded.Command.Data))
}

func TestMarshalCommandResponseMessage(t *testing.T) {
	respData := json.RawMessage(`{"result":"ok"}`)

	env := Envelope{
		Type: MsgTypeCommandResponse,
		CommandResponse: &CommandResponsePayload{
			ID:      "cmd-uuid-001",
			Success: true,
			Data:    respData,
		},
	}

	data, err := json.Marshal(env)
	require.NoError(t, err)

	var decoded Envelope
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, MsgTypeCommandResponse, decoded.Type)
	require.NotNil(t, decoded.CommandResponse)
	require.Equal(t, "cmd-uuid-001", decoded.CommandResponse.ID)
	require.True(t, decoded.CommandResponse.Success)
	require.Empty(t, decoded.CommandResponse.Error)
	require.JSONEq(t, `{"result":"ok"}`, string(decoded.CommandResponse.Data))
}
