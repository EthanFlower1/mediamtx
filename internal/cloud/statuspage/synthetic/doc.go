// Package synthetic defines health-check endpoint configuration for external
// synthetic monitoring providers (Pingdom, Better Uptime, etc.) and exposes
// an HTTP handler that serves as the target for those probes.
//
// Design:
//   - The handler at /status/health returns a JSON body with per-component
//     health signals that synthetic monitors can parse.
//   - Check definitions are declarative so they can be exported as Pingdom or
//     Better Stack configuration via Terraform or API calls.
//
// KAI-375: Synthetic monitoring configuration.
package synthetic
