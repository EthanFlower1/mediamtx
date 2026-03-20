import { useState, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/context'

export default function Setup() {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const navigate = useNavigate()
  const { login } = useAuth()

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (password !== confirm) {
      setError('Passwords do not match')
      return
    }
    const res = await fetch('/api/nvr/auth/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password }),
    })
    if (!res.ok) {
      setError('Setup failed')
      return
    }
    await login(username, password)
    navigate('/live')
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-nvr-bg-primary px-4">
      <div className="w-full max-w-sm bg-nvr-bg-secondary border border-nvr-border rounded-2xl p-6 md:p-8 shadow-2xl">
        <h1 className="text-2xl font-bold text-white text-center mb-2">MediaMTX NVR Setup</h1>
        <p className="text-nvr-text-secondary text-sm text-center mb-6">Create your admin account to get started.</p>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <label className="block text-sm font-medium text-nvr-text-secondary">Username</label>
            <input
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              required
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>
          <div className="space-y-1.5">
            <label className="block text-sm font-medium text-nvr-text-secondary">Password</label>
            <input
              type="password"
              value={password}
              onChange={e => setPassword(e.target.value)}
              required
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>
          <div className="space-y-1.5">
            <label className="block text-sm font-medium text-nvr-text-secondary">Confirm Password</label>
            <input
              type="password"
              value={confirm}
              onChange={e => setConfirm(e.target.value)}
              required
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>
          {error && <p className="text-nvr-danger text-sm">{error}</p>}
          <button
            type="submit"
            className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 min-h-[44px]"
          >
            Create Admin Account
          </button>
        </form>
      </div>
    </div>
  )
}
