// Package pdk implements the ProdataKey (PDK) cloud access control
// integration for the Kaivue platform (KAI-405).
//
// PDK is a cloud-managed access control system. This package provides:
//   - An API client for querying doors, credentials, and access events
//   - A webhook receiver for real-time door event ingestion
//   - Video correlation that links door events to nearby camera recordings
//
// The integration is tenant-scoped: each Kaivue tenant configures their own
// PDK API credentials and door-to-camera mappings.
package pdk
