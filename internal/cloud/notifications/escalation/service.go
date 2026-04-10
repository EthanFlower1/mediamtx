package escalation

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// IDGen generates a random hex ID.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Clock abstracts time for testability.
type Clock func() time.Time

// Config bundles dependencies for Service.
type Config struct {
	DB             *clouddb.DB
	IDGen          IDGen
	Clock          Clock
	Notifier       Notifier
	PagerDutyClient PagerDutyClient
}

// Service manages escalation chains, steps, and per-alert state machines.
type Service struct {
	db       *clouddb.DB
	idGen    IDGen
	clock    Clock
	notifier Notifier
	pd       PagerDutyClient
}

// NewService constructs an escalation Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("escalation: DB is required")
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		db:       cfg.DB,
		idGen:    idGen,
		clock:    clock,
		notifier: cfg.Notifier,
		pd:       cfg.PagerDutyClient,
	}, nil
}

// ---------- Chain CRUD ----------

// CreateChain creates a new escalation chain with its steps.
func (s *Service) CreateChain(ctx context.Context, chain Chain, steps []Step) (Chain, []Step, error) {
	now := s.clock()
	if chain.ChainID == "" {
		chain.ChainID = s.idGen()
	}
	if chain.Name == "" {
		return Chain{}, nil, fmt.Errorf("%w: name is required", ErrInvalidChain)
	}
	if chain.TenantID == "" {
		return Chain{}, nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidChain)
	}
	chain.CreatedAt = now
	chain.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO escalation_chains (chain_id, tenant_id, name, description, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		chain.ChainID, chain.TenantID, chain.Name, chain.Description, chain.Enabled, now, now)
	if err != nil {
		return Chain{}, nil, fmt.Errorf("create chain: %w", err)
	}

	created := make([]Step, len(steps))
	for i, step := range steps {
		step.ChainID = chain.ChainID
		step.TenantID = chain.TenantID
		step.StepOrder = i + 1
		if step.StepID == "" {
			step.StepID = s.idGen()
		}
		if step.TimeoutSeconds <= 0 {
			step.TimeoutSeconds = 300 // default 5 minutes
		}
		step.CreatedAt = now

		_, err := s.db.ExecContext(ctx, `
			INSERT INTO escalation_steps (step_id, chain_id, tenant_id, step_order, target_type, target_id, channel_type, timeout_seconds, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			step.StepID, step.ChainID, step.TenantID, step.StepOrder,
			step.TargetType, step.TargetID, step.ChannelType, step.TimeoutSeconds, now)
		if err != nil {
			return Chain{}, nil, fmt.Errorf("create step %d: %w", i, err)
		}
		created[i] = step
	}

	return chain, created, nil
}

// GetChain retrieves a chain by ID within a tenant.
func (s *Service) GetChain(ctx context.Context, tenantID, chainID string) (Chain, error) {
	var c Chain
	err := s.db.QueryRowContext(ctx, `
		SELECT chain_id, tenant_id, name, description, enabled, created_at, updated_at
		FROM escalation_chains
		WHERE tenant_id = ? AND chain_id = ?`,
		tenantID, chainID).Scan(&c.ChainID, &c.TenantID, &c.Name, &c.Description, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Chain{}, ErrChainNotFound
	}
	if err != nil {
		return Chain{}, fmt.Errorf("get chain: %w", err)
	}
	return c, nil
}

// ListChains returns all escalation chains for a tenant.
func (s *Service) ListChains(ctx context.Context, tenantID string) ([]Chain, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT chain_id, tenant_id, name, description, enabled, created_at, updated_at
		FROM escalation_chains
		WHERE tenant_id = ?
		ORDER BY created_at`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list chains: %w", err)
	}
	defer rows.Close()

	var out []Chain
	for rows.Next() {
		var c Chain
		if err := rows.Scan(&c.ChainID, &c.TenantID, &c.Name, &c.Description, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chain: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteChain removes a chain and its steps.
func (s *Service) DeleteChain(ctx context.Context, tenantID, chainID string) error {
	// Delete steps first.
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM escalation_steps WHERE tenant_id = ? AND chain_id = ?`,
		tenantID, chainID)
	if err != nil {
		return fmt.Errorf("delete steps: %w", err)
	}

	res, err := s.db.ExecContext(ctx, `
		DELETE FROM escalation_chains WHERE tenant_id = ? AND chain_id = ?`,
		tenantID, chainID)
	if err != nil {
		return fmt.Errorf("delete chain: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrChainNotFound
	}
	return nil
}

// GetSteps returns all steps for a chain, ordered by step_order.
func (s *Service) GetSteps(ctx context.Context, tenantID, chainID string) ([]Step, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT step_id, chain_id, tenant_id, step_order, target_type, target_id, channel_type, timeout_seconds, created_at
		FROM escalation_steps
		WHERE tenant_id = ? AND chain_id = ?
		ORDER BY step_order`, tenantID, chainID)
	if err != nil {
		return nil, fmt.Errorf("get steps: %w", err)
	}
	defer rows.Close()

	var out []Step
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.StepID, &st.ChainID, &st.TenantID, &st.StepOrder,
			&st.TargetType, &st.TargetID, &st.ChannelType, &st.TimeoutSeconds, &st.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan step: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// ---------- Alert escalation state machine ----------

// StartEscalation begins tracking an alert through an escalation chain.
// It creates the alert_escalation record in "pending" state and immediately
// notifies the first tier, transitioning to "notified".
func (s *Service) StartEscalation(ctx context.Context, tenantID, alertID, chainID string) (*AlertEscalation, error) {
	steps, err := s.GetSteps(ctx, tenantID, chainID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, ErrChainNoSteps
	}

	now := s.clock()
	nextEsc := now.Add(time.Duration(steps[0].TimeoutSeconds) * time.Second)

	ae := &AlertEscalation{
		AlertID:        alertID,
		TenantID:       tenantID,
		ChainID:        chainID,
		CurrentStep:    1,
		State:          StateNotified,
		NextEscalation: &nextEsc,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO alert_escalations (alert_id, tenant_id, chain_id, current_step, state, next_escalation, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ae.AlertID, ae.TenantID, ae.ChainID, ae.CurrentStep, ae.State, ae.NextEscalation, now, now)
	if err != nil {
		return nil, fmt.Errorf("start escalation: %w", err)
	}

	// Notify first tier.
	if s.notifier != nil {
		_ = s.notifier.Notify(alertID, steps[0])
	}

	return ae, nil
}

// AcknowledgeAlert marks an alert as acknowledged, stopping further escalation.
func (s *Service) AcknowledgeAlert(ctx context.Context, tenantID, alertID, userID string) (*AlertEscalation, error) {
	ae, err := s.getAlertEscalation(ctx, tenantID, alertID)
	if err != nil {
		return nil, err
	}

	if ae.State == StateAcknowledged || ae.State == StateResolved {
		return nil, ErrAlreadyAcknowledged
	}

	now := s.clock()
	ae.State = StateAcknowledged
	ae.AckedBy = userID
	ae.AckedAt = &now
	ae.NextEscalation = nil
	ae.UpdatedAt = now

	_, err = s.db.ExecContext(ctx, `
		UPDATE alert_escalations
		SET state = ?, acked_by = ?, acked_at = ?, next_escalation = NULL, updated_at = ?
		WHERE tenant_id = ? AND alert_id = ?`,
		ae.State, ae.AckedBy, ae.AckedAt, now, tenantID, alertID)
	if err != nil {
		return nil, fmt.Errorf("acknowledge alert: %w", err)
	}

	return ae, nil
}

// ProcessTimeouts checks for alerts whose next_escalation has passed and
// advances them to the next tier. This should be called periodically by a
// background worker.
func (s *Service) ProcessTimeouts(ctx context.Context, tenantID string) (int, error) {
	now := s.clock()
	rows, err := s.db.QueryContext(ctx, `
		SELECT alert_id, tenant_id, chain_id, current_step, state
		FROM alert_escalations
		WHERE tenant_id = ? AND state = ? AND next_escalation <= ?`,
		tenantID, StateNotified, now)
	if err != nil {
		return 0, fmt.Errorf("query timeouts: %w", err)
	}
	defer rows.Close()

	var alerts []AlertEscalation
	for rows.Next() {
		var ae AlertEscalation
		if err := rows.Scan(&ae.AlertID, &ae.TenantID, &ae.ChainID, &ae.CurrentStep, &ae.State); err != nil {
			return 0, fmt.Errorf("scan timeout: %w", err)
		}
		alerts = append(alerts, ae)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	processed := 0
	for _, ae := range alerts {
		if err := s.advanceEscalation(ctx, ae); err != nil {
			continue // log and continue in production
		}
		processed++
	}
	return processed, nil
}

// advanceEscalation moves an alert to the next tier or to the terminal state.
func (s *Service) advanceEscalation(ctx context.Context, ae AlertEscalation) error {
	steps, err := s.GetSteps(ctx, ae.TenantID, ae.ChainID)
	if err != nil {
		return err
	}

	nextStepIdx := ae.CurrentStep // 1-based, so this is the index of the next step
	now := s.clock()

	if nextStepIdx >= len(steps) {
		// All tiers exhausted. Check if the last step was PagerDuty.
		lastStep := steps[len(steps)-1]
		newState := StateExhausted
		if lastStep.ChannelType == ChannelPagerDuty || lastStep.TargetType == TargetPagerDuty {
			newState = StatePagerDutyFallback
			if s.pd != nil {
				_ = s.pd.CreateIncident(ae.AlertID, ae.TenantID, "Escalation exhausted for alert "+ae.AlertID)
			}
		}

		_, err := s.db.ExecContext(ctx, `
			UPDATE alert_escalations
			SET state = ?, next_escalation = NULL, updated_at = ?
			WHERE tenant_id = ? AND alert_id = ?`,
			newState, now, ae.TenantID, ae.AlertID)
		return err
	}

	// Advance to next tier.
	nextStep := steps[nextStepIdx]
	nextEsc := now.Add(time.Duration(nextStep.TimeoutSeconds) * time.Second)

	// If this tier is PagerDuty, fire PD immediately.
	if nextStep.ChannelType == ChannelPagerDuty || nextStep.TargetType == TargetPagerDuty {
		if s.pd != nil {
			_ = s.pd.CreateIncident(ae.AlertID, ae.TenantID, "Escalation tier "+fmt.Sprintf("%d", nextStepIdx+1)+" for alert "+ae.AlertID)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE alert_escalations
		SET current_step = ?, state = ?, next_escalation = ?, updated_at = ?
		WHERE tenant_id = ? AND alert_id = ?`,
		nextStepIdx+1, StateNotified, nextEsc, now, ae.TenantID, ae.AlertID)
	if err != nil {
		return fmt.Errorf("advance escalation: %w", err)
	}

	// Notify the next tier.
	if s.notifier != nil {
		_ = s.notifier.Notify(ae.AlertID, nextStep)
	}

	return nil
}

// GetAlertEscalation returns the current escalation state for an alert.
func (s *Service) GetAlertEscalation(ctx context.Context, tenantID, alertID string) (*AlertEscalation, error) {
	return s.getAlertEscalation(ctx, tenantID, alertID)
}

func (s *Service) getAlertEscalation(ctx context.Context, tenantID, alertID string) (*AlertEscalation, error) {
	var ae AlertEscalation
	var ackedBy sql.NullString
	var ackedAt sql.NullTime
	var nextEsc sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT alert_id, tenant_id, chain_id, current_step, state, acked_by, acked_at, next_escalation, created_at, updated_at
		FROM alert_escalations
		WHERE tenant_id = ? AND alert_id = ?`,
		tenantID, alertID).Scan(
		&ae.AlertID, &ae.TenantID, &ae.ChainID, &ae.CurrentStep, &ae.State,
		&ackedBy, &ackedAt, &nextEsc, &ae.CreatedAt, &ae.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAlertNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get alert escalation: %w", err)
	}

	if ackedBy.Valid {
		ae.AckedBy = ackedBy.String
	}
	if ackedAt.Valid {
		t := ackedAt.Time
		ae.AckedAt = &t
	}
	if nextEsc.Valid {
		t := nextEsc.Time
		ae.NextEscalation = &t
	}

	return &ae, nil
}
