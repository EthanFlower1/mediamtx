package relay

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionManagerCreateAndGet(t *testing.T) {
	sm := NewSessionManager()

	sess := sm.Create("site-abc", "client-xyz")

	require.NotEmpty(t, sess.ID)
	require.Len(t, sess.ID, 32) // 16 bytes = 32 hex chars
	require.Equal(t, "site-abc", sess.SiteID)
	require.Equal(t, "client-xyz", sess.ClientID)
	require.False(t, sess.CreatedAt.IsZero())
	require.False(t, sess.LastActive.IsZero())

	got, ok := sm.Get(sess.ID)
	require.True(t, ok)
	require.Equal(t, sess.ID, got.ID)
	require.Equal(t, sess.SiteID, got.SiteID)
	require.Equal(t, sess.ClientID, got.ClientID)

	// Non-existent session returns false
	_, ok = sm.Get("does-not-exist")
	require.False(t, ok)
}

func TestSessionManagerExpiry(t *testing.T) {
	sm := NewSessionManager()
	sm.sessionTTL = 100 * time.Millisecond

	sess := sm.Create("site-1", "client-1")

	_, ok := sm.Get(sess.ID)
	require.True(t, ok)

	time.Sleep(150 * time.Millisecond)
	sm.Cleanup()

	_, ok = sm.Get(sess.ID)
	require.False(t, ok, "session should have been cleaned up after TTL")
}

func TestSessionManagerConcurrency(t *testing.T) {
	sm := NewSessionManager()

	var wg sync.WaitGroup
	const goroutines = 100

	// Phase 1: concurrent creates
	ids := make([]string, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess := sm.Create("site", "client")
			ids[idx] = sess.ID
		}(i)
	}
	wg.Wait()

	// Phase 2: concurrent gets, touches, and removes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sm.Get(ids[idx])
			sm.Touch(ids[idx])
			if idx%2 == 0 {
				sm.Remove(ids[idx])
			}
		}(i)
	}
	wg.Wait()

	// Verify: odd-indexed sessions still exist, even-indexed removed
	for i := 0; i < goroutines; i++ {
		_, ok := sm.Get(ids[i])
		if i%2 == 0 {
			require.False(t, ok, "even-indexed session %d should be removed", i)
		} else {
			require.True(t, ok, "odd-indexed session %d should still exist", i)
		}
	}
}
