// ui/src/theme/colors.ts

/**
 * HUD color palettes — TypeScript mirror of clients/flutter/lib/theme/nvr_colors.dart.
 *
 * These tables are the source of truth for the values copied into tokens.css.
 * The runtime CSS variable system in tokens.css is what components actually
 * consume; this file exists so TS code can reference palette values directly
 * (e.g., for inline canvas drawing where Tailwind classes don't work).
 */

export type ThemeName = 'dark' | 'oled' | 'light'

export interface HudPalette {
  bgPrimary: string
  bgSecondary: string
  bgTertiary: string
  bgInput: string
  accent: string
  accentHover: string
  textPrimary: string
  textSecondary: string
  textMuted: string
  success: string
  warning: string
  danger: string
  border: string
}

/** Mirrors NvrColors.dark in clients/flutter/lib/theme/nvr_colors.dart */
export const dark: HudPalette = {
  bgPrimary: '#0a0a0a',
  bgSecondary: '#111111',
  bgTertiary: '#1a1a1a',
  bgInput: '#1a1a1a',
  accent: '#f97316',
  accentHover: '#ea580c',
  textPrimary: '#e5e5e5',
  textSecondary: '#737373',
  textMuted: '#404040',
  success: '#22c55e',
  warning: '#eab308',
  danger: '#ef4444',
  border: '#262626',
}

/** Pure-black variant of dark — backgrounds only, everything else inherits. */
export const oled: HudPalette = {
  ...dark,
  bgPrimary: '#000000',
  bgSecondary: '#080808',
  bgTertiary: '#101010',
  bgInput: '#101010',
}

/** Mirrors NvrColors.light in clients/flutter/lib/theme/nvr_colors.dart */
export const light: HudPalette = {
  bgPrimary: '#f5f5f5',
  bgSecondary: '#ffffff',
  bgTertiary: '#e5e5e5',
  bgInput: '#e5e5e5',
  accent: '#ea580c',
  accentHover: '#c2410c',
  textPrimary: '#171717',
  textSecondary: '#525252',
  textMuted: '#a3a3a3',
  success: '#16a34a',
  warning: '#ca8a04',
  danger: '#dc2626',
  border: '#d4d4d4',
}

export const palettes: Record<ThemeName, HudPalette> = { dark, oled, light }
