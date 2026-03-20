import { BrowserRouter, Routes, Route, Navigate, Link } from 'react-router-dom'
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
  if (isLoading) return <div>Loading...</div>
  if (setupRequired) return <Navigate to="/setup" replace />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  return (
    <div>
      <nav style={{ display: 'flex', gap: 16, padding: '8px 16px', background: '#1a1a2e', color: '#fff', alignItems: 'center' }}>
        <strong>MediaMTX NVR</strong>
        <Link to="/live" style={{ color: '#ccc' }}>Live</Link>
        <Link to="/cameras" style={{ color: '#ccc' }}>Cameras</Link>
        <Link to="/recordings" style={{ color: '#ccc' }}>Recordings</Link>
        <Link to="/settings" style={{ color: '#ccc' }}>Settings</Link>
        {user?.role === 'admin' && <Link to="/users" style={{ color: '#ccc' }}>Users</Link>}
        <span style={{ marginLeft: 'auto', fontSize: 14 }}>{user?.username}</span>
        <button onClick={logout} style={{ marginLeft: 8 }}>Logout</button>
      </nav>
      <div style={{ padding: 16 }}>{children}</div>
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
