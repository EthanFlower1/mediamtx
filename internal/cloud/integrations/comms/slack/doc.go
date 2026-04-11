// Package slack implements the Slack integration for the Kaivue comms
// subsystem (KAI-409).
//
// Features:
//   - OAuth 2.0 flow for Slack workspace installation
//   - Block Kit alert cards with ack / triage / watch-clip actions
//   - Slash command handler (/kaivue status, /kaivue alerts)
//   - Webhook signature verification (v0 signing secret)
package slack
