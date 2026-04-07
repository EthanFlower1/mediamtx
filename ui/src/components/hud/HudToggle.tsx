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
          checked
            ? 'border-accent shadow-[0_0_8px_rgba(249,115,22,0.2)]'
            : 'border-border',
          disabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        <span
          aria-hidden="true"
          className={[
            'absolute top-1/2 -translate-y-1/2 w-3.5 h-3.5 rounded-full transition-all duration-150',
            checked
              ? 'right-0.5 bg-accent shadow-[0_0_6px_rgba(249,115,22,0.4)]'
              : 'left-0.5 bg-text-muted',
          ]
            .filter(Boolean)
            .join(' ')}
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
