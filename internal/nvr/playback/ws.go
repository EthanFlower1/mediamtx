package playback

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// HandleWebSocket returns a gin handler that upgrades to WebSocket and drives
// a playback session via the JSON command/event protocol.
func HandleWebSocket(manager *SessionManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var session *PlaybackSession
		var sessionID string

		eventCh := make(chan Event, 32)
		onEvent := func(ev Event) {
			select {
			case eventCh <- ev:
			default:
			}
		}

		// Writer goroutine — serialises events to the WebSocket.
		writerDone := make(chan struct{})
		go func() {
			defer close(writerDone)
			for ev := range eventCh {
				data, _ := json.Marshal(ev)
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second)) //nolint:errcheck
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return
				}
			}
		}()

		// Reader loop — processes commands from the client.
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var cmd Command
			if err := json.Unmarshal(message, &cmd); err != nil {
				msg := "invalid JSON"
				onEvent(Event{EventType: "error", Message: &msg})
				continue
			}

			switch cmd.Cmd {
			case "create":
				if cmd.Start == nil || len(cmd.CameraIDs) == 0 {
					msg := "missing camera_ids or start"
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: &msg})
					continue
				}
				startTime, err := time.Parse(time.RFC3339, *cmd.Start)
				if err != nil {
					msg := "invalid start time"
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: &msg})
					continue
				}

				session, err = manager.CreateSession(cmd.CameraIDs, startTime, 0, onEvent)
				if err != nil {
					msg := err.Error()
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: &msg})
					continue
				}
				sessionID = session.ID()

				streams := make(map[string]string)
				for _, camID := range cmd.CameraIDs {
					streams[camID] = "/api/nvr/playback/stream/" + sessionID + "/" + camID
				}

				onEvent(Event{
					EventType: "created",
					AckSeq:    &cmd.Seq,
					SessionID: &sessionID,
					Streams:   streams,
				})

			case "resume":
				if cmd.SessionID == nil {
					msg := "missing session_id"
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: &msg})
					continue
				}
				session = manager.GetSession(*cmd.SessionID)
				if session == nil {
					msg := "session not found"
					onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: &msg})
					continue
				}
				sessionID = session.ID()

				// Rebind the event callback so this WebSocket connection
				// receives future events for the resumed session.
				session.SetEventCallback(onEvent)

				pos := session.Position()
				playing := session.IsPlaying()
				speed := session.Speed()
				onEvent(Event{
					EventType: "state",
					AckSeq:    &cmd.Seq,
					Playing:   &playing,
					Speed:     &speed,
					Position:  &pos,
				})

			case "play":
				if session != nil {
					session.Play()
				}

			case "pause":
				if session != nil {
					session.Pause()
				}

			case "seek":
				if session != nil && cmd.Position != nil {
					session.Seek(*cmd.Position)
				}

			case "speed":
				if session != nil && cmd.Rate != nil {
					session.SetSpeed(*cmd.Rate)
				}

			case "step":
				if session != nil && cmd.Direction != nil {
					session.Step(*cmd.Direction)
				}

			case "add_camera":
				if session != nil && cmd.CameraID != nil {
					cam, err := manager.DB.GetCamera(*cmd.CameraID)
					if err != nil {
						msg := "camera not found"
						onEvent(Event{EventType: "error", AckSeq: &cmd.Seq, Message: &msg})
						continue
					}
					session.AddCamera(*cmd.CameraID, cam.MediaMTXPath, manager.RecordPath) //nolint:errcheck
					url := "/api/nvr/playback/stream/" + sessionID + "/" + *cmd.CameraID
					onEvent(Event{
						EventType: "stream_added",
						AckSeq:    &cmd.Seq,
						CameraID:  cmd.CameraID,
						URL:       &url,
					})
				}

			case "remove_camera":
				if session != nil && cmd.CameraID != nil {
					session.RemoveCamera(*cmd.CameraID)
					onEvent(Event{
						EventType: "stream_removed",
						AckSeq:    &cmd.Seq,
						CameraID:  cmd.CameraID,
					})
				}

			case "close":
				if session != nil {
					manager.DisposeSession(sessionID)
					session = nil
					sessionID = ""
				}
			}
		}

		// WebSocket disconnected — close event channel and wait for writer.
		close(eventCh)
		<-writerDone

		// Pause the session and schedule deferred disposal after the grace period.
		if session != nil {
			session.Pause()
			capturedID := sessionID
			go func() {
				time.Sleep(manager.GracePeriod)
				s := manager.GetSession(capturedID)
				if s != nil && s.IsIdle(manager.GracePeriod) {
					manager.DisposeSession(capturedID)
				}
			}()
		}
	}
}
