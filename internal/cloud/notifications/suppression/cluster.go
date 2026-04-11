package suppression

import (
	"fmt"
	"sync"
	"time"
)

// clusterKey groups events by tenant + camera + event type.
type clusterKey struct {
	TenantID  string
	CameraID  string
	EventType string
}

// cluster tracks a group of related events within a time window.
type cluster struct {
	ID        string
	Key       clusterKey
	Events    []Event
	FirstSeen time.Time
	LastSeen  time.Time
}

// Clusterer groups related events by camera + type within a time window.
// When a new event arrives and a cluster already exists for the same
// camera + type within the window, the event joins the cluster rather than
// generating a new notification.
type Clusterer struct {
	mu       sync.Mutex
	window   time.Duration
	clusters map[clusterKey]*cluster
	idGen    func() string
}

// NewClusterer creates a Clusterer with the given time window.
func NewClusterer(window time.Duration, idGen func() string) *Clusterer {
	return &Clusterer{
		window:   window,
		clusters: make(map[clusterKey]*cluster),
		idGen:    idGen,
	}
}

// Add evaluates an incoming event against active clusters.
// Returns (clusterID, clusterSize, isNew) where isNew is true if this event
// starts a new cluster (i.e. the first event should not be suppressed).
func (c *Clusterer) Add(ev Event) (clusterID string, clusterSize int, isNew bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := clusterKey{
		TenantID:  ev.TenantID,
		CameraID:  ev.CameraID,
		EventType: ev.EventType,
	}

	existing, ok := c.clusters[key]
	if ok && ev.Timestamp.Sub(existing.LastSeen) <= c.window {
		// Join existing cluster.
		existing.Events = append(existing.Events, ev)
		existing.LastSeen = ev.Timestamp
		return existing.ID, len(existing.Events), false
	}

	// Start a new cluster.
	cl := &cluster{
		ID:        c.idGen(),
		Key:       key,
		Events:    []Event{ev},
		FirstSeen: ev.Timestamp,
		LastSeen:  ev.Timestamp,
	}
	c.clusters[key] = cl
	return cl.ID, 1, true
}

// Summary returns a human-readable summary for a cluster.
func (c *Clusterer) Summary(clusterID string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, cl := range c.clusters {
		if cl.ID == clusterID {
			dur := cl.LastSeen.Sub(cl.FirstSeen)
			durStr := "last moment"
			if dur > time.Minute {
				durStr = fmt.Sprintf("last %d min", int(dur.Minutes())+1)
			} else if dur > time.Second {
				durStr = fmt.Sprintf("last %d sec", int(dur.Seconds())+1)
			}
			return fmt.Sprintf("%d %s events on camera %s, %s",
				len(cl.Events), cl.Key.EventType, cl.Key.CameraID, durStr)
		}
	}
	return ""
}

// Prune removes clusters older than the window. Should be called periodically.
func (c *Clusterer) Prune(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, cl := range c.clusters {
		if now.Sub(cl.LastSeen) > c.window {
			delete(c.clusters, key)
		}
	}
}
