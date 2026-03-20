import { useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate, Link, useLocation } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/context'
import Login from './pages/Login'
import Setup from './pages/Setup'
import LiveView from './pages/LiveView'
import CameraManagement from './pages/CameraManagement'
import Recordings from './pages/Recordings'
import Settings from './pages/Settings'
import UserManagement from './pages/UserManagement'
import ToastContainer from './components/Toast'
import NotificationBell from './components/NotificationBell'
import ErrorBoundary from './components/ErrorBoundary'
import { useNotifications } from './hooks/useNotifications'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, setupRequired } = useAuth()
  if (isLoading) return <div className="flex items-center justify-center h-screen bg-nvr-bg-primary text-nvr-text-secondary">Loading...</div>
  if (setupRequired) return <Navigate to="/setup" replace />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

function NavLink({ to, children, onClick }: { to: string; children: React.ReactNode; onClick?: () => void }) {
  const location = useLocation()
  const isActive = location.pathname === to
  return (
    <Link
      to={to}
      onClick={onClick}
      className={
        isActive
          ? 'text-nvr-accent font-medium transition-colors py-2 md:py-0'
          : 'text-nvr-text-secondary hover:text-nvr-text-primary transition-colors py-2 md:py-0'
      }
    >
      {children}
    </Link>
  )
}

function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout, isAuthenticated } = useAuth()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const { notifications, unreadCount, markAllRead } = useNotifications(isAuthenticated)

  const closeMenu = () => setMobileMenuOpen(false)

  return (
    <div className="min-h-screen bg-nvr-bg-primary">
      <nav className="bg-nvr-bg-secondary border-b border-nvr-border">
        <div className="flex items-center gap-4 px-4 py-2.5">
          <strong className="text-white font-bold text-lg">MediaMTX NVR</strong>
          {/* Desktop nav links */}
          <div className="hidden md:flex items-center gap-4">
            <NavLink to="/live">Live</NavLink>
            <NavLink to="/cameras">Cameras</NavLink>
            <NavLink to="/recordings">Recordings</NavLink>
            <NavLink to="/settings">Settings</NavLink>
            {user?.role === 'admin' && <NavLink to="/users">Users</NavLink>}
          </div>
          <div className="hidden md:flex items-center gap-3 ml-auto">
            <NotificationBell
              notifications={notifications}
              unreadCount={unreadCount}
              onMarkAllRead={markAllRead}
            />
            <span className="text-sm text-nvr-text-secondary">{user?.username}</span>
            <button
              onClick={logout}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors text-sm"
            >
              Logout
            </button>
          </div>
          {/* Mobile notification bell + hamburger */}
          <div className="md:hidden ml-auto flex items-center gap-1">
            <NotificationBell
              notifications={notifications}
              unreadCount={unreadCount}
              onMarkAllRead={markAllRead}
            />
            <button
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
              className="min-w-[44px] min-h-[44px] flex items-center justify-center text-nvr-text-secondary hover:text-nvr-text-primary transition-colors"
              aria-label="Toggle menu"
            >
              {mobileMenuOpen ? (
                <svg xmlns="http://www.w3.org/2000/svg" className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                </svg>
              ) : (
                <svg xmlns="http://www.w3.org/2000/svg" className="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
                </svg>
              )}
            </button>
          </div>
        </div>
        {/* Mobile dropdown menu */}
        {mobileMenuOpen && (
          <div className="md:hidden flex flex-col gap-1 px-4 pb-4 border-t border-nvr-border pt-3">
            <NavLink to="/live" onClick={closeMenu}>Live</NavLink>
            <NavLink to="/cameras" onClick={closeMenu}>Cameras</NavLink>
            <NavLink to="/recordings" onClick={closeMenu}>Recordings</NavLink>
            <NavLink to="/settings" onClick={closeMenu}>Settings</NavLink>
            {user?.role === 'admin' && <NavLink to="/users" onClick={closeMenu}>Users</NavLink>}
            <div className="flex items-center justify-between pt-3 mt-2 border-t border-nvr-border">
              <span className="text-sm text-nvr-text-secondary">{user?.username}</span>
              <button
                onClick={() => { closeMenu(); logout() }}
                className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px]"
              >
                Logout
              </button>
            </div>
          </div>
        )}
      </nav>
      <ToastContainer />
      <main className="p-4 md:p-6">{children}</main>
    </div>
  )
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/setup" element={<Setup />} />
      <Route path="/live" element={<ProtectedRoute><Layout><LiveView /></Layout></ProtectedRoute>} />
      <Route path="/cameras" element={<ProtectedRoute><Layout><CameraManagement /></Layout></ProtectedRoute>} />
      <Route path="/recordings" element={<ProtectedRoute><Layout><Recordings /></Layout></ProtectedRoute>} />
      <Route path="/settings" element={<ProtectedRoute><Layout><Settings /></Layout></ProtectedRoute>} />
      <Route path="/users" element={<ProtectedRoute><Layout><UserManagement /></Layout></ProtectedRoute>} />
      <Route path="/" element={<Navigate to="/live" replace />} />
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
