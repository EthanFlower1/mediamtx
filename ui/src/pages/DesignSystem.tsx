// ui/src/pages/DesignSystem.tsx
import { useState } from 'react'
import { useTheme } from '../theme/useTheme'
import type { ThemeName } from '../theme/colors'
import { HudButton } from '../components/hud/HudButton'
import { HudToggle } from '../components/hud/HudToggle'
import { HudInput } from '../components/hud/HudInput'
import { HudTextarea } from '../components/hud/HudTextarea'
import { HudSelect } from '../components/hud/HudSelect'
import { AnalogSlider } from '../components/hud/AnalogSlider'
import { SegmentedControl } from '../components/hud/SegmentedControl'
import { StatusBadge } from '../components/hud/StatusBadge'
import { CornerBrackets } from '../components/hud/CornerBrackets'
import { SectionCard } from '../components/hud/SectionCard'

const themes: { value: ThemeName; label: string }[] = [
  { value: 'dark', label: 'DARK' },
  { value: 'oled', label: 'OLED' },
  { value: 'light', label: 'LIGHT' },
]

function ShowcaseSection({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return <SectionCard header={title}>{children}</SectionCard>
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

          <ShowcaseSection title="ANALOG SLIDER">
            <SliderShowcase />
          </ShowcaseSection>

          <ShowcaseSection title="SEGMENTED CONTROL">
            <SegmentedShowcase />
          </ShowcaseSection>

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

          <ShowcaseSection title="SECTION CARD">
            <div className="space-y-4">
              <SectionCard header="WITH ACTIONS" actions={<HudButton label="Refresh" variant="secondary" />}>
                <p className="text-body-hud text-text-secondary">A card with a header action button on the right.</p>
              </SectionCard>
              <SectionCard header="FLUSH (NO BODY PADDING)" flush>
                <div className="px-4 py-3 border-b border-border text-mono-data">row 1</div>
                <div className="px-4 py-3 border-b border-border text-mono-data">row 2</div>
                <div className="px-4 py-3 text-mono-data">row 3</div>
              </SectionCard>
            </div>
          </ShowcaseSection>
        </main>
      </div>
    </div>
  )
}

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
        ariaLabel="Theme picker"
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
