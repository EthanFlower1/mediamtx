# React Admin Console — HUD Rewrite Design Spec

## Overview

Rewrite the MediaMTX NVR React admin console (`ui/`) to share the same design language as the Flutter primary client. The React app remains a **configuration tool** — it does not gain Live View, Playback, Search, or Screenshots. It does adopt Flutter's HUD visual identity, Flutter's information architecture for everything in the configuration surface, and a reusable React component library that mirrors Flutter's HUD widgets.

The work is split into **two sequential sub-projects**, each shipped as a single big-bang PR:

- **SP1 — HUD Design System Foundation** — almost entirely additive: theme tokens, fonts, an 11-component HUD library, and a `/design-system` showcase route. Existing pages untouched; the only edits to existing code are a route registration, a small branding-hook patch in `App.tsx`, theme bootstrap in `main.tsx`, and the Tailwind config extension. Lands first.
- **SP2 — App Shell + Page Migration** — depends on SP1 in `main`. Replaces `App.tsx`, restructures the route table to mirror Flutter's IA, splits the Settings monolith into seven sub-pages, re-skins the heavy existing components, and rewrites the simple ones. Deletes the old design.

After SP2 lands, every screen in the React admin console renders in the Flutter HUD style across three runtime-switchable themes (dark, OLED, light), responsive from mobile up to desktop.

## Design Decisions

| Decision                  | Choice                                                                                            | Rationale                                                                                          |
| ------------------------- | ------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Scope                     | Configuration surface only                                                                         | React app is the admin console; Flutter owns Live/Playback/Search/Screenshots                       |
| Information architecture  | Mirror Flutter's IA                                                                                | The two clients should feel like sibling apps, not unrelated tools                                  |
| Form-factor               | Desktop + phone responsive (Tailwind `lg` breakpoint at 1024px)                                    | User configures from both                                                                           |
| Themes                    | Dark + OLED + Light, runtime-switchable via `data-theme` attribute on `<html>`                     | Flutter ships dark + light; existing app already has dark + OLED — keep both, add light             |
| Theme implementation      | CSS custom properties consumed by Tailwind via `rgb(var(...) / <alpha-value>)`                     | Class-toggle theme switching, no rebuild, no flash                                                  |
| Existing components       | Mixed: re-skin heavy ones (canvas, recurrence, forms), rewrite simple ones (toast, dialogs)        | Preserve hard-won correctness in canvas/recurrence/form code; rewrite is cheap for pure-markup UI   |
| Migration strategy        | Big-bang per sub-project (1 PR per sub-project)                                                    | Page-by-page creates inconsistency; flag-gating doubles the surface area; long-lived branches rot   |
| Sequencing                | SP1 ships and merges first, then SP2                                                               | SP2 imports from `~/components/hud/*`                                                               |
| Landing page              | `/dashboard` (system health)                                                                       | Matches Flutter; "what's the system doing right now" is a more useful first impression than a list |
| Fonts                     | `@fontsource/jetbrains-mono` + `@fontsource/ibm-plex-sans` (self-hosted via npm)                   | No CDN dependency, no manual woff2 file management, both fonts already used by Flutter              |
| HUD primitives in SP1     | 11 components (button, toggle, input, textarea, select, slider, segmented, badge, brackets, card, kv-row) plus a barrel re-export | Skip RotaryKnob and CameraThumbnail (PTZ/streaming, not config-relevant)                            |
| Showcase route            | `/design-system`, admin-gated, lives in production                                                 | Doubles as a manual visual regression check                                                         |
| Tests                     | None added during the rewrite                                                                      | Current React app has zero tests; adding them blows the scope apart                                 |
| Backend changes           | None                                                                                               | Every page hits an existing endpoint; if Flutter needs something we don't expose, drop the screen   |
| `.nvmrc` for `ui/`        | Pin Node 20                                                                                        | Vite 5 needs Node 18+; Node 16 crashes on `crypto.getRandomValues`                                  |

## Sub-Project Decomposition

```
SP1 — HUD design system foundation
└── 1 PR: ui/src/theme/* + ui/src/components/hud/* + /design-system showcase route + fonts + .nvmrc
    Estimated: ~1600 LOC, all additive, no existing files modified

SP2 — Shell + page migration (depends on SP1 being merged)
└── 1 PR: new <AppLayout>, new IA mirroring Flutter, all pages migrated, drops old App.tsx shell
    Estimated: ~8000 LOC churn, net delta ~+1800 LOC
```

**Sequencing rule:** SP1 ships and merges to `main` first. SP2 cannot start in earnest until SP1 is in `main`, because SP2's PR will import HUD components from `~/components/hud/*`.

After SP2 ships, we revisit this brainstorming flow if any further work is needed (e.g., the deferred RotaryKnob, additional theme variants, or a follow-up to clean up anything that surfaced during the migration).

---

## SP1 — HUD Design System Foundation

### File layout

```
ui/src/
├── theme/                    NEW
│   ├── colors.ts             TS mirror of nvr_colors.dart (palette names + types)
│   ├── typography.ts         TS mirror of nvr_typography.dart (style names + class names)
│   ├── tokens.css            CSS custom-properties for dark / oled / light
│   ├── fonts.css             @font-face declarations + font weight imports
│   └── useTheme.ts           React hook + theme switcher logic
│
├── components/hud/           NEW
│   ├── HudButton.tsx
│   ├── HudToggle.tsx
│   ├── HudInput.tsx
│   ├── HudTextarea.tsx
│   ├── HudSelect.tsx
│   ├── AnalogSlider.tsx
│   ├── SegmentedControl.tsx
│   ├── StatusBadge.tsx
│   ├── CornerBrackets.tsx
│   ├── SectionCard.tsx
│   ├── KvRow.tsx
│   └── index.ts              barrel re-export
│
└── pages/
    └── DesignSystem.tsx      NEW — admin-gated showcase route
```

Plus targeted edits to existing files:

- `ui/tailwind.config.js` — extended with CSS-variable-backed colors and font families
- `ui/src/main.tsx` — apply persisted theme to `<html data-theme>` synchronously before React hydrates; import `theme/tokens.css` and `theme/fonts.css`
- `ui/src/App.tsx` — register the `/design-system` route, and patch the inline `useBranding` hook so the existing branding feature keeps working under the new accent variable (see "Branding compatibility" below)
- `ui/.nvmrc` — contains `20`
- `ui/package.json` — adds `@fontsource/jetbrains-mono` and `@fontsource/ibm-plex-sans`

**No existing pages, components, or hooks beyond the four files listed above are modified in SP1.** The HUD library exists alongside the old design until SP2 swaps everything over. SP1 can land safely on its own with minimal risk to the running app — the only user-visible changes are the new `/design-system` route and a new theme switcher embedded inside that route.

### Branding compatibility

The existing KAI-58 branding feature stores a `accent_color` hex string and writes it to a CSS variable named `--nvr-branding-accent` via the inline `useBranding` hook in `App.tsx`. The new SP1 token system uses `--nvr-accent` (as an `R G B` triplet, not a hex string), so the old plumbing would silently stop working when SP1 lands.

SP1 patches the existing `useBranding` hook in `App.tsx` to:

1. Convert the hex string from the API into an `R G B` triplet (e.g., `#f97316` → `249 115 22`)
2. Write the triplet to `--nvr-accent` via `document.documentElement.style.setProperty('--nvr-accent', '249 115 22')`
3. Continue to also write the original `--nvr-branding-accent` variable for any unmigrated code paths until SP2 cleans them up

This is the only behavioural change SP1 makes to existing code. Hex-to-RGB conversion is a four-line helper. The branding admin feature continues to work end-to-end after SP1 merges.

### Tailwind + CSS variables

```js
// tailwind.config.js — colors block
extend: {
  colors: {
    'bg-primary':   'rgb(var(--nvr-bg-primary)   / <alpha-value>)',
    'bg-secondary': 'rgb(var(--nvr-bg-secondary) / <alpha-value>)',
    'bg-tertiary':  'rgb(var(--nvr-bg-tertiary)  / <alpha-value>)',
    'bg-input':     'rgb(var(--nvr-bg-input)     / <alpha-value>)',
    accent:         'rgb(var(--nvr-accent)       / <alpha-value>)',
    'accent-hover': 'rgb(var(--nvr-accent-hover) / <alpha-value>)',
    'text-primary':   'rgb(var(--nvr-text-primary)   / <alpha-value>)',
    'text-secondary': 'rgb(var(--nvr-text-secondary) / <alpha-value>)',
    'text-muted':     'rgb(var(--nvr-text-muted)     / <alpha-value>)',
    success: 'rgb(var(--nvr-success) / <alpha-value>)',
    warning: 'rgb(var(--nvr-warning) / <alpha-value>)',
    danger:  'rgb(var(--nvr-danger)  / <alpha-value>)',
    border:  'rgb(var(--nvr-border)  / <alpha-value>)',
  },
  fontFamily: {
    mono: ['"JetBrains Mono"', 'ui-monospace', 'monospace'],
    sans: ['"IBM Plex Sans"', 'system-ui', 'sans-serif'],
  },
}
```

```css
/* ui/src/theme/tokens.css */
:root, [data-theme="dark"] {
  --nvr-bg-primary:   10  10  10;   /* #0a0a0a */
  --nvr-bg-secondary: 17  17  17;   /* #111111 */
  --nvr-bg-tertiary:  26  26  26;   /* #1a1a1a */
  --nvr-bg-input:     26  26  26;
  --nvr-accent:       249 115 22;   /* #f97316 */
  --nvr-accent-hover: 234 88  12;
  --nvr-text-primary:   229 229 229;
  --nvr-text-secondary: 115 115 115;
  --nvr-text-muted:     64  64  64;
  --nvr-success: 34  197 94;
  --nvr-warning: 234 179 8;
  --nvr-danger:  239 68  68;
  --nvr-border:  38  38  38;
}

[data-theme="oled"] {
  --nvr-bg-primary:   0 0 0;
  --nvr-bg-secondary: 8 8 8;
  --nvr-bg-tertiary:  16 16 16;
  --nvr-bg-input:     16 16 16;
  /* accent + status + text inherit from dark */
}

[data-theme="light"] {
  --nvr-bg-primary:   245 245 245;
  --nvr-bg-secondary: 255 255 255;
  --nvr-bg-tertiary:  229 229 229;
  --nvr-bg-input:     229 229 229;
  --nvr-accent:       234 88  12;
  --nvr-accent-hover: 194 65  12;
  --nvr-text-primary:   23 23 23;
  --nvr-text-secondary: 82 82 82;
  --nvr-text-muted:     163 163 163;
  --nvr-success: 22  163 74;
  --nvr-warning: 202 138 4;
  --nvr-danger:  220 38  38;
  --nvr-border:  212 212 212;
}
```

The OLED theme inherits accent + status + text colors from dark. OLED is purely a black-background tweak.

The KAI-58 branding hook (`useBranding`) keeps working — see "Branding compatibility" above for the precise change SP1 makes to keep custom-branded accent colors flowing into the new variable system.

### Typography helpers

Two self-hosted fonts via `@fontsource`:

```bash
npm i @fontsource/jetbrains-mono @fontsource/ibm-plex-sans
```

`fonts.css` imports the weights actually used (mono 400/500/700, sans 400/500/600).

Type ramp helpers exposed as Tailwind component classes via `@layer components` so the markup stays clean and the type ramp matches `clients/flutter/lib/theme/nvr_typography.dart` exactly:

| Class                  | Mirrors          | Spec                                                           |
| ---------------------- | ---------------- | -------------------------------------------------------------- |
| `.text-mono-label`     | `monoLabel`      | mono 9px, weight 500, tracking 1.5, `text-text-muted`           |
| `.text-mono-section`   | `monoSection`    | mono 10px, weight 700, tracking 2, `text-accent`                |
| `.text-mono-data`      | `monoData`       | mono 12px, weight 400, `text-text-primary`                      |
| `.text-mono-data-lg`   | `monoDataLarge`  | mono 16px, weight 500, `text-text-primary`                      |
| `.text-mono-timestamp` | `monoTimestamp`  | mono 12px, weight 400, `text-accent`                            |
| `.text-mono-status`    | `monoStatus`     | mono 9px, weight 500, tracking 1, `text-success`                |
| `.text-page-title`     | `pageTitle`      | sans 16px, weight 600, `text-text-primary`                      |
| `.text-camera-name`    | `cameraName`     | sans 13px, weight 500, `text-text-primary`                      |
| `.text-body`           | `body`           | sans 12px, line-height 1.5, `text-text-secondary`               |
| `.text-button`         | `button`         | sans 12px, weight 600, `text-text-primary`                      |
| `.text-alert`          | `alert`          | sans 12px, weight 400, `text-danger`                            |

### HUD components — file-by-file API

All exported from `components/hud/index.ts` so consumers do `import { HudButton, StatusBadge } from '~/components/hud'`.

```ts
// HudButton.tsx — mirrors clients/flutter/lib/widgets/hud/hud_button.dart
export type HudButtonVariant = 'primary' | 'secondary' | 'danger' | 'tactical'
export interface HudButtonProps {
  label: string
  onClick?: () => void
  variant?: HudButtonVariant       // default 'primary'
  icon?: ReactNode                  // optional leading icon (16px)
  loading?: boolean                 // shows spinner, disables click
  disabled?: boolean
  type?: 'button' | 'submit'
  fullWidth?: boolean
  'aria-label'?: string
}
```

`tactical` variant uses mono font + uppercase, others use sans + sentence case.

```ts
// HudToggle.tsx — mirrors hud_toggle.dart
export interface HudToggleProps {
  checked: boolean
  onChange: (checked: boolean) => void
  label?: string                    // optional inline label
  disabled?: boolean
}

// HudInput.tsx
export interface HudInputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'className'> {
  label?: string                    // mono-label rendered above
  error?: string                    // alert text below
  hint?: string                     // body-secondary text below
  monoData?: boolean                // render value in mono font (default true for IDs/URLs/numbers)
}

// HudTextarea.tsx — same shape as HudInput, multi-line
// HudSelect.tsx — same + options: { value: string; label: string }[]

// AnalogSlider.tsx — mirrors analog_slider.dart
export interface AnalogSliderProps {
  label: string
  value: number
  min: number
  max: number
  step?: number
  onChange: (value: number) => void
  valueFormatter?: (v: number) => string
  disabled?: boolean
}
```

Visual: bordered horizontal track, accent-colored fill, current value displayed in `mono-data-lg` to the right.

```ts
// SegmentedControl.tsx — mirrors segmented_control.dart
export interface SegmentedControlProps<T extends string> {
  options: { value: T; label: string; icon?: ReactNode }[]
  value: T
  onChange: (value: T) => void
}

// StatusBadge.tsx — mirrors status_badge.dart
export type StatusVariant = 'online' | 'offline' | 'degraded' | 'recording' | 'warning'
export interface StatusBadgeProps {
  variant: StatusVariant
  label?: string                    // defaults to variant name uppercased
  pulse?: boolean                   // animated pulse for live states
}

// CornerBrackets.tsx — decorative HUD frame
export interface CornerBracketsProps {
  children: ReactNode
  size?: 'sm' | 'md' | 'lg'         // bracket length
  color?: 'accent' | 'border' | 'success' | 'danger'
}

// SectionCard.tsx — bordered card with mono section header
export interface SectionCardProps {
  header: string                    // rendered as text-mono-section
  actions?: ReactNode               // optional right-aligned actions in the header
  children: ReactNode
}

// KvRow.tsx — key-value row used in detail/info panes
export interface KvRowProps {
  label: string                     // mono-label
  value: ReactNode                  // mono-data unless wrapped in JSX
  copyable?: boolean                // copy-to-clipboard button on hover
}
```

**Deferred (not built in SP1):**

- `RotaryKnob` — Flutter has it for PTZ. Web admin console doesn't do PTZ, so skip.
- `CameraThumbnail` — streaming widget, not config.

These can be added in a follow-up PR if a real consumer appears.

### Theme switching mechanics

```ts
// ui/src/theme/useTheme.ts
type Theme = 'dark' | 'oled' | 'light'

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(
    () => (localStorage.getItem('nvr-theme') as Theme) ?? 'dark'
  )

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('nvr-theme', theme)
  }, [theme])

  return { theme, setTheme: setThemeState }
}
```

`main.tsx` reads `localStorage` and sets `data-theme` synchronously *before* React hydrates so there is no flash of wrong theme on reload.

The theme switcher itself ships in SP2 as a `<SegmentedControl>` on `/settings/preferences` with the three options. SP1 just provides the hook + CSS plumbing — and exposes a temporary switcher inside `/design-system` for testing.

### `/design-system` showcase route

Admin-gated route (`user.role === 'admin'`) at `/design-system`. Renders one section per HUD primitive showing every variant in every state (default, hover, focus, disabled, loading). Includes a theme switcher at the top so we can flip dark/oled/light without leaving the page.

Used during SP1 dev to verify each component visually. Stays in the app afterwards as a reference and a visual regression check surface.

Not gated by env var — leaving it in production is fine, it has no destructive actions.

### SP1 PR shape

| File group                                                              | Estimated LOC  |
| ----------------------------------------------------------------------- | -------------- |
| `theme/` (colors, typography, tokens.css, fonts.css, useTheme)          | ~250           |
| `components/hud/*` (11 components + index barrel)                       | ~900           |
| `pages/DesignSystem.tsx`                                                 | ~400           |
| `App.tsx` route registration + branding-hook patch                       | ~25            |
| `main.tsx` (theme bootstrap, token + font CSS imports)                   | ~10            |
| `tailwind.config.js`                                                     | ~30            |
| `package.json` (font deps)                                               | ~3             |
| `.nvmrc`                                                                 | 1              |
| **Total**                                                                 | **~1620 LOC** |

Almost entirely additive. The only modifications to existing code are: route registration in `App.tsx`, the branding-hook patch in `App.tsx` (~10 lines), the theme bootstrap + CSS imports in `main.tsx` (~10 lines), and the Tailwind config extension. No existing pages, components, hooks, or business logic are touched.

---

## SP2 — App Shell + Page Migration

### Complete route table

```
Public:
  /login                          Login
  /setup                          Setup (first-run wizard)

Authenticated (wrapped in <AppLayout>):
  /dashboard                      Dashboard (system health) — landing page after login

  /cameras                        CameraListPage
  /cameras/new                    AddCameraPage
  /cameras/:id                    CameraDetailPage  (default tab: General)
  /cameras/:id/streams            CameraDetailPage  (Streams tab)
  /cameras/:id/recording-rules    RecordingRulesPage
  /cameras/:id/zones              ZoneEditorPage
  /cameras/:id/ai                 CameraDetailPage  (AI tab)
  /cameras/:id/onvif              CameraDetailPage  (ONVIF tab)

  /schedules                      SchedulesPage  (top-level recording rule templates)

  /settings                       redirect → /settings/preferences
  /settings/preferences           PreferencesPage  (theme, density, notifications, branding)
  /settings/users                 UsersPage         (admin only)
  /settings/audit                 AuditPage         (admin only)
  /settings/storage               StoragePage
  /settings/backup                BackupPage        (admin only)
  /settings/performance           PerformancePage
  /settings/system                SystemPage        (admin only — network, TLS, updates)

  /download                       DownloadClient

  /design-system                  DesignSystem      (admin only — built in SP1)

Redirects (preserve any old bookmarks):
  /                               → /dashboard
  /live, /recordings, /playback,
  /clips, /screenshots            → /download
  /users    (old top-level)       → /settings/users
  /audit    (old top-level)       → /settings/audit
  /health   (current /dashboard)  → /dashboard
```

### App shell — `<AppLayout>` architecture

```
AppLayout
├── DesktopNav        (lg+ : top bar)
│   ├── Brand link        ← uses branding hook (logo + product name)
│   ├── NavItem * N        ← Dashboard, Cameras, Schedules, Settings, Download
│   ├── NotificationBell
│   └── UserMenu
│
├── MobileSidebar     (<lg : slide-in from right, hamburger trigger)
│   ├── Same nav items as DesktopNav
│   ├── Theme switcher       ← quick access
│   └── Logout
│
├── StorageBanner    (warning strip if storage critical)
├── <Outlet />       (page content)
└── ToastContainer
```

**Mobile-responsive cutoff:** Tailwind's `lg` (1024px). Below that → hamburger + sidebar. Above → top nav.

Nav items rendered as `<NavItem>` which wraps react-router's `<NavLink>` with HUD active-state styling (accent-colored bottom border on desktop, accent fill on mobile).

### Pages — disposition table

| New page                                | Source                                 | Disposition                                    | Notes                                                                                                                          |
| --------------------------------------- | -------------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `Login.tsx`                             | existing                               | rewrite                                        | Centered card with `<CornerBrackets>`, branding logo, HUD form fields                                                          |
| `Setup.tsx`                             | existing                               | rewrite                                        | First-run wizard, 2-step. Use `<SegmentedControl>` for step indicator                                                          |
| `Dashboard.tsx`                         | existing                               | rewrite                                        | Stat tiles + `<SectionCard>` panels for CPU, memory, recordings pipeline, AI pipeline. Mirrors Flutter `dashboard_screen.dart` |
| `DownloadClient.tsx`                    | existing                               | rewrite                                        | Mostly static, platform cards                                                                                                  |
| `cameras/CameraListPage.tsx`            | extracted from `CameraManagement.tsx`  | rewrite shell, reuse hooks                     | Camera grid with `<StatusBadge>`, search/filter bar, "Add Camera" `<HudButton>`. Mirrors `camera_list_screen.dart`              |
| `cameras/CameraDetailPage.tsx`          | extracted from `CameraManagement.tsx`  | rewrite shell, embed re-skinned components     | Two-column layout. Tabbed `<SegmentedControl>` for Streams / Recording / AI / ONVIF / Zones. Embeds re-skinned components      |
| `cameras/AddCameraPage.tsx`             | extracted from `CameraManagement.tsx`  | rewrite                                        | Discovery → probe → confirm flow. Mirrors `add_camera_screen.dart` + `discovery_card.dart`                                     |
| `cameras/RecordingRulesPage.tsx`        | NEW route, wraps existing component    | re-skin `RecordingRules.tsx`                   | The 472-line component stays; wrap in a page with breadcrumb + `<SectionCard>` frame                                            |
| `cameras/ZoneEditorPage.tsx`            | NEW route, wraps existing component    | re-skin `DetectionZoneEditor.tsx`              | Page wrapper around the canvas component. Canvas math untouched                                                                |
| `schedules/SchedulesPage.tsx`           | NEW page                               | new                                            | Top-level template management. Mirrors `schedules_screen.dart`                                                                  |
| `settings/SettingsLayout.tsx`           | NEW                                    | new                                            | Two-column layout: left rail with sub-nav (`<NavItem>` per sub-page), right column = `<Outlet/>`. Mobile: sub-nav becomes a top `<SegmentedControl>` |
| `settings/PreferencesPage.tsx`          | extracted from `Settings.tsx`          | rewrite                                        | Theme switcher (`<SegmentedControl>` dark/oled/light), density, notifications, branding                                        |
| `settings/UsersPage.tsx`                | from `UserManagement.tsx`              | rewrite                                        | User CRUD table, role badges using `<StatusBadge>`                                                                             |
| `settings/AuditPage.tsx`                | from `AuditLog.tsx`                    | rewrite                                        | Filter bar + paginated table, mono font for action/resource columns                                                            |
| `settings/StoragePage.tsx`              | from `StorageManagement.tsx`           | rewrite                                        | Per-camera storage breakdown, retention controls, bulk cleanup                                                                 |
| `settings/BackupPage.tsx`               | extracted from `Settings.tsx`          | rewrite                                        | DB backup/restore with `<HudButton>` actions, restore via file input                                                           |
| `settings/PerformancePage.tsx`          | extracted from `Settings.tsx`          | rewrite                                        | Metrics + sparklines (reuse the metrics fetch logic, restyle the chart container)                                              |
| `settings/SystemPage.tsx`               | extracted from `Settings.tsx`          | rewrite                                        | Network config, TLS status, update check, all in `<SectionCard>`s                                                              |

### Existing components — disposition table

| Component                              | Action     | Reason                                          |
| -------------------------------------- | ---------- | ----------------------------------------------- |
| `DetectionZoneEditor.tsx` (406 LOC)    | re-skin    | Canvas coordinate math is hard-won correctness  |
| `RecordingRules.tsx` (472 LOC)         | re-skin    | Recurrence logic + day-of-week handling         |
| `CameraSettings.tsx` (159 LOC)         | re-skin    | Form validation logic                           |
| `AnalyticsConfig.tsx` (163 LOC)        | re-skin    | Rule serialization logic                        |
| `RelayControls.tsx` (116 LOC)          | re-skin    | ONVIF call sequencing                           |
| `SchedulePreview.tsx` (141 LOC)        | re-skin    | Calendar rendering math                         |
| `Toast.tsx` (127 LOC)                  | rewrite    | Pure markup, easy to redo with HUD primitives   |
| `ConfirmDialog.tsx` (79 LOC)           | rewrite    | Pure markup                                     |
| `NotificationBell.tsx` (141 LOC)       | rewrite    | Dropdown + count badge, simple                  |
| `StorageBanner.tsx` (69 LOC)           | rewrite    | One-line warning strip                          |
| `ErrorBoundary.tsx` (57 LOC)           | rewrite    | Minimal — only the fallback markup changes      |
| `KeyboardShortcutsHelp.tsx` (85 LOC)   | rewrite    | Modal with table — simple                       |

**"Re-skin" precise definition:** keep the file's exports, props, hooks usage, state machines, side effects, and any computation/math/algorithms unchanged. Replace only:

1. `className` strings (Tailwind utility soup → HUD tokens / `<HudButton>` / `<HudInput>` etc.)
2. Inline `<button>` / `<input>` / `<select>` / `<svg>` icon JSX → HUD components
3. Container `<div className="bg-... border-...">` → `<SectionCard>` / `<CornerBrackets>`

If a re-skin tempts a restructure of logic, stop and flag for a separate PR.

### Files deleted in SP2

- `ui/src/pages/Settings.tsx` (the 2000-line monolith) — split into 7 sub-pages
- `ui/src/pages/UserManagement.tsx` → `ui/src/pages/settings/UsersPage.tsx`
- `ui/src/pages/AuditLog.tsx` → `ui/src/pages/settings/AuditPage.tsx`
- `ui/src/pages/StorageManagement.tsx` → `ui/src/pages/settings/StoragePage.tsx`
- The current inline `Layout`/`NavItem`/`MobileNavItem` definitions in `ui/src/App.tsx`

### SP2 PR shape and migration ordering

The SP2 PR follows this commit order so the diff is reviewable:

1. Add new files — new layout, new HUD-using pages, new routes, theme switcher
2. Re-skin existing components — one component per commit so each is reviewable in isolation
3. Wire up the new `App.tsx` + `AppRoutes` — switch the router to the new IA
4. Delete old files — `Settings.tsx`, `UserManagement.tsx`, `AuditLog.tsx`, `StorageManagement.tsx`, inline layout
5. Verify — `npm run build` clean, manual click-through every route

| Group                                                          | Estimated LOC delta |
| -------------------------------------------------------------- | ------------------- |
| New pages (`cameras/*`, `schedules/*`, `settings/*`)           | +3500               |
| New layout (AppLayout, DesktopNav, MobileSidebar, UserMenu)    | +600                |
| Component re-skins (6 files, mostly Tailwind class swaps)      | ±400                |
| Component rewrites (6 files)                                   | ±500                |
| New `App.tsx` (much thinner)                                   | -300 net            |
| Deletions (old `Settings.tsx` monolith, old top-level pages)   | -2500               |
| **Total churn**                                                | **~8000 LOC**       |
| **Net delta**                                                  | **~+1800 LOC**      |

The PR description includes a checklist of every route + a screenshot of each in dark/oled/light, so review can happen visually.

---

## Non-Goals

These are explicitly out of scope to prevent the rewrite from blowing up:

- **No new backend endpoints.** Every page hits an API that already exists. If a Flutter screen depends on something the Go backend doesn't expose to the React app, we either skip the screen or make a note for a future PR.
- **No tests added.** The current React app has zero tests. Adding tests during a rewrite blows the scope apart. Leave for later.
- **No streaming, no playback, no live view, no clip search, no screenshots gallery.** Per the "config only" directive.
- **No i18n.** English-only, same as today.
- **No keyboard shortcuts redesign.** Existing shortcuts (`?`, etc.) carry over via the rewritten `KeyboardShortcutsHelp`.
- **No port of `RotaryKnob` or PTZ controls.** Web admin doesn't do PTZ.
- **No restructure of business logic in re-skinned components.** Class names and JSX shells only — algorithms, hooks usage, and side effects stay identical.
- **No port of Flutter screens that don't exist in the React app today.** The new IA mirrors Flutter only for surfaces both clients need (cameras, schedules, settings sub-panels). Live View / Playback / Search / Screenshots stay Flutter-only.

## Open Questions

These are flagged for the user to confirm before SP1 implementation begins. None block writing the implementation plan, but each could change a small detail.

1. **OLED token independence.** The spec inherits accent + status + text colors from dark when the theme is OLED. If you'd rather have fully independent OLED tokens (e.g., dimmer accent, different border color), say so and I'll split them in `tokens.css`.
2. **`@fontsource` vs. manual woff2.** Spec uses `@fontsource/jetbrains-mono` and `@fontsource/ibm-plex-sans` packages. Alternative is downloading WOFF2 files into `ui/public/fonts/` and writing manual `@font-face` declarations. Same end result, slightly more files in the repo, no npm dependency.
3. **`/settings/system` granularity.** Spec keeps Network + TLS + Updates on a single page. They could be split into `/settings/network`, `/settings/tls`, `/settings/updates` if you want each in its own route.
4. **`RecordingRulesPage` and `ZoneEditorPage` route placement.** Spec gives them their own top-level routes (`/cameras/:id/recording-rules`, `/cameras/:id/zones`). Alternative is keeping them as drawers / modals inside `CameraDetailPage`. Top-level routes match Flutter; drawers are slightly less navigation churn.
5. **`/dashboard` as landing page.** Spec lands on `/dashboard` to match Flutter. If you'd rather land on `/cameras` (which is what the current React app does), say so.

## Sequencing After This Spec

1. Spec committed to `feat/react-hud-rewrite-spec` worktree branch
2. User reviews spec doc, approves or requests changes
3. After approval, invoke the `superpowers:writing-plans` skill → produces SP1 implementation plan as `docs/superpowers/plans/2026-04-06-react-hud-foundation-plan.md`
4. User reviews SP1 plan
5. SP1 implementation begins on a separate worktree branch `feat/react-hud-foundation`
6. SP1 PR opened, reviewed, merged
7. After SP1 is in `main`, return to brainstorming/spec/plan flow for SP2 with SP1 as a known foundation
8. SP2 implementation begins on `feat/react-hud-page-migration`
9. SP2 PR opened, reviewed, merged
10. React admin console now matches Flutter HUD design across all configuration surfaces
