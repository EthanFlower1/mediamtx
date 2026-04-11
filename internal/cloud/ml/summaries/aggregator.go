package summaries

import (
	"context"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// notableCategories are event categories that merit individual mention
// in summaries because they represent security-relevant or unusual events.
var notableCategories = map[EventCategory]struct{}{
	CategoryTamper:     {},
	CategoryFall:       {},
	CategoryOffline:    {},
	CategoryLoitering:  {},
	CategoryTailgating: {},
}

// Aggregator collects and groups events from the event store for a tenant
// within a time window.
type Aggregator struct {
	db *clouddb.DB
}

// NewAggregator constructs an Aggregator backed by the provided DB handle.
func NewAggregator(db *clouddb.DB) *Aggregator {
	return &Aggregator{db: db}
}

// Aggregate queries the event store for events belonging to the given tenant
// within [start, end) and returns them grouped by camera and category.
func (a *Aggregator) Aggregate(ctx context.Context, tenantID string, start, end time.Time) (*AggregatedEvents, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}

	rows, err := a.db.QueryContext(ctx, `
		SELECT event_id, tenant_id, camera_id, category, detail, timestamp
		FROM nvr_events
		WHERE tenant_id = ? AND timestamp >= ? AND timestamp < ?
		ORDER BY timestamp ASC`,
		tenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("aggregator query: %w", err)
	}
	defer rows.Close()

	agg := &AggregatedEvents{
		TenantID:         tenantID,
		StartTime:        start,
		EndTime:          end,
		ByCameraCategory: make(map[string]map[EventCategory]int),
		TotalByCategory:  make(map[EventCategory]int),
	}

	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.EventID, &ev.TenantID, &ev.CameraID,
			&ev.Category, &ev.Detail, &ev.Timestamp); err != nil {
			return nil, fmt.Errorf("aggregator scan: %w", err)
		}

		// Per-camera category count.
		if agg.ByCameraCategory[ev.CameraID] == nil {
			agg.ByCameraCategory[ev.CameraID] = make(map[EventCategory]int)
		}
		agg.ByCameraCategory[ev.CameraID][ev.Category]++

		// Global category count.
		agg.TotalByCategory[ev.Category]++
		agg.TotalEvents++

		// Capture notable events for individual mention.
		if _, ok := notableCategories[ev.Category]; ok {
			agg.NotableEvents = append(agg.NotableEvents, ev)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("aggregator rows: %w", err)
	}
	if agg.TotalEvents == 0 {
		return nil, ErrNoEvents
	}
	return agg, nil
}

// AggregateFromEvents builds an AggregatedEvents from an in-memory slice.
// This is used for testing and for on-demand summaries where events are
// already loaded.
func AggregateFromEvents(tenantID string, events []Event, start, end time.Time) (*AggregatedEvents, error) {
	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if len(events) == 0 {
		return nil, ErrNoEvents
	}

	agg := &AggregatedEvents{
		TenantID:         tenantID,
		StartTime:        start,
		EndTime:          end,
		ByCameraCategory: make(map[string]map[EventCategory]int),
		TotalByCategory:  make(map[EventCategory]int),
	}

	for _, ev := range events {
		if ev.TenantID != tenantID {
			continue // enforce tenant isolation
		}
		if agg.ByCameraCategory[ev.CameraID] == nil {
			agg.ByCameraCategory[ev.CameraID] = make(map[EventCategory]int)
		}
		agg.ByCameraCategory[ev.CameraID][ev.Category]++
		agg.TotalByCategory[ev.Category]++
		agg.TotalEvents++

		if _, ok := notableCategories[ev.Category]; ok {
			agg.NotableEvents = append(agg.NotableEvents, ev)
		}
	}
	if agg.TotalEvents == 0 {
		return nil, ErrNoEvents
	}
	return agg, nil
}
