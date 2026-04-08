# internal/shared/sidecar

Generic supervisor framework for managed subprocess "sidecars" used by the
Kaivue Recording Server — notably **Zitadel** (embedded IdP, see KAI-220) and
**MediaMTX** (streaming core, see KAI-259).

The recording-server binary runs these external services as child processes
rather than requiring a container runtime on the customer's hardware. This
package owns their lifecycle: start, health-check, crash-restart with
backoff, log capture, and graceful shutdown.

## Contract

A sidecar implements the `Sidecar` interface:

```go
type Sidecar interface {
    Name() string
    Command(ctx context.Context) *exec.Cmd
    HealthCheck(ctx context.Context) error
    OnReady()
    Env() []string
    WorkDir() string
}
```

### Network binding

Sidecar implementations **MUST** bind only to `127.0.0.1` (or `::1`) or to
unix domain sockets. The supervisor cannot enforce this at runtime, but every
Sidecar in this repo is reviewed against that contract. Binding to `0.0.0.0`
or to a routable interface is a security bug — all inter-component traffic
must flow over the tsnet mesh, never the sidecar's raw listen socket.

## Usage

```go
import (
    "context"
    "log/slog"
    "time"

    "github.com/bluenviron/mediamtx/internal/shared/logging"
    "github.com/bluenviron/mediamtx/internal/shared/sidecar"
)

func main() {
    log := logging.New(logging.Options{Component: "recorder"})

    sup := sidecar.NewSupervisor(sidecar.Config{
        HealthInterval: 5 * time.Second,
        GracePeriod:    15 * time.Second,
        Logger:         log,
    })
    defer sup.Shutdown()

    // Start MediaMTX sidecar.
    if err := sup.Start(context.Background(), newMediaMTXSidecar()); err != nil {
        log.Error("mediamtx start", slog.Any("err", err))
    }

    // Start Zitadel sidecar.
    if err := sup.Start(context.Background(), newZitadelSidecar()); err != nil {
        log.Error("zitadel start", slog.Any("err", err))
    }

    // Block until both are ready before exposing the public API.
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()
    _ = sup.WaitReady(ctx, "mediamtx")
    _ = sup.WaitReady(ctx, "zitadel")
}
```

## Example: MediaMTX sidecar (KAI-259)

```go
type mediamtxSidecar struct {
    binary     string // path to the bundled mediamtx binary
    configPath string // generated mediamtx.yml on disk
    apiAddr    string // e.g. "127.0.0.1:9997"
}

func (m *mediamtxSidecar) Name() string { return "mediamtx" }

func (m *mediamtxSidecar) Command(ctx context.Context) *exec.Cmd {
    return exec.CommandContext(ctx, m.binary, m.configPath)
}

func (m *mediamtxSidecar) HealthCheck(ctx context.Context) error {
    req, _ := http.NewRequestWithContext(ctx, "GET",
        "http://"+m.apiAddr+"/v3/config/global/get", nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return fmt.Errorf("mediamtx api status %d", resp.StatusCode)
    }
    return nil
}

func (m *mediamtxSidecar) OnReady()      { /* publish api addr, etc. */ }
func (m *mediamtxSidecar) Env() []string { return []string{"MTX_LOGLEVEL=info"} }
func (m *mediamtxSidecar) WorkDir() string { return "" }
```

## Example: Zitadel sidecar (KAI-220)

```go
type zitadelSidecar struct {
    binary     string
    configPath string
    grpcAddr   string // unix:/run/kaivue/zitadel.sock or 127.0.0.1:8080
}

func (z *zitadelSidecar) Name() string { return "zitadel" }

func (z *zitadelSidecar) Command(ctx context.Context) *exec.Cmd {
    return exec.CommandContext(ctx,
        z.binary, "start-from-init", "--config", z.configPath)
}

func (z *zitadelSidecar) HealthCheck(ctx context.Context) error {
    conn, err := grpc.DialContext(ctx, z.grpcAddr,
        grpc.WithTransportCredentials(insecure.NewCredentials()),
        grpc.WithBlock())
    if err != nil { return err }
    defer conn.Close()
    // Use the Zitadel healthz RPC.
    return healthpb.NewHealthClient(conn).Check(ctx, ...)
}

func (z *zitadelSidecar) OnReady()         {}
func (z *zitadelSidecar) Env() []string    { return []string{"ZITADEL_LOG_LEVEL=info"} }
func (z *zitadelSidecar) WorkDir() string  { return "" }
```

## Behaviour

- **Start**: launches the subprocess, starts pumping stdout/stderr into
  `slog` via `internal/shared/logging` with `component=<Name()>`.
- **Health**: polls `HealthCheck` every `Config.HealthInterval` (default 5s).
  The first nil return closes the ready channel and invokes `OnReady` exactly
  once per start. Subsequent failures trigger a restart.
- **Backoff**: exponential from 500ms, doubling, capped at 60s, with ±25%
  jitter. The counter resets on explicit Stop/Restart but not on a healthy
  run (so a flapping sidecar walks the backoff up).
- **Shutdown**: sends SIGTERM to the sidecar's process group, waits up to
  `Config.GracePeriod`, then escalates to SIGKILL. Shutdown blocks until
  every registered sidecar has exited.
- **Metrics**: `Supervisor.Stats()` returns `Running`, `Ready`, `CrashCount`,
  `LastRestart`, `StartedAt`, `Uptime` per sidecar — wire this into `/healthz`
  or Prometheus exporters.

## What this package does NOT do

- It does not provide concrete Sidecar implementations — those land in
  KAI-220 (Zitadel) and KAI-259 (MediaMTX).
- It does not enforce the network-binding contract (see above).
- It does not manage sidecar config files, TLS cert rotation, or auth
  token provisioning; those belong to the component that owns the sidecar.
