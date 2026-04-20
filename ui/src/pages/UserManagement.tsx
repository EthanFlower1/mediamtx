import { useState, useEffect, FormEvent, useMemo } from 'react'
import { Navigate } from 'react-router-dom'
import { apiFetch } from '../api/client'
import { useAuth } from '../auth/context'
import { Camera } from '../hooks/useCameras'
import ConfirmDialog from '../components/ConfirmDialog'

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

function getInitials(username: string): string {
  return username.slice(0, 2).toUpperCase()
}

function permissionSummary(perms: string, cameras: Camera[]): string {
  if (!perms || perms === '*') return 'All cameras'
  const ids = parsePermissions(perms)
  if (!ids || ids.length === 0) return 'No cameras'
  if (ids.length <= 2) {
    return ids.map(id => cameras.find(c => c.id === id)?.name || id.slice(0, 8)).join(', ')
  }
  const first = cameras.find(c => c.id === ids[0])?.name || ids[0].slice(0, 8)
  return `${first} +${ids.length - 1} more`
}

// ===== Modal Component =====
function Modal({ open, onClose, children }: { open: boolean; onClose: () => void; children: React.ReactNode }) {
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose} role="dialog" aria-modal="true" aria-label="User dialog">
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" aria-hidden="true" />
      <div
        className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto"
        onClick={e => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  )
}

// ===== Add/Edit User Modal Form =====
function UserForm({
  user,
  cameras,
  onSave,
  onCancel,
}: {
  user?: User
  cameras: Camera[]
  onSave: () => void
  onCancel: () => void
}) {
  const isEdit = !!user
  const [username, setUsername] = useState(user?.username || '')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState(user?.role || 'viewer')
  const [allCameras, setAllCameras] = useState(!user?.camera_permissions || user?.camera_permissions === '*')
  const [selectedCameraIds, setSelectedCameraIds] = useState<string[]>(() => {
    if (user) {
      const parsed = parsePermissions(user.camera_permissions)
      return parsed || []
    }
    return []
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

    const permStr = allCameras ? '*' : JSON.stringify(selectedCameraIds)

    if (isEdit && user) {
      const body: Record<string, string> = { role, camera_permissions: permStr }
      if (password) body.password = password

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
    } else {
      if (!username || !password) {
        setError('Username and password are required')
        setSaving(false)
        return
      }

      const res = await apiFetch('/users', {
        method: 'POST',
        body: JSON.stringify({
          username,
          password,
          role,
          camera_permissions: permStr,
        }),
      })

      if (res.ok) {
        onSave()
      } else {
        const data = await res.json().catch(() => ({}))
        setError(data.error || 'Failed to create user')
      }
    }
    setSaving(false)
  }

  return (
    <form onSubmit={handleSubmit} className="p-5">
      <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">
        {isEdit ? `Edit ${user.username}` : 'Add User'}
      </h3>

      <div className="space-y-4">
        <div>
          <label htmlFor="user-form-username" className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Username</label>
          {isEdit ? (
            <input
              id="user-form-username"
              type="text"
              value={user.username}
              disabled
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-muted cursor-not-allowed"
            />
          ) : (
            <input
              id="user-form-username"
              type="text"
              value={username}
              onChange={e => setUsername(e.target.value)}
              required
              placeholder="Enter username"
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          )}
        </div>

        <div>
          <label htmlFor="user-form-password" className="block text-sm font-medium text-nvr-text-secondary mb-1.5">
            Password {isEdit && <span className="text-nvr-text-muted font-normal">(leave blank to keep current)</span>}
          </label>
          <input
            id="user-form-password"
            type="password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            required={!isEdit}
            placeholder={isEdit ? 'Enter new password' : 'Enter password'}
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />
        </div>

        {/* Role toggle cards */}
        <div>
          <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Role</label>
          <div className="grid grid-cols-2 gap-2" role="radiogroup" aria-label="User role">
            <button
              type="button"
              role="radio"
              aria-checked={role === 'admin'}
              onClick={() => setRole('admin')}
              className={`p-3 rounded-lg border text-center transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                role === 'admin'
                  ? 'bg-nvr-accent/10 border-nvr-accent text-nvr-accent'
                  : 'bg-nvr-bg-input border-nvr-border text-nvr-text-secondary hover:bg-nvr-bg-tertiary'
              }`}
            >
              <p className="text-sm font-medium">Admin</p>
              <p className="text-xs mt-0.5 opacity-70">Full access</p>
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={role === 'viewer'}
              onClick={() => setRole('viewer')}
              className={`p-3 rounded-lg border text-center transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                role === 'viewer'
                  ? 'bg-nvr-accent/10 border-nvr-accent text-nvr-accent'
                  : 'bg-nvr-bg-input border-nvr-border text-nvr-text-secondary hover:bg-nvr-bg-tertiary'
              }`}
            >
              <p className="text-sm font-medium">Viewer</p>
              <p className="text-xs mt-0.5 opacity-70">View only</p>
            </button>
          </div>
        </div>

        {/* Camera permissions */}
        <div>
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
      </div>

      {error && <p role="alert" className="text-nvr-danger text-sm mt-3">{error}</p>}

      <div className="flex justify-end gap-2 mt-6 pt-4 border-t border-nvr-border">
        <button
          type="button"
          onClick={onCancel}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          {saving && <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
          {saving ? 'Saving...' : isEdit ? 'Save Changes' : 'Create User'}
        </button>
      </div>
    </form>
  )
}

// ===== Change Password Modal =====
function ChangePasswordModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (newPassword !== confirmPassword) {
      setError('Passwords do not match')
      return
    }

    setSaving(true)
    const res = await apiFetch('/auth/password', {
      method: 'PUT',
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    })

    if (res.ok) {
      setSuccess(true)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
    } else {
      const data = await res.json().catch(() => ({}))
      setError(data.error || 'Failed to change password')
    }
    setSaving(false)
  }

  return (
    <Modal open={open} onClose={onClose}>
      <form onSubmit={handleSubmit} className="p-5">
        <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">Change Password</h3>

        {success ? (
          <div className="py-4">
            <div className="bg-green-500/10 border border-green-500/20 rounded-lg p-4 text-center">
              <svg xmlns="http://www.w3.org/2000/svg" className="w-8 h-8 mx-auto mb-2 text-green-400" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M22 11.08V12a10 10 0 11-5.93-9.14" /><polyline points="22 4 12 14.01 9 11.01" /></svg>
              <p className="text-sm text-green-400 font-medium">Password changed successfully</p>
              <p className="text-xs text-nvr-text-muted mt-1">You may need to log in again.</p>
            </div>
            <div className="flex justify-end mt-4">
              <button
                type="button"
                onClick={onClose}
                className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                Close
              </button>
            </div>
          </div>
        ) : (
          <>
            <div className="space-y-4">
              <div>
                <label htmlFor="chpw-current" className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Current Password</label>
                <input
                  id="chpw-current"
                  type="password"
                  value={currentPassword}
                  onChange={e => setCurrentPassword(e.target.value)}
                  required
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>
              <div>
                <label htmlFor="chpw-new" className="block text-sm font-medium text-nvr-text-secondary mb-1.5">New Password</label>
                <input
                  id="chpw-new"
                  type="password"
                  value={newPassword}
                  onChange={e => setNewPassword(e.target.value)}
                  required
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>
              <div>
                <label htmlFor="chpw-confirm" className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Confirm New Password</label>
                <input
                  id="chpw-confirm"
                  type="password"
                  value={confirmPassword}
                  onChange={e => setConfirmPassword(e.target.value)}
                  required
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>
            </div>

            {error && <p role="alert" className="text-nvr-danger text-sm mt-3">{error}</p>}

            <div className="flex justify-end gap-2 mt-6 pt-4 border-t border-nvr-border">
              <button
                type="button"
                onClick={onClose}
                className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={saving}
                className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                {saving && <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
                {saving ? 'Changing...' : 'Change Password'}
              </button>
            </div>
          </>
        )}
      </form>
    </Modal>
  )
}

// ===== Main Component =====
export default function UserManagement() {
  const { user: currentUser } = useAuth()
  const [users, setUsers] = useState<User[]>([])
  const [cameras, setCameras] = useState<Camera[]>([])
  const [showAddModal, setShowAddModal] = useState(false)
  const [editingUser, setEditingUser] = useState<User | null>(null)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)
  const [showPasswordModal, setShowPasswordModal] = useState(false)
  const isAdmin = currentUser?.role === 'admin'

  // Page title
  useEffect(() => {
    document.title = 'Users — Raikada'
    return () => { document.title = 'Raikada' }
  }, [])

  const showViewerTip = useMemo(() => {
    if (users.length === 0) return false
    return users.length === 1 && users[0].role === 'admin'
  }, [users])

  const refresh = async () => {
    const res = await apiFetch('/users')
    if (res.ok) setUsers(await res.json())
  }

  const refreshCameras = async () => {
    const res = await apiFetch('/cameras')
    if (res.ok) setCameras(await res.json())
  }

  useEffect(() => {
    if (!isAdmin) return
    refresh()
    refreshCameras()
  }, [isAdmin])

  const handleDelete = async (id: string) => {
    await apiFetch(`/users/${id}`, { method: 'DELETE' })
    if (editingUser?.id === id) setEditingUser(null)
    setConfirmDeleteId(null)
    refresh()
  }

  // Guard: only admins can access this page
  if (!isAdmin) {
    return <Navigate to="/live" replace />
  }

  return (
    <div>
      {/* Your Account card */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-full bg-nvr-accent/20 flex items-center justify-center">
              <span className="text-sm font-semibold text-nvr-accent">{getInitials(currentUser?.username || '??')}</span>
            </div>
            <div>
              <p className="text-sm font-medium text-nvr-text-primary">{currentUser?.username}</p>
              <p className="text-xs text-nvr-text-muted capitalize">{currentUser?.role}</p>
            </div>
          </div>
          <button
            onClick={() => setShowPasswordModal(true)}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Change Password
          </button>
        </div>
      </div>

      {/* Header with Add User */}
      <div className="flex items-center justify-between mb-4">
        <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Users</h1>
        <button
          onClick={() => setShowAddModal(true)}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
          Add User
        </button>
      </div>

      {/* User cards */}
      {users.length === 0 ? (
        <div className="text-center py-12">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 mx-auto mb-3 text-nvr-text-muted/40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4-4v2" /><circle cx="9" cy="7" r="4" /><path d="M23 21v-2a4 4 0 00-3-3.87" /><path d="M16 3.13a4 4 0 010 7.75" /></svg>
          <p className="text-nvr-text-muted">No users found.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {users.map(u => (
            <div
              key={u.id}
              className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 hover:border-nvr-border transition-colors group"
            >
              <div className="flex items-start gap-3">
                {/* Avatar */}
                <div className={`w-10 h-10 rounded-full flex items-center justify-center shrink-0 ${
                  u.role === 'admin' ? 'bg-nvr-accent/20' : 'bg-nvr-bg-tertiary'
                }`}>
                  <span className={`text-sm font-semibold ${
                    u.role === 'admin' ? 'text-nvr-accent' : 'text-nvr-text-secondary'
                  }`}>
                    {getInitials(u.username)}
                  </span>
                </div>

                {/* Info */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 mb-0.5">
                    <p className="text-sm font-medium text-nvr-text-primary truncate">{u.username}</p>
                    <span className={`px-2 py-0.5 rounded text-xs font-medium shrink-0 ${
                      u.role === 'admin'
                        ? 'bg-nvr-accent/10 text-nvr-accent'
                        : 'bg-nvr-bg-tertiary text-nvr-text-muted'
                    }`}>
                      {u.role}
                    </span>
                  </div>
                  <p className="text-xs text-nvr-text-muted truncate">{permissionSummary(u.camera_permissions, cameras)}</p>
                </div>
              </div>

              {/* Actions */}
              <div className="flex gap-2 mt-3 pt-3 border-t border-nvr-border/50">
                <button
                  onClick={() => setEditingUser(u)}
                  className="flex-1 bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium py-1.5 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  Edit
                </button>
                {u.id !== currentUser?.id && (
                  <button
                    onClick={() => setConfirmDeleteId(u.id)}
                    className="bg-nvr-danger/10 hover:bg-nvr-danger/20 text-nvr-danger font-medium px-3 py-1.5 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    Delete
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Tip for single-admin setups */}
      {showViewerTip && (
        <div className="mt-4 flex items-start gap-3 bg-nvr-accent/5 border border-nvr-accent/15 rounded-xl px-4 py-3">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4 text-nvr-accent shrink-0 mt-0.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><line x1="12" y1="16" x2="12" y2="12" /><line x1="12" y1="8" x2="12.01" y2="8" /></svg>
          <p className="text-sm text-nvr-text-secondary">
            <span className="font-medium text-nvr-text-primary">Tip:</span> Create viewer accounts to give team members read-only access to camera feeds.
          </p>
        </div>
      )}

      {/* Add User Modal */}
      <Modal open={showAddModal} onClose={() => setShowAddModal(false)}>
        <UserForm
          cameras={cameras}
          onSave={() => { setShowAddModal(false); refresh() }}
          onCancel={() => setShowAddModal(false)}
        />
      </Modal>

      {/* Edit User Modal */}
      <Modal open={editingUser !== null} onClose={() => setEditingUser(null)}>
        {editingUser && (
          <UserForm
            user={editingUser}
            cameras={cameras}
            onSave={() => { setEditingUser(null); refresh() }}
            onCancel={() => setEditingUser(null)}
          />
        )}
      </Modal>

      {/* Change Password Modal */}
      <ChangePasswordModal
        open={showPasswordModal}
        onClose={() => setShowPasswordModal(false)}
      />

      {/* Delete Confirmation */}
      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete User"
        message="Are you sure you want to delete this user? This action cannot be undone."
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={() => confirmDeleteId && handleDelete(confirmDeleteId)}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </div>
  )
}
