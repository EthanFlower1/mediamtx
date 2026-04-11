// Package notifications — adapter.go provides adapter wrappers that bridge
// the CommsDeliveryChannel and PushDeliveryChannel interfaces to the unified
// DeliveryChannel interface. This allows the Dispatcher and ChannelRegistry
// to route to all 10 notification adapters through a single interface.
package notifications

import (
	"context"
	"time"
)

// MessageTypeComms is the synthetic MessageType used by comms adapters
// (Slack, Teams, PagerDuty, Opsgenie, Webhook).
const MessageTypeComms MessageType = "comms"

// MessageTypePush is the synthetic MessageType used by push adapters
// (FCM, APNs, WebPush).
const MessageTypePush MessageType = "push"

// -----------------------------------------------------------------------
// CommsChannelAdapter — wraps CommsDeliveryChannel as DeliveryChannel
// -----------------------------------------------------------------------

// CommsChannelAdapter wraps a CommsDeliveryChannel (Slack, Teams, PagerDuty,
// Opsgenie, Webhook) so it satisfies the DeliveryChannel interface and can
// be registered in the ChannelRegistry.
type CommsChannelAdapter struct {
	inner CommsDeliveryChannel
}

// NewCommsChannelAdapter returns a DeliveryChannel that delegates to the
// given CommsDeliveryChannel.
func NewCommsChannelAdapter(inner CommsDeliveryChannel) *CommsChannelAdapter {
	return &CommsChannelAdapter{inner: inner}
}

// Name returns the channel type as a lowercase string (e.g. "slack").
func (a *CommsChannelAdapter) Name() string {
	return string(a.inner.Type())
}

// SupportedTypes returns [MessageTypeComms].
func (a *CommsChannelAdapter) SupportedTypes() []MessageType {
	return []MessageType{MessageTypeComms}
}

// Send converts a Message to a CommsMessage, delegates to the inner
// channel, and maps the result back to a DeliveryResult.
func (a *CommsChannelAdapter) Send(ctx context.Context, msg Message) (DeliveryResult, error) {
	cm := messageToComms(msg)
	cr := a.inner.Send(ctx, cm)
	return commsResultToDelivery(cr, msg), nil
}

// BatchSend converts the Message into one CommsMessage per recipient,
// delegates to the inner channel's BatchSend, and maps results back.
func (a *CommsChannelAdapter) BatchSend(ctx context.Context, msg Message) ([]DeliveryResult, error) {
	cms := make([]CommsMessage, len(msg.To))
	for i, r := range msg.To {
		cm := messageToComms(msg)
		cm.ID = msg.ID
		if len(msg.To) > 1 {
			cm.Extra = copyMap(cm.Extra)
			if cm.Extra == nil {
				cm.Extra = make(map[string]string)
			}
			cm.Extra["recipient"] = r.Address
		}
		cms[i] = cm
	}

	crs := a.inner.BatchSend(ctx, cms)
	results := make([]DeliveryResult, len(crs))
	for i, cr := range crs {
		dr := commsResultToDelivery(cr, msg)
		if i < len(msg.To) {
			dr.Recipient = msg.To[i].Address
		}
		results[i] = dr
	}
	return results, nil
}

// CheckHealth delegates to the inner channel and translates to HealthStatus.
func (a *CommsChannelAdapter) CheckHealth(ctx context.Context) (HealthStatus, error) {
	start := time.Now()
	err := a.inner.CheckHealth(ctx)
	latency := time.Since(start)
	hs := HealthStatus{
		Healthy:   err == nil,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}
	if err != nil {
		hs.Message = err.Error()
	}
	return hs, nil
}

// -----------------------------------------------------------------------
// PushChannelAdapter — wraps PushDeliveryChannel as DeliveryChannel
// -----------------------------------------------------------------------

// PushChannelAdapter wraps a PushDeliveryChannel (FCM, APNs, WebPush) so
// it satisfies the DeliveryChannel interface and can be registered in the
// ChannelRegistry.
type PushChannelAdapter struct {
	inner PushDeliveryChannel
	name  string
}

// NewPushChannelAdapter returns a DeliveryChannel that delegates to the
// given PushDeliveryChannel. The name parameter is used as the registry
// key and Prometheus label (e.g. "fcm", "apns", "webpush").
func NewPushChannelAdapter(inner PushDeliveryChannel, name string) *PushChannelAdapter {
	return &PushChannelAdapter{inner: inner, name: name}
}

// Name returns the adapter name (e.g. "fcm").
func (a *PushChannelAdapter) Name() string {
	return a.name
}

// SupportedTypes returns [MessageTypePush].
func (a *PushChannelAdapter) SupportedTypes() []MessageType {
	return []MessageType{MessageTypePush}
}

// Send converts a Message to a PushMessage, delegates to the inner
// channel, and maps the result back to a DeliveryResult.
func (a *PushChannelAdapter) Send(ctx context.Context, msg Message) (DeliveryResult, error) {
	pm := messageToPush(msg)
	pr, err := a.inner.Send(ctx, pm)
	if err != nil {
		return DeliveryResult{
			MessageID:    msg.ID,
			Recipient:    pm.Target,
			State:        DeliveryStateFailed,
			Error:        err,
			ErrorMessage: err.Error(),
			Timestamp:    time.Now().UTC(),
		}, nil
	}
	return pushResultToDelivery(pr, msg), nil
}

// BatchSend converts the Message into a BatchMessage with one target per
// recipient, delegates to the inner channel, and maps results back.
func (a *PushChannelAdapter) BatchSend(ctx context.Context, msg Message) ([]DeliveryResult, error) {
	bm := BatchMessage{
		MessageID: msg.ID,
		TenantID:  msg.TenantID,
		Title:     msg.Subject,
		Body:      msg.Body,
		Data:      msg.Metadata,
	}
	bm.Targets = make([]Target, len(msg.To))
	for i, r := range msg.To {
		bm.Targets[i] = Target{
			DeviceToken: r.Address,
			Platform:    platformFromMetadata(r.Metadata),
		}
	}

	prs, err := a.inner.BatchSend(ctx, bm)
	if err != nil {
		// Return all-failed results
		results := make([]DeliveryResult, len(msg.To))
		for i, r := range msg.To {
			results[i] = DeliveryResult{
				MessageID:    msg.ID,
				Recipient:    r.Address,
				State:        DeliveryStateFailed,
				Error:        err,
				ErrorMessage: err.Error(),
				Timestamp:    time.Now().UTC(),
			}
		}
		return results, nil
	}

	results := make([]DeliveryResult, len(prs))
	for i, pr := range prs {
		results[i] = pushResultToDelivery(pr, msg)
	}
	return results, nil
}

// CheckHealth delegates to the inner channel and translates to HealthStatus.
func (a *PushChannelAdapter) CheckHealth(ctx context.Context) (HealthStatus, error) {
	start := time.Now()
	err := a.inner.CheckHealth(ctx)
	latency := time.Since(start)
	hs := HealthStatus{
		Healthy:   err == nil,
		Latency:   latency,
		CheckedAt: time.Now().UTC(),
	}
	if err != nil {
		hs.Message = err.Error()
	}
	return hs, nil
}

// -----------------------------------------------------------------------
// Mapping helpers
// -----------------------------------------------------------------------

// messageToComms translates a unified Message into a CommsMessage.
func messageToComms(msg Message) CommsMessage {
	return CommsMessage{
		ID:        msg.ID,
		TenantID:  msg.TenantID,
		Summary:   msg.Subject,
		Body:      msg.Body,
		Severity:  severityFromPriority(msg.Priority),
		Timestamp: time.Now().UTC(),
		Extra:     msg.Metadata,
	}
}

// messageToPush translates a unified Message into a PushMessage targeting
// the first recipient.
func messageToPush(msg Message) PushMessage {
	pm := PushMessage{
		MessageID: msg.ID,
		TenantID:  msg.TenantID,
		Title:     msg.Subject,
		Body:      msg.Body,
		Data:      msg.Metadata,
	}
	if len(msg.To) > 0 {
		pm.Target = msg.To[0].Address
		pm.Platform = platformFromMetadata(msg.To[0].Metadata)
	}
	switch msg.Priority {
	case PriorityHigh, PriorityCritical:
		pm.Priority = "high"
	default:
		pm.Priority = "normal"
	}
	return pm
}

// commsResultToDelivery maps a CommsDeliveryResult to a DeliveryResult.
func commsResultToDelivery(cr CommsDeliveryResult, msg Message) DeliveryResult {
	dr := DeliveryResult{
		MessageID:    cr.MessageID,
		State:        mapCommsState(cr.State),
		ErrorMessage: cr.ErrorMessage,
		Timestamp:    time.Now().UTC(),
	}
	if len(msg.To) > 0 {
		dr.Recipient = msg.To[0].Address
	}
	return dr
}

// pushResultToDelivery maps a PushDeliveryResult to a DeliveryResult.
func pushResultToDelivery(pr PushDeliveryResult, msg Message) DeliveryResult {
	return DeliveryResult{
		MessageID:         msg.ID,
		ProviderMessageID: pr.PlatformID,
		Recipient:         pr.Target,
		State:             mapPushState(pr.State),
		ErrorMessage:      pr.ErrorMessage,
		Timestamp:         time.Now().UTC(),
	}
}

// mapCommsState maps CommsDeliveryState to DeliveryState.
func mapCommsState(s CommsDeliveryState) DeliveryState {
	switch s {
	case CommsDeliverySuccess:
		return DeliveryStateDelivered
	case CommsDeliveryFailure:
		return DeliveryStateFailed
	default:
		return DeliveryStateFailed
	}
}

// mapPushState maps PushDeliveryState to DeliveryState.
func mapPushState(s PushDeliveryState) DeliveryState {
	switch s {
	case PushStateDelivered:
		return DeliveryStateDelivered
	case PushStateFailed:
		return DeliveryStateFailed
	case PushStateThrottled:
		return DeliveryStateFailed
	case PushStateUnreachable:
		return DeliveryStateSuppressed
	default:
		return DeliveryStateFailed
	}
}

// severityFromPriority maps Priority to Severity for comms channels.
func severityFromPriority(p Priority) Severity {
	switch p {
	case PriorityCritical:
		return SeverityCritical
	case PriorityHigh:
		return SeverityHigh
	case PriorityNormal:
		return SeverityWarning
	default:
		return SeverityInfo
	}
}

// platformFromMetadata extracts a Platform from recipient metadata.
func platformFromMetadata(md map[string]string) Platform {
	if md == nil {
		return ""
	}
	return Platform(md["platform"])
}

// copyMap returns a shallow copy of the map.
func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
