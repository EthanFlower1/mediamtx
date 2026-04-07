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
  color: string
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
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {dot && (
        <span
          className={[
            'inline-block w-1.5 h-1.5 rounded-full shadow-[0_0_6px_currentColor]',
            c.dot,
            c.text,
            pulse ? 'animate-pulse-dot' : '',
          ]
            .filter(Boolean)
            .join(' ')}
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
