import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './auth/context'
import Login from './pages/Login'
import Setup from './pages/Setup'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading, setupRequired } = useAuth()
  if (isLoading) return <div>Loading...</div>
  if (setupRequired) return <Navigate to="/setup" replace />
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{children}</>
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/setup" element={<Setup />} />
      <Route path="/live" element={<ProtectedRoute><div>Live View</div></ProtectedRoute>} />
      <Route path="/cameras" element={<ProtectedRoute><div>Camera Management</div></ProtectedRoute>} />
      <Route path="/recordings" element={<ProtectedRoute><div>Recordings</div></ProtectedRoute>} />
      <Route path="/settings" element={<ProtectedRoute><div>Settings</div></ProtectedRoute>} />
      <Route path="/users" element={<ProtectedRoute><div>User Management</div></ProtectedRoute>} />
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
