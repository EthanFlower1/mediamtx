package comms

import "context"

// Sender is the interface that platform-specific integrations must implement.
type Sender interface {
	// Platform returns which messaging platform this sender targets.
	Platform() Platform

	// PostAlert sends an alert card to the specified channel.
	PostAlert(ctx context.Context, channelRef string, alert Alert) (PostResult, error)

	// HandleAction processes an interactive card action (ack, triage, watch clip).
	HandleAction(ctx context.Context, action CardAction) (ActionResult, error)
}
