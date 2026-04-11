package preferences

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NowFunc returns the current time. Overridable for testing quiet hours.
type NowFunc func() time.Time

// Config bundles dependencies for Service.
type Config struct {
	DB    *clouddb.DB
	IDGen IDGen
	Now   NowFunc
}

// Service manages per-user per-camera notification preferences with quiet
// hours and severity thresholds.
type Service struct {
	db    *clouddb.DB
	idGen IDGen
	now   NowFunc
}

// New constructs a Service.
func New(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("preferences: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{db: cfg.DB, idGen: idGen, now: now}, nil
}

// Upsert creates or updates a preference entry. The unique key is
// (tenant_id, user_id, camera_id, event_type).
func (s *Service) Upsert(ctx context.Context, p Pref) (Pref, error) {
	now := s.now()
	if p.PrefID == "" {
		p.PrefID = s.idGen()
	}
	if p.SeverityMin == "" {
		p.SeverityMin = SeverityInfo
	}
	if p.QuietTimezone == "" {
		p.QuietTimezone = "UTC"
	}
	p.UpdatedAt = now

	channelsJSON, err := json.Marshal(p.Channels)
	if err != nil {
		return Pref{}, fmt.Errorf("marshal channels: %w", err)
	}
	daysJSON, err := json.Marshal(p.QuietDays)
	if err != nil {
		return Pref{}, fmt.Errorf("marshal quiet_days: %w", err)
	}

	// Use empty string in DB for NULL-like wildcard matching via the unique index.
	camID := p.CameraID
	evType := p.EventType

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_notification_prefs
			(pref_id, tenant_id, user_id, camera_id, event_type, channels,
			 severity_min, quiet_start, quiet_end, quiet_timezone, quiet_days,
			 enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, user_id, camera_id, event_type) DO UPDATE SET
			channels       = excluded.channels,
			severity_min   = excluded.severity_min,
			quiet_start    = excluded.quiet_start,
			quiet_end      = excluded.quiet_end,
			quiet_timezone = excluded.quiet_timezone,
			quiet_days     = excluded.quiet_days,
			enabled        = excluded.enabled,
			updated_at     = excluded.updated_at`,
		p.PrefID, p.TenantID, p.UserID, camID, evType,
		string(channelsJSON), string(p.SeverityMin),
		p.QuietStart, p.QuietEnd, p.QuietTimezone, string(daysJSON),
		p.Enabled, now, now)
	if err != nil {
		return Pref{}, fmt.Errorf("upsert pref: %w", err)
	}
	p.CreatedAt = now
	return p, nil
}

// Get returns a single preference by ID.
func (s *Service) Get(ctx context.Context, prefID string) (Pref, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT pref_id, tenant_id, user_id, camera_id, event_type, channels,
		       severity_min, quiet_start, quiet_end, quiet_timezone, quiet_days,
		       enabled, created_at, updated_at
		FROM user_notification_prefs
		WHERE pref_id = ?`, prefID)
	return scanPref(row)
}

// List returns all preferences for a user in a tenant.
func (s *Service) List(ctx context.Context, tenantID, userID string) ([]Pref, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pref_id, tenant_id, user_id, camera_id, event_type, channels,
		       severity_min, quiet_start, quiet_end, quiet_timezone, quiet_days,
		       enabled, created_at, updated_at
		FROM user_notification_prefs
		WHERE tenant_id = ? AND user_id = ?
		ORDER BY camera_id, event_type`, tenantID, userID)
	if err != nil {
		return nil, fmt.Errorf("list prefs: %w", err)
	}
	defer rows.Close()

	var out []Pref
	for rows.Next() {
		p, err := scanPrefRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete removes a preference by ID.
func (s *Service) Delete(ctx context.Context, prefID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM user_notification_prefs WHERE pref_id = ?`, prefID)
	if err != nil {
		return fmt.Errorf("delete pref: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPrefNotFound
	}
	return nil
}

// Resolve finds the best-matching preference for a given (tenant, user,
// camera, event_type) tuple. Resolution priority (most-specific wins):
//
//  1. Exact camera + exact event
//  2. Exact camera + wildcard event (event_type = "")
//  3. Wildcard camera (camera_id = "") + exact event
//  4. Wildcard camera + wildcard event (the default)
//
// Returns ErrPrefNotFound if no matching preference exists.
func (s *Service) Resolve(ctx context.Context, tenantID, userID, cameraID, eventType string) (Pref, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pref_id, tenant_id, user_id, camera_id, event_type, channels,
		       severity_min, quiet_start, quiet_end, quiet_timezone, quiet_days,
		       enabled, created_at, updated_at
		FROM user_notification_prefs
		WHERE tenant_id = ? AND user_id = ? AND enabled = 1
		  AND (camera_id = ? OR camera_id = '')
		  AND (event_type = ? OR event_type = '')
		ORDER BY
		  CASE WHEN camera_id != '' AND event_type != '' THEN 0
		       WHEN camera_id != '' AND event_type  = '' THEN 1
		       WHEN camera_id  = '' AND event_type != '' THEN 2
		       ELSE 3 END
		LIMIT 1`,
		tenantID, userID, cameraID, eventType)
	if err != nil {
		return Pref{}, fmt.Errorf("resolve pref: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return Pref{}, ErrPrefNotFound
	}
	return scanPrefRow(rows)
}

// ResolveDelivery resolves the best preference and evaluates quiet hours
// and severity filtering. If the event severity is below the threshold or
// quiet hours are active, the result indicates suppression.
func (s *Service) ResolveDelivery(
	ctx context.Context,
	tenantID, userID, cameraID, eventType string,
	eventSeverity Severity,
) (ResolvedDelivery, error) {
	pref, err := s.Resolve(ctx, tenantID, userID, cameraID, eventType)
	if err != nil {
		return ResolvedDelivery{}, err
	}

	// Severity check.
	if SeverityRank(eventSeverity) < SeverityRank(pref.SeverityMin) {
		return ResolvedDelivery{
			Pref:       pref,
			Suppressed: true,
			Reason:     fmt.Sprintf("event severity %s below threshold %s", eventSeverity, pref.SeverityMin),
		}, nil
	}

	// Quiet hours check.
	if suppressed, reason := s.isQuietHoursActive(pref); suppressed {
		return ResolvedDelivery{
			Pref:       pref,
			Suppressed: true,
			Reason:     reason,
		}, nil
	}

	return ResolvedDelivery{Pref: pref, Suppressed: false}, nil
}

// ResolveChannels is a convenience that returns just the channel list for
// delivery, or nil if suppressed or not found.
func (s *Service) ResolveChannels(
	ctx context.Context,
	tenantID, userID, cameraID, eventType string,
	eventSeverity Severity,
) ([]notifications.ChannelType, error) {
	rd, err := s.ResolveDelivery(ctx, tenantID, userID, cameraID, eventType, eventSeverity)
	if err != nil {
		if errors.Is(err, ErrPrefNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if rd.Suppressed {
		return nil, nil
	}
	return rd.Pref.Channels, nil
}

// isQuietHoursActive checks if the current time falls within the preference's
// quiet window.
func (s *Service) isQuietHoursActive(p Pref) (bool, string) {
	if p.QuietStart == "" || p.QuietEnd == "" {
		return false, ""
	}

	loc, err := time.LoadLocation(p.QuietTimezone)
	if err != nil {
		// Invalid timezone: don't suppress.
		return false, ""
	}

	now := s.now().In(loc)

	// Check day-of-week filter.
	if len(p.QuietDays) > 0 {
		weekday := int(now.Weekday()) // 0=Sun
		found := false
		for _, d := range p.QuietDays {
			if d == weekday {
				found = true
				break
			}
		}
		if !found {
			return false, ""
		}
	}

	startH, startM, err1 := parseHHMM(p.QuietStart)
	endH, endM, err2 := parseHHMM(p.QuietEnd)
	if err1 != nil || err2 != nil {
		return false, ""
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	var active bool
	if startMinutes <= endMinutes {
		// Same-day range, e.g. 22:00-23:00 or 09:00-17:00
		active = nowMinutes >= startMinutes && nowMinutes < endMinutes
	} else {
		// Overnight range, e.g. 22:00-06:00
		active = nowMinutes >= startMinutes || nowMinutes < endMinutes
	}

	if active {
		return true, fmt.Sprintf("quiet hours active (%s-%s %s)", p.QuietStart, p.QuietEnd, p.QuietTimezone)
	}
	return false, ""
}

// parseHHMM parses "HH:MM" into hours and minutes.
func parseHHMM(s string) (int, int, error) {
	var h, m int
	_, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil {
		return 0, 0, err
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time %q", s)
	}
	return h, m, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanPref(row scanner) (Pref, error) {
	var p Pref
	var channelsJSON, daysJSON string
	var camID, evType *string
	var quietStart, quietEnd *string

	err := row.Scan(
		&p.PrefID, &p.TenantID, &p.UserID, &camID, &evType,
		&channelsJSON, &p.SeverityMin,
		&quietStart, &quietEnd, &p.QuietTimezone, &daysJSON,
		&p.Enabled, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Pref{}, fmt.Errorf("scan pref: %w", err)
	}
	if camID != nil {
		p.CameraID = *camID
	}
	if evType != nil {
		p.EventType = *evType
	}
	if quietStart != nil {
		p.QuietStart = *quietStart
	}
	if quietEnd != nil {
		p.QuietEnd = *quietEnd
	}

	if err := json.Unmarshal([]byte(channelsJSON), &p.Channels); err != nil {
		return Pref{}, fmt.Errorf("unmarshal channels: %w", err)
	}
	if err := json.Unmarshal([]byte(daysJSON), &p.QuietDays); err != nil {
		return Pref{}, fmt.Errorf("unmarshal quiet_days: %w", err)
	}
	return p, nil
}

func scanPrefRow(rows scanner) (Pref, error) {
	return scanPref(rows)
}

// ErrPrefNotFound is returned when no matching preference exists.
var ErrPrefNotFound = errors.New("preferences: not found")
