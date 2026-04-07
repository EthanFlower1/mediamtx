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
            ]
              .filter(Boolean)
              .join(' ')}
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
          <span id={`${id}-hint`} className="text-body-hud text-text-secondary">
            {hint}
          </span>
        ) : null}
      </div>
    )
  },
)
