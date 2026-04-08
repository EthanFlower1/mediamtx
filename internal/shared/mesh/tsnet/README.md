# internal/shared/mesh/tsnet

Shared wrapper around [`tailscale.com/tsnet`](https://pkg.go.dev/tailscale.com/tsnet)
that turns each Kaivue on-prem component (Directory, Recorder, Gateway)
into a node on the customer's private mesh tailnet.

## Why a mesh

Kaivue on-prem deployments are often split across hardware: one
Directory box, several Recorders, and one or more Gateways that
front the stream/playback API. Those boxes need to talk to each
other securely even when:

- the customer's LAN uses RFC1918 addresses that overlap with other
  customers,
- there is no reachable public IP,
- the operator does not want to configure port forwards or VPN
  clients on each box.

A tailnet coordinated by the embedded Headscale from
[KAI-240](https://linear.app/ ../KAI-240) solves all three problems:
every component is issued a stable `100.x.y.z` address, MagicDNS
resolves peer hostnames, and the WireGuard tunnel is brought up over
the customer's existing NATs.

## How a component becomes a node

1. Pairing (KAI-243) mints a Headscale pre-auth key per component.
2. The component boots and calls either
   `internal/directory/mesh.New` or `internal/recorder/mesh.New`,
   passing the pre-auth key.
3. Those convenience constructors forward into
   `internal/shared/mesh/tsnet.New`, which configures a
   `tsnet.Server`, persists its state under `NodeConfig.StateDir`,
   registers against the embedded Headscale coordinator, and blocks
   until the node is reachable.
4. The component uses `Node.Listen` to publish its Connect-Go
   services and `Node.Dial` to reach peers by hostname
   (e.g. `directory-<uuid>:8443`).

Role hostnames are prefixed by the convenience constructors:

- Directory nodes become `directory-<uuid>`
- Recorder nodes become `recorder-<uuid>`

Gateway and any future role gain their own thin wrappers as they land.

## Package boundary (seam #1)

`internal/directory/mesh/` and `internal/recorder/mesh/` both import
`internal/shared/mesh/tsnet/` but **must not** import each other.
The depguard rule from KAI-236 enforces this. Keep role-specific
logic out of the shared package.

## Test mode

Real `tsnet` needs:

- a persistent state directory,
- a reachable Headscale (or Tailscale SaaS) control URL,
- raw network access for WireGuard UDP,

none of which is friendly to hermetic unit tests. Passing
`NodeConfig.TestMode = true` bypasses all of that. A test-mode Node:

- registers itself in a process-wide in-memory map keyed on
  `Hostname`,
- returns an ordinary `127.0.0.1:0` listener from `Listen`,
- routes `Dial("hostname:port")` to that peer's listener via the
  in-memory map, returning `ErrUnknownHost` if no peer is registered
  under the hostname,
- synthesizes a deterministic `127.x.y.z` address for `Addr`.

Two test-mode nodes in the same process can connect to each other
without touching the network, which is exactly what the Directory
↔ Recorder integration tests need.

## Build tags

### `realtsnet`

`real_smoke_test.go` brings up a real `tsnet.Server` against an
actual control URL. It is compiled only with `-tags realtsnet` and
is intended for human operators running against a local Headscale.
CI does not run it. To run it locally:

```
KAIVUE_TSNET_AUTHKEY=hskey-...                           \
KAIVUE_TSNET_CONTROL_URL=https://headscale.local         \
go test -tags realtsnet ./internal/shared/mesh/tsnet/...
```

### `tsnetstub`

`stub.go` replaces the real backend with one that always returns
`tsnet: real backend disabled by tsnetstub build tag`. The stub is
for environments that cannot or do not want to compile the
`tailscale.com` dependency tree (pulls in gvisor and wireguard-go).
Those builds must run every component in `TestMode`.

```
go build -tags tsnetstub ./...
```

## Dependencies

- **KAI-240** — embedded Headscale coordinator. Provides the
  `ControlURL` value; until it lands, leave the field empty for
  local smoke tests.
- **KAI-243** — pairing tokens. Responsible for minting the
  pre-auth keys that populate `NodeConfig.AuthKey`.
- **KAI-241** — `step-ca`. Issues the mTLS certs that ride **on top**
  of the mesh tunnel. That wiring lives in `internal/shared/certmgr`,
  not here.
