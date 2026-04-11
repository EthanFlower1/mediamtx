package summaries

import (
	"context"
	"fmt"
	"log"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// DeliveryService delivers generated summaries to tenant users via the
// notification infrastructure. It maps summary periods to notification
// event types:
//   - PeriodDaily  -> "summary.daily"
//   - PeriodWeekly -> "summary.weekly"
type DeliveryService struct {
	notifSvc  *notifications.Service
	formatter *Formatter
	logger    *log.Logger
}

// NewDeliveryService constructs a DeliveryService.
func NewDeliveryService(notifSvc *notifications.Service, logger *log.Logger) *DeliveryService {
	if logger == nil {
		logger = log.Default()
	}
	return &DeliveryService{
		notifSvc:  notifSvc,
		formatter: NewFormatter(),
		logger:    logger,
	}
}

// Deliver routes a summary to all notification channels configured for
// the tenant. It logs delivery outcomes but does not fail on partial
// delivery — best-effort semantics.
func (ds *DeliveryService) Deliver(ctx context.Context, s *Summary) error {
	eventType := summaryEventType(s.Period)

	targets, err := ds.notifSvc.RouteNotification(ctx, s.TenantID, eventType)
	if err != nil {
		return fmt.Errorf("delivery route: %w", err)
	}

	if len(targets) == 0 {
		ds.logger.Printf("summaries: no delivery targets for tenant %s event %s", s.TenantID, eventType)
		return nil
	}

	for _, target := range targets {
		status := notifications.StatusSent
		errMsg := ""

		if deliverErr := ds.deliverToChannel(ctx, s, target); deliverErr != nil {
			status = notifications.StatusFailed
			errMsg = deliverErr.Error()
			ds.logger.Printf("summaries: delivery failed tenant=%s user=%s channel=%s: %v",
				s.TenantID, target.UserID, target.Channel.ChannelType, deliverErr)
		}

		// Log the delivery attempt regardless of outcome.
		_ = ds.notifSvc.LogDelivery(ctx, notifications.LogEntry{
			TenantID:     s.TenantID,
			UserID:       target.UserID,
			EventType:    eventType,
			ChannelType:  target.Channel.ChannelType,
			Status:       status,
			ErrorMessage: errMsg,
		})
	}

	return nil
}

// deliverToChannel dispatches a summary to a single channel target.
// In production this would call the actual email/webhook/push provider;
// for now it formats the content and logs the delivery.
func (ds *DeliveryService) deliverToChannel(_ context.Context, s *Summary, target notifications.DeliveryTarget) error {
	switch target.Channel.ChannelType {
	case notifications.ChannelEmail:
		content := ds.formatter.FormatHTML(s)
		ds.logger.Printf("summaries: email delivery tenant=%s user=%s len=%d",
			s.TenantID, target.UserID, len(content))
	case notifications.ChannelWebhook:
		content := ds.formatter.FormatPlainText(s)
		ds.logger.Printf("summaries: webhook delivery tenant=%s user=%s len=%d",
			s.TenantID, target.UserID, len(content))
	default:
		content := ds.formatter.FormatPlainText(s)
		ds.logger.Printf("summaries: %s delivery tenant=%s user=%s len=%d",
			target.Channel.ChannelType, s.TenantID, target.UserID, len(content))
	}
	return nil
}

func summaryEventType(p SummaryPeriod) string {
	return "summary." + string(p)
}
