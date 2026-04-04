import { useEffect } from 'react'

interface ShortcutEntry {
  keys: string
  description: string
}

const GLOBAL_SHORTCUTS: ShortcutEntry[] = [
  { keys: '?', description: 'Show this help overlay' },
  { keys: 'Esc', description: 'Close modal / exit clip mode' },
]


function ShortcutGroup({ title, shortcuts }: { title: string; shortcuts: ShortcutEntry[] }) {
  return (
    <div className="mb-5 last:mb-0">
      <h3 className="text-xs font-semibold text-nvr-text-muted uppercase tracking-wider mb-2">
        {title}
      </h3>
      <div className="space-y-1.5">
        {shortcuts.map((s) => (
          <div key={s.keys} className="flex items-center justify-between gap-4">
            <span className="text-sm text-nvr-text-secondary">{s.description}</span>
            <kbd className="shrink-0 inline-flex items-center px-2 py-0.5 rounded bg-nvr-bg-primary border border-nvr-border text-xs font-mono text-nvr-text-primary">
              {s.keys}
            </kbd>
          </div>
        ))}
      </div>
    </div>
  )
}

export default function KeyboardShortcutsHelp({ onClose }: { onClose: () => void }) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape' || e.key === '?') {
        e.preventDefault()
        onClose()
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
      <div
        className="relative z-10 bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl w-full max-w-md mx-4 overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-nvr-border">
          <h2 className="text-lg font-semibold text-nvr-text-primary">Keyboard Shortcuts</h2>
          <button
            onClick={onClose}
            className="text-nvr-text-muted hover:text-nvr-text-primary transition-colors p-1 rounded-lg hover:bg-nvr-bg-tertiary focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            aria-label="Close"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Body */}
        <div className="px-5 py-4 max-h-[60vh] overflow-y-auto">
          <ShortcutGroup title="Global" shortcuts={GLOBAL_SHORTCUTS} />
        </div>

        {/* Footer */}
        <div className="px-5 py-3 border-t border-nvr-border bg-nvr-bg-tertiary/30">
          <p className="text-xs text-nvr-text-muted text-center">
            Press <kbd className="px-1.5 py-0.5 rounded bg-nvr-bg-primary border border-nvr-border text-[10px] font-mono">?</kbd> or <kbd className="px-1.5 py-0.5 rounded bg-nvr-bg-primary border border-nvr-border text-[10px] font-mono">Esc</kbd> to close
          </p>
        </div>
      </div>
    </div>
  )
}
