package summaries

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"
)

// IDGen generates random hex IDs.
type IDGen func() string

func defaultIDGen() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SchedulerConfig bundles dependencies for the Scheduler.
type SchedulerConfig struct {
	Aggregator      *Aggregator
	PromptBuilder   *PromptBuilder
	TritonClient    *TritonClient
	DeliveryService *DeliveryService
	IDGen           IDGen
	Logger          *log.Logger
	// TenantLister returns the list of active tenant IDs.
	// The scheduler calls this on each tick to discover tenants.
	TenantLister func(ctx context.Context) ([]string, error)
}

// Scheduler runs periodic summary generation. It maintains separate
// timers for daily and weekly cadences and processes each tenant
// independently to enforce per-tenant data isolation.
type Scheduler struct {
	cfg    SchedulerConfig
	idGen  IDGen
	logger *log.Logger

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler constructs a Scheduler. Call Start to begin background
// processing, and Stop to shut down gracefully.
func NewScheduler(cfg SchedulerConfig) *Scheduler {
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = defaultIDGen
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Scheduler{
		cfg:    cfg,
		idGen:  idGen,
		logger: logger,
	}
}

// Start begins the background scheduler goroutines. It returns immediately.
func (s *Scheduler) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runDaily(ctx)
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runWeekly(ctx)
	}()

	s.logger.Printf("summaries: scheduler started")
}

// Stop gracefully shuts down the scheduler, waiting for in-flight
// summary generation to complete.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	s.logger.Printf("summaries: scheduler stopped")
}

// GenerateOnDemand runs summary generation for a single tenant and period
// synchronously. This is the entry point for the API endpoint.
func (s *Scheduler) GenerateOnDemand(ctx context.Context, tenantID string, period SummaryPeriod) (*Summary, error) {
	now := time.Now().UTC()
	start, end := periodBounds(now, period)
	return s.generateForTenant(ctx, tenantID, period, start, end)
}

func (s *Scheduler) runDaily(ctx context.Context) {
	// Calculate time until next midnight UTC.
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runForAllTenants(ctx, PeriodDaily)
		}
	}
}

func (s *Scheduler) runWeekly(ctx context.Context) {
	ticker := time.NewTicker(7 * 24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runForAllTenants(ctx, PeriodWeekly)
		}
	}
}

func (s *Scheduler) runForAllTenants(ctx context.Context, period SummaryPeriod) {
	if s.cfg.TenantLister == nil {
		s.logger.Printf("summaries: no tenant lister configured, skipping %s run", period)
		return
	}

	tenants, err := s.cfg.TenantLister(ctx)
	if err != nil {
		s.logger.Printf("summaries: failed to list tenants: %v", err)
		return
	}

	now := time.Now().UTC()
	start, end := periodBounds(now, period)

	for _, tenantID := range tenants {
		select {
		case <-ctx.Done():
			return
		default:
		}

		summary, err := s.generateForTenant(ctx, tenantID, period, start, end)
		if err != nil {
			s.logger.Printf("summaries: generate failed tenant=%s period=%s: %v",
				tenantID, period, err)
			continue
		}

		if s.cfg.DeliveryService != nil {
			if err := s.cfg.DeliveryService.Deliver(ctx, summary); err != nil {
				s.logger.Printf("summaries: delivery failed tenant=%s: %v", tenantID, err)
			}
		}
	}
}

func (s *Scheduler) generateForTenant(ctx context.Context, tenantID string, period SummaryPeriod, start, end time.Time) (*Summary, error) {
	// Step 1: Aggregate events.
	agg, err := s.cfg.Aggregator.Aggregate(ctx, tenantID, start, end)
	if err != nil {
		return nil, fmt.Errorf("aggregate: %w", err)
	}

	// Step 2: Build prompt.
	userPrompt := s.cfg.PromptBuilder.Build(agg)
	sysPrompt := s.cfg.PromptBuilder.SystemPrompt()

	// Step 3: LLM inference via Triton.
	text, err := s.cfg.TritonClient.Infer(ctx, sysPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("infer: %w", err)
	}

	// Step 4: Build summary.
	summary := &Summary{
		SummaryID:   s.idGen(),
		TenantID:    tenantID,
		Period:      period,
		StartTime:   start,
		EndTime:     end,
		Text:        text,
		EventCount:  agg.TotalEvents,
		GeneratedAt: time.Now().UTC(),
	}

	return summary, nil
}

// periodBounds returns the [start, end) time window for the given period
// relative to the reference time.
func periodBounds(ref time.Time, period SummaryPeriod) (time.Time, time.Time) {
	switch period {
	case PeriodWeekly:
		end := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, time.UTC)
		start := end.AddDate(0, 0, -7)
		return start, end
	default: // daily
		end := time.Date(ref.Year(), ref.Month(), ref.Day(), 0, 0, 0, 0, time.UTC)
		start := end.AddDate(0, 0, -1)
		return start, end
	}
}
