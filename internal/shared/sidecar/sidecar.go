package sidecar

import (
	"context"
	"os/exec"
)

// Sidecar describes a single managed subprocess.
//
// Implementations are expected to be stateless with respect to the
// supervisor: Command() is called fresh on every (re)start so that the
// returned *exec.Cmd is not reused across process lifetimes — exec.Cmd
// values are single-use.
//
// # Binding contract
//
// Implementations MUST configure the underlying subprocess to bind
// only to the loopback interface (127.0.0.1, ::1) or to unix domain
// sockets. The supervisor cannot enforce this; it is a reviewer
// concern. See the package doc comment.
type Sidecar interface {
	// Name returns a stable, short identifier for this sidecar. It
	// is used as the logging "component" field and as the lookup
	// key in the Supervisor. Two Sidecars with the same Name in
	// one Supervisor are a programming error.
	Name() string

	// Command returns a new *exec.Cmd representing the subprocess
	// to launch. The supervisor will attach stdout/stderr pipes,
	// apply Env() and WorkDir(), then Start() the command.
	//
	// Command must return a fresh *exec.Cmd on every call;
	// returning a previously-used exec.Cmd will panic inside
	// Start().
	Command(ctx context.Context) *exec.Cmd

	// HealthCheck performs a cheap liveness probe against the
	// running sidecar. It must be safe to call repeatedly at the
	// supervisor's configured interval. Returning nil signals
	// "healthy"; any non-nil error is treated as unhealthy.
	//
	// The first nil return after a Start causes the supervisor to
	// invoke OnReady exactly once.
	HealthCheck(ctx context.Context) error

	// OnReady is called exactly once per successful start, right
	// after HealthCheck first returns nil. Use it to publish the
	// sidecar's listen address, trigger a warm-up RPC, or signal
	// a downstream component that the sidecar is usable.
	OnReady()

	// Env returns additional environment variables for the
	// subprocess in "KEY=VALUE" form. The current process
	// environment is inherited in addition to these entries.
	Env() []string

	// WorkDir returns the working directory for the subprocess.
	// An empty string means "inherit from the supervisor".
	WorkDir() string
}
