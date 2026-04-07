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
