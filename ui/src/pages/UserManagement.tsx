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
      <h1>User Management</h1>
      <button onClick={() => setShowAdd(!showAdd)}>Add User</button>

      {showAdd && (
        <form onSubmit={handleAdd} style={{ margin: '16px 0', padding: 12, border: '1px solid #ccc' }}>
          <div><label>Username</label><input name="username" required /></div>
          <div><label>Password</label><input name="password" type="password" required /></div>
          <div>
            <label>Role</label>
            <select name="role">
              <option value="viewer">Viewer</option>
              <option value="admin">Admin</option>
            </select>
          </div>
          <button type="submit">Create</button>
        </form>
      )}

      <table style={{ width: '100%', marginTop: 16 }}>
        <thead><tr><th>Username</th><th>Role</th><th>Actions</th></tr></thead>
        <tbody>
          {users.map(u => (
            <tr key={u.id}>
              <td>{u.username}</td>
              <td>{u.role}</td>
              <td><button onClick={() => handleDelete(u.id)}>Delete</button></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
