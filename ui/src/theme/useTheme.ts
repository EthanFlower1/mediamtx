// ui/src/theme/useTheme.ts
import { useState, useCallback } from 'react'
import type { ThemeName } from './colors'

const STORAGE_KEY = 'nvr-hud-theme'

/**
 * Local React state for the HUD theme.
 *
 * IMPORTANT (SP1): this hook does NOT write to <html data-theme="..."> in
 * SP1. Doing so would conflict with the existing legacy `html.theme-oled`
 * toggle. SP1's only consumer is the /design-system showcase route, which
 * applies `data-theme` to a local container div instead.
 *
 * SP2 will replace this hook with a globalized version that writes to
 * <html> as part of the production theme switcher in
 * /settings/preferences.
 */
export function useTheme(): {
  theme: ThemeName
  setTheme: (theme: ThemeName) => void
} {
  const [theme, setThemeState] = useState<ThemeName>(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY)
      if (stored === 'dark' || stored === 'oled' || stored === 'light') {
        return stored
      }
    } catch {
      // localStorage may throw in private mode — fall through to default
    }
    return 'dark'
  })

  const setTheme = useCallback((next: ThemeName) => {
    setThemeState(next)
    try {
      localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // localStorage may throw in private mode — silently ignore
    }
  }, [])

  return { theme, setTheme }
}
