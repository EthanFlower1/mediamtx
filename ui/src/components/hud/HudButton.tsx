// ui/src/components/hud/HudButton.tsx
import { type ButtonHTMLAttributes, type ReactNode } from 'react'

export type HudButtonVariant = 'primary' | 'secondary' | 'danger' | 'tactical'

export interface HudButtonProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'> {
  label: string
  variant?: HudButtonVariant
  icon?: ReactNode
  loading?: boolean
  fullWidth?: boolean
}

const variantClasses: Record<HudButtonVariant, string> = {
  primary:
    'bg-accent text-bg-primary border border-transparent hover:bg-accent-hover',
  secondary:
    'bg-bg-tertiary text-text-primary border border-border hover:bg-bg-input',
  danger:
    'bg-danger/[0.13] text-danger border border-danger/[0.27] hover:bg-danger/[0.2]',
  tactical:
    'bg-bg-tertiary text-accent border border-accent/[0.27] hover:bg-accent/[0.13]',
}

const labelTextClass: Record<HudButtonVariant, string> = {
  primary: 'text-button-hud',
  secondary: 'text-button-hud',
  danger: 'text-button-hud',
  // tactical uses mono font + uppercase per the Flutter widget
  tactical: 'font-mono text-[10px] font-medium tracking-[0.1em] uppercase',
}

export function HudButton({
  label,
  variant = 'primary',
  icon,
  loading = false,
  disabled,
  fullWidth = false,
  type = 'button',
  ...rest
}: HudButtonProps) {
  const isDisabled = disabled || loading

  return (
    <button
      type={type}
      disabled={isDisabled}
      className={[
        'inline-flex items-center justify-center gap-1.5 px-4 py-2 rounded-[4px]',
        'transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50',
        isDisabled ? 'opacity-50 cursor-not-allowed' : 'cursor-pointer',
        fullWidth ? 'w-full' : '',
        variantClasses[variant],
        labelTextClass[variant],
      ].join(' ')}
      {...rest}
    >
      {loading ? (
        <Spinner />
      ) : icon ? (
        <span className="shrink-0 inline-flex items-center justify-center w-3.5 h-3.5">
          {icon}
        </span>
      ) : null}
      <span>{label}</span>
    </button>
  )
}

function Spinner() {
  return (
    <svg
      className="w-3.5 h-3.5 animate-spin"
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
    >
      <circle
        className="opacity-25"
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeWidth="4"
      />
      <path
        className="opacity-75"
        fill="currentColor"
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
      />
    </svg>
  )
}
