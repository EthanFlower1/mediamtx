import { useState, useEffect, FormEvent } from 'react'
import { apiFetch } from '../api/client'
import { useAuth } from '../auth/context'
import { Camera } from '../hooks/useCameras'

interface User {
  id: string
  username: string
  role: string
  camera_permissions: string
}

/** Parse camera_permissions into an array of camera IDs, or null for "*" (all). */
function parsePermissions(perms: string): string[] | null {
  if (!perms || perms === '*') return null
  try {
    const parsed = JSON.parse(perms)
    if (Array.isArray(parsed)) return parsed
  } catch { /* ignore */ }
  return null
}

/** Inline edit form for a user row. */
function UserEditForm({
  user,
  cameras,
  onSave,
  onCancel,
}: {
  user: User
  cameras: Camera[]
  onSave: () => void
  onCancel: () => void
}) {
  const [role, setRole] = useState(user.role)
  const [password, setPassword] = useState('')
  const [allCameras, setAllCameras] = useState(!user.camera_permissions || user.camera_permissions === '*')
  const [selectedCameraIds, setSelectedCameraIds] = useState<string[]>(() => {
    const parsed = parsePermissions(user.camera_permissions)
    return parsed || []
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const toggleCamera = (id: string) => {
    setSelectedCameraIds(prev =>
      prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]
    )
  }

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError('')

    const body: Record<string, string> = { role }

    if (password) {
      body.password = password
    }

    if (allCameras) {
      body.camera_permissions = '*'
    } else {
      body.camera_permissions = JSON.stringify(selectedCameraIds)
    }

    const res = await apiFetch(`/users/${user.id}`, {
      method: 'PUT',
      body: JSON.stringify(body),
    })

    if (res.ok) {
      onSave()
    } else {
      const data = await res.json().catch(() => ({}))
      setError(data.error || 'Failed to update user')
    }
    setSaving(false)
  }

  return (
    <form onSubmit={handleSubmit} className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-4 mt-2">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
        <div>
          <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Username</label>
          <input
            type="text"
            value={user.username}
            disabled
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-muted cursor-not-allowed"
          />
        </div>
        <div>
          <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Role</label>
          <select
            value={role}
            onChange={e => setRole(e.target.value)}
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          >
            <option value="viewer">Viewer</option>
            <option value="admin">Admin</option>
          </select>
        </div>
      </div>

      <div className="mb-4">
        <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">
          New Password <span className="text-nvr-text-muted font-normal">(leave blank to keep current)</span>
        </label>
        <input
          type="password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="Enter new password"
          className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        />
      </div>

      <div className="mb-4">
        <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Camera Permissions</label>
        <label className="flex items-center gap-2 mb-3 cursor-pointer">
          <input
            type="checkbox"
            checked={allCameras}
            onChange={e => setAllCameras(e.target.checked)}
            className="w-4 h-4 rounded border-nvr-border text-nvr-accent focus:ring-nvr-accent bg-nvr-bg-input"
          />
          <span className="text-sm text-nvr-text-primary">All cameras</span>
        </label>
        {!allCameras && (
          <div className="flex flex-wrap gap-2">
            {cameras.length === 0 && (
              <span className="text-sm text-nvr-text-muted">No cameras configured</span>
            )}
            {cameras.map(cam => (
              <label
                key={cam.id}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm cursor-pointer border transition-colors ${
                  selectedCameraIds.includes(cam.id)
                    ? 'bg-nvr-accent/20 border-nvr-accent text-nvr-accent'
                    : 'bg-nvr-bg-input border-nvr-border text-nvr-text-secondary hover:bg-nvr-border/30'
                }`}
              >
                <input
                  type="checkbox"
                  checked={selectedCameraIds.includes(cam.id)}
                  onChange={() => toggleCamera(cam.id)}
                  className="sr-only"
                />
                {cam.name}
              </label>
            ))}
          </div>
        )}
      </div>

      {error && <p className="text-nvr-danger text-sm mb-3">{error}</p>}

      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
        >
          {saving ? 'Saving...' : 'Save Changes'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}

/** Permission badges for the user table. */
function PermissionBadges({ perms, cameras }: { perms: string; cameras: Camera[] }) {
  if (!perms || perms === '*') {
    return <span className="bg-nvr-success/15 text-nvr-success px-2 py-0.5 rounded text-xs font-medium">All cameras</span>
  }
  const ids = parsePermissions(perms)
  if (!ids || ids.length === 0) {
    return <span className="text-nvr-text-muted text-xs">None</span>
  }
  return (
    <div className="flex flex-wrap gap-1">
      {ids.map(id => {
        const cam = cameras.find(c => c.id === id)
        return (
          <span
            key={id}
            className="bg-nvr-accent/10 text-nvr-accent px-2 py-0.5 rounded text-xs font-medium"
          >
            {cam?.name || id.slice(0, 8)}
          </span>
        )
      })}
    </div>
  )
}

export default function UserManagement() {
  const { user: currentUser } = useAuth()
  const [users, setUsers] = useState<User[]>([])
  const [cameras, setCameras] = useState<Camera[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [editingUserId, setEditingUserId] = useState<string | null>(null)

  // Password change state (for current user).
  const [showPasswordChange, setShowPasswordChange] = useState(false)
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [passwordMsg, setPasswordMsg] = useState('')
  const [passwordError, setPasswordError] = useState('')
  const [changingPassword, setChangingPassword] = useState(false)

  const refresh = async () => {
    const res = await apiFetch('/users')
    if (res.ok) setUsers(await res.json())
  }

  const refreshCameras = async () => {
    const res = await apiFetch('/cameras')
    if (res.ok) setCameras(await res.json())
  }

  useEffect(() => {
    refresh()
    refreshCameras()
  }, [])

  const handleAdd = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const formData = new FormData(e.currentTarget)
    await apiFetch('/users', {
      method: 'POST',
      body: JSON.stringify({
        username: formData.get('username'),
        password: formData.get('password'),
        role: formData.get('role'),
        camera_permissions: '*',
      }),
    })
    setShowAdd(false)
    refresh()
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this user?')) return
    await apiFetch(`/users/${id}`, { method: 'DELETE' })
    if (editingUserId === id) setEditingUserId(null)
    refresh()
  }

  const handlePasswordChange = async (e: FormEvent) => {
    e.preventDefault()
    setPasswordMsg('')
    setPasswordError('')
    setChangingPassword(true)

    const res = await apiFetch('/auth/password', {
      method: 'PUT',
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    })

    if (res.ok) {
      setPasswordMsg('Password changed successfully. You may need to log in again.')
      setCurrentPassword('')
      setNewPassword('')
    } else {
      const data = await res.json().catch(() => ({}))
      setPasswordError(data.error || 'Failed to change password')
    }
    setChangingPassword(false)
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-nvr-text-primary">User Management</h1>
        <div className="flex gap-2">
          <button
            onClick={() => { setShowPasswordChange(!showPasswordChange); setShowAdd(false) }}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors"
          >
            Change My Password
          </button>
          <button
            onClick={() => { setShowAdd(!showAdd); setShowPasswordChange(false) }}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
          >
            Add User
          </button>
        </div>
      </div>

      {/* Change own password form */}
      {showPasswordChange && (
        <form onSubmit={handlePasswordChange} className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5 mb-6">
          <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">Change Password</h3>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-4">
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Current Password</label>
              <input
                type="password"
                value={currentPassword}
                onChange={e => setCurrentPassword(e.target.value)}
                required
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
              />
            </div>
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">New Password</label>
              <input
                type="password"
                value={newPassword}
                onChange={e => setNewPassword(e.target.value)}
                required
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
              />
            </div>
          </div>
          {passwordError && <p className="text-nvr-danger text-sm mb-3">{passwordError}</p>}
          {passwordMsg && <p className="text-nvr-success text-sm mb-3">{passwordMsg}</p>}
          <div className="flex gap-2">
            <button
              type="submit"
              disabled={changingPassword}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
            >
              {changingPassword ? 'Changing...' : 'Change Password'}
            </button>
            <button
              type="button"
              onClick={() => { setShowPasswordChange(false); setPasswordMsg(''); setPasswordError('') }}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* Add user form */}
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
          <div className="flex gap-2">
            <button
              type="submit"
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
            >
              Create
            </button>
            <button
              type="button"
              onClick={() => setShowAdd(false)}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* User table */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-nvr-border">
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Username</th>
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Role</th>
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Camera Permissions</th>
              <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Actions</th>
            </tr>
          </thead>
          <tbody>
            {users.map(u => (
              <tr key={u.id} className="border-b border-nvr-border/50">
                <td colSpan={4} className="p-0">
                  <div
                    className={`flex items-center hover:bg-nvr-bg-tertiary/50 transition-colors ${editingUserId === u.id ? 'bg-nvr-accent/5' : ''}`}
                  >
                    <div className="px-4 py-3 text-sm text-nvr-text-primary font-medium w-1/4">{u.username}</div>
                    <div className="px-4 py-3 text-sm w-1/6">
                      {u.role === 'admin' ? (
                        <span className="bg-nvr-accent/10 text-nvr-accent px-2 py-0.5 rounded text-xs font-medium">Admin</span>
                      ) : (
                        <span className="text-nvr-text-secondary text-xs capitalize">{u.role}</span>
                      )}
                    </div>
                    <div className="px-4 py-3 text-sm flex-1">
                      <PermissionBadges perms={u.camera_permissions} cameras={cameras} />
                    </div>
                    <div className="px-4 py-3 text-sm flex gap-2 shrink-0">
                      <button
                        onClick={() => setEditingUserId(editingUserId === u.id ? null : u.id)}
                        className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors"
                      >
                        {editingUserId === u.id ? 'Close' : 'Edit'}
                      </button>
                      {u.id !== currentUser?.id && (
                        <button
                          onClick={() => handleDelete(u.id)}
                          className="bg-nvr-danger hover:bg-nvr-danger-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors"
                        >
                          Delete
                        </button>
                      )}
                    </div>
                  </div>
                  {editingUserId === u.id && (
                    <div className="px-4 pb-4">
                      <UserEditForm
                        user={u}
                        cameras={cameras}
                        onSave={() => { setEditingUserId(null); refresh() }}
                        onCancel={() => setEditingUserId(null)}
                      />
                    </div>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {users.length === 0 && (
          <p className="text-center py-8 text-nvr-text-muted">No users found.</p>
        )}
      </div>
    </div>
  )
}
