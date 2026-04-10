// Package automation bridges Prometheus metrics to Statuspage.io component
// status updates. It runs a background loop that:
//
//  1. Queries a Prometheus endpoint for per-component health signals.
//  2. Evaluates threshold rules to determine component status.
//  3. Pushes status changes to Statuspage.io via the provider.Provider interface.
//
// The automation loop is designed to be idempotent: repeated evaluations with
// the same metric values produce zero API calls. Only transitions trigger
// updates, avoiding unnecessary Statuspage.io rate-limit consumption.
//
// KAI-375: Status automation + per-component monitoring integration.
package automation
