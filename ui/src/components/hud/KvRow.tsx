// ui/src/components/hud/KvRow.tsx
import { useState, type ReactNode } from 'react'

export interface KvRowProps {
  label: string
  /** String values are rendered in mono; ReactNode values render as-is. */
  value: ReactNode
  /** Show a copy-to-clipboard button when the row is hovered. */
  copyable?: boolean
}

/**
 * Key-value row used in detail and info panels. Label is mono-label
 * (uppercase, tracked); value is mono-data (12px monospace) for strings,
 * or whatever JSX you pass in.
 *
 * The copyable button only appears for string values, since copying a
 * ReactNode doesn't have a sensible meaning.
 */
export function KvRow({ label, value, copyable = false }: KvRowProps) {
  const [copied, setCopied] = useState(false)
  const isString = typeof value === 'string'

  const handleCopy = async () => {
    if (!isString) return
    try {
      await navigator.clipboard.writeText(value as string)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // clipboard API may be blocked — silently ignore
    }
  }

  return (
    <div className="group flex items-baseline gap-3">
      <div className="text-mono-label w-28 shrink-0">{label}</div>
      <div className="flex-1 min-w-0 flex items-center gap-2">
        <div className={isString ? 'text-mono-data truncate' : ''}>{value}</div>
        {copyable && isString && (
          <button
            type="button"
            onClick={handleCopy}
            aria-label={`Copy ${label}`}
            className="opacity-0 group-hover:opacity-100 focus:opacity-100 text-text-muted hover:text-accent transition-opacity"
          >
            {copied ? (
              <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
                <path d="M3 8l3 3 7-7" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            ) : (
              <svg className="w-3.5 h-3.5" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true">
                <rect x="5" y="5" width="9" height="9" rx="1" />
                <path d="M11 5V3a1 1 0 00-1-1H3a1 1 0 00-1 1v7a1 1 0 001 1h2" />
              </svg>
            )}
          </button>
        )}
      </div>
    </div>
  )
}
