# Contributing to ui-v2

This is the shared React codebase for the Kaivue web frontend. It renders in three contexts:

- **Customer Admin** (`/admin/*`) — on-prem and cloud customer management
- **Integrator Portal** (`/command/*`) — fleet management at `command.yourbrand.com`
- **On-prem embed** — same build served from the Go binary via `//go:embed` at `https://nvr.acme.local/admin`

## Prerequisite: TypeScript typecheck

Every PR that touches `ui-v2/` must pass the TypeScript typecheck. This is a **required CI status check** — the `typecheck (ui-v2)` GitHub Actions workflow must be green before merge.

### Run locally before pushing

```bash
cd ui-v2
npm install
npm run typecheck   # must exit 0
npm run build       # must exit 0
npm run test        # must exit 0
npm run lint        # must exit 0
```

### What the typecheck enforces

- `tsconfig.app.json` has `strict: true`, `noUnusedLocals: true`, `noUnusedParameters: true`, and `noFallthroughCasesInSwitch: true`. These cannot be relaxed to make errors go away — fix the actual types.
- `skipLibCheck: true` applies only to third-party `.d.ts` files inside `node_modules`, not to generated artifacts in the project root.

### Fixing typecheck failures

If `npm run typecheck` fails on your branch:

1. Read the compiler output carefully — it reports file path, line, and error code.
2. Fix the types in source files under `src/`. Do not weaken `tsconfig.app.json` or `tsconfig.json`.
3. If the error comes from an auto-generated file (e.g., from the Connect-Go proto pipeline), regenerate the client rather than editing the generated file by hand.
4. Re-run `npm run typecheck` until it exits 0.

### Fixing lint failures

The lint gate runs ESLint with `--max-warnings 0`. Both errors and warnings fail the build.

Common patterns:
- `react-hooks/exhaustive-deps`: wrap the function in `useCallback` with the correct deps rather than suppressing the rule.
- `react-refresh/only-export-components`: if a module legitimately exports both a component and a hook from the same file (e.g., to avoid circular imports), use a file-level `/* eslint-disable ... */` block comment with a justification.
- `no-console` in test files: wrap with `// eslint-disable-next-line no-console` on the line directly above the statement.

## Code style

- All customer-visible strings go through `react-i18next` — no hardcoded strings outside translation files.
- All lists that can exceed 100 rows must be virtualized (TanStack Table + virtualizer).
- WCAG 2.1 AA accessibility is required. `axe-core` runs in CI; zero critical/serious violations are allowed.
- Connect-Go API clients are auto-generated from `.proto` files — never hand-write API call sites.
