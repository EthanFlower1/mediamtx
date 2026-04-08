# Runtime mode (`mode:`)

> Tracking ticket: [KAI-237](https://linear.app/kaivue/issue/KAI-237)

Kaivue Recording Server v1 splits what used to be a single "NVR" binary
into three cooperating roles. The role a given process plays is selected
at boot via the top-level `mode:` field in `mediamtx.yml`:

```yaml
# mediamtx.yml
mode: all-in-one
```

## Supported values

| Value          | Subsystems booted                                       | Use case |
| -------------- | ------------------------------------------------------- | -------- |
| *unset / `""`* | Legacy single-NVR (pre-KAI-237 behavior, unchanged)     | Existing deployments upgrading in place. |
| `directory`    | Directory subsystem only: admin UI, sidecar supervisor, cloud Directory client. No capture pipeline. | Central control plane running on a management box or VM. |
| `recorder`     | Recorder subsystem only: capture pipeline, embedded MediaMTX sidecar, Directory client. No admin UI. | Dedicated capture appliance, typically paired 1:N with a Directory. |
| `all-in-one`   | Both of the above in a single process, with automatic pairing on first boot. | Small / home deployments, demos, evaluation. |

Any other value is rejected at config-load time with a descriptive
error.

## Default behavior

`mode:` defaults to the empty string, which is treated as "legacy" and
boots the exact code path used before KAI-237 landed. Existing
`mediamtx.yml` files keep working without modification - upgrading to
a binary that understands `mode:` is a no-op unless you opt in.

## Environment variable

Like all top-level fields, `mode` can be overridden at runtime via an
environment variable:

```sh
MTX_MODE=recorder ./mediamtx
```

## Implementation status (v1 rollout)

KAI-237 adds the config field, validation, constants, and the boot-time
dispatch shim. The non-legacy hooks currently log a clearly marked stub
message; the real subsystem wiring lands in the following tickets:

- KAI-226 - cloud Directory client
- KAI-243 / KAI-244 - in-process auto-pairing for `all-in-one`
- KAI-246 - shared sidecar supervisor
- KAI-250 - recorder-state SQLite store

Until those tickets land, choosing `directory`, `recorder`, or
`all-in-one` will log a WARN line like:

```
[KAI-237] directory subsystem boot is a stub; real wiring tracked in KAI-226 / KAI-246
```

and then fall through to the default boot sequence. **Production
deployments should continue to use the legacy (unset) mode until the
dependent tickets are merged.**

## Design reference

See the v1 roadmap §4.2 and the multi-recording-server design doc
(`docs/superpowers/specs/2026-04-07-multi-recording-server-design.md`).
