import { useState, useEffect, useCallback } from 'react'

export interface ToastMessage {
  id: string
  type: 'info' | 'success' | 'warning' | 'error'
  title: string
  message: string
  timestamp: Date
}

const MAX_VISIBLE = 5
const AUTO_DISMISS_MS = 5000

const typeStyles: Record<ToastMessage['type'], { bg: string; border: string; icon: string }> = {
  info: {
    bg: 'bg-blue-900/80',
    border: 'border-blue-500',
    icon: '\u2139\uFE0F',
  },
  success: {
    bg: 'bg-green-900/80',
    border: 'border-green-500',
    icon: '\u2705',
  },
  warning: {
    bg: 'bg-amber-900/80',
    border: 'border-amber-500',
    icon: '\u26A0\uFE0F',
  },
  error: {
    bg: 'bg-red-900/80',
    border: 'border-red-500',
    icon: '\u274C',
  },
}

function ToastItem({ toast, onDismiss }: { toast: ToastMessage; onDismiss: (id: string) => void }) {
  const [visible, setVisible] = useState(false)
  const style = typeStyles[toast.type]

  useEffect(() => {
    // Trigger slide-in animation after mount.
    const raf = requestAnimationFrame(() => setVisible(true))
    return () => cancelAnimationFrame(raf)
  }, [])

  useEffect(() => {
    const timer = setTimeout(() => onDismiss(toast.id), AUTO_DISMISS_MS)
    return () => clearTimeout(timer)
  }, [toast.id, onDismiss])

  return (
    <div
      className={`
        ${style.bg} ${style.border} border-l-4 rounded-lg px-4 py-3 shadow-lg backdrop-blur-sm
        transition-all duration-300 ease-out max-w-sm w-full
        ${visible ? 'translate-x-0 opacity-100' : 'translate-x-full opacity-0'}
      `}
    >
      <div className="flex items-start gap-2">
        <span className="text-base mt-0.5 flex-shrink-0" role="img" aria-label={toast.type}>
          {style.icon}
        </span>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-semibold text-nvr-text-primary">{toast.title}</p>
          <p className="text-xs text-nvr-text-secondary mt-0.5 truncate">{toast.message}</p>
        </div>
        <button
          onClick={() => onDismiss(toast.id)}
          className="text-nvr-text-muted hover:text-nvr-text-primary transition-colors text-lg leading-none flex-shrink-0"
          aria-label="Dismiss"
        >
          &times;
        </button>
      </div>
    </div>
  )
}

let addToastExternal: ((toast: ToastMessage) => void) | null = null

export function pushToast(toast: ToastMessage) {
  if (addToastExternal) {
    addToastExternal(toast)
  }
}

export default function ToastContainer() {
  const [toasts, setToasts] = useState<ToastMessage[]>([])

  const addToast = useCallback((toast: ToastMessage) => {
    setToasts(prev => {
      const next = [...prev, toast]
      // Keep only the most recent MAX_VISIBLE toasts.
      if (next.length > MAX_VISIBLE) {
        return next.slice(next.length - MAX_VISIBLE)
      }
      return next
    })
  }, [])

  const dismissToast = useCallback((id: string) => {
    setToasts(prev => prev.filter(t => t.id !== id))
  }, [])

  // Expose addToast globally for the SSE hook.
  useEffect(() => {
    addToastExternal = addToast
    return () => { addToastExternal = null }
  }, [addToast])

  return (
    <div className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 pointer-events-none">
      {toasts.map(toast => (
        <div key={toast.id} className="pointer-events-auto">
          <ToastItem toast={toast} onDismiss={dismissToast} />
        </div>
      ))}
    </div>
  )
}
