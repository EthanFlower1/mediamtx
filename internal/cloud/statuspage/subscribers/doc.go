// Package subscribers integrates the statuspage system with notification
// channels so that customers and integrators can subscribe to status updates
// via Email, SMS (Twilio), webhook, RSS, Slack, and Microsoft Teams.
//
// It owns the status_subscribers and status_events tables (migration 0023)
// and fans out notifications through the cloud/notifications channel
// abstraction when status changes or incidents occur.
package subscribers
