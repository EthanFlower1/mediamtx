package cloudconnector

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventForwarderQueues(t *testing.T) {
	var mu sync.Mutex
	var sent []EventPayload

	sender := func(e EventPayload) error {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, e)
		return nil
	}

	fwd := NewEventForwarder(sender, 16)

	e1 := EventPayload{Kind: "motion", CameraID: "cam-1", Timestamp: time.Now()}
	e2 := EventPayload{Kind: "offline", CameraID: "cam-2", Timestamp: time.Now()}

	fwd.Forward(e1)
	fwd.Forward(e2)
	fwd.Flush()

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 2)
	require.Equal(t, "cam-1", sent[0].CameraID)
	require.Equal(t, "cam-2", sent[1].CameraID)
	require.Equal(t, int64(0), fwd.Dropped())
}

func TestEventForwarderDropsWhenFull(t *testing.T) {
	sender := func(e EventPayload) error {
		return nil
	}

	fwd := NewEventForwarder(sender, 2)

	fwd.Forward(EventPayload{Kind: "a"})
	fwd.Forward(EventPayload{Kind: "b"})
	fwd.Forward(EventPayload{Kind: "c"})

	require.GreaterOrEqual(t, fwd.Dropped(), int64(1))
}
