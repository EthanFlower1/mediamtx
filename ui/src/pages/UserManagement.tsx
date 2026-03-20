import { useState, useEffect, FormEvent } from 'react'
import { apiFetch } from '../api/client'

interface User {
  id: string
  username: string
  role: string
  camera_permissions: string
}

export default function UserManagement() {
  const [users, setUsers] = useState<User[]>([])
  const [showAdd, setShowAdd] = useState(false)

  const refresh = async () => {
    const res = await apiFetch('/users')
    if (res.ok) setUsers(await res.json())
  }

  useEffect(() => { refresh() }, [])

  const handleAdd = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const formData = new FormData(e.currentTarget)
    await apiFetch('/users', {
      method: 'POST',
      body: JSON.stringify({
        username: formData.get('username'),
        password: formData.get('password'),
        role: formData.get('role'),
      }),
    })
    setShowAdd(false)
    refresh()
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this user?')) return
    await apiFetch(`/users/${id}`, { method: 'DELETE' })
    refresh()
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-nvr-text-primary">User Management</h1>
        <button
          onClick={() => setShowAdd(!showAdd)}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
        >
          Add User
        </button>
      </div>

      {showAdd && (
        <form onSubmit={handleAdd} className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5 mb-6">
          <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">New User</h3>
          <div className="mb-4">
            <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Username</label>
            <input
              name="username"
              required
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>
          <div className="mb-4">
            <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Password</label>
            <input
              name="password"
              type="password"
              required
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>
          <div className="mb-4">
            <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Role</label>
            <select
              name="role"
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            >
              <option value="viewer">Viewer</option>
              <option value="admin">Admin</option>
            </select>
          </div>
          <button
            type="submit"
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
          >
            Create
          </button>
        </form>
      )}

      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-nvr-border">
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Username</th>
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Role</th>
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map(u => (
              <tr key={u.id} className="border-b border-nvr-border/50 hover:bg-nvr-bg-tertiary/50 transition-colors">
                <td className="px-4 py-3 text-sm text-nvr-text-primary font-medium">{u.username}</td>
                <td className="px-4 py-3 text-sm">
                  {u.role === 'admin' ? (
                    <span className="bg-nvr-accent/10 text-nvr-accent px-2 py-0.5 rounded text-xs font-medium">Admin</span>
                  ) : (
                    <span className="text-nvr-text-secondary text-xs">{u.role}</span>
                  )}
                </td>
                <td className="px-4 py-3 text-sm">
                  <button
                    onClick={() => handleDelete(u.id)}
                    className="bg-nvr-danger hover:bg-nvr-danger-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
