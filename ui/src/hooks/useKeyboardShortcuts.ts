import { useEffect, useCallback } from 'react'

interface ShortcutConfig {
  key: string
  ctrl?: boolean
  shift?: boolean
  alt?: boolean
  handler: () => void
  description: string
  /** Only fire when no input/textarea/select is focused */
  ignoreWhenEditing?: boolean
}

function isEditableElement(el: Element | null): boolean {
  if (!el) return false
  const tag = el.tagName.toLowerCase()
  if (tag === 'input' || tag === 'textarea' || tag === 'select') return true
  if ((el as HTMLElement).isContentEditable) return true
  return false
}

export function useKeyboardShortcuts(shortcuts: ShortcutConfig[]) {
  const handler = useCallback(
    (e: KeyboardEvent) => {
      for (const s of shortcuts) {
        const keyMatch = e.key.toLowerCase() === s.key.toLowerCase()
        const ctrlMatch = s.ctrl ? e.ctrlKey || e.metaKey : !e.ctrlKey && !e.metaKey
        const shiftMatch = s.shift ? e.shiftKey : !e.shiftKey
        const altMatch = s.alt ? e.altKey : !e.altKey

        if (keyMatch && ctrlMatch && shiftMatch && altMatch) {
          if (s.ignoreWhenEditing !== false && isEditableElement(document.activeElement)) {
            continue
          }
          e.preventDefault()
          s.handler()
          return
        }
      }
    },
    [shortcuts],
  )

  useEffect(() => {
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [handler])
}

export type { ShortcutConfig }
