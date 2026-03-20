import { useState, FormEvent, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/context'

type PasswordStrength = 'weak' | 'medium' | 'strong'

function getPasswordStrength(pw: string): PasswordStrength {
  if (pw.length >= 12) return 'strong'
  if (pw.length >= 8) return 'medium'
  return 'weak'
}

const strengthConfig: Record<PasswordStrength, { label: string; color: string; width: string }> = {
  weak: { label: 'Weak', color: 'bg-nvr-danger', width: 'w-1/3' },
  medium: { label: 'Medium', color: 'bg-nvr-warning', width: 'w-2/3' },
  strong: { label: 'Strong', color: 'bg-nvr-success', width: 'w-full' },
}

export default function Setup() {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const navigate = useNavigate()
  const { login } = useAuth()

  const strength = useMemo(() => getPasswordStrength(password), [password])
  const sc = strengthConfig[strength]

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }

    if (password.length < 6) {
      setError('Password must be at least 6 characters.')
      return
    }

    setLoading(true)
    try {
      const res = await fetch('/api/nvr/auth/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => null)
        setError(data?.error ?? 'Setup failed. Please try again.')
        setLoading(false)
        return
      }

      setSuccess(true)
      await login(username, password)

      // Brief pause so the user sees the success message
      setTimeout(() => navigate('/live'), 1200)
    } catch {
      setError('Setup failed. Please try again.')
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-nvr-bg-primary px-4">
      <div className="w-full max-w-sm">
        {/* Brand */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-nvr-accent/15 mb-4">
            <svg className="w-6 h-6 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-white tracking-tight">Welcome to MediaMTX NVR</h1>
          <p className="text-sm text-nvr-text-secondary mt-2">Create your admin account to get started.</p>
        </div>

        {/* Card */}
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-2xl p-6 shadow-2xl">
          {/* Step indicator */}
          <div className="flex items-center gap-2 mb-6">
            <span className="w-6 h-6 rounded-full bg-nvr-accent text-white text-xs font-bold flex items-center justify-center">1</span>
            <span className="text-xs text-nvr-text-muted">Step 1 of 1 &mdash; Admin Account</span>
          </div>

          {success ? (
            <div className="flex flex-col items-center gap-4 py-6">
              <div className="w-12 h-12 rounded-full bg-nvr-success/20 flex items-center justify-center">
                <svg className="w-6 h-6 text-nvr-success" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <div className="text-center">
                <p className="text-sm font-semibold text-white">Account created!</p>
                <p className="text-xs text-nvr-text-muted mt-1">Redirecting...</p>
              </div>
            </div>
          ) : (
            <form onSubmit={handleSubmit} className="flex flex-col gap-3">
              <div>
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Username</label>
                <input
                  type="text"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  required
                  autoComplete="username"
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Password</label>
                <input
                  type="password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Choose a strong password"
                  required
                  autoComplete="new-password"
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
                {/* Password strength indicator */}
                {password.length > 0 && (
                  <div className="mt-2">
                    <div className="h-1 rounded-full bg-nvr-bg-tertiary overflow-hidden">
                      <div className={`h-full rounded-full ${sc.color} ${sc.width} transition-all duration-300`} />
                    </div>
                    <p className={`text-xs mt-1 ${
                      strength === 'weak' ? 'text-nvr-danger' :
                      strength === 'medium' ? 'text-nvr-warning' : 'text-nvr-success'
                    }`}>
                      {sc.label}
                    </p>
                  </div>
                )}
              </div>

              <div>
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Confirm Password</label>
                <input
                  type="password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  placeholder="Re-enter password"
                  required
                  autoComplete="new-password"
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>

              {error && (
                <div className="flex items-start gap-3 bg-nvr-danger/10 border-l-4 border-nvr-danger rounded-r-lg px-4 py-3">
                  <svg className="w-4 h-4 text-nvr-danger mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                    <circle cx="12" cy="12" r="10" />
                    <line x1="12" y1="8" x2="12" y2="12" />
                    <line x1="12" y1="16" x2="12.01" y2="16" />
                  </svg>
                  <p className="text-sm text-nvr-danger">{error}</p>
                </div>
              )}

              <button
                type="submit"
                disabled={loading}
                className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed mt-1 flex items-center justify-center gap-2"
              >
                {loading ? (
                  <>
                    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                    </svg>
                    Creating account...
                  </>
                ) : (
                  'Create Admin Account'
                )}
              </button>
            </form>
          )}
        </div>

        {/* Footer */}
        <p className="text-center text-xs text-nvr-text-muted mt-6">Powered by MediaMTX</p>
      </div>
    </div>
  )
}
