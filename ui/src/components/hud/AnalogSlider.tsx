// ui/src/components/hud/AnalogSlider.tsx
import { useCallback, useRef, useState, type PointerEvent as RPointerEvent } from 'react'

export interface AnalogSliderProps {
  label?: string
  value: number
  min?: number
  max?: number
  step?: number
  tickCount?: number
  disabled?: boolean
  onChange: (value: number) => void
  valueFormatter?: (value: number) => string
}

/**
 * Mirrors clients/flutter/lib/widgets/hud/analog_slider.dart.
 *
 * 24px-tall control: a 6px track with gradient fill, an 18px thumb that
 * grows to 20px while dragging with an accent glow, and a row of tick
 * marks below. Pointer events handle mouse + touch in one path; keyboard
 * arrows nudge the value when focused. role='slider' + aria-valuemin/max/now
 * for screen readers.
 */
export function AnalogSlider({
  label,
  value,
  min = 0,
  max = 1,
  step,
  tickCount = 11,
  disabled = false,
  onChange,
  valueFormatter,
}: AnalogSliderProps) {
  const trackRef = useRef<HTMLDivElement>(null)
  const [dragging, setDragging] = useState(false)

  const fraction = Math.max(0, Math.min(1, (value - min) / (max - min || 1)))

  const display =
    valueFormatter?.(value) ??
    (max === 1 && min === 0
      ? `${Math.round(value * 100)}%`
      : value.toFixed(0))

  const valueFromPointer = useCallback(
    (clientX: number): number => {
      const el = trackRef.current
      if (!el) return value
      const rect = el.getBoundingClientRect()
      const dx = Math.max(0, Math.min(rect.width, clientX - rect.left))
      const f = dx / rect.width
      let next = min + f * (max - min)
      if (step && step > 0) {
        next = Math.round(next / step) * step
      }
      return Math.max(min, Math.min(max, next))
    },
    [min, max, step, value],
  )

  const handlePointerDown = (e: RPointerEvent<HTMLDivElement>) => {
    if (disabled) return
    e.currentTarget.setPointerCapture(e.pointerId)
    setDragging(true)
    onChange(valueFromPointer(e.clientX))
  }

  const handlePointerMove = (e: RPointerEvent<HTMLDivElement>) => {
    if (!dragging || disabled) return
    onChange(valueFromPointer(e.clientX))
  }

  const handlePointerUp = (e: RPointerEvent<HTMLDivElement>) => {
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId)
    }
    setDragging(false)
  }

  return (
    <div className={['flex flex-col gap-1.5', disabled ? 'opacity-50' : ''].filter(Boolean).join(' ')}>
      {label && (
        <div className="flex items-center justify-between">
          <span className="text-mono-label">{label}</span>
          <span className="font-mono text-[9px] text-accent">{display}</span>
        </div>
      )}
      <div
        ref={trackRef}
        role="slider"
        aria-valuemin={min}
        aria-valuemax={max}
        aria-valuenow={value}
        aria-disabled={disabled || undefined}
        aria-label={label}
        tabIndex={disabled ? -1 : 0}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerUp}
        onKeyDown={(e) => {
          if (disabled) return
          const nudge = step ?? (max - min) / 100
          if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') {
            e.preventDefault()
            onChange(Math.max(min, value - nudge))
          } else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') {
            e.preventDefault()
            onChange(Math.min(max, value + nudge))
          }
        }}
        className={[
          'relative h-6 select-none',
          disabled ? 'cursor-not-allowed' : 'cursor-pointer',
          'focus:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 rounded',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        {/* Track */}
        <div className="absolute left-0 right-0 top-1/2 -translate-y-1/2 h-1.5 bg-bg-tertiary border border-border rounded" />
        {/* Fill */}
        <div
          className="absolute left-0 top-1/2 -translate-y-1/2 h-1.5 rounded bg-gradient-to-r from-accent to-accent/40"
          style={{ width: `${fraction * 100}%` }}
        />
        {/* Thumb */}
        <div
          aria-hidden="true"
          className={[
            'absolute top-1/2 -translate-y-1/2 -translate-x-1/2 rounded-full bg-bg-tertiary border-2 border-accent transition-all',
            dragging
              ? 'w-5 h-5 shadow-accent-glow-lg'
              : 'w-[18px] h-[18px] shadow-accent-glow-sm',
          ]
            .filter(Boolean)
            .join(' ')}
          style={{ left: `${fraction * 100}%` }}
        />
      </div>
      {/* Tick marks */}
      <div className="flex justify-between mt-0.5">
        {Array.from({ length: tickCount }).map((_, i) => (
          <div key={i} className="w-px h-1 bg-border" aria-hidden="true" />
        ))}
      </div>
    </div>
  )
}
