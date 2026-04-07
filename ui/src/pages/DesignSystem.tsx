// ui/src/pages/DesignSystem.tsx
import { useState } from 'react'
import { useTheme } from '../theme/useTheme'
import type { ThemeName } from '../theme/colors'
import { HudButton } from '../components/hud/HudButton'
import { HudToggle } from '../components/hud/HudToggle'
import { HudInput } from '../components/hud/HudInput'

const themes: { value: ThemeName; label: string }[] = [
  { value: 'dark', label: 'DARK' },
  { value: 'oled', label: 'OLED' },
  { value: 'light', label: 'LIGHT' },
]

/**
 * Reusable wrapper for each showcase section. The HUD components themselves
 * provide a SectionCard primitive but it doesn't exist yet at this point in
 * SP1; this local wrapper is intentional and gets replaced by SectionCard
 * once chunk 11 lands.
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

/**
 * Showcase route at /design-system. Renders one section per HUD primitive.
 * Used during SP1 dev to verify each component visually and stays in
 * production as a reference.
 *
 * Auth: requires authentication via <ProtectedRoute>, but is not gated
 * by role at the route level. The nav-bar entry is hidden from non-admin
 * users in SP2's nav rewrite; the showcase has no destructive actions so
 * route-level role restriction is unnecessary.
 *
 * The data-theme attribute is applied to a local container div, NOT to
 * <html>, so this route's theme switcher doesn't fight the legacy
 * html.theme-oled toggle.
 */
export default function DesignSystem() {
  // Instance-local state in SP1; see useTheme.ts for the SP2 globalization note.
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
                  aria-pressed={theme === t.value}
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
              <div><span className="text-mono-control">.TEXT-MONO-CONTROL — 9PX MUTED</span></div>
              <div><span className="text-page-title">.text-page-title — 16px sans semibold</span></div>
              <div><span className="text-camera-name">.text-camera-name — 13px sans medium</span></div>
              <div><span className="text-body-hud">.text-body-hud — 12px sans 1.5 line-height secondary</span></div>
              <div><span className="text-button-hud">.text-button-hud — 12px sans semibold</span></div>
              <div><span className="text-alert-hud">.text-alert-hud — 12px sans danger</span></div>
            </div>
          </ShowcaseSection>

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

          <ShowcaseSection title="HUD TOGGLE">
            <ToggleShowcase />
          </ShowcaseSection>

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
        </main>
      </div>
    </div>
  )
}

function ToggleShowcase() {
  const [a, setA] = useState(true)
  const [b, setB] = useState(false)
  return (
    <div className="grid grid-cols-2 sm:grid-cols-5 gap-6">
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
      <div>
        <HudToggle
          checked={false}
          onChange={() => {}}
          ariaLabel="Mute audio"
          showStateLabel={false}
        />
      </div>
    </div>
  )
}
