# Cloud Portal

Multi-site cloud control plane UI for Raikada — ported from the Claude Design
"Cloud Portal" handoff bundle (April 2026). Implements the Tactical HUD design
system across 13 screens: Overview, Live Wall, Playback, Search, Sites,
Devices, Recording Rules, Users & Roles, Remote Access, Alerts, Audit, Billing,
Onboarding.

This is **separate from `ui/`** (which is the on-prem admin console). The cloud
portal is for customers and integrators managing many sites at once.

## Stack

- Vite + React 18 + TypeScript
- `lucide-react` icons (replaced the prototype's UMD `lucide` global)
- Inline-styled components, design tokens from `src/styles/tokens.css`

## Run

```sh
npm install
npm run dev   # http://localhost:5180
```

## Status

Pure client-side prototype with mocked `Northwall Security` integrator data
(14 sites, ~270 cameras). No backend wiring yet — the next step is hooking
sites, cameras, alerts, rules and users to the cloud control plane API in
`internal/cloud/apiserver/`.
