package lpr

import (
	"context"
	"log/slog"
)

// LPREventPublisher is the interface through which the LPR pipeline emits
// plate read events to downstream consumers (forensic search index, analytics
// dashboards, notification fan-out).
//
// TODO(KAI-254): wire the production implementation to
// DirectoryIngest.PublishAIEvents once that RPC is available. Until then the
// LoggingPublisher and NoopPublisher stubs satisfy this interface.
//
// TODO(KAI-370): the notification fan-out service is not yet built. Watchlist
// matches with Type=deny or Type=alert should be forwarded here once KAI-370
// lands.
type LPREventPublisher interface {
	// PublishRead emits a single plate read event. Implementations must be
	// safe for concurrent calls. A non-nil error means the event was NOT
	// delivered and should be logged; the pipeline should not retry
	// indefinitely to avoid blocking frame processing.
	PublishRead(ctx context.Context, read PlateRead) error

	// PublishWatchlistMatch emits an alert event for a plate that matched a
	// deny or alert watchlist entry. WatchlistMatchEvent carries the matched
	// entry's IDs for downstream routing.
	//
	// TODO(KAI-370): production implementation forwards to the notification
	// fan-out service. For now the LoggingPublisher logs at Warn level.
	PublishWatchlistMatch(ctx context.Context, ev WatchlistMatchEvent) error
}

// WatchlistMatchEvent is the payload for a watchlist hit.
type WatchlistMatchEvent struct {
	PlateRead    PlateRead
	WatchlistID  string
	EntryID      string
	WatchlistType string // "allow" | "deny" | "alert"
}

// NoopPublisher silently discards all events. Suitable for benchmarks and
// unit tests that do not care about publication side-effects.
type NoopPublisher struct{}

// PublishRead implements LPREventPublisher.
func (NoopPublisher) PublishRead(_ context.Context, _ PlateRead) error { return nil }

// PublishWatchlistMatch implements LPREventPublisher.
func (NoopPublisher) PublishWatchlistMatch(_ context.Context, _ WatchlistMatchEvent) error {
	return nil
}

// LoggingPublisher logs every plate read and watchlist match as a structured
// slog record. It is a stub that stands in for the DirectoryIngest wiring
// until KAI-254 lands.
type LoggingPublisher struct {
	logger *slog.Logger
}

// NewLoggingPublisher creates a LoggingPublisher. If logger is nil the default
// slog logger is used.
func NewLoggingPublisher(logger *slog.Logger) *LoggingPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &LoggingPublisher{logger: logger}
}

// PublishRead implements LPREventPublisher.
func (p *LoggingPublisher) PublishRead(ctx context.Context, r PlateRead) error {
	p.logger.LogAttrs(ctx, slog.LevelInfo, "lpr_plate_read",
		slog.String("tenant_id", r.TenantID),
		slog.String("camera_id", r.CameraID),
		slog.String("plate_text", r.PlateText),
		slog.Float64("confidence", float64(r.Confidence)),
		slog.String("region", r.Region),
		slog.Time("timestamp", r.Timestamp),
	)
	return nil
}

// PublishWatchlistMatch implements LPREventPublisher.
func (p *LoggingPublisher) PublishWatchlistMatch(ctx context.Context, ev WatchlistMatchEvent) error {
	p.logger.LogAttrs(ctx, slog.LevelWarn, "lpr_watchlist_match",
		slog.String("tenant_id", ev.PlateRead.TenantID),
		slog.String("camera_id", ev.PlateRead.CameraID),
		slog.String("plate_text", ev.PlateRead.PlateText),
		slog.String("watchlist_id", ev.WatchlistID),
		slog.String("entry_id", ev.EntryID),
		slog.String("watchlist_type", ev.WatchlistType),
	)
	return nil
}
