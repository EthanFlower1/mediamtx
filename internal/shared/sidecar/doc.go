// Package sidecar provides a generic supervisor framework for managed
// subprocess "sidecars" such as Zitadel and MediaMTX.
//
// The Supervisor owns a fleet of named Sidecar implementations. For each
// one it runs a control loop that:
//
//   - Starts the underlying OS process via exec.Cmd.
//   - Captures stdout/stderr through io.Pipe and re-emits each line
//     through an slog.Logger tagged with the sidecar's Name() as the
//     "component" field (see internal/shared/logging).
//   - Waits for HealthCheck(ctx) to return nil before declaring the
//     sidecar ready, calling OnReady() exactly once per successful
//     start.
//   - If the process exits, or the health check fails, restarts the
//     sidecar with exponential backoff (base 500ms, cap 60s, plus
//     jitter) until Shutdown is called.
//   - On Shutdown, sends SIGTERM to every running sidecar and waits
//     up to GracePeriod for them to exit before escalating to SIGKILL.
//
// # Network binding contract
//
// Sidecar implementations MUST bind only to the loopback interface
// (127.0.0.1) or to unix domain sockets. The supervisor cannot
// actually enforce this — the kernel does not give a parent process
// a clean hook to observe a child's listen(2) calls — but every
// Sidecar in this tree is expected to honor the contract. Reviewers
// should reject sidecar implementations that bind to 0.0.0.0 or a
// routable address.
//
// # Metrics
//
// Supervisor.Stats returns a snapshot of CrashCount, LastRestart
// and Uptime per sidecar, intended to be surfaced by the owning
// process (e.g. /metrics or /healthz).
package sidecar
