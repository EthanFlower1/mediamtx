import { useState, useEffect, FormEvent } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import { apiFetch } from '../api/client'
import RecordingRules from '../components/RecordingRules'

/** Small component to fetch and display the effective recording mode for a camera. */
function RecordingModeBadge({ cameraId }: { cameraId: string }) {
  const [mode, setMode] = useState<string | null>(null)

  useEffect(() => {
    apiFetch(`/cameras/${cameraId}/recording-status`).then(async res => {
      if (res.ok) {
        const data = await res.json()
        setMode(data.effective_mode)
      }
    })
  }, [cameraId])

  if (mode === null) return <span style={{ color: '#6b7280' }}>--</span>

  const styles: Record<string, React.CSSProperties> = {
    always: { color: '#60a5fa', background: 'rgba(59,130,246,0.15)', padding: '2px 8px', borderRadius: 9999, fontSize: 12, fontWeight: 600 },
    events: { color: '#fbbf24', background: 'rgba(245,158,11,0.15)', padding: '2px 8px', borderRadius: 9999, fontSize: 12, fontWeight: 600 },
    off: { color: '#6b7280', background: 'rgba(107,114,128,0.15)', padding: '2px 8px', borderRadius: 9999, fontSize: 12, fontWeight: 600 },
  }

  return <span style={styles[mode] ?? styles.off}>{mode === 'always' ? 'Always' : mode === 'events' ? 'Events' : 'Off'}</span>
}

export default function CameraManagement() {
  const { cameras, loading, refresh } = useCameras()
  const [showAdd, setShowAdd] = useState(false)
  const [discovering, setDiscovering] = useState(false)
  const [discovered, setDiscovered] = useState<any[]>([])
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

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
    if (selectedCamera?.id === id) setSelectedCamera(null)
    refresh()
  }

  const handleRowClick = (cam: Camera) => {
    setSelectedCamera(prev => prev?.id === cam.id ? null : cam)
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
            <th>Name</th><th>Status</th><th>Recording</th><th>RTSP URL</th><th>PTZ</th><th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {cameras.map(cam => (
            <tr
              key={cam.id}
              onClick={() => handleRowClick(cam)}
              style={{
                cursor: 'pointer',
                background: selectedCamera?.id === cam.id ? 'rgba(59,130,246,0.1)' : 'transparent',
              }}
            >
              <td>{cam.name}</td>
              <td style={{ color: cam.status === 'online' ? 'green' : 'red' }}>{cam.status}</td>
              <td><RecordingModeBadge cameraId={cam.id} /></td>
              <td>{cam.rtsp_url}</td>
              <td>{cam.ptz_capable ? 'Yes' : 'No'}</td>
              <td>
                <button onClick={(e) => { e.stopPropagation(); handleDelete(cam.id) }}>Delete</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {selectedCamera && (
        <div style={{ marginTop: 24, padding: 16, border: '1px solid #2a2a3e', borderRadius: 8, background: '#12121e' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
            <h2 style={{ margin: 0, fontSize: 18 }}>
              Recording Rules &mdash; {selectedCamera.name}
            </h2>
            <button
              onClick={() => setSelectedCamera(null)}
              style={{ background: 'none', border: 'none', color: '#6b7280', cursor: 'pointer', fontSize: 18 }}
            >
              &times;
            </button>
          </div>
          <RecordingRules cameraId={selectedCamera.id} />
        </div>
      )}
    </div>
  )
}
