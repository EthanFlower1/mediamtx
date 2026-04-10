// Package provider implements an HTTP client for the Atlassian Statuspage.io
// v1 REST API (https://developer.statuspage.io). It supports component CRUD,
// incident management, and component-group operations.
//
// The client is safe for concurrent use and exposes an interface (Provider) so
// that callers can swap in a mock for testing without an HTTP server.
//
// KAI-375: Statuspage.io setup + per-component monitoring integration.
package provider
