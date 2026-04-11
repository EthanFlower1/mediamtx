package summaries

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/notifications"
)

// MessageDispatcher is the interface for dispatching notification messages.
// It matches the Dispatch method on *notifications.Dispatcher so the real
// dispatcher can be used directly, while tests can supply a fake.
type MessageDispatcher interface {
	Dispatch(ctx context.Context, msg notifications.Message) ([]notifications.DeliveryResult, error)
}

// DeliveryService delivers generated summaries to tenant users via the
// notification infrastructure. It maps summary periods to notification
// event types:
//   - PeriodDaily  -> "summary.daily"
//   - PeriodWeekly -> "summary.weekly"
type DeliveryService struct {
	notifSvc   *notifications.Service
	dispatcher MessageDispatcher
	formatter  *Formatter
	logger     *log.Logger
}

// NewDeliveryService constructs a DeliveryService. If dispatcher is nil the
// service falls back to log-only delivery for backward compatibility.
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

// SetDispatcher configures the message dispatcher used by deliverToChannel.
// This allows wiring the real notifications.Dispatcher after construction.
func (ds *DeliveryService) SetDispatcher(d MessageDispatcher) {
	ds.dispatcher = d
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
// When a dispatcher is configured it constructs a real notifications.Message
// and sends it. Otherwise it falls back to logging for backward compatibility.
func (ds *DeliveryService) deliverToChannel(ctx context.Context, s *Summary, target notifications.DeliveryTarget) error {
	var body, htmlBody string
	var msgType notifications.MessageType

	switch target.Channel.ChannelType {
	case notifications.ChannelEmail:
		body = ds.formatter.FormatPlainText(s)
		htmlBody = ds.formatter.FormatHTML(s)
		msgType = notifications.MessageTypeEmail
	case notifications.ChannelWebhook:
		body = ds.formatter.FormatPlainText(s)
		msgType = notifications.MessageTypeEmail // webhook uses plain text body
	default:
		body = ds.formatter.FormatPlainText(s)
		msgType = notifications.MessageTypeEmail
	}

	// If no dispatcher is wired, fall back to log-only delivery.
	if ds.dispatcher == nil {
		ds.logger.Printf("summaries: %s delivery (log-only) tenant=%s user=%s len=%d",
			target.Channel.ChannelType, s.TenantID, target.UserID, len(body))
		return nil
	}

	msg := notifications.Message{
		Type:     msgType,
		TenantID: s.TenantID,
		To: []notifications.Recipient{
			{Address: target.UserID},
		},
		Subject:  fmt.Sprintf("%s Event Summary (%s – %s)", periodLabel(s.Period), s.StartTime.Format(time.DateOnly), s.EndTime.Format(time.DateOnly)),
		Body:     body,
		HTMLBody: htmlBody,
	}

	results, err := ds.dispatcher.Dispatch(ctx, msg)
	if err != nil {
		return fmt.Errorf("summaries: dispatch to %s for user %s: %w",
			target.Channel.ChannelType, target.UserID, err)
	}

	for _, r := range results {
		ds.logger.Printf("summaries: dispatched tenant=%s user=%s channel=%s state=%s",
			s.TenantID, target.UserID, target.Channel.ChannelType, r.State)
	}

	return nil
}

// DeliverToTargets delivers a summary to the given targets without routing
// through the notification service. This is useful when the caller has
// already resolved delivery targets (e.g., in tests or batch pipelines).
func (ds *DeliveryService) DeliverToTargets(ctx context.Context, s *Summary, targets []notifications.DeliveryTarget) error {
	for _, target := range targets {
		if err := ds.deliverToChannel(ctx, s, target); err != nil {
			ds.logger.Printf("summaries: delivery failed tenant=%s user=%s channel=%s: %v",
				s.TenantID, target.UserID, target.Channel.ChannelType, err)
		}
	}
	return nil
}

func summaryEventType(p SummaryPeriod) string {
	return "summary." + string(p)
}
