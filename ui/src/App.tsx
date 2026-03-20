import { BrowserRouter, Routes, Route, Navigate, Link, useLocation } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/context'
import Login from './pages/Login'
import Setup from './pages/Setup'
import LiveView from './pages/LiveView'
import CameraManagement from './pages/CameraManagement'
import Recordings from './pages/Recordings'
import Settings from './pages/Settings'
import UserManagement from './pages/UserManagement'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, setupRequired } = useAuth()
  if (isLoading) return <div className="flex items-center justify-center h-screen bg-nvr-bg-primary text-nvr-text-secondary">Loading...</div>
  if (setupRequired) return <Navigate to="/setup" replace />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

function NavLink({ to, children }: { to: string; children: React.ReactNode }) {
  const location = useLocation()
  const isActive = location.pathname === to
  return (
    <Link
      to={to}
      className={
        isActive
          ? 'text-nvr-accent font-medium transition-colors'
          : 'text-nvr-text-secondary hover:text-nvr-text-primary transition-colors'
      }
    >
      {children}
    </Link>
  )
}

function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  return (
    <div className="min-h-screen bg-nvr-bg-primary">
      <nav className="flex items-center gap-4 px-4 py-2.5 bg-nvr-bg-secondary border-b border-nvr-border">
        <strong className="text-white font-bold text-lg">MediaMTX NVR</strong>
        <NavLink to="/live">Live</NavLink>
        <NavLink to="/cameras">Cameras</NavLink>
        <NavLink to="/recordings">Recordings</NavLink>
        <NavLink to="/settings">Settings</NavLink>
        {user?.role === 'admin' && <NavLink to="/users">Users</NavLink>}
        <span className="ml-auto text-sm text-nvr-text-secondary">{user?.username}</span>
        <button
          onClick={logout}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors text-sm"
        >
          Logout
        </button>
      </nav>
      <main className="p-6">{children}</main>
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
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )
}
