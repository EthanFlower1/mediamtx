# Kaivue Web (`ui-v2/`)

KAI-307 scaffold for the next-generation Kaivue Recording Server web frontend.
This is a single React + TypeScript codebase that ships **two runtime contexts**
from one build:

- `/admin/*` — Customer Admin Web App (per-tenant, embedded into the on-prem Directory binary via `//go:embed`)
- `/command/*` — Integrator Portal (`command.yourbrand.com`)

> The legacy admin console under `ui/` is **not** affected by this scaffold and
> continues to ship until parity is reached.

## Stack

| Layer | Tech |
| --- | --- |
| Framework | React 18 + TypeScript 5 |
| Build | Vite 5 |
| Routing | React Router 6 |
| Local state | Zustand |
| Server state | TanStack Query |
| API client | Connect-Go (generated, stubbed in `src/api/client.ts`) |
| UI | shadcn/ui (Tailwind + CSS variables for white-label) |
| Forms | React Hook Form + Zod |
| i18n | react-i18next (EN / ES / FR / DE) |
| Tests | Vitest + Playwright |
| A11y | axe-core wired into Vitest setup |

## Layout

```
ui-v2/
  index.html
  package.json
  vite.config.ts
  vitest.config.ts
  playwright.config.ts
  tailwind.config.ts
  components.json          # shadcn config (no components installed yet)
  src/
    main.tsx               # entry: providers + router
    App.tsx                # /admin and /command route table
    api/client.ts          # Connect-Go client stub
    contexts/runtime.tsx   # useRuntimeContext() hook
    i18n/                  # react-i18next bootstrap + locale JSONs
    routes/
      admin/               # Customer Admin pages
      command/             # Integrator Portal pages
    styles/globals.css     # Tailwind layers + theme CSS variables
    lib/utils.ts           # cn() helper
    test/setup.ts          # vitest setup + axe runner
  e2e/                     # Playwright tests (placeholder)
```

## Setup

```bash
cd ui-v2
npm install
```

## Scripts

| Command | Purpose |
| --- | --- |
| `npm run dev` | Start Vite dev server on `:5174` (proxies `/api` to `:9997`) |
| `npm run build` | Type-check and produce a production build into `dist/` |
| `npm run test` | Run Vitest unit tests once |
| `npm run test:e2e` | Run Playwright E2E tests (placeholder) |
| `npm run lint` | ESLint with `--max-warnings 0` |
| `npm run typecheck` | `tsc --noEmit` |

## Runtime context detection

`src/contexts/runtime.tsx` exposes `useRuntimeContext()`. The current scaffold
detects context by URL prefix only:

```ts
const { kind, basePath } = useRuntimeContext();
// kind: 'admin' | 'command' | 'unknown'
```

Subsequent tickets layer in:

1. Auth token claim (KAI-308)
2. `/api/v1/discover` probe (KAI-309)

## i18n rules

**No hardcoded customer-visible strings.** Every label, error message, and
aria description must flow through `t('namespace.key')`. Per-integrator string
overrides will be layered on top of the default JSONs in KAI-358.

Locale files live at `src/i18n/locales/<lang>/common.json`. EN is populated;
ES/FR/DE are empty placeholders to be filled in by translation work.

## Accessibility

`axe-core` is wired into the Vitest setup file. Any unit test can call:

```ts
import { runAxe } from '@/test/setup';
const violations = await runAxe(container);
```

CI must fail on any `critical` or `serious` violation.

## Status

This is a scaffold. No business logic yet. The smoke test in
`src/contexts/runtime.test.tsx` exercises the route-based runtime detection
and runs an axe sweep on the rendered output.
