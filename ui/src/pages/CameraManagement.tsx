import { useState, FormEvent } from 'react'
import { useCameras } from '../hooks/useCameras'
import { apiFetch } from '../api/client'

export default function CameraManagement() {
  const { cameras, loading, refresh } = useCameras()
  const [showAdd, setShowAdd] = useState(false)
  const [discovering, setDiscovering] = useState(false)
  const [discovered, setDiscovered] = useState<any[]>([])

  const handleDiscover = async () => {
    setDiscovering(true)
    const res = await apiFetch('/cameras/discover', { method: 'POST' })
    if (!res.ok) {
      setDiscovering(false)
      return
    }

    const poll = setInterval(async () => {
      const statusRes = await apiFetch('/cameras/discover/status')
      if (statusRes.ok) {
        const data = await statusRes.json()
        if (data.status === 'complete') {
          clearInterval(poll)
          const resultsRes = await apiFetch('/cameras/discover/results')
          if (resultsRes.ok) setDiscovered(await resultsRes.json())
          setDiscovering(false)
        }
      }
    }, 2000)
  }

  const handleAddCamera = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const formData = new FormData(e.currentTarget)
    const res = await apiFetch('/cameras', {
      method: 'POST',
      body: JSON.stringify({
        name: formData.get('name'),
        rtsp_url: formData.get('rtsp_url'),
        record: formData.get('record') === 'on',
      }),
    })
    if (res.ok) {
      setShowAdd(false)
      refresh()
    }
  }

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this camera?')) return
    await apiFetch(`/cameras/${id}`, { method: 'DELETE' })
    refresh()
  }

  if (loading) return <div>Loading...</div>

  return (
    <div>
      <h1>Camera Management</h1>

      <div style={{ marginBottom: 16 }}>
        <button onClick={() => setShowAdd(!showAdd)}>Add Camera</button>
        <button onClick={handleDiscover} disabled={discovering} style={{ marginLeft: 8 }}>
          {discovering ? 'Scanning...' : 'Discover ONVIF Cameras'}
        </button>
      </div>

      {discovered.length > 0 && (
        <div style={{ marginBottom: 16, padding: 12, border: '1px solid #ccc' }}>
          <h3>Discovered Cameras</h3>
          {discovered.map((d, i) => (
            <div key={i} style={{ marginBottom: 8 }}>
              <strong>{d.manufacturer} {d.model}</strong> — {d.xaddr}
              <button onClick={() => {
                setShowAdd(true)
              }} style={{ marginLeft: 8 }}>Add</button>
            </div>
          ))}
        </div>
      )}

      {showAdd && (
        <form onSubmit={handleAddCamera} style={{ marginBottom: 16, padding: 12, border: '1px solid #ccc' }}>
          <h3>Add Camera</h3>
          <div><label>Name</label><input name="name" required /></div>
          <div><label>RTSP URL</label><input name="rtsp_url" required /></div>
          <div><label><input name="record" type="checkbox" /> Enable Recording</label></div>
          <button type="submit">Add</button>
        </form>
      )}

      <table style={{ width: '100%', borderCollapse: 'collapse' }}>
        <thead>
          <tr>
            <th>Name</th><th>Status</th><th>RTSP URL</th><th>PTZ</th><th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {cameras.map(cam => (
            <tr key={cam.id}>
              <td>{cam.name}</td>
              <td style={{ color: cam.status === 'online' ? 'green' : 'red' }}>{cam.status}</td>
              <td>{cam.rtsp_url}</td>
              <td>{cam.ptz_capable ? 'Yes' : 'No'}</td>
              <td>
                <button onClick={() => handleDelete(cam.id)}>Delete</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
