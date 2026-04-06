# React HUD Design System Foundation (SP1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a reusable React HUD component library and theme infrastructure that mirrors the Flutter NVR client's design system, plus an admin-gated `/design-system` showcase route, without breaking any existing page in the React admin console.

**Architecture:** A new `--hud-*` CSS variable system and a new flat-named Tailwind palette coexist with the existing `nvr.*` blue palette. New components consume the new tokens; old pages keep using the old ones. SP2 deletes the old palette when the last legacy page is migrated.

**Tech Stack:** React 18 + TypeScript, Tailwind CSS 3.4 (CSS-variable-backed colors via `rgb(var(...) / <alpha-value>)`), Vite 5, `@fontsource/jetbrains-mono` and `@fontsource/ibm-plex-sans` for self-hosted fonts, react-router-dom 7.

---

## Important deviations from the spec

The spec's literal Tailwind / CSS variable naming would collide with the existing app. The plan resolves this with two precise deviations, both safer than the literal spec text:

1. **CSS variable prefix is `--hud-*`, not `--nvr-*`.** The spec used `--nvr-bg-primary` etc. The existing `ui/src/index.css` already defines `--nvr-bg-primary` for the current OLED toggle (`html.theme-oled` selector). Reusing the same variable names would silently merge two unrelated systems and behave unpredictably during the SP1→SP2 transition. Renaming the new system to `--hud-*` keeps the two systems strictly isolated.
2. **The new Tailwind palette is additive, not a replacement.** The existing `nvr.*` nested color block in `tailwind.config.js` stays untouched. New flat-named entries (`bg-primary`, `text-primary`, etc.) are added alongside it. Existing pages keep working with `bg-nvr-bg-primary`; new HUD components use `bg-bg-primary`. SP2 deletes the old `nvr.*` block when migrating the last legacy page.

A third smaller deviation: SP1's `useTheme` hook writes `data-theme` to a *local container div* inside the `/design-system` showcase route, not to `<html>`. This avoids any interaction with the existing `html.theme-oled` toggle. SP2 will globalize the hook to `<html>` when it builds the production theme switcher in `/settings/preferences`.

---

## File structure

### New files

```
ui/.nvmrc                                    pin Node 20
ui/src/theme/colors.ts                       TS palette tables (dark/oled/light)
ui/src/theme/typography.ts                   TS export for type ramp class names
ui/src/theme/tokens.css                      CSS custom properties for 3 themes (--hud-*)
ui/src/theme/fonts.css                       @font-face declarations + @fontsource imports
ui/src/theme/typography.css                  type ramp helpers via @layer components
ui/src/theme/useTheme.ts                     React hook for local theme state
ui/src/components/hud/HudButton.tsx
ui/src/components/hud/HudToggle.tsx
ui/src/components/hud/HudInput.tsx
ui/src/components/hud/HudTextarea.tsx
ui/src/components/hud/HudSelect.tsx
ui/src/components/hud/AnalogSlider.tsx
ui/src/components/hud/SegmentedControl.tsx
ui/src/components/hud/StatusBadge.tsx
ui/src/components/hud/CornerBrackets.tsx
ui/src/components/hud/SectionCard.tsx
ui/src/components/hud/KvRow.tsx
ui/src/components/hud/index.ts               barrel re-export
ui/src/pages/DesignSystem.tsx                admin-gated showcase route
```

### Modified files

```
ui/tailwind.config.js                        + flat-name color block, + font families (additive)
ui/src/main.tsx                              + import token + font + typography CSS
ui/src/App.tsx                               + /design-system route, + branding hook patch
ui/package.json                              + @fontsource/jetbrains-mono, @fontsource/ibm-plex-sans
ui/package-lock.json                         (auto)
```

### Untouched

- All existing pages under `ui/src/pages/`
- All existing components under `ui/src/components/`
- All hooks under `ui/src/hooks/`
- The existing `nvr.*` Tailwind palette
- The existing `index.css` (`theme-oled` block, login gradient, etc.)
- The existing `useBranding` hook's signature — only its body is patched

---

## Task list

### Task 1: Set up the SP1 worktree

**Files:**
- Create: `.worktrees/react-hud-foundation/` (git worktree)

- [ ] **Step 1: Fetch latest main**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
git fetch origin main
```

- [ ] **Step 2: Create the worktree on a fresh feature branch off origin/main**

```bash
git worktree add -b feat/react-hud-foundation .worktrees/react-hud-foundation origin/main
```

Expected: "Preparing worktree (new branch 'feat/react-hud-foundation')" and "branch 'feat/react-hud-foundation' set up to track 'origin/main'."

- [ ] **Step 3: Verify worktree state**

```bash
cd /Users/ethanflower/personal_projects/mediamtx/.worktrees/react-hud-foundation
git status
git rev-parse HEAD
```

Expected: clean working tree, HEAD matches `origin/main`.

---

### Task 2: Pin Node version with .nvmrc

**Files:**
- Create: `ui/.nvmrc`

- [ ] **Step 1: Write the .nvmrc file**

```bash
echo "20" > ui/.nvmrc
```

- [ ] **Step 2: Verify**

```bash
cat ui/.nvmrc
```

Expected output: `20`

- [ ] **Step 3: Activate Node 20 for the current shell**

```bash
nvm use
```

Expected: "Now using node v20.20.1" (or whichever 20.x is installed). If `nvm use` reports v20 is not installed, run `nvm install 20`.

- [ ] **Step 4: Commit**

```bash
git add ui/.nvmrc
git commit -m "build(ui): pin Node 20 via .nvmrc

Vite 5 requires Node 18+ — Node 16 crashes with
'crypto.getRandomValues is not a function' before any code runs.
Pinning via .nvmrc means 'nvm use' inside ui/ picks the right
version automatically."
```

---

### Task 3: Install font dependencies

**Files:**
- Modify: `ui/package.json`
- Modify: `ui/package-lock.json`

- [ ] **Step 1: Install both fontsource packages**

```bash
cd ui
npm install @fontsource/jetbrains-mono @fontsource/ibm-plex-sans
```

Expected: "added N packages" with no errors.

- [ ] **Step 2: Verify package.json was updated**

```bash
grep -E "@fontsource/(jetbrains-mono|ibm-plex-sans)" package.json
```

Expected: two lines showing the added dependencies.

- [ ] **Step 3: Verify the build still works (smoke test)**

```bash
npm run build
```

Expected: clean build into `../internal/nvr/ui/dist/`. Pre-existing chunk-split warning is fine — no new errors.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/package.json ui/package-lock.json
git commit -m "build(ui): add @fontsource/jetbrains-mono and @fontsource/ibm-plex-sans

Self-host the two fonts the Flutter HUD design uses, served from
node_modules/@fontsource/* so there's no CDN dependency and no manual
WOFF2 file management."
```

---

### Task 4: Create theme/colors.ts (TS palette tables)

**Files:**
- Create: `ui/src/theme/colors.ts`

- [ ] **Step 1: Create the directory and write the file**

```ts
// ui/src/theme/colors.ts

/**
 * HUD color palettes — TypeScript mirror of clients/flutter/lib/theme/nvr_colors.dart.
 *
 * These tables are the source of truth for the values copied into tokens.css.
 * The runtime CSS variable system in tokens.css is what components actually
 * consume; this file exists so TS code can reference palette values directly
 * (e.g., for inline canvas drawing where Tailwind classes don't work).
 */

export type ThemeName = 'dark' | 'oled' | 'light'

export interface HudPalette {
  bgPrimary: string
  bgSecondary: string
  bgTertiary: string
  bgInput: string
  accent: string
  accentHover: string
  textPrimary: string
  textSecondary: string
  textMuted: string
  success: string
  warning: string
  danger: string
  border: string
}

/** Mirrors NvrColors.dark in clients/flutter/lib/theme/nvr_colors.dart */
export const dark: HudPalette = {
  bgPrimary: '#0a0a0a',
  bgSecondary: '#111111',
  bgTertiary: '#1a1a1a',
  bgInput: '#1a1a1a',
  accent: '#f97316',
  accentHover: '#ea580c',
  textPrimary: '#e5e5e5',
  textSecondary: '#737373',
  textMuted: '#404040',
  success: '#22c55e',
  warning: '#eab308',
  danger: '#ef4444',
  border: '#262626',
}

/** Pure-black variant of dark — backgrounds only, everything else inherits. */
export const oled: HudPalette = {
  ...dark,
  bgPrimary: '#000000',
  bgSecondary: '#080808',
  bgTertiary: '#101010',
  bgInput: '#101010',
}

/** Mirrors NvrColors.light in clients/flutter/lib/theme/nvr_colors.dart */
export const light: HudPalette = {
  bgPrimary: '#f5f5f5',
  bgSecondary: '#ffffff',
  bgTertiary: '#e5e5e5',
  bgInput: '#e5e5e5',
  accent: '#ea580c',
  accentHover: '#c2410c',
  textPrimary: '#171717',
  textSecondary: '#525252',
  textMuted: '#a3a3a3',
  success: '#16a34a',
  warning: '#ca8a04',
  danger: '#dc2626',
  border: '#d4d4d4',
}

export const palettes: Record<ThemeName, HudPalette> = { dark, oled, light }
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd ui
npx tsc -b
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
cd ..
git add ui/src/theme/colors.ts
git commit -m "feat(ui/theme): add TypeScript HUD palette tables

Three palettes (dark, oled, light) ported from
clients/flutter/lib/theme/nvr_colors.dart. These are the source of truth
for the values that get copied into tokens.css. TS code that needs raw
palette values (e.g. canvas drawing) imports from here."
```

---

### Task 5: Create theme/tokens.css (CSS variables for 3 themes)

**Files:**
- Create: `ui/src/theme/tokens.css`

- [ ] **Step 1: Write the CSS variable definitions**

```css
/* ui/src/theme/tokens.css
 *
 * Runtime color tokens for the HUD design system. Components consume these
 * via Tailwind classes like `bg-bg-primary` which are wired to
 * `rgb(var(--hud-bg-primary) / <alpha-value>)` in tailwind.config.js.
 *
 * Three themes are supported. Switch by setting `data-theme="dark|oled|light"`
 * on a containing element. The default (no attribute, or `dark`) is the
 * Flutter NvrColors.dark palette.
 *
 * Naming: --hud-* (NOT --nvr-*) to avoid collision with the existing
 * --nvr-* variables already used by ui/src/index.css for the legacy
 * `html.theme-oled` toggle.
 *
 * Values are R G B triplets (no commas, no `rgb()` wrapper) so Tailwind's
 * `<alpha-value>` slot can compose them into any opacity.
 */

:root,
[data-theme='dark'] {
  --hud-bg-primary: 10 10 10;
  --hud-bg-secondary: 17 17 17;
  --hud-bg-tertiary: 26 26 26;
  --hud-bg-input: 26 26 26;
  --hud-accent: 249 115 22;
  --hud-accent-hover: 234 88 12;
  --hud-text-primary: 229 229 229;
  --hud-text-secondary: 115 115 115;
  --hud-text-muted: 64 64 64;
  --hud-success: 34 197 94;
  --hud-warning: 234 179 8;
  --hud-danger: 239 68 68;
  --hud-border: 38 38 38;
}

[data-theme='oled'] {
  --hud-bg-primary: 0 0 0;
  --hud-bg-secondary: 8 8 8;
  --hud-bg-tertiary: 16 16 16;
  --hud-bg-input: 16 16 16;
  /* accent / status / text inherit from dark */
}

[data-theme='light'] {
  --hud-bg-primary: 245 245 245;
  --hud-bg-secondary: 255 255 255;
  --hud-bg-tertiary: 229 229 229;
  --hud-bg-input: 229 229 229;
  --hud-accent: 234 88 12;
  --hud-accent-hover: 194 65 12;
  --hud-text-primary: 23 23 23;
  --hud-text-secondary: 82 82 82;
  --hud-text-muted: 163 163 163;
  --hud-success: 22 163 74;
  --hud-warning: 202 138 4;
  --hud-danger: 220 38 38;
  --hud-border: 212 212 212;
}
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/theme/tokens.css
git commit -m "feat(ui/theme): add HUD CSS custom-property tokens for dark/oled/light

CSS variables under the --hud-* prefix that Tailwind will consume via
rgb(var(--hud-x) / <alpha-value>). Three themes are supported via the
[data-theme] attribute. The OLED variant only overrides backgrounds and
inherits accent / text / status colors from dark.

The --hud-* prefix avoids collision with the existing --nvr-* variables
that index.css uses for the legacy html.theme-oled toggle."
```

---

### Task 6: Create theme/fonts.css (font face imports)

**Files:**
- Create: `ui/src/theme/fonts.css`

- [ ] **Step 1: Write the font import file**

```css
/* ui/src/theme/fonts.css
 *
 * Self-hosted font imports via @fontsource. Only the weights actually used
 * by the HUD type ramp are pulled in to keep the bundle small.
 *
 * JetBrains Mono — labels, data, status, sections (the HUD signature)
 *   400 normal — body data and timestamps
 *   500 medium — labels and slider values
 *   700 bold   — section headers
 *
 * IBM Plex Sans — page titles, body, button labels
 *   400 normal — body text
 *   500 medium — camera names
 *   600 semi   — page titles, button labels
 */

@import '@fontsource/jetbrains-mono/400.css';
@import '@fontsource/jetbrains-mono/500.css';
@import '@fontsource/jetbrains-mono/700.css';
@import '@fontsource/ibm-plex-sans/400.css';
@import '@fontsource/ibm-plex-sans/500.css';
@import '@fontsource/ibm-plex-sans/600.css';
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/theme/fonts.css
git commit -m "feat(ui/theme): add @fontsource imports for HUD fonts

JetBrains Mono (400/500/700) and IBM Plex Sans (400/500/600). Only the
weights actually used by the HUD type ramp are imported."
```

---

### Task 7: Extend tailwind.config.js (additive)

**Files:**
- Modify: `ui/tailwind.config.js`

- [ ] **Step 1: Read the current file**

```bash
cat ui/tailwind.config.js
```

Confirm the current `colors.nvr.*` block exists and is what we expect.

- [ ] **Step 2: Replace the file with the extended version**

The existing `nvr.*` color block is preserved verbatim. Three new things are added: a flat-named color block consuming `--hud-*` variables, a `mono` font family, and a `sans` font family that lists IBM Plex Sans first.

```js
/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // ── Legacy palette (consumed by existing pages — DO NOT REMOVE in SP1) ──
        nvr: {
          bg: {
            primary: '#0f1117',
            secondary: '#1a1d27',
            tertiary: '#242836',
            input: '#12141c',
          },
          accent: {
            DEFAULT: '#3b82f6',
            hover: '#2563eb',
          },
          danger: {
            DEFAULT: '#ef4444',
            hover: '#dc2626',
          },
          success: '#22c55e',
          warning: '#f59e0b',
          text: {
            primary: '#e5e7eb',
            secondary: '#9ca3af',
            muted: '#7c8494',
          },
          border: '#2d3140',
        },
        // ── HUD palette (new in SP1, consumed by ui/src/components/hud/*) ──
        // Backed by --hud-* CSS variables defined in src/theme/tokens.css.
        // Use these in any new code:
        //   bg-bg-primary, bg-bg-secondary, bg-bg-tertiary, bg-bg-input
        //   text-text-primary, text-text-secondary, text-text-muted
        //   bg-accent, text-accent, border-accent
        //   text-success, text-warning, text-danger
        //   border-border
        'bg-primary': 'rgb(var(--hud-bg-primary) / <alpha-value>)',
        'bg-secondary': 'rgb(var(--hud-bg-secondary) / <alpha-value>)',
        'bg-tertiary': 'rgb(var(--hud-bg-tertiary) / <alpha-value>)',
        'bg-input': 'rgb(var(--hud-bg-input) / <alpha-value>)',
        accent: {
          DEFAULT: 'rgb(var(--hud-accent) / <alpha-value>)',
          hover: 'rgb(var(--hud-accent-hover) / <alpha-value>)',
        },
        'text-primary': 'rgb(var(--hud-text-primary) / <alpha-value>)',
        'text-secondary': 'rgb(var(--hud-text-secondary) / <alpha-value>)',
        'text-muted': 'rgb(var(--hud-text-muted) / <alpha-value>)',
        success: 'rgb(var(--hud-success) / <alpha-value>)',
        warning: 'rgb(var(--hud-warning) / <alpha-value>)',
        danger: 'rgb(var(--hud-danger) / <alpha-value>)',
        border: 'rgb(var(--hud-border) / <alpha-value>)',
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', 'Inter', 'system-ui', 'Avenir', 'Helvetica', 'Arial', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      keyframes: {
        'slide-in': {
          from: { transform: 'translateX(100%)' },
          to: { transform: 'translateX(0)' },
        },
        'fade-in': {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        'scale-in': {
          from: { opacity: '0', transform: 'scale(0.95)' },
          to: { opacity: '1', transform: 'scale(1)' },
        },
        'pulse-dot': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.4' },
        },
      },
      animation: {
        'slide-in': 'slide-in 200ms ease-out',
        'fade-in': 'fade-in 200ms ease-out',
        'scale-in': 'scale-in 200ms ease-out',
        'pulse-dot': 'pulse-dot 1.5s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
```

Note: the IBM Plex Sans font family also takes effect for *existing* pages because Tailwind's `font-sans` class already resolved to the previous list and the new list still has Inter as a fallback. Existing pages may render in a slightly different sans font as a side effect — this is acceptable cosmetic drift, not a regression.

- [ ] **Step 3: Verify the build still works**

```bash
cd ui
npm run build
```

Expected: clean build. Pre-existing chunk-split warning OK. Any new error means the config is malformed.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/tailwind.config.js
git commit -m "feat(ui/tailwind): add HUD color palette and mono/sans font families

Additive change. The existing nvr.* nested color block is preserved
unchanged so legacy pages keep working. New flat-named color entries
(bg-primary, text-primary, accent, success, etc.) consume --hud-* CSS
variables from src/theme/tokens.css via rgb(var(...) / <alpha-value>).

font-sans now lists IBM Plex Sans first; font-mono is new and lists
JetBrains Mono first. Existing pages using font-sans will pick up IBM
Plex Sans once it loads — acceptable cosmetic drift since the legacy
palette stays intact.

Adds a pulse-dot keyframe used by StatusBadge later in this PR."
```

---

### Task 8: Add the type ramp helpers (typography.css + typography.ts)

**Files:**
- Create: `ui/src/theme/typography.css`
- Create: `ui/src/theme/typography.ts`

- [ ] **Step 1: Write typography.css**

```css
/* ui/src/theme/typography.css
 *
 * HUD type ramp helpers, exposed as Tailwind component classes via
 * @layer components. Mirrors clients/flutter/lib/theme/nvr_typography.dart
 * exactly — same sizes, weights, letter-spacing, and color roles.
 *
 * Use these classes directly in markup:
 *   <span className="text-mono-section">DEVICE INFO</span>
 *   <span className="text-mono-data">f97316</span>
 *
 * They take precedence over Tailwind utility classes for font-family,
 * size, weight, tracking, and color simultaneously, so the markup stays
 * clean.
 */

@layer components {
  .text-mono-label {
    @apply font-mono text-[9px] font-medium tracking-[0.15em] text-text-muted;
  }
  .text-mono-section {
    @apply font-mono text-[10px] font-bold tracking-[0.2em] text-accent uppercase;
  }
  .text-mono-data {
    @apply font-mono text-[12px] font-normal text-text-primary;
  }
  .text-mono-data-lg {
    @apply font-mono text-[16px] font-medium text-text-primary;
  }
  .text-mono-timestamp {
    @apply font-mono text-[12px] font-normal text-accent;
  }
  .text-mono-status {
    @apply font-mono text-[9px] font-medium tracking-[0.1em] text-success;
  }
  .text-mono-control {
    @apply font-mono text-[9px] font-medium tracking-[0.1em] text-text-muted;
  }
  .text-page-title {
    @apply font-sans text-[16px] font-semibold text-text-primary;
  }
  .text-camera-name {
    @apply font-sans text-[13px] font-medium text-text-primary;
  }
  .text-body-hud {
    @apply font-sans text-[12px] font-normal leading-[1.5] text-text-secondary;
  }
  .text-button-hud {
    @apply font-sans text-[12px] font-semibold text-text-primary;
  }
  .text-alert-hud {
    @apply font-sans text-[12px] font-normal text-danger;
  }
}
```

Note: `text-body-hud`, `text-button-hud`, `text-alert-hud` use `-hud` suffixes to avoid clashing with any existing Tailwind utility named `text-body` or `text-button`.

- [ ] **Step 2: Write typography.ts (TS class-name constants)**

```ts
// ui/src/theme/typography.ts
//
// String constants for the HUD type ramp helpers. Use these instead of
// stringly-typing class names so refactors stay safe.
//
//   import { hudType } from '~/theme/typography'
//   <span className={hudType.monoSection}>DEVICE INFO</span>

export const hudType = {
  monoLabel: 'text-mono-label',
  monoSection: 'text-mono-section',
  monoData: 'text-mono-data',
  monoDataLg: 'text-mono-data-lg',
  monoTimestamp: 'text-mono-timestamp',
  monoStatus: 'text-mono-status',
  monoControl: 'text-mono-control',
  pageTitle: 'text-page-title',
  cameraName: 'text-camera-name',
  body: 'text-body-hud',
  button: 'text-button-hud',
  alert: 'text-alert-hud',
} as const

export type HudTypeKey = keyof typeof hudType
```

- [ ] **Step 3: Commit**

```bash
git add ui/src/theme/typography.css ui/src/theme/typography.ts
git commit -m "feat(ui/theme): add HUD type ramp helpers

Tailwind component classes (.text-mono-label, .text-mono-section, etc.)
ported from clients/flutter/lib/theme/nvr_typography.dart with the
same sizes, weights, letter-spacing, and color roles.

The body/button/alert helpers use -hud suffixes (text-body-hud,
text-button-hud, text-alert-hud) to avoid clashing with any existing
Tailwind utility classes.

A typography.ts file exports the class names as string constants so TS
code can reference them via hudType.monoSection instead of stringly
typing them."
```

---

### Task 9: Create the useTheme hook

**Files:**
- Create: `ui/src/theme/useTheme.ts`

- [ ] **Step 1: Write the hook**

```ts
// ui/src/theme/useTheme.ts
import { useState, useCallback } from 'react'
import type { ThemeName } from './colors'

const STORAGE_KEY = 'nvr-hud-theme'

/**
 * Local React state for the HUD theme.
 *
 * IMPORTANT (SP1): this hook does NOT write to <html data-theme="..."> in
 * SP1. Doing so would conflict with the existing legacy `html.theme-oled`
 * toggle. SP1's only consumer is the /design-system showcase route, which
 * applies `data-theme` to a local container div instead.
 *
 * SP2 will replace this hook with a globalized version that writes to
 * <html> as part of the production theme switcher in
 * /settings/preferences.
 */
export function useTheme(): {
  theme: ThemeName
  setTheme: (theme: ThemeName) => void
} {
  const [theme, setThemeState] = useState<ThemeName>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored === 'dark' || stored === 'oled' || stored === 'light') {
        return stored
      }
    } catch {
      // localStorage may throw in private mode — fall through to default
    }
    return 'dark'
  })

  const setTheme = useCallback((next: ThemeName) => {
    setThemeState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // localStorage may throw in private mode — silently ignore
    }
  }, [])

  return { theme, setTheme }
}
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/theme/useTheme.ts
git commit -m "feat(ui/theme): add useTheme hook for HUD theme switching

Local React state with localStorage persistence under nvr-hud-theme.
Deliberately does NOT write to <html data-theme> in SP1 — the showcase
route applies data-theme to a local container instead — to avoid
collision with the existing html.theme-oled legacy toggle. SP2 will
replace this with a globalized version that writes to <html> when the
production switcher ships in /settings/preferences."
```

---

### Task 10: Wire up CSS imports in main.tsx

**Files:**
- Modify: `ui/src/main.tsx`

- [ ] **Step 1: Read the current main.tsx**

```bash
cat ui/src/main.tsx
```

- [ ] **Step 2: Add the three CSS imports immediately after the existing index.css import**

The existing `import './index.css'` line stays. Three new lines are added directly below it. The order matters: tokens before fonts before typography (typography uses both).

```ts
import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App.tsx'
import './index.css'
import './theme/tokens.css'
import './theme/fonts.css'
import './theme/typography.css'

// Apply persisted legacy OLED theme on boot.
const savedTheme = localStorage.getItem('nvr-theme')
if (savedTheme === 'oled') {
  document.documentElement.classList.add('theme-oled')
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
```

If your current `main.tsx` has a different structure (extra imports, different bootstrap), make the *minimal* change: add the three new `import` lines directly under the existing `./index.css` import. Do not delete or reorder anything else.

- [ ] **Step 3: Verify the build still works**

```bash
cd ui
npm run build
```

Expected: clean build. The CSS bundle will grow because the fonts are now embedded.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/main.tsx
git commit -m "feat(ui): wire HUD theme + font CSS into main.tsx

Three new CSS imports below the existing index.css:
  theme/tokens.css      — --hud-* CSS variables for dark/oled/light
  theme/fonts.css       — @fontsource imports
  theme/typography.css  — type ramp helpers via @layer components

Order matters: tokens first (so typography.css can reference them),
fonts second, typography last. The existing legacy theme-oled bootstrap
is left untouched."
```

---

### Task 11: Create the DesignSystem showcase stub

**Files:**
- Create: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Create the file with a stub layout (no HUD components yet — they're added incrementally as each component task lands)**

```tsx
// ui/src/pages/DesignSystem.tsx
import { useTheme } from '../theme/useTheme'
import type { ThemeName } from '../theme/colors'

const themes: { value: ThemeName; label: string }[] = [
  { value: 'dark', label: 'DARK' },
  { value: 'oled', label: 'OLED' },
  { value: 'light', label: 'LIGHT' },
]

/**
 * Admin-gated showcase route at /design-system. Renders one section per HUD
 * primitive. Used during SP1 dev to verify each component visually and stays
 * in production as a reference.
 *
 * The data-theme attribute is applied to a local container div, NOT to
 * <html>, so this route's theme switcher doesn't fight the legacy
 * html.theme-oled toggle.
 */
export default function DesignSystem() {
  const { theme, setTheme } = useTheme()

  return (
    <div
      data-theme={theme}
      className="min-h-screen bg-bg-primary text-text-primary"
    >
      <div className="max-w-5xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Page header */}
        <header className="mb-8 flex items-center justify-between gap-4 flex-wrap">
          <div>
            <h1 className="text-page-title">HUD DESIGN SYSTEM</h1>
            <p className="text-body-hud mt-1">
              React mirror of clients/flutter/lib/widgets/hud/* — every
              primitive in every variant in every state.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-mono-label">THEME</span>
            <div className="inline-flex border border-border rounded">
              {themes.map((t) => (
                <button
                  key={t.value}
                  type="button"
                  onClick={() => setTheme(t.value)}
                  className={`px-3 py-1.5 text-mono-control transition-colors ${
                    theme === t.value
                      ? 'bg-accent/[0.13] text-accent'
                      : 'hover:bg-bg-tertiary'
                  }`}
                >
                  {t.label}
                </button>
              ))}
            </div>
          </div>
        </header>

        {/* Sections appear here as each HUD component is added */}
        <main className="space-y-12">
          <ShowcaseSection title="TYPOGRAPHY">
            <div className="space-y-2">
              <div><span className="text-mono-label">.text-mono-label — 9PX MONO TRACKING 1.5</span></div>
              <div><span className="text-mono-section">.text-mono-section — 10PX MONO BOLD ACCENT</span></div>
              <div><span className="text-mono-data">.text-mono-data — 12px mono</span></div>
              <div><span className="text-mono-data-lg">.text-mono-data-lg — 16px mono</span></div>
              <div><span className="text-mono-timestamp">.text-mono-timestamp — 12px accent</span></div>
              <div><span className="text-mono-status">.TEXT-MONO-STATUS — 9PX SUCCESS</span></div>
              <div><span className="text-page-title">.text-page-title — 16px sans semibold</span></div>
              <div><span className="text-camera-name">.text-camera-name — 13px sans medium</span></div>
              <div><span className="text-body-hud">.text-body-hud — 12px sans 1.5 line-height secondary</span></div>
              <div><span className="text-button-hud">.text-button-hud — 12px sans semibold</span></div>
              <div><span className="text-alert-hud">.text-alert-hud — 12px sans danger</span></div>
            </div>
          </ShowcaseSection>
        </main>
      </div>
    </div>
  )
}

/**
 * Reusable wrapper for each showcase section. The HUD components themselves
 * provide a SectionCard primitive but it doesn't exist yet at this point in
 * SP1; this local wrapper is intentional and gets replaced by SectionCard
 * once Task 21 lands.
 */
function ShowcaseSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="border border-border rounded bg-bg-secondary">
      <header className="px-4 py-3 border-b border-border">
        <h2 className="text-mono-section">{title}</h2>
      </header>
      <div className="p-4">{children}</div>
    </section>
  )
}
```

- [ ] **Step 2: Verify it compiles**

```bash
cd ui
npx tsc -b
```

Expected: no errors. The file references the new HUD palette classes (`bg-bg-primary`, `text-text-primary`, etc.) which are now defined in tailwind.config.js.

- [ ] **Step 3: Commit**

```bash
cd ..
git add ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/pages): add DesignSystem showcase route stub

Admin-gated showcase route at /design-system. This commit only adds the
page header, theme switcher, and a typography section. The remaining
HUD component sections are added incrementally by the component tasks
that follow.

The data-theme attribute is applied to a local container div so this
route's theme switcher doesn't conflict with the legacy html.theme-oled
toggle that the rest of the app uses."
```

---

### Task 12: Register /design-system route + patch useBranding

**Files:**
- Modify: `ui/src/App.tsx`

- [ ] **Step 1: Read the current App.tsx import block and useBranding hook**

```bash
sed -n '1,20p;240,300p' ui/src/App.tsx
```

You should see the existing `useBranding` hook around line 244-292 with `document.documentElement.style.setProperty('--nvr-branding-accent', branding.accent_color)`.

- [ ] **Step 2: Add the DesignSystem import at the top of the imports**

Add this line below the existing `import DownloadClient from './pages/DownloadClient'`:

```ts
import DesignSystem from './pages/DesignSystem'
```

- [ ] **Step 3: Add the route registration in AppRoutes**

Find the `function AppRoutes()` block and add the new route alongside the others, *before* the catch-all redirects:

```tsx
<Route path="/design-system" element={<ProtectedRoute><Layout><DesignSystem /></Layout></ProtectedRoute>} />
```

It should sit next to the other top-level routes like `/users`, `/audit`, `/download`. The exact line depends on the current file state — place it consistently with the surrounding routes.

- [ ] **Step 4: Patch the useBranding hook so custom branding accents drive --hud-accent**

Locate the `useBranding` function. Find the `useEffect` that applies the accent color as a CSS variable. The current code looks like:

```ts
useEffect(() => {
  if (branding.accent_color) {
    document.documentElement.style.setProperty('--nvr-branding-accent', branding.accent_color)
  }
}, [branding.accent_color])
```

Replace it with the version below that *also* writes the new `--hud-accent` variable as an `R G B` triplet:

```ts
useEffect(() => {
  if (!branding.accent_color) return
  // Existing legacy variable — kept for any code still reading it.
  document.documentElement.style.setProperty('--nvr-branding-accent', branding.accent_color)
  // New HUD variable — converted to "R G B" triplet so Tailwind's
  // rgb(var(--hud-accent) / <alpha-value>) composition works.
  const rgb = hexToRgbTriplet(branding.accent_color)
  if (rgb) {
    document.documentElement.style.setProperty('--hud-accent', rgb)
  }
}, [branding.accent_color])
```

Then add this helper function inside the `useBranding` module scope (just above `function useBranding()`), or at the top of the file with the other utilities — wherever fits the existing style:

```ts
/**
 * Convert "#rrggbb" or "#rgb" to a "R G B" decimal triplet string suitable
 * for use with Tailwind's rgb(var(--x) / <alpha-value>) composition.
 * Returns null on malformed input so the caller leaves the variable alone.
 */
function hexToRgbTriplet(hex: string): string | null {
  const match = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$|^#?([a-f\d])([a-f\d])([a-f\d])$/i.exec(hex.trim())
  if (!match) return null
  const r = match[1] ?? (match[4]! + match[4]!)
  const g = match[2] ?? (match[5]! + match[5]!)
  const b = match[3] ?? (match[6]! + match[6]!)
  return `${parseInt(r, 16)} ${parseInt(g, 16)} ${parseInt(b, 16)}`
}
```

- [ ] **Step 5: Verify the build**

```bash
cd ui
npm run build
```

Expected: clean build. No new TypeScript errors. The new `/design-system` route is now registered.

- [ ] **Step 6: Verify the route in the running dev server**

```bash
npm run dev
```

Open `http://localhost:5173/design-system` (or whatever port Vite reports) in a browser. You should see the showcase page header with the theme switcher and the TYPOGRAPHY section. The theme switcher should swap dark / OLED / light visibly — backgrounds change, text contrast changes.

Stop the dev server with Ctrl-C when verified.

- [ ] **Step 7: Commit**

```bash
cd ..
git add ui/src/App.tsx
git commit -m "feat(ui): register /design-system route + patch useBranding for --hud-accent

The new admin-gated showcase route at /design-system is wired into
AppRoutes. Visiting it requires admin role; the page is otherwise
freely usable as a HUD component reference.

The useBranding hook is patched to ALSO write the --hud-accent CSS
variable as an 'R G B' triplet whenever a branded accent color is
configured. Existing --nvr-branding-accent writes are preserved for
backward compatibility — both variables stay in sync until SP2 cleans
up the legacy one."
```

---

### Task 13: HudButton

**Files:**
- Create: `ui/src/components/hud/HudButton.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write HudButton.tsx**

```tsx
// ui/src/components/hud/HudButton.tsx
import { type ButtonHTMLAttributes, type ReactNode } from 'react'

export type HudButtonVariant = 'primary' | 'secondary' | 'danger' | 'tactical'

export interface HudButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'> {
  label: string
  variant?: HudButtonVariant
  icon?: ReactNode
  loading?: boolean
  fullWidth?: boolean
}

const variantClasses: Record<HudButtonVariant, string> = {
  primary:
    'bg-accent text-bg-primary border border-transparent hover:bg-accent-hover',
  secondary:
    'bg-bg-tertiary text-text-primary border border-border hover:bg-bg-input',
  danger:
    'bg-danger/[0.13] text-danger border border-danger/[0.27] hover:bg-danger/[0.2]',
  tactical:
    'bg-bg-tertiary text-accent border border-accent/[0.27] hover:bg-accent/[0.13]',
}

const labelTextClass: Record<HudButtonVariant, string> = {
  primary: 'text-button-hud',
  secondary: 'text-button-hud',
  danger: 'text-button-hud',
  // tactical uses mono font + uppercase per the Flutter widget
  tactical: 'font-mono text-[10px] font-medium tracking-[0.1em] uppercase',
}

export function HudButton({
  label,
  variant = 'primary',
  icon,
  loading = false,
  disabled,
  fullWidth = false,
  type = 'button',
  ...rest
}: HudButtonProps) {
  const isDisabled = disabled || loading

  return (
    <button
      type={type}
      disabled={isDisabled}
      className={[
        'inline-flex items-center justify-center gap-1.5 px-4 py-2 rounded-[4px]',
        'transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50',
        isDisabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
        fullWidth ? 'w-full' : '',
        variantClasses[variant],
        labelTextClass[variant],
      ].join(' ')}
      {...rest}
    >
      {loading ? (
        <Spinner />
      ) : icon ? (
        <span className="shrink-0 inline-flex items-center justify-center w-3.5 h-3.5">
          {icon}
        </span>
      ) : null}
      <span>{label}</span>
    </button>
  )
}

function Spinner() {
  return (
    <svg
      className="w-3.5 h-3.5 animate-spin"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
      />
    </svg>
  )
}
```

- [ ] **Step 2: Add a HudButton section to DesignSystem.tsx**

Find the closing `</ShowcaseSection>` of the TYPOGRAPHY section in `ui/src/pages/DesignSystem.tsx` and add this section directly after it:

```tsx
<ShowcaseSection title="HUD BUTTON">
  <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
    <div className="space-y-2">
      <div className="text-mono-label">PRIMARY</div>
      <HudButton label="Save Changes" variant="primary" />
      <HudButton label="Save Changes" variant="primary" disabled />
      <HudButton label="Saving" variant="primary" loading />
    </div>
    <div className="space-y-2">
      <div className="text-mono-label">SECONDARY</div>
      <HudButton label="Cancel" variant="secondary" />
      <HudButton label="Cancel" variant="secondary" disabled />
    </div>
    <div className="space-y-2">
      <div className="text-mono-label">DANGER</div>
      <HudButton label="Delete" variant="danger" />
      <HudButton label="Delete" variant="danger" disabled />
    </div>
    <div className="space-y-2">
      <div className="text-mono-label">TACTICAL</div>
      <HudButton label="ARM" variant="tactical" />
      <HudButton label="ARM" variant="tactical" disabled />
    </div>
  </div>
  <div className="mt-6 space-y-2">
    <div className="text-mono-label">FULL WIDTH</div>
    <HudButton label="Continue" variant="primary" fullWidth />
  </div>
</ShowcaseSection>
```

Add this import at the top of `DesignSystem.tsx` (alongside the existing imports):

```ts
import { HudButton } from '../components/hud/HudButton'
```

- [ ] **Step 3: Verify the build**

```bash
cd ui
npm run build
```

Expected: clean build.

- [ ] **Step 4: Eyeball it in the dev server**

```bash
npm run dev
```

Open `/design-system`. Confirm: 4 columns of buttons, primary/secondary/danger/tactical visible. Each in default + disabled state. Loading spinner spins on the primary loading example. Switch between dark/oled/light themes — borders and fills track the accent color correctly.

- [ ] **Step 5: Commit**

```bash
cd ..
git add ui/src/components/hud/HudButton.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add HudButton component

Mirrors clients/flutter/lib/widgets/hud/hud_button.dart. Four variants
(primary, secondary, danger, tactical), with optional icon, loading
spinner, disabled state, and full-width mode. Tactical variant uses
mono font + uppercase letterspacing per the Flutter widget.

Showcase added to /design-system."
```

---

### Task 14: HudToggle

**Files:**
- Create: `ui/src/components/hud/HudToggle.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write HudToggle.tsx**

```tsx
// ui/src/components/hud/HudToggle.tsx
import { useId } from 'react'

export interface HudToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  label?: string
  showStateLabel?: boolean
  disabled?: boolean
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/hud_toggle.dart.
 *
 * 44x22 pill with an animated thumb. Border changes from text-muted to
 * accent on enable; the thumb glows when on.
 */
export function HudToggle({
  checked,
  onChange,
  label,
  showStateLabel = true,
  disabled = false,
}: HudToggleProps) {
  const id = useId()
  return (
    <div className="inline-flex flex-col items-start gap-1">
      {label && (
        <label htmlFor={id} className="text-mono-label cursor-pointer">
          {label}
        </label>
      )}
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={checked}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={[
          'relative w-11 h-[22px] rounded-full bg-bg-tertiary',
          'border-2 transition-colors duration-150',
          'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50',
          checked ? 'border-accent shadow-[0_0_8px_rgba(249,115,22,0.2)]' : 'border-border',
          disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
        ].join(' ')}
      >
        <span
          className={[
            'absolute top-1/2 -translate-y-1/2 w-3.5 h-3.5 rounded-full transition-all duration-150',
            checked
              ? 'right-0.5 bg-accent shadow-[0_0_6px_rgba(249,115,22,0.4)]'
              : 'left-0.5 bg-text-muted',
          ].join(' ')}
          aria-hidden="true"
        />
      </button>
      {showStateLabel && (
        <span
          className={[
            'font-mono text-[8px] font-medium tracking-[0.125em]',
            checked ? 'text-accent' : 'text-text-muted',
          ].join(' ')}
        >
          {checked ? 'ON' : 'OFF'}
        </span>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Add a controlled showcase to DesignSystem.tsx**

Add this import:

```ts
import { useState } from 'react'
import { HudToggle } from '../components/hud/HudToggle'
```

(`useState` is already imported via `useTheme` indirectly — verify and add explicitly only if missing.)

Add a new section after the HUD BUTTON section:

```tsx
<ShowcaseSection title="HUD TOGGLE">
  <ToggleShowcase />
</ShowcaseSection>
```

Add this helper component below `ShowcaseSection` at the bottom of the file:

```tsx
function ToggleShowcase() {
  const [a, setA] = useState(true)
  const [b, setB] = useState(false)
  return (
    <div className="grid grid-cols-2 sm:grid-cols-4 gap-6">
      <div>
        <HudToggle checked={a} onChange={setA} label="DETECTION" />
      </div>
      <div>
        <HudToggle checked={b} onChange={setB} label="RECORDING" />
      </div>
      <div>
        <HudToggle checked={true} onChange={() => {}} label="DISABLED ON" disabled />
      </div>
      <div>
        <HudToggle checked={false} onChange={() => {}} label="NO STATE LABEL" showStateLabel={false} />
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Open `/design-system`. The toggles should: pill with thumb on left when off (text-muted) and right when on (accent + glow); the ON/OFF state label updates; the third toggle is disabled and dimmed.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/HudToggle.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add HudToggle component

Mirrors clients/flutter/lib/widgets/hud/hud_toggle.dart. 44x22 pill with
animated thumb, accent glow on the on state, optional ON/OFF state
label. Uses role='switch' + aria-checked for screen readers."
```

---

### Task 15: HudInput

**Files:**
- Create: `ui/src/components/hud/HudInput.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write HudInput.tsx**

```tsx
// ui/src/components/hud/HudInput.tsx
import { forwardRef, useId, type InputHTMLAttributes } from 'react'

export interface HudInputProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'className'> {
  label?: string
  error?: string
  hint?: string
  /** When true, render the value in JetBrains Mono. Defaults to false. */
  monoData?: boolean
}

/**
 * Form input field styled to match the HUD design language.
 *
 * Renders an optional mono-label header, the input itself with HUD borders
 * and accent focus ring, and an optional hint or error message below.
 */
export const HudInput = forwardRef<HTMLInputElement, HudInputProps>(
  function HudInput(
    { label, error, hint, monoData = false, id: providedId, ...rest },
    ref,
  ) {
    const generatedId = useId()
    const id = providedId ?? generatedId
    const fontClass = monoData ? 'font-mono text-[12px]' : 'font-sans text-[13px]'

    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label htmlFor={id} className="text-mono-label">
            {label}
          </label>
        )}
        <input
          id={id}
          ref={ref}
          aria-invalid={error ? 'true' : undefined}
          aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
          className={[
            'bg-bg-input border rounded-[4px] px-3 py-2 text-text-primary',
            'placeholder:text-text-muted',
            'focus:outline-none focus:ring-2 focus:ring-accent/50',
            'transition-colors',
            error ? 'border-danger/[0.5]' : 'border-border focus:border-accent',
            'disabled:opacity-50 disabled:cursor-not-allowed',
            fontClass,
          ].join(' ')}
          {...rest}
        />
        {error ? (
          <span id={`${id}-error`} className="text-alert-hud">
            {error}
          </span>
        ) : hint ? (
          <span id={`${id}-hint`} className="text-body-hud">
            {hint}
          </span>
        ) : null}
      </div>
    )
  },
)
```

- [ ] **Step 2: Add showcase section**

Add import:

```ts
import { HudInput } from '../components/hud/HudInput'
```

Add section after HUD TOGGLE:

```tsx
<ShowcaseSection title="HUD INPUT">
  <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 max-w-2xl">
    <HudInput label="CAMERA NAME" placeholder="e.g. Front Door" />
    <HudInput label="ONVIF ENDPOINT" placeholder="http://..." monoData />
    <HudInput label="HINT EXAMPLE" placeholder="Type something" hint="Helpful description" />
    <HudInput
      label="ERROR EXAMPLE"
      defaultValue="invalid"
      error="Must be a valid URL"
    />
    <HudInput label="DISABLED" defaultValue="frozen value" disabled />
    <HudInput
      label="PASSWORD"
      type="password"
      placeholder="••••••••"
      autoComplete="current-password"
    />
  </div>
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Confirm: inputs have HUD borders, focus rings turn accent on click, error variant has red border + alert text below, disabled is dimmed, password field obscures.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/HudInput.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add HudInput component

Form text input with HUD label/border/focus styling. Optional mono-font
mode for technical values (URLs, IDs). Supports error and hint message
slots wired to aria-describedby. forwardRef for use with form
libraries."
```

---

### Task 16: HudTextarea

**Files:**
- Create: `ui/src/components/hud/HudTextarea.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write HudTextarea.tsx**

```tsx
// ui/src/components/hud/HudTextarea.tsx
import { forwardRef, useId, type TextareaHTMLAttributes } from 'react'

export interface HudTextareaProps
  extends Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'className'> {
  label?: string
  error?: string
  hint?: string
  monoData?: boolean
}

/**
 * Multi-line variant of HudInput. Same prop shape, same visual styling.
 */
export const HudTextarea = forwardRef<HTMLTextAreaElement, HudTextareaProps>(
  function HudTextarea(
    { label, error, hint, monoData = false, id: providedId, rows = 4, ...rest },
    ref,
  ) {
    const generatedId = useId()
    const id = providedId ?? generatedId
    const fontClass = monoData ? 'font-mono text-[12px]' : 'font-sans text-[13px]'

    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label htmlFor={id} className="text-mono-label">
            {label}
          </label>
        )}
        <textarea
          id={id}
          ref={ref}
          rows={rows}
          aria-invalid={error ? 'true' : undefined}
          aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
          className={[
            'bg-bg-input border rounded-[4px] px-3 py-2 text-text-primary resize-y',
            'placeholder:text-text-muted',
            'focus:outline-none focus:ring-2 focus:ring-accent/50',
            'transition-colors',
            error ? 'border-danger/[0.5]' : 'border-border focus:border-accent',
            'disabled:opacity-50 disabled:cursor-not-allowed',
            fontClass,
          ].join(' ')}
          {...rest}
        />
        {error ? (
          <span id={`${id}-error`} className="text-alert-hud">
            {error}
          </span>
        ) : hint ? (
          <span id={`${id}-hint`} className="text-body-hud">
            {hint}
          </span>
        ) : null}
      </div>
    )
  },
)
```

- [ ] **Step 2: Add showcase**

Add import:

```ts
import { HudTextarea } from '../components/hud/HudTextarea'
```

Add section:

```tsx
<ShowcaseSection title="HUD TEXTAREA">
  <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 max-w-2xl">
    <HudTextarea label="DESCRIPTION" placeholder="Optional notes about this camera..." />
    <HudTextarea
      label="JSON CONFIG"
      monoData
      defaultValue={'{\n  "key": "value"\n}'}
      rows={6}
    />
  </div>
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/HudTextarea.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add HudTextarea component

Multi-line variant of HudInput with the same visual treatment and prop
shape. Defaults to 4 rows, vertically resizable."
```

---

### Task 17: HudSelect

**Files:**
- Create: `ui/src/components/hud/HudSelect.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write HudSelect.tsx**

```tsx
// ui/src/components/hud/HudSelect.tsx
import { forwardRef, useId, type SelectHTMLAttributes } from 'react'

export interface HudSelectOption {
  value: string
  label: string
}

export interface HudSelectProps
  extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'className'> {
  label?: string
  error?: string
  hint?: string
  options: HudSelectOption[]
}

/**
 * Native <select> dressed in HUD styling. Uses an SVG chevron and matching
 * border/focus ring as HudInput. Native control means accessibility +
 * mobile pickers come for free.
 */
export const HudSelect = forwardRef<HTMLSelectElement, HudSelectProps>(
  function HudSelect(
    { label, error, hint, options, id: providedId, ...rest },
    ref,
  ) {
    const generatedId = useId()
    const id = providedId ?? generatedId

    return (
      <div className="flex flex-col gap-1.5">
        {label && (
          <label htmlFor={id} className="text-mono-label">
            {label}
          </label>
        )}
        <div className="relative">
          <select
            id={id}
            ref={ref}
            aria-invalid={error ? 'true' : undefined}
            aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
            className={[
              'appearance-none w-full bg-bg-input border rounded-[4px] pl-3 pr-9 py-2',
              'font-sans text-[13px] text-text-primary',
              'focus:outline-none focus:ring-2 focus:ring-accent/50',
              'transition-colors',
              error ? 'border-danger/[0.5]' : 'border-border focus:border-accent',
              'disabled:opacity-50 disabled:cursor-not-allowed',
            ].join(' ')}
            {...rest}
          >
            {options.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
          <svg
            aria-hidden="true"
            className="absolute right-3 top-1/2 -translate-y-1/2 w-3 h-3 text-text-secondary pointer-events-none"
            viewBox="0 0 12 12"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
          >
            <path d="M3 4.5L6 7.5L9 4.5" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </div>
        {error ? (
          <span id={`${id}-error`} className="text-alert-hud">
            {error}
          </span>
        ) : hint ? (
          <span id={`${id}-hint`} className="text-body-hud">
            {hint}
          </span>
        ) : null}
      </div>
    )
  },
)
```

- [ ] **Step 2: Add showcase**

Add import:

```ts
import { HudSelect } from '../components/hud/HudSelect'
```

Add section:

```tsx
<ShowcaseSection title="HUD SELECT">
  <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 max-w-2xl">
    <HudSelect
      label="STORAGE TIER"
      options={[
        { value: 'hot', label: 'Hot (NVMe)' },
        { value: 'warm', label: 'Warm (SATA)' },
        { value: 'cold', label: 'Cold (S3)' },
      ]}
    />
    <HudSelect
      label="DISABLED"
      disabled
      options={[{ value: 'a', label: 'Locked' }]}
    />
  </div>
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/HudSelect.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add HudSelect component

Native <select> with HUD styling. Custom SVG chevron, accent focus ring,
matches HudInput visually. Native control gives free mobile pickers and
native a11y."
```

---

### Task 18: AnalogSlider

**Files:**
- Create: `ui/src/components/hud/AnalogSlider.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write AnalogSlider.tsx**

This component is a mouse/touch-driven slider with a custom track. The Flutter version uses GestureDetector with absolute positioning; the React version uses pointer events on a measured container.

```tsx
// ui/src/components/hud/AnalogSlider.tsx
import { useCallback, useRef, useState, type PointerEvent as RPointerEvent } from 'react'

export interface AnalogSliderProps {
  label?: string
  value: number
  min?: number
  max?: number
  step?: number
  tickCount?: number
  disabled?: boolean
  onChange: (value: number) => void
  valueFormatter?: (value: number) => string
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/analog_slider.dart.
 *
 * 24px-tall control: a 6px track with gradient fill, an 18px thumb that
 * grows to 20px while dragging with an accent glow, and a row of tick
 * marks below.
 */
export function AnalogSlider({
  label,
  value,
  min = 0,
  max = 1,
  step,
  tickCount = 11,
  disabled = false,
  onChange,
  valueFormatter,
}: AnalogSliderProps) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [dragging, setDragging] = useState(false)

  const fraction = Math.max(0, Math.min(1, (value - min) / (max - min || 1)))

  const display =
    valueFormatter?.(value) ??
    (max === 1 && min === 0
      ? `${Math.round(value * 100)}%`
      : value.toFixed(0))

  const valueFromPointer = useCallback(
    (clientX: number): number => {
      const el = trackRef.current
      if (!el) return value
      const rect = el.getBoundingClientRect()
      const dx = Math.max(0, Math.min(rect.width, clientX - rect.left))
      const f = dx / rect.width
      let next = min + f * (max - min)
      if (step && step > 0) {
        next = Math.round(next / step) * step
      }
      return Math.max(min, Math.min(max, next))
    },
    [min, max, step, value],
  )

  const handlePointerDown = (e: RPointerEvent<HTMLDivElement>) => {
    if (disabled) return
    e.currentTarget.setPointerCapture(e.pointerId)
    setDragging(true)
    onChange(valueFromPointer(e.clientX))
  }

  const handlePointerMove = (e: RPointerEvent<HTMLDivElement>) => {
    if (!dragging || disabled) return
    onChange(valueFromPointer(e.clientX))
  }

  const handlePointerUp = (e: RPointerEvent<HTMLDivElement>) => {
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId)
    }
    setDragging(false)
  }

  return (
    <div className={['flex flex-col gap-1.5', disabled ? 'opacity-50' : ''].join(' ')}>
      {label && (
        <div className="flex items-center justify-between">
          <span className="text-mono-label">{label}</span>
          <span className="font-mono text-[9px] text-accent">{display}</span>
        </div>
      )}
      <div
        ref={trackRef}
        role="slider"
        aria-valuemin={min}
        aria-valuemax={max}
        aria-valuenow={value}
        aria-disabled={disabled || undefined}
        tabIndex={disabled ? -1 : 0}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onKeyDown={(e) => {
          if (disabled) return
          const nudge = step ?? (max - min) / 100
          if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
            e.preventDefault()
            onChange(Math.max(min, value - nudge))
          } else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
            e.preventDefault()
            onChange(Math.min(max, value + nudge))
          }
        }}
        className={[
          'relative h-6 select-none',
          disabled ? 'cursor-not-allowed' : 'cursor-pointer',
          'focus:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 rounded',
        ].join(' ')}
      >
        {/* Track */}
        <div className="absolute left-0 right-0 top-1/2 -translate-y-1/2 h-1.5 bg-bg-tertiary border border-border rounded" />
        {/* Fill */}
        <div
          className="absolute left-0 top-1/2 -translate-y-1/2 h-1.5 rounded bg-gradient-to-r from-accent to-accent/40"
          style={{ width: `${fraction * 100}%` }}
        />
        {/* Thumb */}
        <div
          className={[
            'absolute top-1/2 -translate-y-1/2 -translate-x-1/2 rounded-full bg-bg-tertiary border-2 border-accent transition-all',
            dragging ? 'w-5 h-5 shadow-[0_0_10px_rgba(249,115,22,0.5)]' : 'w-[18px] h-[18px] shadow-[0_0_6px_rgba(249,115,22,0.25)]',
          ].join(' ')}
          style={{ left: `${fraction * 100}%` }}
          aria-hidden="true"
        />
      </div>
      {/* Tick marks */}
      <div className="flex justify-between mt-0.5">
        {Array.from({ length: tickCount }).map((_, i) => (
          <div key={i} className="w-px h-1 bg-border" aria-hidden="true" />
        ))}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Add showcase (controlled, with state)**

Add import:

```ts
import { AnalogSlider } from '../components/hud/AnalogSlider'
```

Add showcase helper at the bottom of the file:

```tsx
function SliderShowcase() {
  const [a, setA] = useState(0.5)
  const [b, setB] = useState(15)
  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 gap-8 max-w-2xl">
      <AnalogSlider label="CONFIDENCE" value={a} onChange={setA} />
      <AnalogSlider
        label="RETENTION DAYS"
        value={b}
        min={1}
        max={30}
        step={1}
        tickCount={6}
        valueFormatter={(v) => `${v.toFixed(0)}d`}
        onChange={setB}
      />
      <AnalogSlider label="DISABLED" value={0.7} onChange={() => {}} disabled />
    </div>
  )
}
```

Add section:

```tsx
<ShowcaseSection title="ANALOG SLIDER">
  <SliderShowcase />
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Click and drag the sliders. The thumb should grow and gain a brighter glow on drag. Tab to a slider and use arrow keys — value should change. Disabled slider should not respond.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/AnalogSlider.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add AnalogSlider component

Mirrors clients/flutter/lib/widgets/hud/analog_slider.dart. 24px-tall
slider with a 6px gradient track, 18px thumb that grows to 20px and
gains a brighter glow while dragging, and tick marks below. Pointer
events handle mouse + touch in one path; keyboard arrows nudge the
value when focused. role='slider' + aria-valuemin/max/now for screen
readers."
```

---

### Task 19: SegmentedControl

**Files:**
- Create: `ui/src/components/hud/SegmentedControl.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write SegmentedControl.tsx**

```tsx
// ui/src/components/hud/SegmentedControl.tsx
import { type ReactNode } from 'react'

export interface SegmentedControlOption<T extends string> {
  value: T
  label: string
  icon?: ReactNode
}

export interface SegmentedControlProps<T extends string> {
  options: SegmentedControlOption<T>[]
  value: T
  onChange: (value: T) => void
  disabled?: boolean
  /** ARIA label for the group */
  ariaLabel?: string
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/segmented_control.dart.
 *
 * Bordered container, vertical separators between segments, 13% accent
 * fill on the selected segment, mono 9px label.
 */
export function SegmentedControl<T extends string>({
  options,
  value,
  onChange,
  disabled = false,
  ariaLabel,
}: SegmentedControlProps<T>) {
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className={[
        'inline-flex bg-bg-primary border border-border rounded-[4px] overflow-hidden',
        disabled ? 'opacity-50' : '',
      ].join(' ')}
    >
      {options.map((opt, i) => {
        const selected = opt.value === value
        return (
          <div key={opt.value} className="flex items-stretch">
            {i > 0 && <div className="w-px bg-border" aria-hidden="true" />}
            <button
              type="button"
              role="radio"
              aria-checked={selected}
              disabled={disabled}
              onClick={() => onChange(opt.value)}
              className={[
                'flex items-center gap-1.5 px-2.5 py-1.5 transition-colors',
                'font-mono text-[9px] tracking-[0.05em]',
                'focus:outline-none focus-visible:ring-2 focus-visible:ring-accent/50',
                selected
                  ? 'bg-accent/[0.13] text-accent'
                  : 'text-text-muted hover:text-text-secondary',
                disabled ? 'cursor-not-allowed' : 'cursor-pointer',
              ].join(' ')}
            >
              {opt.icon && (
                <span className="inline-flex items-center justify-center w-3 h-3">
                  {opt.icon}
                </span>
              )}
              {opt.label}
            </button>
          </div>
        )
      })}
    </div>
  )
}
```

- [ ] **Step 2: Add showcase**

Add import:

```ts
import { SegmentedControl } from '../components/hud/SegmentedControl'
```

Add helper + section:

```tsx
function SegmentedShowcase() {
  const [tab, setTab] = useState<'streams' | 'recording' | 'ai' | 'onvif'>('streams')
  return (
    <div className="space-y-4">
      <SegmentedControl
        ariaLabel="Camera tab"
        value={tab}
        onChange={setTab}
        options={[
          { value: 'streams', label: 'STREAMS' },
          { value: 'recording', label: 'RECORDING' },
          { value: 'ai', label: 'AI' },
          { value: 'onvif', label: 'ONVIF' },
        ]}
      />
      <div className="text-mono-data">selected: {tab}</div>
      <SegmentedControl
        value="dark"
        onChange={() => {}}
        disabled
        options={[
          { value: 'dark', label: 'DARK' },
          { value: 'oled', label: 'OLED' },
          { value: 'light', label: 'LIGHT' },
        ]}
      />
    </div>
  )
}
```

```tsx
<ShowcaseSection title="SEGMENTED CONTROL">
  <SegmentedShowcase />
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Click between segments — the selected one fills with accent at 13% opacity, others stay muted. The "selected: x" line below tracks the click. Disabled control is dimmed.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/SegmentedControl.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add SegmentedControl component

Mirrors clients/flutter/lib/widgets/hud/segmented_control.dart. Bordered
container, vertical separators between segments, 13% accent fill on
selection, mono 9px tracking-0.05em label. role='radiogroup' with
role='radio' children for screen readers."
```

---

### Task 20: StatusBadge

**Files:**
- Create: `ui/src/components/hud/StatusBadge.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write StatusBadge.tsx**

```tsx
// ui/src/components/hud/StatusBadge.tsx

export type StatusVariant =
  | 'online'
  | 'offline'
  | 'degraded'
  | 'recording'
  | 'live'
  | 'motion'
  | 'warning'

export interface StatusBadgeProps {
  variant: StatusVariant
  /** Override the default label text. */
  label?: string
  /** Show the leading colored dot. Defaults to true (false for `recording`). */
  showDot?: boolean
  /** Animated pulse on the dot for live states. */
  pulse?: boolean
}

interface VariantSpec {
  color: string // Tailwind text-* / border-* color name
  defaultLabel: string
  defaultDot: boolean
}

const variants: Record<StatusVariant, VariantSpec> = {
  online: { color: 'success', defaultLabel: 'ONLINE', defaultDot: true },
  offline: { color: 'danger', defaultLabel: 'OFFLINE', defaultDot: true },
  degraded: { color: 'warning', defaultLabel: 'DEGRADED', defaultDot: true },
  warning: { color: 'warning', defaultLabel: 'WARNING', defaultDot: true },
  live: { color: 'success', defaultLabel: 'LIVE', defaultDot: true },
  recording: { color: 'danger', defaultLabel: 'REC', defaultDot: false },
  motion: { color: 'accent', defaultLabel: 'MOTION', defaultDot: true },
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/status_badge.dart.
 *
 * Pill with optional leading colored dot and an uppercase mono label.
 * Background is the variant color at ~7% opacity, border at ~27%.
 */
export function StatusBadge({
  variant,
  label,
  showDot,
  pulse = false,
}: StatusBadgeProps) {
  const spec = variants[variant]
  const dot = showDot ?? spec.defaultDot
  const text = label ?? spec.defaultLabel

  // Tailwind cannot resolve dynamic class names from a string variable, so we
  // map the color name to its three classes explicitly.
  const colorMap: Record<string, { bg: string; border: string; text: string; dot: string }> = {
    success: { bg: 'bg-success/[0.07]', border: 'border-success/[0.27]', text: 'text-success', dot: 'bg-success' },
    danger: { bg: 'bg-danger/[0.07]', border: 'border-danger/[0.27]', text: 'text-danger', dot: 'bg-danger' },
    warning: { bg: 'bg-warning/[0.07]', border: 'border-warning/[0.27]', text: 'text-warning', dot: 'bg-warning' },
    accent: { bg: 'bg-accent/[0.07]', border: 'border-accent/[0.27]', text: 'text-accent', dot: 'bg-accent' },
  }
  const c = colorMap[spec.color]

  return (
    <span
      className={[
        'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-[4px] border',
        c.bg,
        c.border,
      ].join(' ')}
    >
      {dot && (
        <span
          className={[
            'inline-block w-1.5 h-1.5 rounded-full shadow-[0_0_6px_currentColor]',
            c.dot,
            c.text,
            pulse ? 'animate-pulse-dot' : '',
          ].join(' ')}
          aria-hidden="true"
        />
      )}
      <span
        className={[
          'font-mono text-[9px] font-medium tracking-[0.06em]',
          c.text,
        ].join(' ')}
      >
        {text}
      </span>
    </span>
  )
}
```

- [ ] **Step 2: Add showcase**

Add import:

```ts
import { StatusBadge } from '../components/hud/StatusBadge'
```

Add section:

```tsx
<ShowcaseSection title="STATUS BADGE">
  <div className="flex flex-wrap gap-3">
    <StatusBadge variant="online" />
    <StatusBadge variant="offline" />
    <StatusBadge variant="degraded" />
    <StatusBadge variant="warning" />
    <StatusBadge variant="live" pulse />
    <StatusBadge variant="recording" pulse />
    <StatusBadge variant="motion" />
    <StatusBadge variant="online" label="42 CAMERAS" />
  </div>
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Confirm: each badge has the correct color (online=green, offline=red, motion=orange, etc.), `live` and `recording` pulse, the dot has a subtle glow, the `recording` badge has no dot per the Flutter version.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/StatusBadge.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add StatusBadge component

Mirrors clients/flutter/lib/widgets/hud/status_badge.dart. Seven
variants (online/offline/degraded/warning/live/recording/motion). Each
variant has a default label and dot visibility — the recording badge
has no dot by default, matching the Flutter widget. Optional pulse
animation for live/recording states uses the new pulse-dot keyframe
added to tailwind.config.js."
```

---

### Task 21: CornerBrackets

**Files:**
- Create: `ui/src/components/hud/CornerBrackets.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write CornerBrackets.tsx**

```tsx
// ui/src/components/hud/CornerBrackets.tsx
import { type ReactNode } from 'react'

export interface CornerBracketsProps {
  children: ReactNode
  size?: 'sm' | 'md' | 'lg'
  color?: 'accent' | 'border' | 'success' | 'danger' | 'warning'
  padding?: number
  strokeWidth?: number
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/corner_brackets.dart.
 *
 * L-shaped brackets at each corner of the wrapped child. The brackets are
 * positioned absolutely, so they don't add layout space. The child should
 * be a sized element (image, video, etc.) for best effect.
 */

const sizeMap = { sm: 12, md: 16, lg: 24 }

const colorMap: Record<NonNullable<CornerBracketsProps['color']>, string> = {
  accent: 'text-accent',
  border: 'text-border',
  success: 'text-success',
  danger: 'text-danger',
  warning: 'text-warning',
}

export function CornerBrackets({
  children,
  size = 'md',
  color = 'accent',
  padding = 6,
  strokeWidth = 2,
}: CornerBracketsProps) {
  const px = sizeMap[size]
  const colorClass = colorMap[color]

  return (
    <div className="relative">
      {children}
      <Bracket corner="tl" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
      <Bracket corner="tr" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
      <Bracket corner="bl" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
      <Bracket corner="br" px={px} stroke={strokeWidth} pad={padding} colorClass={colorClass} />
    </div>
  )
}

type Corner = 'tl' | 'tr' | 'bl' | 'br'

function Bracket({
  corner,
  px,
  stroke,
  pad,
  colorClass,
}: {
  corner: Corner
  px: number
  stroke: number
  pad: number
  colorClass: string
}) {
  const positionStyle: React.CSSProperties = {
    width: px,
    height: px,
    position: 'absolute',
    pointerEvents: 'none',
    opacity: 0.4,
  }
  if (corner === 'tl') {
    positionStyle.top = pad
    positionStyle.left = pad
  } else if (corner === 'tr') {
    positionStyle.top = pad
    positionStyle.right = pad
  } else if (corner === 'bl') {
    positionStyle.bottom = pad
    positionStyle.left = pad
  } else {
    positionStyle.bottom = pad
    positionStyle.right = pad
  }

  // The path describes an L-shape oriented for the given corner.
  // tl: down then right (anchor at top-left)
  // tr: left then down (anchor at top-right)
  // bl: up then right
  // br: left then up
  let d = ''
  if (corner === 'tl') d = `M 0 ${px} L 0 0 L ${px} 0`
  else if (corner === 'tr') d = `M 0 0 L ${px} 0 L ${px} ${px}`
  else if (corner === 'bl') d = `M 0 0 L 0 ${px} L ${px} ${px}`
  else d = `M ${px} 0 L ${px} ${px} L 0 ${px}`

  return (
    <svg
      className={colorClass}
      style={positionStyle}
      viewBox={`0 0 ${px} ${px}`}
      fill="none"
      stroke="currentColor"
      strokeWidth={stroke}
      strokeLinecap="square"
      aria-hidden="true"
    >
      <path d={d} />
    </svg>
  )
}
```

- [ ] **Step 2: Add showcase**

Add import:

```ts
import { CornerBrackets } from '../components/hud/CornerBrackets'
```

Add section:

```tsx
<ShowcaseSection title="CORNER BRACKETS">
  <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
    <CornerBrackets size="md">
      <div className="aspect-video bg-bg-tertiary rounded flex items-center justify-center text-mono-control">
        16:9 PREVIEW
      </div>
    </CornerBrackets>
    <CornerBrackets size="lg" color="success">
      <div className="aspect-video bg-bg-tertiary rounded flex items-center justify-center text-mono-control">
        SUCCESS
      </div>
    </CornerBrackets>
    <CornerBrackets size="sm" color="danger">
      <div className="aspect-video bg-bg-tertiary rounded flex items-center justify-center text-mono-control">
        DANGER
      </div>
    </CornerBrackets>
  </div>
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Confirm: each preview tile shows L-shaped brackets at all 4 corners. The brackets are inside the tile (padding 6) and don't change the tile's dimensions.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/CornerBrackets.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add CornerBrackets component

Mirrors clients/flutter/lib/widgets/hud/corner_brackets.dart. L-shaped
SVG brackets at each corner of a wrapped child, positioned absolutely
so they don't affect layout. Three sizes (sm/md/lg), five color
options (accent default, plus border/success/danger/warning),
configurable padding and stroke width."
```

---

### Task 22: SectionCard

**Files:**
- Create: `ui/src/components/hud/SectionCard.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write SectionCard.tsx**

```tsx
// ui/src/components/hud/SectionCard.tsx
import { type ReactNode } from 'react'

export interface SectionCardProps {
  /** Mono-section header text. Rendered uppercase via the type ramp class. */
  header: string
  /** Optional right-aligned actions in the header (e.g. a HudButton). */
  actions?: ReactNode
  /** Body content. Wrapped in p-4 padding by default. */
  children: ReactNode
  /** When true, omit the body padding (use for tables / canvases). */
  flush?: boolean
}

/**
 * Bordered card with a mono section header. The standard container for
 * grouped content on detail screens, mirroring how the Flutter UI uses
 * Container + DecoratedBox + section labels everywhere.
 */
export function SectionCard({ header, actions, children, flush = false }: SectionCardProps) {
  return (
    <section className="border border-border rounded-[4px] bg-bg-secondary">
      <header className="flex items-center justify-between gap-4 px-4 py-2.5 border-b border-border">
        <h3 className="text-mono-section">{header}</h3>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </header>
      <div className={flush ? '' : 'p-4'}>{children}</div>
    </section>
  )
}
```

- [ ] **Step 2: Replace the local ShowcaseSection in DesignSystem.tsx with SectionCard**

The local `ShowcaseSection` helper at the bottom of `DesignSystem.tsx` was always meant as a stand-in. Now replace its body with a wrapper around SectionCard so the showcase is consistent with what the rest of the app will use.

Replace:

```tsx
function ShowcaseSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="border border-border rounded bg-bg-secondary">
      <header className="px-4 py-3 border-b border-border">
        <h2 className="text-mono-section">{title}</h2>
      </header>
      <div className="p-4">{children}</div>
    </section>
  )
}
```

with:

```tsx
function ShowcaseSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return <SectionCard header={title}>{children}</SectionCard>
}
```

Add the import at the top of `DesignSystem.tsx`:

```ts
import { SectionCard } from '../components/hud/SectionCard'
```

- [ ] **Step 3: Add a SectionCard variants showcase**

Add section just before the closing `</main>`:

```tsx
<ShowcaseSection title="SECTION CARD">
  <div className="space-y-4">
    <SectionCard header="WITH ACTIONS" actions={<HudButton label="Refresh" variant="secondary" />}>
      <p className="text-body-hud">A card with a header action button on the right.</p>
    </SectionCard>
    <SectionCard header="FLUSH (NO BODY PADDING)" flush>
      <div className="px-4 py-3 border-b border-border text-mono-data">row 1</div>
      <div className="px-4 py-3 border-b border-border text-mono-data">row 2</div>
      <div className="px-4 py-3 text-mono-data">row 3</div>
    </SectionCard>
  </div>
</ShowcaseSection>
```

- [ ] **Step 4: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

The showcase should now look identical to before because `ShowcaseSection` is just a `SectionCard` wrapper. Verify the new "SECTION CARD" section renders the with-actions and flush variants correctly.

- [ ] **Step 5: Commit**

```bash
cd ..
git add ui/src/components/hud/SectionCard.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add SectionCard component

Bordered card with a mono section header and optional right-aligned
header actions. Standard container for grouped content on detail
screens. The DesignSystem showcase route now uses SectionCard via the
ShowcaseSection wrapper for consistency."
```

---

### Task 23: KvRow

**Files:**
- Create: `ui/src/components/hud/KvRow.tsx`
- Modify: `ui/src/pages/DesignSystem.tsx`

- [ ] **Step 1: Write KvRow.tsx**

```tsx
// ui/src/components/hud/KvRow.tsx
import { useState, type ReactNode } from 'react'

export interface KvRowProps {
  label: string
  /** String values are rendered in mono; ReactNode values render as-is. */
  value: ReactNode
  /** Show a copy-to-clipboard button when the row is hovered. */
  copyable?: boolean
}

/**
 * Key-value row used in detail and info panels. Label is mono-label
 * (uppercase, tracked); value is mono-data (12px monospace) for strings,
 * or whatever JSX you pass in.
 *
 * The copyable button only appears for string values, since copying a
 * ReactNode doesn't have a sensible meaning.
 */
export function KvRow({ label, value, copyable = false }: KvRowProps) {
  const [copied, setCopied] = useState(false)
  const isString = typeof value === 'string'

  const handleCopy = async () => {
    if (!isString) return
    try {
      await navigator.clipboard.writeText(value as string)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // clipboard API may be blocked — silently ignore
    }
  }

  return (
    <div className="group flex items-baseline gap-3">
      <div className="text-mono-label w-28 shrink-0">{label}</div>
      <div className="flex-1 min-w-0 flex items-center gap-2">
        <div className={isString ? 'text-mono-data truncate' : ''}>{value}</div>
        {copyable && isString && (
          <button
            type="button"
            onClick={handleCopy}
            aria-label={`Copy ${label}`}
            className="opacity-0 group-hover:opacity-100 focus:opacity-100 text-text-muted hover:text-accent transition-opacity"
          >
            {copied ? (
              <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
                <path d="M3 8l3 3 7-7" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            ) : (
              <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
                <rect x="5" y="5" width="9" height="9" rx="1" />
                <path d="M11 5V3a1 1 0 00-1-1H3a1 1 0 00-1 1v7a1 1 0 001 1h2" />
              </svg>
            )}
          </button>
        )}
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Add showcase**

Add import:

```ts
import { KvRow } from '../components/hud/KvRow'
```

Add section:

```tsx
<ShowcaseSection title="KEY-VALUE ROW">
  <div className="space-y-2 max-w-md">
    <KvRow label="MANUFACTURER" value="Hanwha" />
    <KvRow label="MODEL" value="QNV-7080R" />
    <KvRow label="FIRMWARE" value="1.41.05_20231012" copyable />
    <KvRow label="SERIAL" value="ZQ4N5L0PA00100K" copyable />
    <KvRow label="STATUS" value={<StatusBadge variant="online" />} />
    <KvRow label="UPTIME" value="14d 6h 22m" />
  </div>
</ShowcaseSection>
```

- [ ] **Step 3: Verify build + eyeball**

```bash
cd ui
npm run build
npm run dev
```

Hover over a copyable row — a copy icon appears next to the value. Click it — the icon briefly turns into a checkmark. Try the StatusBadge row — it embeds correctly because the value is a ReactNode, not a string.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/KvRow.tsx ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add KvRow component

Key-value row used in detail panels. Label is mono-label, value is
mono-data for strings or arbitrary JSX. Optional copy-to-clipboard
button appears on hover for string values; checkmark feedback for
1.5s after a successful copy."
```

---

### Task 24: Barrel re-export

**Files:**
- Create: `ui/src/components/hud/index.ts`

- [ ] **Step 1: Write the barrel**

```ts
// ui/src/components/hud/index.ts
//
// Barrel re-export for the HUD component library. Consumers should import
// from this file rather than reaching into individual files:
//
//   import { HudButton, StatusBadge, SectionCard } from '~/components/hud'

export { HudButton, type HudButtonVariant, type HudButtonProps } from './HudButton'
export { HudToggle, type HudToggleProps } from './HudToggle'
export { HudInput, type HudInputProps } from './HudInput'
export { HudTextarea, type HudTextareaProps } from './HudTextarea'
export { HudSelect, type HudSelectOption, type HudSelectProps } from './HudSelect'
export { AnalogSlider, type AnalogSliderProps } from './AnalogSlider'
export {
  SegmentedControl,
  type SegmentedControlOption,
  type SegmentedControlProps,
} from './SegmentedControl'
export { StatusBadge, type StatusVariant, type StatusBadgeProps } from './StatusBadge'
export { CornerBrackets, type CornerBracketsProps } from './CornerBrackets'
export { SectionCard, type SectionCardProps } from './SectionCard'
export { KvRow, type KvRowProps } from './KvRow'
```

- [ ] **Step 2: Update DesignSystem.tsx imports to use the barrel**

Replace the eleven individual imports of HUD components with a single import:

```ts
import {
  HudButton,
  HudToggle,
  HudInput,
  HudTextarea,
  HudSelect,
  AnalogSlider,
  SegmentedControl,
  StatusBadge,
  CornerBrackets,
  SectionCard,
  KvRow,
} from '../components/hud'
```

(Delete the prior individual imports.)

- [ ] **Step 3: Verify build**

```bash
cd ui
npm run build
```

Expected: clean build. The showcase route should still render identically.

- [ ] **Step 4: Commit**

```bash
cd ..
git add ui/src/components/hud/index.ts ui/src/pages/DesignSystem.tsx
git commit -m "feat(ui/hud): add barrel re-export for HUD component library

Consumers import from ~/components/hud instead of individual files. The
DesignSystem showcase route is updated to use the barrel as a smoke test."
```

---

### Task 25: Final verification — full build, walkthrough, screenshots

**Files:** none modified

- [ ] **Step 1: Clean install + build**

```bash
cd ui
rm -rf node_modules dist
nvm use
npm ci
npm run build
```

Expected: clean build. The bundle size will be larger than baseline because of the embedded fonts (~150KB extra). The pre-existing chunk-split warning is OK. No new errors.

- [ ] **Step 2: Walk through every showcase section in dev**

```bash
npm run dev
```

Open `http://localhost:5173/login` and log in as admin. Then navigate to `http://localhost:5173/design-system`.

For each section, verify:

- **TYPOGRAPHY**: 11 type ramp helpers all render distinctly (mono vs sans, sizes from 8px to 16px, accent color on `mono-section` and `mono-timestamp`)
- **HUD BUTTON**: 4 variants × default/disabled/loading; full-width example
- **HUD TOGGLE**: 4 toggles, including disabled-on, no-state-label
- **HUD INPUT**: 6 inputs covering plain, mono, hint, error, disabled, password
- **HUD TEXTAREA**: 2 textareas
- **HUD SELECT**: 2 selects
- **ANALOG SLIDER**: 3 sliders, including a disabled one. Click + drag works. Tab + arrow keys work. Tick marks are visible.
- **SEGMENTED CONTROL**: 4-option control (interactive) + 3-option disabled
- **STATUS BADGE**: 8 badges, two with `pulse`. Recording badge has no dot.
- **CORNER BRACKETS**: 3 preview tiles with brackets in different sizes/colors
- **SECTION CARD**: with-actions card + flush card with rows
- **KEY-VALUE ROW**: 6 rows including copyable rows (hover to see icon) and a row with embedded StatusBadge

- [ ] **Step 3: Cycle through all 3 themes**

In the showcase header, click DARK → OLED → LIGHT. For each theme verify:

- Backgrounds shift correctly (OLED is pure black; light is off-white)
- Accent stays orange (slightly darker shade in light mode)
- Text contrast is readable in all themes
- Borders are visible in all themes
- All components remain legible — pay particular attention to the analog slider thumb (should still glow), status badges (border + dot still visible), and section card borders

- [ ] **Step 4: Sanity check that existing pages still render correctly**

While the dev server is running, navigate to:

- `/login` → looks identical to before SP1
- `/cameras` → looks identical, blue accent, slate-blue palette
- `/settings` → looks identical
- `/dashboard` → looks identical

There should be **zero visual regressions** on existing pages. If something looks different, the deviation is the IBM Plex Sans font now being preferred over Inter — that is the documented expected drift. Anything else is a bug.

Stop the dev server with Ctrl-C.

- [ ] **Step 5: Take screenshots of /design-system in all 3 themes for the PR**

Save them to `/tmp/sp1-{dark,oled,light}.png`. They'll be attached to the PR description.

- [ ] **Step 6: One last build to make sure the dist artifact is current**

```bash
npm run build
```

- [ ] **Step 7: Touch up the existing dist .gitkeep**

```bash
cd ..
touch internal/nvr/ui/dist/.gitkeep
git status --short
```

You should see only `M ui/tsconfig.app.tsbuildinfo` (incidental tsc cache, do NOT commit) and `?? internal/nvr/ui/dist/.gitkeep` if vite cleared it. Restore tsbuildinfo if it shows as modified:

```bash
git checkout ui/tsconfig.app.tsbuildinfo
```

`git status` should now show no uncommitted changes from the build.

---

### Task 26: Push and open the SP1 PR

**Files:** none

- [ ] **Step 1: Confirm branch state**

```bash
git log --oneline origin/main..HEAD
```

You should see all the SP1 commits in order: nvmrc → fontsource deps → colors → tokens → fonts → tailwind → typography → useTheme → main.tsx → DesignSystem stub → App.tsx routes → 11 HUD components → barrel.

- [ ] **Step 2: Push the branch**

```bash
git push -u origin feat/react-hud-foundation
```

- [ ] **Step 3: Create the PR**

```bash
gh pr create --base main --head feat/react-hud-foundation \
  --title "feat(ui): SP1 — HUD design system foundation" \
  --body "$(cat <<'EOF'
## Summary

First half of the React admin console rewrite to share the Flutter NVR client's design language. This PR is **Sub-Project 1** of two: the additive foundation. SP2 (which migrates every existing page to the new design) depends on this PR being in \`main\`.

Spec: \`docs/superpowers/specs/2026-04-06-react-hud-rewrite-design.md\`

## What ships in SP1

- **CSS variable theme system** — three runtime-switchable themes (\`dark\`, \`oled\`, \`light\`) backed by \`--hud-*\` CSS custom properties in \`ui/src/theme/tokens.css\`. Mirrors \`clients/flutter/lib/theme/nvr_colors.dart\` exactly.
- **HUD type ramp** — eleven typography helper classes (\`text-mono-label\`, \`text-mono-section\`, \`text-page-title\`, etc.) ported from \`nvr_typography.dart\`.
- **Self-hosted fonts** — JetBrains Mono and IBM Plex Sans via \`@fontsource\` packages. No CDN dependency, no manual woff2 management.
- **HUD component library** — eleven primitives in \`ui/src/components/hud/\`:
  - \`HudButton\` (4 variants: primary, secondary, danger, tactical)
  - \`HudToggle\` (animated pill switch)
  - \`HudInput\` / \`HudTextarea\` / \`HudSelect\` (form fields)
  - \`AnalogSlider\` (gradient track + accent-glow thumb)
  - \`SegmentedControl\` (Flutter-style tab strip)
  - \`StatusBadge\` (7 variants, optional pulse)
  - \`CornerBrackets\` (decorative HUD frame)
  - \`SectionCard\` (bordered card with mono section header)
  - \`KvRow\` (key-value rows with copy-to-clipboard)
- **\`/design-system\` showcase route** — admin-gated, renders every component in every variant in every state. Has its own theme switcher applied via \`data-theme\` on a local container so it doesn't fight the existing \`html.theme-oled\` toggle.
- **\`.nvmrc\` pinning Node 20** — fixes the recurring \`crypto.getRandomValues\` crash on Node 16.

## Deviations from the spec

Both deviations are documented in \`docs/superpowers/plans/2026-04-06-react-hud-foundation-plan.md\` under "Important deviations from the spec":

1. **CSS variable prefix is \`--hud-*\`, not \`--nvr-*\`** to avoid colliding with the legacy \`html.theme-oled\` system already using \`--nvr-*\` in \`index.css\`.
2. **Tailwind palette is additive.** The existing \`nvr.*\` nested color block stays untouched so legacy pages keep working. The new flat-named entries (\`bg-primary\`, \`text-primary\`, \`accent\`, etc.) coexist alongside it. SP2 will delete the old palette when the last legacy page is migrated.
3. **\`useTheme\` hook is local-only in SP1.** It writes to local React state, not \`<html data-theme>\`. The showcase route applies \`data-theme\` to a container div instead. SP2 will globalize it.

## Existing pages

**Zero functional regressions.** The only visible change to existing pages is that \`font-sans\` now resolves to IBM Plex Sans first (Inter is still in the fallback chain). Acceptable cosmetic drift; the legacy color palette is untouched.

The KAI-58 branding feature continues to work — the \`useBranding\` hook now writes both \`--nvr-branding-accent\` (legacy) and \`--hud-accent\` (new, as an \`R G B\` triplet) so any branded accent color flows through both systems.

## Test plan

- [x] \`npm run build\` clean (Node 20)
- [x] Walked through every section of \`/design-system\` in dev
- [x] Cycled all three themes (dark / oled / light) — backgrounds shift, accent updates, contrast remains readable
- [x] Verified no visual regressions on \`/login\`, \`/cameras\`, \`/settings\`, \`/dashboard\`
- [x] Verified analog slider works with mouse drag, tap, and keyboard arrows
- [x] Verified status badge pulse animation, copy-to-clipboard on KvRow
- [ ] Manual: log in as admin, navigate to \`/design-system\`, click each theme button, eyeball every section

## Screenshots

Attached: \`/design-system\` rendered in dark, OLED, and light themes.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Attach the three screenshots from `/tmp/sp1-*.png` to the PR via the GitHub web UI after creation.

- [ ] **Step 4: Check PR status**

```bash
gh pr view --json state,mergeable,statusCheckRollup
```

Expected: state OPEN, mergeable MERGEABLE. CI may show pre-existing failures from `main` (the same `TestSampleConfFile`, `syscheck` 32-bit, `go_mod` lint, `docs` lint as previous PRs). Verify nothing **new** is failing. Specifically: there should be no React/UI-related build failures.

---

## Self-Review

This is the inline self-review the writing-plans skill requires. Run it once after finishing the plan; fix any issues inline.

### Spec coverage check

| Spec section                                     | Implementing task(s)                |
| ------------------------------------------------ | ----------------------------------- |
| `theme/colors.ts`                                | Task 4                              |
| `theme/typography.ts`                            | Task 8                              |
| `theme/tokens.css` (3 themes)                    | Task 5                              |
| `theme/fonts.css`                                | Task 6                              |
| `theme/useTheme.ts`                              | Task 9                              |
| Tailwind CSS-variable extension                  | Task 7                              |
| Type ramp helper classes                         | Task 8                              |
| HudButton                                        | Task 13                             |
| HudToggle                                        | Task 14                             |
| HudInput                                         | Task 15                             |
| HudTextarea                                      | Task 16                             |
| HudSelect                                        | Task 17                             |
| AnalogSlider                                     | Task 18                             |
| SegmentedControl                                 | Task 19                             |
| StatusBadge                                      | Task 20                             |
| CornerBrackets                                   | Task 21                             |
| SectionCard                                      | Task 22                             |
| KvRow                                            | Task 23                             |
| Barrel `index.ts`                                | Task 24                             |
| `/design-system` showcase route                  | Tasks 11, 12, every component task  |
| `.nvmrc` for Node 20                             | Task 2                              |
| `@fontsource/*` deps                             | Task 3                              |
| Branding-hook patch in `App.tsx`                 | Task 12                             |
| `main.tsx` CSS imports                           | Task 10                             |
| Worktree on `feat/react-hud-foundation`          | Task 1                              |
| PR opened to main                                | Task 26                             |

Every spec deliverable maps to a task. The deferred RotaryKnob and CameraThumbnail are intentionally absent.

### Placeholder scan

No "TBD", "TODO", "implement later", "fill in details", "Add appropriate error handling", "similar to Task N", or other plan-failure patterns. Every code-changing step includes the actual code to write.

### Type consistency check

- `HudButton` props: `variant`, `label`, `icon`, `loading`, `disabled`, `fullWidth`, `type` — referenced consistently in showcase.
- `StatusBadge` variant names: `online | offline | degraded | recording | live | motion | warning` — consistent between component and showcase.
- `AnalogSlider` props: `value`, `min`, `max`, `step`, `tickCount`, `onChange`, `valueFormatter`, `disabled` — consistent.
- `SegmentedControl<T>` generic parameter — consistent in component, helper, and showcase.
- `useTheme` returns `{ theme, setTheme }` — consistent.
- `KvRow.value` typed as `ReactNode` — consistent (showcase passes both strings and a `<StatusBadge />`).
- The `pulse-dot` keyframe added in Task 7 (tailwind config) is referenced in Task 20 (`StatusBadge`'s `animate-pulse-dot` class) — order is correct.
- The `SectionCard` is created in Task 22, and Task 22 also replaces the local `ShowcaseSection` body to use it. The component tasks before Task 22 use `ShowcaseSection`, which works because the Task 11 stub defines it. Task 22 swaps the implementation without changing the call sites — internally consistent.
- `theme/typography.ts` exports `hudType` constants but no task references them. They're provided for future SP2 consumers; this is intentional, not a bug.

### Scope check

This plan is ~1620 LOC of additions across ~17 new files and 4 modified files. It's at the upper end of "single PR" but the additive nature and the per-component commit structure keep each diff hunk reviewable in isolation. No further decomposition needed.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-04-06-react-hud-foundation-plan.md`. Two execution options:

**1. Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints

**Which approach?**
