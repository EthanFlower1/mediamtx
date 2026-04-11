// Package comms implements first-party messaging integrations (Slack, Teams)
// for the Kaivue cloud platform (KAI-409).
//
// Each integration can post alert cards to configured channels, handle
// interactive actions (acknowledge, triage, watch clip), and deep-link
// back into the NVR clip viewer.
//
// Routing rules map event types to one or more destination channels
// across Slack workspaces and Teams tenants.
package comms
