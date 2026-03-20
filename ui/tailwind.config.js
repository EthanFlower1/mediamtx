/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
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
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', 'Avenir', 'Helvetica', 'Arial', 'sans-serif'],
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
      },
      animation: {
        'slide-in': 'slide-in 200ms ease-out',
        'fade-in': 'fade-in 200ms ease-out',
        'scale-in': 'scale-in 200ms ease-out',
      },
    },
  },
  plugins: [],
}
