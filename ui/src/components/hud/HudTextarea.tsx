// ui/src/components/hud/HudTextarea.tsx
import { forwardRef, useId, type TextareaHTMLAttributes } from 'react'

export interface HudTextareaProps
  extends Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'className'> {
  label?: string
  error?: string
  hint?: string
  monoData?: boolean
}

/**
 * Multi-line variant of HudInput. Same prop shape, same visual styling.
 */
export const HudTextarea = forwardRef<HTMLTextAreaElement, HudTextareaProps>(
  function HudTextarea(
    { label, error, hint, monoData = false, id: providedId, rows = 4, ...rest },
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
        <textarea
          id={id}
          ref={ref}
          rows={rows}
          aria-invalid={error ? 'true' : undefined}
          aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined}
          className={[
            'bg-bg-input border rounded-[4px] px-3 py-2 text-text-primary resize-y',
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
