package mediamtxsupervisor

import (
	"context"
	"os/exec"

	"github.com/bluenviron/mediamtx/internal/shared/sidecar"
)

// MediaMTXSidecar adapts a Raikada subprocess invocation to the
// sidecar.Sidecar interface so it can be managed by the generic
// process supervisor in internal/shared/sidecar.
//
// On every (re)start the generic supervisor calls Command() and
// streams stdout/stderr through structured logging. After the first
// successful HealthCheck (which delegates to a Controller.Healthy)
// the generic supervisor invokes OnReady, at which point the
// MediaMTXSupervisor's first reload is safe to run.
type MediaMTXSidecar struct {
	// SidecarName is the short identifier used for logging and
	// supervisor lookups. Must be unique within a Supervisor.
	SidecarName string

	// Binary is the path to the raikada executable.
	Binary string

	// ConfigPath is the YAML config to launch with. Per CLAUDE.md
	// callers MUST NOT modify this file's runtime settings; it's
	// the operator-managed mediamtx.yml.
	ConfigPath string

	// EnvVars are extra environment entries (KEY=VALUE) appended
	// to the inherited process env.
	EnvVars []string

	// Workdir is the subprocess cwd. Empty inherits.
	Workdir string

	// Probe is the readiness/liveness probe — typically the
	// HTTPController's Healthy method.
	Probe func(ctx context.Context) error

	// Ready is invoked once per successful (re)start, after the
	// first nil Probe. Use it to trigger the first MediaMTXSupervisor
	// reload.
	Ready func()
}

// Compile-time interface check.
var _ sidecar.Sidecar = (*MediaMTXSidecar)(nil)

func (m *MediaMTXSidecar) Name() string { return m.SidecarName }

func (m *MediaMTXSidecar) Command(ctx context.Context) *exec.Cmd {
	args := []string{}
	if m.ConfigPath != "" {
		args = append(args, m.ConfigPath)
	}
	return exec.CommandContext(ctx, m.Binary, args...)
}

func (m *MediaMTXSidecar) HealthCheck(ctx context.Context) error {
	if m.Probe == nil {
		return nil
	}
	return m.Probe(ctx)
}

func (m *MediaMTXSidecar) OnReady() {
	if m.Ready != nil {
		m.Ready()
	}
}

func (m *MediaMTXSidecar) Env() []string   { return m.EnvVars }
func (m *MediaMTXSidecar) WorkDir() string { return m.Workdir }
