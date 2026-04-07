/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        // ── Legacy palette (consumed by existing pages — DO NOT REMOVE in SP1) ──
        nvr: {
          bg: {
            primary: '#0f1117',
            secondary: '#1a1d27',
            tertiary: '#242836',
            input: '#12141c',
          },
          accent: {
            DEFAULT: '#3b82f6',
            hover: '#2563eb',
          },
          danger: {
            DEFAULT: '#ef4444',
            hover: '#dc2626',
          },
          success: '#22c55e',
          warning: '#f59e0b',
          text: {
            primary: '#e5e7eb',
            secondary: '#9ca3af',
            muted: '#7c8494',
          },
          border: '#2d3140',
        },
        // ── HUD palette (new in SP1, consumed by ui/src/components/hud/*) ──
        // Backed by --hud-* CSS variables defined in src/theme/tokens.css.
        // Use these in any new code:
        //   bg-bg-primary, bg-bg-secondary, bg-bg-tertiary, bg-bg-input
        //   text-text-primary, text-text-secondary, text-text-muted
        //   bg-accent, text-accent, border-accent
        //   text-success, text-warning, text-danger
        //   border-border
        'bg-primary': 'rgb(var(--hud-bg-primary) / <alpha-value>)',
        'bg-secondary': 'rgb(var(--hud-bg-secondary) / <alpha-value>)',
        'bg-tertiary': 'rgb(var(--hud-bg-tertiary) / <alpha-value>)',
        'bg-input': 'rgb(var(--hud-bg-input) / <alpha-value>)',
        accent: {
          DEFAULT: 'rgb(var(--hud-accent) / <alpha-value>)',
          hover: 'rgb(var(--hud-accent-hover) / <alpha-value>)',
        },
        'text-primary': 'rgb(var(--hud-text-primary) / <alpha-value>)',
        'text-secondary': 'rgb(var(--hud-text-secondary) / <alpha-value>)',
        'text-muted': 'rgb(var(--hud-text-muted) / <alpha-value>)',
        success: 'rgb(var(--hud-success) / <alpha-value>)',
        warning: 'rgb(var(--hud-warning) / <alpha-value>)',
        danger: 'rgb(var(--hud-danger) / <alpha-value>)',
        border: 'rgb(var(--hud-border) / <alpha-value>)',
      },
      fontFamily: {
        sans: ['"IBM Plex Sans"', 'Inter', 'system-ui', 'Avenir', 'Helvetica', 'Arial', 'sans-serif'],
        mono: ['"JetBrains Mono"', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'],
      },
      keyframes: {
        'slide-in': {
          from: { transform: 'translateX(100%)' },
          to: { transform: 'translateX(0)' },
        },
        'fade-in': {
          from: { opacity: '0' },
          to: { opacity: '1' },
        },
        'scale-in': {
          from: { opacity: '0', transform: 'scale(0.95)' },
          to: { opacity: '1', transform: 'scale(1)' },
        },
        'pulse-dot': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.4' },
        },
      },
      animation: {
        'slide-in': 'slide-in 200ms ease-out',
        'fade-in': 'fade-in 200ms ease-out',
        'scale-in': 'scale-in 200ms ease-out',
        'pulse-dot': 'pulse-dot 1.5s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}
