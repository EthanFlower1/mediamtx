import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { BrowserRouter, Routes, Route, Navigate, Link, useLocation, useNavigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/context'
import Login from './pages/Login'
import Setup from './pages/Setup'
import CameraManagement from './pages/CameraManagement'
import Settings from './pages/Settings'
import Dashboard from './pages/Dashboard'
import UserManagement from './pages/UserManagement'
import AuditLog from './pages/AuditLog'
import DownloadClient from './pages/DownloadClient'
import ToastContainer from './components/Toast'
import NotificationBell from './components/NotificationBell'
import ErrorBoundary from './components/ErrorBoundary'
import StorageBanner from './components/StorageBanner'
import KeyboardShortcutsHelp from './components/KeyboardShortcutsHelp'
import SubResellerHierarchy from './pages/integrator/SubResellerHierarchy'
import { useNotifications } from './hooks/useNotifications'
import { useKeyboardShortcuts } from './hooks/useKeyboardShortcuts'
import { apiFetch } from './api/client'

/* ------------------------------------------------------------------ */
/*  Storage warning hook (lightweight poll for nav indicator)          */
/* ------------------------------------------------------------------ */
function useStorageWarning(isAuthenticated: boolean) {
  const [warning, setWarning] = useState(false)

  const check = useCallback(() => {
    apiFetch('/system/storage')
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setWarning(data.warning || data.critical)
        }
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    if (!isAuthenticated) return
    check()
    const id = setInterval(check, 30000)
    return () => clearInterval(id)
  }, [isAuthenticated, check])

  return warning
}

/* ------------------------------------------------------------------ */
/*  Protected route guard                                              */
/* ------------------------------------------------------------------ */
function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, setupRequired } = useAuth()

  if (isLoading) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center bg-nvr-bg-primary gap-4">
        <svg
          className="w-8 h-8 text-nvr-accent animate-spin"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
          />
        </svg>
        <span className="text-lg font-semibold text-white tracking-wide">MediaMTX NVR</span>
        <span className="text-sm text-nvr-text-muted">Loading...</span>
      </div>
    )
  }

  if (setupRequired) return <Navigate to="/setup" replace />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

/* ------------------------------------------------------------------ */
/*  Nav link (desktop)                                                 */
/* ------------------------------------------------------------------ */
interface NavLinkProps {
  to: string
  icon: React.ReactNode
  label: string
  badge?: boolean
}

function NavItem({ to, icon, label, badge }: NavLinkProps) {
  const location = useLocation()
  const isActive = location.pathname === to

  return (
    <Link
      to={to}
      className={`relative flex items-center gap-2 px-3 py-2.5 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded ${
        isActive
          ? 'text-white border-b-2 border-nvr-accent'
          : 'text-nvr-text-secondary hover:text-nvr-text-primary border-b-2 border-transparent'
      }`}
    >
      {icon}
      {label}
      {badge && (
        <span className="absolute top-1.5 right-0.5 w-2 h-2 rounded-full bg-nvr-warning" />
      )}
    </Link>
  )
}

/* ------------------------------------------------------------------ */
/*  Mobile sidebar link                                                */
/* ------------------------------------------------------------------ */
function MobileNavItem({
  to,
  icon,
  label,
  badge,
  onClick,
}: NavLinkProps & { onClick: () => void }) {
  const location = useLocation()
  const isActive = location.pathname === to

  return (
    <Link
      to={to}
      onClick={onClick}
      className={`flex items-center gap-3 px-4 py-3 text-sm font-medium rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
        isActive
          ? 'text-white bg-nvr-bg-tertiary'
          : 'text-nvr-text-secondary hover:text-nvr-text-primary hover:bg-nvr-bg-tertiary/50'
      }`}
    >
      {icon}
      <span className="flex-1">{label}</span>
      {badge && <span className="w-2 h-2 rounded-full bg-nvr-warning" />}
    </Link>
  )
}

/* ------------------------------------------------------------------ */
/*  User avatar / dropdown                                             */
/* ------------------------------------------------------------------ */
function UserMenu() {
  const { user, logout } = useAuth()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    if (open) document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  const initials = (user?.username ?? '?').slice(0, 2).toUpperCase()

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(!open)}
        className="w-8 h-8 rounded-full bg-nvr-accent/20 text-nvr-accent text-xs font-bold flex items-center justify-center hover:bg-nvr-accent/30 transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        aria-label="User menu"
      >
        {initials}
      </button>
      {open && (
        <div className="absolute right-0 mt-2 w-48 bg-nvr-bg-secondary border border-nvr-border rounded-lg shadow-xl overflow-hidden z-50">
          <div className="px-4 py-3 border-b border-nvr-border">
            <p className="text-sm font-medium text-nvr-text-primary">{user?.username}</p>
            <p className="text-xs text-nvr-text-muted capitalize">{user?.role}</p>
          </div>
          <button
            onClick={() => {
              setOpen(false)
              navigate('/settings')
            }}
            className="w-full text-left px-4 py-2.5 text-sm text-nvr-text-secondary hover:text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Change Password
          </button>
          <button
            onClick={() => {
              setOpen(false)
              logout()
            }}
            className="w-full text-left px-4 py-2.5 text-sm text-nvr-danger hover:bg-nvr-bg-tertiary transition-colors border-t border-nvr-border focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Logout
          </button>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  SVG icon helpers                                                   */
/* ------------------------------------------------------------------ */
const IconCamera = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
  </svg>
)

const IconDashboard = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
  </svg>
)

const IconAudit = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
  </svg>
)

const IconSettings = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
    <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
  </svg>
)

const IconDownload = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
  </svg>
)

const IconUsers = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
  </svg>
)

const IconResellers = (
  <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 21h16.5M4.5 3h15M5.25 3v18m13.5-18v18M9 6.75h1.5m-1.5 3h1.5m-1.5 3h1.5m3-6H15m-1.5 3H15m-1.5 3H15M9 21v-3.375c0-.621.504-1.125 1.125-1.125h3.75c.621 0 1.125.504 1.125 1.125V21" />
  </svg>
)

/* ------------------------------------------------------------------ */
/*  Branding hook (fetch once, listen for updates)                     */
/* ------------------------------------------------------------------ */
interface Branding {
  product_name: string
  accent_color: string
  logo_url: string
}

function useBranding() {
  const [branding, setBranding] = useState<Branding>({
    product_name: 'MediaMTX NVR',
    accent_color: '#6366f1',
    logo_url: '',
  })

  useEffect(() => {
    fetch('/api/nvr/system/branding')
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setBranding(prev => ({ ...prev, ...data }))
        }
      })
      .catch(() => {})
  }, [])

  // Listen for updates from the settings page.
  useEffect(() => {
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail
      if (detail) {
        setBranding(prev => ({ ...prev, ...detail }))
      }
    }
    window.addEventListener('branding-updated', handler)
    return () => window.removeEventListener('branding-updated', handler)
  }, [])

  // Apply accent color as a CSS custom property.
  useEffect(() => {
    if (branding.accent_color) {
      document.documentElement.style.setProperty('--nvr-branding-accent', branding.accent_color)
    }
  }, [branding.accent_color])

  return branding
}

/* ------------------------------------------------------------------ */
/*  Main layout shell                                                  */
/* ------------------------------------------------------------------ */
function Layout({ children }: { children: React.ReactNode }) {
  const { user, isAuthenticated } = useAuth()
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [showShortcutsHelp, setShowShortcutsHelp] = useState(false)
  const [showShortcutsHint, setShowShortcutsHint] = useState(() => {
    return !localStorage.getItem('nvr-shortcuts-seen')
  })
  const { notifications, unreadCount, markAllRead } = useNotifications(isAuthenticated)
  const storageWarning = useStorageWarning(isAuthenticated)
  const branding = useBranding()
  const location = useLocation()

  // Update document title with branding product name.
  useEffect(() => {
    document.title = branding.product_name
  }, [branding.product_name])

  // Show keyboard shortcuts hint for 10 seconds on first visit, then dismiss
  useEffect(() => {
    if (showShortcutsHint) {
      const timer = setTimeout(() => {
        setShowShortcutsHint(false)
        localStorage.setItem('nvr-shortcuts-seen', 'true')
      }, 10000)
      return () => clearTimeout(timer)
    }
  }, [showShortcutsHint])

  // Global keyboard shortcut: ? to toggle shortcuts help
  const globalShortcuts = useMemo(() => [
    {
      key: '?',
      shift: true,
      handler: () => setShowShortcutsHelp(prev => !prev),
      description: 'Show keyboard shortcuts help',
    },
  ], [])
  useKeyboardShortcuts(globalShortcuts)

  // Auto-close sidebar on route change
  useEffect(() => {
    setSidebarOpen(false)
  }, [location.pathname])

  const closeSidebar = () => setSidebarOpen(false)

  const navLinks: NavLinkProps[] = [
    { to: '/cameras', icon: IconCamera, label: 'Cameras' },
    { to: '/dashboard', icon: IconDashboard, label: 'Health' },
    { to: '/settings', icon: IconSettings, label: 'Settings', badge: storageWarning },
    ...(user?.role === 'admin'
      ? [
          { to: '/users', icon: IconUsers, label: 'Users' },
          { to: '/resellers', icon: IconResellers, label: 'Resellers' },
          { to: '/audit', icon: IconAudit, label: 'Audit Log' },
        ]
      : []),
    { to: '/download', icon: IconDownload, label: 'Get Client' },
  ]

  return (
    <div className="min-h-screen bg-nvr-bg-primary">
      {/* ---- Top nav bar ---- */}
      <nav className="bg-nvr-bg-secondary border-b border-nvr-border">
        <div className="max-w-7xl mx-auto flex items-center h-14 px-4 sm:px-6 lg:px-8">
          {/* Brand */}
          <Link to="/cameras" className="flex items-center gap-2 mr-8">
            {branding.logo_url ? (
              <img src={branding.logo_url} alt="" className="w-7 h-7 rounded-md object-contain" />
            ) : (
              <div className="w-7 h-7 rounded-md bg-nvr-accent/20 flex items-center justify-center">
                <svg className="w-4 h-4 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
              </div>
            )}
            <span className="text-white font-bold text-base tracking-tight hidden sm:block">{branding.product_name}</span>
          </Link>

          {/* Desktop nav links (center) */}
          <div className="hidden md:flex items-center gap-1 flex-1">
            {navLinks.map((link) => (
              <NavItem key={link.to} {...link} />
            ))}
          </div>

          {/* Right section */}
          <div className="hidden md:flex items-center gap-3 ml-auto">
            <NotificationBell
              notifications={notifications}
              unreadCount={unreadCount}
              onMarkAllRead={markAllRead}
            />
            <UserMenu />
          </div>

          {/* Mobile: notification bell + hamburger */}
          <div className="md:hidden flex items-center gap-2 ml-auto">
            <NotificationBell
              notifications={notifications}
              unreadCount={unreadCount}
              onMarkAllRead={markAllRead}
            />
            <button
              onClick={() => setSidebarOpen(true)}
              className="w-10 h-10 flex items-center justify-center text-nvr-text-secondary hover:text-nvr-text-primary transition-colors rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              aria-label="Open menu"
            >
              <svg className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>
          </div>
        </div>
      </nav>

      {/* ---- Mobile sidebar overlay ---- */}
      {sidebarOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          {/* Backdrop */}
          <div
            className="absolute inset-0 bg-black/60 backdrop-blur-sm"
            onClick={closeSidebar}
          />
          {/* Sidebar panel */}
          <div className="absolute top-0 right-0 bottom-0 w-72 bg-nvr-bg-secondary border-l border-nvr-border shadow-2xl flex flex-col animate-slide-in">
            {/* Header */}
            <div className="flex items-center justify-between h-14 px-4 border-b border-nvr-border">
              <span className="text-white font-bold text-base">Menu</span>
              <button
                onClick={closeSidebar}
                className="w-10 h-10 flex items-center justify-center text-nvr-text-secondary hover:text-nvr-text-primary transition-colors rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                aria-label="Close menu"
              >
                <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Nav links */}
            <div className="flex-1 overflow-y-auto px-3 py-4 flex flex-col gap-1">
              {navLinks.map((link) => (
                <MobileNavItem key={link.to} {...link} onClick={closeSidebar} />
              ))}
            </div>

            {/* User section at bottom */}
            <div className="border-t border-nvr-border px-4 py-4">
              <div className="flex items-center gap-3 mb-4">
                <div className="w-9 h-9 rounded-full bg-nvr-accent/20 text-nvr-accent text-xs font-bold flex items-center justify-center">
                  {(user?.username ?? '?').slice(0, 2).toUpperCase()}
                </div>
                <div>
                  <p className="text-sm font-medium text-nvr-text-primary">{user?.username}</p>
                  <p className="text-xs text-nvr-text-muted capitalize">{user?.role}</p>
                </div>
              </div>
              <UserLogoutButton onClose={closeSidebar} />
            </div>
          </div>
        </div>
      )}

      {/* ---- Storage banner + toast layer ---- */}
      <StorageBanner />
      <ToastContainer />

      {/* ---- Page content ---- */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
        {children}
      </main>

      {/* ---- Keyboard shortcuts hint (first visit) ---- */}
      {showShortcutsHint && (
        <div className="fixed bottom-4 left-4 bg-nvr-bg-secondary border border-nvr-border rounded-lg px-4 py-2 shadow-xl z-40 flex items-center gap-2 animate-fade-in">
          <span className="text-xs text-nvr-text-secondary">Press</span>
          <kbd className="bg-nvr-bg-tertiary text-nvr-text-primary text-xs px-1.5 py-0.5 rounded font-mono">?</kbd>
          <span className="text-xs text-nvr-text-secondary">for keyboard shortcuts</span>
          <button
            onClick={() => { setShowShortcutsHint(false); localStorage.setItem('nvr-shortcuts-seen', 'true') }}
            className="text-nvr-text-muted hover:text-nvr-text-primary ml-2 text-sm"
            aria-label="Dismiss"
          >
            &times;
          </button>
        </div>
      )}

      {/* ---- Keyboard shortcuts help overlay ---- */}
      {showShortcutsHelp && (
        <KeyboardShortcutsHelp onClose={() => setShowShortcutsHelp(false)} />
      )}
    </div>
  )
}

function UserLogoutButton({ onClose }: { onClose: () => void }) {
  const { logout } = useAuth()
  return (
    <button
      onClick={() => {
        onClose()
        logout()
      }}
      className="w-full text-sm text-nvr-danger hover:bg-nvr-bg-tertiary rounded-lg px-4 py-2.5 text-left transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
    >
      Logout
    </button>
  )
}

/* ------------------------------------------------------------------ */
/*  Routes                                                             */
/* ------------------------------------------------------------------ */
function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/setup" element={<Setup />} />
      <Route path="/cameras" element={<ProtectedRoute><Layout><CameraManagement /></Layout></ProtectedRoute>} />
      <Route path="/dashboard" element={<ProtectedRoute><Layout><Dashboard /></Layout></ProtectedRoute>} />
      <Route path="/settings" element={<ProtectedRoute><Layout><Settings /></Layout></ProtectedRoute>} />
      <Route path="/users" element={<ProtectedRoute><Layout><UserManagement /></Layout></ProtectedRoute>} />
      <Route path="/resellers" element={<ProtectedRoute><Layout><SubResellerHierarchy /></Layout></ProtectedRoute>} />
      <Route path="/audit" element={<ProtectedRoute><Layout><AuditLog /></Layout></ProtectedRoute>} />
      <Route path="/download" element={<ProtectedRoute><Layout><DownloadClient /></Layout></ProtectedRoute>} />
      {/* Redirect old non-admin routes to the download client page */}
      <Route path="/live" element={<Navigate to="/download" replace />} />
      <Route path="/recordings" element={<Navigate to="/download" replace />} />
      <Route path="/playback" element={<Navigate to="/download" replace />} />
      <Route path="/clips" element={<Navigate to="/download" replace />} />
      <Route path="/" element={<Navigate to="/cameras" replace />} />
    </Routes>
  )
}

export default function App() {
  return (
    <ErrorBoundary>
      <BrowserRouter>
        <AuthProvider>
          <AppRoutes />
        </AuthProvider>
      </BrowserRouter>
    </ErrorBoundary>
  )
}
