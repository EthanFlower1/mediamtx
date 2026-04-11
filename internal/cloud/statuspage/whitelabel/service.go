package whitelabel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/statuspage"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var subdomainRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Config bundles dependencies for Service.
type Config struct {
	DB             *clouddb.DB
	StatusPageSvc  *statuspage.Service
	IDGen          IDGen
}

// Service manages per-integrator white-label status page configuration,
// subdomain routing, component filtering, and subscriber notifications.
type Service struct {
	db            *clouddb.DB
	statusPageSvc *statuspage.Service
	idGen         IDGen
}

// NewService constructs a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("whitelabel-status: DB is required")
	}
	if cfg.StatusPageSvc == nil {
		return nil, errors.New("whitelabel-status: StatusPageSvc is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	return &Service{db: cfg.DB, statusPageSvc: cfg.StatusPageSvc, idGen: idGen}, nil
}

// UpsertConfig creates or updates the status page configuration for an
// integrator. The subdomain must be unique across all integrators.
func (s *Service) UpsertConfig(ctx context.Context, cfg StatusPageConfig) (StatusPageConfig, error) {
	if err := validateSubdomain(cfg.Subdomain); err != nil {
		return StatusPageConfig{}, err
	}
	now := time.Now().UTC()
	cfg.UpdatedAt = now

	componentJSON, err := json.Marshal(cfg.ComponentIDs)
	if err != nil {
		return StatusPageConfig{}, fmt.Errorf("marshal component_ids: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO integrator_status_configs
			(integrator_id, subdomain, custom_domain, page_title, logo_url, favicon_url,
			 primary_color, secondary_color, accent_color, header_bg_color,
			 footer_text, custom_css, component_ids, support_url, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (integrator_id) DO UPDATE SET
			subdomain = excluded.subdomain,
			custom_domain = excluded.custom_domain,
			page_title = excluded.page_title,
			logo_url = excluded.logo_url,
			favicon_url = excluded.favicon_url,
			primary_color = excluded.primary_color,
			secondary_color = excluded.secondary_color,
			accent_color = excluded.accent_color,
			header_bg_color = excluded.header_bg_color,
			footer_text = excluded.footer_text,
			custom_css = excluded.custom_css,
			component_ids = excluded.component_ids,
			support_url = excluded.support_url,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		cfg.IntegratorID, cfg.Subdomain, cfg.CustomDomain, cfg.PageTitle,
		cfg.LogoURL, cfg.FaviconURL, cfg.PrimaryColor, cfg.SecondaryColor,
		cfg.AccentColor, cfg.HeaderBgColor, cfg.FooterText, cfg.CustomCSS,
		string(componentJSON), cfg.SupportURL, cfg.Enabled, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") &&
			strings.Contains(err.Error(), "subdomain") {
			return StatusPageConfig{}, ErrSubdomainTaken
		}
		return StatusPageConfig{}, fmt.Errorf("upsert status config: %w", err)
	}
	cfg.CreatedAt = now
	return cfg, nil
}

// GetConfig returns the status page config for an integrator.
func (s *Service) GetConfig(ctx context.Context, integratorID string) (StatusPageConfig, error) {
	return s.scanConfig(ctx, `
		SELECT integrator_id, subdomain, custom_domain, page_title, logo_url, favicon_url,
		       primary_color, secondary_color, accent_color, header_bg_color,
		       footer_text, custom_css, component_ids, support_url, enabled, created_at, updated_at
		FROM integrator_status_configs
		WHERE integrator_id = ?`, integratorID)
}

// GetConfigBySubdomain resolves a subdomain to the integrator's status page
// config. This is the primary routing mechanism for incoming requests.
func (s *Service) GetConfigBySubdomain(ctx context.Context, subdomain string) (StatusPageConfig, error) {
	return s.scanConfig(ctx, `
		SELECT integrator_id, subdomain, custom_domain, page_title, logo_url, favicon_url,
		       primary_color, secondary_color, accent_color, header_bg_color,
		       footer_text, custom_css, component_ids, support_url, enabled, created_at, updated_at
		FROM integrator_status_configs
		WHERE subdomain = ?`, subdomain)
}

// GetConfigByCustomDomain resolves a custom domain to the integrator's status
// page config. Falls back to subdomain lookup if no custom domain match.
func (s *Service) GetConfigByCustomDomain(ctx context.Context, domain string) (StatusPageConfig, error) {
	return s.scanConfig(ctx, `
		SELECT integrator_id, subdomain, custom_domain, page_title, logo_url, favicon_url,
		       primary_color, secondary_color, accent_color, header_bg_color,
		       footer_text, custom_css, component_ids, support_url, enabled, created_at, updated_at
		FROM integrator_status_configs
		WHERE custom_domain = ?`, domain)
}

// DeleteConfig removes the status page config for an integrator.
func (s *Service) DeleteConfig(ctx context.Context, integratorID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM integrator_status_configs WHERE integrator_id = ?`, integratorID)
	if err != nil {
		return fmt.Errorf("delete status config: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrConfigNotFound
	}
	return nil
}

// RenderPublicPage builds the full public status page view for an integrator,
// filtering components to only those in the config's component_ids list. If
// component_ids is empty, all components for the integrator's tenant are shown.
func (s *Service) RenderPublicPage(ctx context.Context, integratorID string) (PublicStatusPage, error) {
	cfg, err := s.GetConfig(ctx, integratorID)
	if err != nil {
		return PublicStatusPage{}, err
	}
	if !cfg.Enabled {
		return PublicStatusPage{}, ErrPageDisabled
	}

	// The integrator_id is the tenant_id for the status page service.
	summary, err := s.statusPageSvc.GetStatusSummary(ctx, integratorID)
	if err != nil {
		return PublicStatusPage{}, fmt.Errorf("get status summary: %w", err)
	}

	// Filter components based on the integrator's component_ids config.
	allowedSet := make(map[string]bool, len(cfg.ComponentIDs))
	for _, id := range cfg.ComponentIDs {
		allowedSet[id] = true
	}

	var components []ComponentStatus
	for _, svc := range summary.Services {
		if len(allowedSet) > 0 && !allowedSet[svc.CheckID] {
			continue
		}
		components = append(components, ComponentStatus{
			CheckID:     svc.CheckID,
			ServiceName: svc.ServiceName,
			DisplayName: svc.DisplayName,
			Status:      string(svc.Status),
			LastChecked: svc.LastCheckedAt,
		})
	}

	// Compute overall status from filtered components.
	overall := statuspage.StatusOperational
	for _, c := range components {
		cs := statuspage.ServiceStatus(c.Status)
		if statusWorseThan(cs, overall) {
			overall = cs
		}
	}

	var incidents []IncidentView
	for _, inc := range summary.ActiveIncidents {
		updates, _ := s.statusPageSvc.ListIncidentUpdates(ctx, integratorID, inc.IncidentID)
		var views []UpdateView
		for _, u := range updates {
			views = append(views, UpdateView{
				Status:    string(u.Status),
				Message:   u.Message,
				CreatedAt: u.CreatedAt,
			})
		}
		incidents = append(incidents, IncidentView{
			IncidentID:       inc.IncidentID,
			Title:            inc.Title,
			Severity:         string(inc.Severity),
			Status:           string(inc.Status),
			AffectedServices: inc.AffectedServices,
			StartedAt:        inc.StartedAt,
			ResolvedAt:       inc.ResolvedAt,
			Updates:          views,
		})
	}

	return PublicStatusPage{
		Config:          cfg,
		OverallStatus:   string(overall),
		Components:      components,
		ActiveIncidents: incidents,
	}, nil
}

// RenderPublicPageBySubdomain resolves a subdomain then renders the page.
func (s *Service) RenderPublicPageBySubdomain(ctx context.Context, subdomain string) (PublicStatusPage, error) {
	cfg, err := s.GetConfigBySubdomain(ctx, subdomain)
	if err != nil {
		return PublicStatusPage{}, err
	}
	return s.RenderPublicPage(ctx, cfg.IntegratorID)
}

// RenderPublicPageByCustomDomain resolves a custom domain then renders the page.
func (s *Service) RenderPublicPageByCustomDomain(ctx context.Context, domain string) (PublicStatusPage, error) {
	cfg, err := s.GetConfigByCustomDomain(ctx, domain)
	if err != nil {
		return PublicStatusPage{}, err
	}
	return s.RenderPublicPage(ctx, cfg.IntegratorID)
}

// --- Subscriber management ---

// Subscribe adds an email subscriber for the integrator's status page.
// A confirmation token is generated; the caller is responsible for sending
// the confirmation email.
func (s *Service) Subscribe(ctx context.Context, integratorID, email string) (Subscriber, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if !emailRE.MatchString(email) {
		return Subscriber{}, ErrInvalidEmail
	}

	now := time.Now().UTC()
	sub := Subscriber{
		SubscriberID: s.idGen(),
		IntegratorID: integratorID,
		Email:        email,
		Confirmed:    false,
		ConfirmToken: s.idGen(),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO status_page_subscribers
			(subscriber_id, integrator_id, email, confirmed, confirm_token, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sub.SubscriberID, sub.IntegratorID, sub.Email, sub.Confirmed,
		sub.ConfirmToken, sub.CreatedAt, sub.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Subscriber{}, ErrSubscriberExists
		}
		return Subscriber{}, fmt.Errorf("insert subscriber: %w", err)
	}
	return sub, nil
}

// ConfirmSubscriber confirms a subscriber using the confirmation token.
func (s *Service) ConfirmSubscriber(ctx context.Context, integratorID, token string) error {
	if token == "" {
		return ErrInvalidToken
	}
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `
		UPDATE status_page_subscribers
		SET confirmed = ?, updated_at = ?
		WHERE integrator_id = ? AND confirm_token = ? AND confirmed = ?`,
		true, now, integratorID, token, false)
	if err != nil {
		return fmt.Errorf("confirm subscriber: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInvalidToken
	}
	return nil
}

// Unsubscribe removes a subscriber by email.
func (s *Service) Unsubscribe(ctx context.Context, integratorID, email string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM status_page_subscribers
		WHERE integrator_id = ? AND email = ?`, integratorID, email)
	if err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSubscriberNotFound
	}
	return nil
}

// ListConfirmedSubscribers returns all confirmed subscribers for an integrator.
// Used by the notification pipeline to fan out incident updates.
func (s *Service) ListConfirmedSubscribers(ctx context.Context, integratorID string) ([]Subscriber, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT subscriber_id, integrator_id, email, confirmed, confirm_token, created_at, updated_at
		FROM status_page_subscribers
		WHERE integrator_id = ? AND confirmed = ?
		ORDER BY email`, integratorID, true)
	if err != nil {
		return nil, fmt.Errorf("list confirmed subscribers: %w", err)
	}
	defer rows.Close()

	var out []Subscriber
	for rows.Next() {
		var sub Subscriber
		if err := rows.Scan(&sub.SubscriberID, &sub.IntegratorID, &sub.Email,
			&sub.Confirmed, &sub.ConfirmToken, &sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscriber: %w", err)
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

// --- Helpers ---

func (s *Service) scanConfig(ctx context.Context, query string, args ...interface{}) (StatusPageConfig, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	var cfg StatusPageConfig
	var componentJSON string
	err := row.Scan(
		&cfg.IntegratorID, &cfg.Subdomain, &cfg.CustomDomain, &cfg.PageTitle,
		&cfg.LogoURL, &cfg.FaviconURL, &cfg.PrimaryColor, &cfg.SecondaryColor,
		&cfg.AccentColor, &cfg.HeaderBgColor, &cfg.FooterText, &cfg.CustomCSS,
		&componentJSON, &cfg.SupportURL, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return StatusPageConfig{}, ErrConfigNotFound
		}
		return StatusPageConfig{}, fmt.Errorf("scan status config: %w", err)
	}
	if componentJSON != "" {
		if err := json.Unmarshal([]byte(componentJSON), &cfg.ComponentIDs); err != nil {
			return StatusPageConfig{}, fmt.Errorf("unmarshal component_ids: %w", err)
		}
	}
	return cfg, nil
}

func validateSubdomain(sub string) error {
	if sub == "" || !subdomainRE.MatchString(sub) {
		return fmt.Errorf("%w: %q", ErrInvalidSubdomain, sub)
	}
	return nil
}

// statusWorseThan returns true if a is worse than b.
func statusWorseThan(a, b statuspage.ServiceStatus) bool {
	order := map[statuspage.ServiceStatus]int{
		statuspage.StatusOperational: 0,
		statuspage.StatusDegraded:    1,
		statuspage.StatusPartialOut:  2,
		statuspage.StatusMajorOut:    3,
	}
	return order[a] > order[b]
}
