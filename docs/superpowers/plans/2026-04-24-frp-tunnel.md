# FRP Reverse Tunnel — Production Remote Access

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the JSON-over-WSS proxy with an embedded frp reverse tunnel, giving remote clients direct access to all on-prem services (API, HLS, WebRTC, RTSP) with the same API surface — just a different hostname.

**Architecture:** The on-prem Directory embeds an frp client (frpc) that connects outbound to our frp server (frps) on EC2. When remote access is enabled, all local ports are tunneled through frp to `{alias}.raikada.com`. Remote clients hit the same API paths they'd use on LAN. The WSS cloud connector stays for registration/heartbeat only — data flows through frp's optimized binary tunnel.

**Tech Stack:** Go 1.26+, `github.com/fatedier/frp` (Apache 2.0), Caddy for TLS termination, existing cloud broker for discovery

---

## File Structure

### On-Prem (Directory side)

| File | Responsibility |
|------|----------------|
| `internal/directory/tunnel/tunnel.go` | Embedded frp client — start/stop, configure proxy rules |
| `internal/directory/tunnel/tunnel_test.go` | Unit tests |
| `internal/directory/boot.go` | Modify: start tunnel when cloud enabled |
| `internal/directory/adminapi/cloud.go` | Modify: trigger tunnel start/stop on settings change |

### Cloud Side

| File | Responsibility |
|------|----------------|
| `cmd/cloudbroker/main.go` | Modify: embed frp server alongside broker |
| `internal/cloud/frpserver/server.go` | Embedded frp server with subdomain routing |
| `internal/cloud/frpserver/server_test.go` | Unit tests |

---

## Phase 1: Embed frp Server in Cloud Broker

### Task 1: Add frp dependency and create server wrapper

**Files:**
- Create: `internal/cloud/frpserver/server.go`
- Create: `internal/cloud/frpserver/server_test.go`

- [ ] **Step 1: Add frp to go.mod**

```bash
go get github.com/fatedier/frp@latest
```

- [ ] **Step 2: Write the failing test**

```go
// internal/cloud/frpserver/server_test.go
package frpserver

import (
	"testing"
	"time"
)

func TestServerStartsAndStops(t *testing.T) {
	s, err := New(Config{
		BindAddr:      "127.0.0.1",
		BindPort:      17000,
		VhostHTTPPort: 17080,
		SubDomainHost: "test.local",
		Token:         "test-token",
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	go s.Run()
	time.Sleep(500 * time.Millisecond)

	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/cloud/frpserver/... -v -run TestServer
```

- [ ] **Step 4: Write implementation**

```go
// internal/cloud/frpserver/server.go
package frpserver

import (
	"context"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/server"
)

// Config for the embedded frp server.
type Config struct {
	BindAddr      string // e.g. "0.0.0.0"
	BindPort      int    // e.g. 7000
	VhostHTTPPort int    // e.g. 8080 (Caddy reverse-proxies HTTPS → here)
	SubDomainHost string // e.g. "raikada.com" — clients get {alias}.raikada.com
	Token         string // shared auth token between frpc and frps
}

// Server wraps the frp server for embedding.
type Server struct {
	svc    *server.Service
	cancel context.CancelFunc
}

// New creates a new embedded frp server.
func New(cfg Config) (*Server, error) {
	frpsCfg := &v1.ServerConfig{
		BindAddr:      cfg.BindAddr,
		BindPort:      cfg.BindPort,
		VhostHTTPPort: cfg.VhostHTTPPort,
		SubDomainHost: cfg.SubDomainHost,
	}
	frpsCfg.Auth.Token = cfg.Token

	svc, err := server.NewService(frpsCfg)
	if err != nil {
		return nil, err
	}

	return &Server{svc: svc}, nil
}

// Run starts the frp server. Blocks until Close is called.
func (s *Server) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	return s.svc.Run(ctx)
}

// Close stops the frp server.
func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}
	s.svc.Close()
	return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
go test ./internal/cloud/frpserver/... -v -run TestServer
```

- [ ] **Step 6: Commit**

```bash
git add internal/cloud/frpserver/ go.mod go.sum
git commit -m "feat(frpserver): embed frp server for production reverse tunnel"
```

---

### Task 2: Add frp server to cloud broker

**Files:**
- Modify: `cmd/cloudbroker/main.go`

- [ ] **Step 1: Add frp server startup to the broker**

Read `cmd/cloudbroker/main.go`. Add:
- A new flag: `-frp-port` (default 7000) for the frp control port
- A new flag: `-frp-http-port` (default 7080) for the frp vhost HTTP port
- A new flag: `-subdomain-host` (default "raikada.com")
- Start the frp server in a goroutine before the HTTP server
- Use the same `-token` flag for frp auth

```go
import "github.com/bluenviron/mediamtx/internal/cloud/frpserver"

// Add flags
frpPort := flag.Int("frp-port", 7000, "frp control port")
frpHTTPPort := flag.Int("frp-http-port", 7080, "frp vhost HTTP port")
subdomainHost := flag.String("subdomain-host", "raikada.com", "subdomain host for frp")

// Start frp server
frpSrv, err := frpserver.New(frpserver.Config{
    BindAddr:      "0.0.0.0",
    BindPort:      *frpPort,
    VhostHTTPPort: *frpHTTPPort,
    SubDomainHost: *subdomainHost,
    Token:         *token,
})
if err != nil {
    log.Error("frp server failed", "error", err)
    os.Exit(1)
}
go frpSrv.Run()
defer frpSrv.Close()
```

- [ ] **Step 2: Verify compilation**

```bash
go build ./cmd/cloudbroker/...
```

- [ ] **Step 3: Commit**

```bash
git add cmd/cloudbroker/main.go
git commit -m "feat(cloudbroker): embed frp server for reverse tunneling"
```

---

## Phase 2: Embed frp Client in Directory

### Task 3: Create tunnel wrapper for the Directory

**Files:**
- Create: `internal/directory/tunnel/tunnel.go`
- Create: `internal/directory/tunnel/tunnel_test.go`

The tunnel wraps frp's client library. When started, it connects outbound to the frp server and exposes local services (API, HLS, WebRTC) under a subdomain.

- [ ] **Step 1: Write the failing test**

```go
// internal/directory/tunnel/tunnel_test.go
package tunnel

import (
	"testing"
)

func TestConfigValidation(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatal("expected error for empty config")
	}

	_, err = New(Config{
		ServerAddr: "connect.raikada.com",
		ServerPort: 7000,
		Token:      "test",
		SubDomain:  "my-home",
		LocalPorts: LocalPorts{
			API:    9995,
			HLS:    8898,
			WebRTC: 8889,
		},
	})
	if err != nil {
		t.Fatalf("valid config: %v", err)
	}
}
```

- [ ] **Step 2: Write implementation**

```go
// internal/directory/tunnel/tunnel.go
package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/source"
)

// LocalPorts describes the local services to tunnel.
type LocalPorts struct {
	API      int // Directory API (default 9995)
	HLS      int // HLS streaming (default 8898)
	WebRTC   int // WebRTC signaling (default 8889)
	RTSP     int // RTSP (default 8554)
	Playback int // Playback server (default 9996)
}

// Config for the embedded frp client tunnel.
type Config struct {
	ServerAddr string // frp server address, e.g. "connect.raikada.com"
	ServerPort int    // frp server port, e.g. 7000
	Token      string // shared auth token
	SubDomain  string // site alias, e.g. "ethans-home"
	LocalPorts LocalPorts
	Logger     *slog.Logger
}

// Tunnel wraps an frp client that tunnels local services to the cloud.
type Tunnel struct {
	cfg    Config
	svc    *client.Service
	cancel context.CancelFunc
}

// New creates a new tunnel. Call Run() to connect.
func New(cfg Config) (*Tunnel, error) {
	if cfg.ServerAddr == "" || cfg.ServerPort == 0 || cfg.Token == "" || cfg.SubDomain == "" {
		return nil, fmt.Errorf("server_addr, server_port, token, and subdomain are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Tunnel{cfg: cfg}, nil
}

// Run connects to the frp server and starts tunneling. Blocks until
// Stop() is called or the context is cancelled.
func (t *Tunnel) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	commonCfg := &v1.ClientCommonConfig{
		ServerAddr: t.cfg.ServerAddr,
		ServerPort: t.cfg.ServerPort,
	}
	commonCfg.Auth.Token = t.cfg.Token

	proxies := t.buildProxies()

	// Create config source with our proxy definitions.
	cfgSource := source.NewConfigSource()
	cfgSource.ReplaceAll(proxies, nil)
	aggregator := source.NewAggregator(cfgSource)

	svc, err := client.NewService(client.ServiceOptions{
		Common:                 commonCfg,
		ConfigSourceAggregator: aggregator,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("create frp client: %w", err)
	}
	t.svc = svc

	t.cfg.Logger.Info("tunnel: connecting to frp server",
		"server", fmt.Sprintf("%s:%d", t.cfg.ServerAddr, t.cfg.ServerPort),
		"subdomain", t.cfg.SubDomain)

	return svc.Run(ctx)
}

// Stop disconnects the tunnel.
func (t *Tunnel) Stop() {
	if t.cancel != nil {
		t.cancel()
	}
	if t.svc != nil {
		t.svc.GracefulClose(5 * time.Second)
	}
}

// buildProxies creates frp proxy configurations for each local service.
func (t *Tunnel) buildProxies() []v1.ProxyConfigurer {
	var proxies []v1.ProxyConfigurer
	sub := t.cfg.SubDomain

	// HTTP proxies — these get subdomain routing via frp vhost.
	if t.cfg.LocalPorts.API > 0 {
		proxies = append(proxies, &v1.HTTPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:      sub + "-api",
				Type:      "http",
				LocalIP:   "127.0.0.1",
				LocalPort: t.cfg.LocalPorts.API,
			},
			SubDomain: sub,
			Locations: []string{"/api/", "/healthz", "/admin"},
		})
	}

	if t.cfg.LocalPorts.HLS > 0 {
		proxies = append(proxies, &v1.HTTPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:      sub + "-hls",
				Type:      "http",
				LocalIP:   "127.0.0.1",
				LocalPort: t.cfg.LocalPorts.HLS,
			},
			SubDomain: sub,
			Locations: []string{"/nvr/"},
		})
	}

	if t.cfg.LocalPorts.WebRTC > 0 {
		proxies = append(proxies, &v1.HTTPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:      sub + "-webrtc",
				Type:      "http",
				LocalIP:   "127.0.0.1",
				LocalPort: t.cfg.LocalPorts.WebRTC,
			},
			SubDomain: sub,
			Locations: []string{"/whep", "/whip"},
		})
	}

	if t.cfg.LocalPorts.Playback > 0 {
		proxies = append(proxies, &v1.HTTPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name:      sub + "-playback",
				Type:      "http",
				LocalIP:   "127.0.0.1",
				LocalPort: t.cfg.LocalPorts.Playback,
			},
			SubDomain: sub,
			Locations: []string{"/list", "/get", "/seek"},
		})
	}

	return proxies
}
```

NOTE: The frp v1 config API may differ slightly from the research. Read the actual imported types after `go get` and adjust struct fields. The key pattern is:
- `v1.HTTPProxyConfig` with `SubDomain` and `Locations` for path-based routing
- All proxies share the same subdomain, differentiated by URL path

- [ ] **Step 3: Run test**

```bash
go test ./internal/directory/tunnel/... -v
```

- [ ] **Step 4: Commit**

```bash
git add internal/directory/tunnel/
git commit -m "feat(tunnel): embed frp client for reverse tunneling local services"
```

---

### Task 4: Integrate tunnel into Directory boot and admin API

**Files:**
- Modify: `internal/directory/boot.go`
- Modify: `internal/directory/adminapi/cloud.go`

- [ ] **Step 1: Add Tunnel field to DirectoryServer**

In `boot.go`, add to the `DirectoryServer` struct:

```go
Tunnel       *tunnel.Tunnel
tunnelCancel context.CancelFunc
```

Add import: `"github.com/bluenviron/mediamtx/internal/directory/tunnel"`

- [ ] **Step 2: Replace cloud connector startup with tunnel**

In `ApplyCloudSettings`, replace the cloud connector start with:

```go
func (ds *DirectoryServer) ApplyCloudSettings(url, token, alias string) {
    log := ds.logger

    // Stop existing tunnel + connector.
    if ds.tunnelCancel != nil {
        ds.tunnelCancel()
        ds.tunnelCancel = nil
    }
    if ds.cloudCancel != nil {
        ds.cloudCancel()
        ds.cloudCancel = nil
        ds.CloudConn = nil
    }
    if ds.Tunnel != nil {
        ds.Tunnel.Stop()
        ds.Tunnel = nil
    }

    if url == "" {
        log.Info("directory: remote access disabled")
        return
    }

    // Start cloud connector (WSS for registration/heartbeat).
    // ... existing cloud connector code ...

    // Start frp tunnel (for actual data).
    tun, err := tunnel.New(tunnel.Config{
        ServerAddr: "connect.raikada.com",
        ServerPort: 7000,
        Token:      token,
        SubDomain:  alias,
        LocalPorts: tunnel.LocalPorts{
            API:      9995,
            HLS:      8898,
            WebRTC:   8889,
            Playback: 9996,
        },
        Logger: log,
    })
    if err != nil {
        log.Error("tunnel: failed to create", "error", err)
        return
    }

    tunnelCtx, tunnelCancel := context.WithCancel(context.Background())
    ds.tunnelCancel = tunnelCancel
    ds.Tunnel = tun
    go tun.Run(tunnelCtx)
}
```

Do the same in the boot sequence (step 8).

- [ ] **Step 3: Add shutdown logic**

In `Shutdown()`, add before the cloud connector shutdown:

```go
if ds.tunnelCancel != nil {
    ds.tunnelCancel()
}
if ds.Tunnel != nil {
    ds.Tunnel.Stop()
}
```

- [ ] **Step 4: Verify compilation**

```bash
go build ./internal/directory/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/directory/boot.go
git commit -m "feat(directory): integrate frp tunnel into boot and settings"
```

---

## Phase 3: Deploy and Configure Caddy

### Task 5: Update EC2 deployment for frp

**Files:**
- Modify: `infra/environments/testing/cloudbroker/user_data.sh`
- Modify: `infra/environments/testing/cloudbroker/main.tf` (add port 7000 to security group)
- Modify: `infra/environments/testing/cloudbroker/deploy.sh`

- [ ] **Step 1: Add port 7000 to security group**

In `main.tf`, add to the security group ingress:

```hcl
  ingress {
    from_port   = 7000
    to_port     = 7000
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "FRP control"
  }
```

- [ ] **Step 2: Update Caddy to route subdomains**

Update the Caddyfile on EC2 to route `*.raikada.com` to frp's vhost port:

```
connect.raikada.com {
    reverse_proxy localhost:8080
}

*.raikada.com {
    reverse_proxy localhost:7080
}
```

This means:
- `connect.raikada.com` → cloud broker (registration, resolve, debug)
- `ethans-home.raikada.com` → frp vhost → tunneled to on-prem services

- [ ] **Step 3: Update deploy script**

Add frp flags to the systemd service:

```
ExecStart=/opt/raikada/cloudbroker -addr :8080 -token ${token} -tenant raikada -frp-port 7000 -frp-http-port 7080 -subdomain-host raikada.com
```

- [ ] **Step 4: Add wildcard DNS in Cloudflare**

Add a CNAME record: `*.raikada.com` → `connect.raikada.com`

This routes all subdomains to the same EC2 IP.

- [ ] **Step 5: Apply Terraform and deploy**

```bash
cd infra/environments/testing/cloudbroker
terraform apply
./deploy.sh
```

- [ ] **Step 6: Commit**

```bash
git add infra/environments/testing/cloudbroker/
git commit -m "feat(infra): add frp port and wildcard subdomain routing"
```

---

## Phase 4: End-to-End Test

### Task 6: Test remote access via frp tunnel

- [ ] **Step 1: Start Directory with tunnel enabled**

```bash
MTX_MODE=directory \
MTX_CLOUDCONNECTURL=wss://connect.raikada.com/ws/directory \
MTX_CLOUDCONNECTTOKEN="<token>" \
MTX_CLOUDSITEALIAS=ethans-home \
./mediamtx
```

- [ ] **Step 2: Verify tunnel connects**

Check logs for: `tunnel: connecting to frp server`

- [ ] **Step 3: Test remote API access**

```bash
curl https://ethans-home.raikada.com/healthz
curl https://ethans-home.raikada.com/api/v1/cameras
```

- [ ] **Step 4: Test remote HLS streaming**

```bash
ffplay "https://ethans-home.raikada.com/nvr/d6e6ea91-.../main/index.m3u8"
```

- [ ] **Step 5: Test remote playback**

```bash
curl "https://ethans-home.raikada.com/list?path=nvr/d6e6ea91-.../main~b7327385"
```

---

## What This Gives You

| Before (JSON tunnel) | After (frp tunnel) |
|---|---|
| Base64-encoded segments over WSS | Raw binary HTTP proxy |
| Single WSS bottleneck | Per-request connection pooling |
| Custom proxy code to maintain | Battle-tested frp library |
| Cloud-only API endpoints | Same API, different hostname |
| HLS only | HLS + WebRTC + Playback + API |
| No auth on streams | Same JWT auth as on-prem |

**Remote client experience:**
- LAN: `http://192.168.1.100:8898/nvr/.../main/index.m3u8`
- Cloud: `https://ethans-home.raikada.com/nvr/.../main/index.m3u8`

Same path. Same API. Just the hostname changes.
