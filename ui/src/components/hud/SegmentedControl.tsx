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
      ]
        .filter(Boolean)
        .join(' ')}
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
              ]
                .filter(Boolean)
                .join(' ')}
            >
              {opt.icon && (
                <span aria-hidden="true" className="inline-flex items-center justify-center w-3 h-3">
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
