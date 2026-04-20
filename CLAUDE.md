# Raikada - Development Guidelines

## Critical: Do NOT modify mediamtx.yml runtime settings

The `mediamtx.yml` file contains runtime configuration for the running server. **Never** change these settings to make tests pass:

- `nvr: true` — must stay true
- `api: true` — must stay true  
- `playback: true` — must stay true
- `logLevel: debug` — must stay debug
- `nvrJWTSecret` — never change or clear; the database keys are encrypted with this secret
- Camera paths under `paths:` — do not remove

If `TestSampleConfFile` or similar tests fail because mediamtx.yml doesn't match defaults, fix the **test** to account for NVR settings, not the config file.

CI enforces this via `scripts/check-mediamtx-config.sh` — see `.github/workflows/config-guard.yml`.

## Worktree Convention

- Always create worktrees in `.worktrees/<ticket-id>` on branch `feat/<ticket-id>-<short-description>`
- Work entirely within the worktree
- When done, push the branch and create a PR to main

## Architecture

- Go backend with NVR subsystem in `internal/nvr/`
- React admin console (setup/config only) in `ui/`
- Flutter primary client in `clients/flutter/`
- SQLite database via `modernc.org/sqlite` (pure Go, no CGO)
- ONVIF camera integration via `internal/nvr/onvif/`
- Gin HTTP router for API
