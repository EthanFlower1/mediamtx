import { createContext, useContext, useState, useEffect, ReactNode } from 'react'
import { setAccessToken, apiFetch } from '../api/client'

interface AuthState {
  isAuthenticated: boolean
  isLoading: boolean
  user: { id: string; username: string; role: string } | null
  login: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  setupRequired: boolean
}

const AuthContext = createContext<AuthState | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [isAuthenticated, setIsAuthenticated] = useState(false)
  const [isLoading, setIsLoading] = useState(true)
  const [user, setUser] = useState<AuthState['user']>(null)
  const [setupRequired, setSetupRequired] = useState(false)

  useEffect(() => {
    // Try to refresh token on mount
    fetch('/api/nvr/auth/refresh', { method: 'POST', credentials: 'include' })
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setAccessToken(data.access_token)
          setIsAuthenticated(true)
          setUser({ id: data.user.id, username: data.user.username, role: data.user.role })
        } else {
          // Check if setup is needed
          const healthRes = await fetch('/api/nvr/system/health')
          if (healthRes.status === 503) {
            setSetupRequired(true)
          }
        }
      })
      .catch(() => {})
      .finally(() => setIsLoading(false))
  }, [])

  const login = async (username: string, password: string) => {
    const res = await fetch('/api/nvr/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
      credentials: 'include',
    })
    if (!res.ok) throw new Error('Invalid credentials')
    const data = await res.json()
    setAccessToken(data.access_token)
    setUser({ id: data.user.id, username: data.user.username, role: data.user.role })
    setIsAuthenticated(true)
  }

  const logout = async () => {
    await apiFetch('/auth/revoke', { method: 'POST' })
    setAccessToken(null)
    setUser(null)
    setIsAuthenticated(false)
  }

  return (
    <AuthContext.Provider value={{ isAuthenticated, isLoading, user, login, logout, setupRequired }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
