// ui/src/pages/DesignSystem.tsx
import { useTheme } from '../theme/useTheme'
import type { ThemeName } from '../theme/colors'

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
        </main>
      </div>
    </div>
  )
}
