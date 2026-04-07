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
          ]
            .filter(Boolean)
            .join(' ')}
          {...rest}
        />
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
