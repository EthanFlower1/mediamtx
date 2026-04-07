import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App.tsx'
import './index.css'
import './theme/tokens.css'
import './theme/fonts.css'

// Apply saved theme before first render to avoid flash
const savedTheme = localStorage.getItem('nvr-theme')
if (savedTheme === 'oled') {
  document.documentElement.classList.add('theme-oled')
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
