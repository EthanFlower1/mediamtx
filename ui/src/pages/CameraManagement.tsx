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

interface DiscoveredProfile {
  token: string
  name: string
  stream_uri: string
  video_codec?: string
  width?: number
  height?: number
}

interface DiscoveredCamera {
  xaddr: string
  manufacturer: string
  model: string
  firmware: string
  profiles?: DiscoveredProfile[]
}

export default function CameraManagement() {
  const { cameras, loading, refresh } = useCameras()
  const [showAdd, setShowAdd] = useState(false)
  const [discovering, setDiscovering] = useState(false)
  const [discovered, setDiscovered] = useState<DiscoveredCamera[]>([])
  const [selectedDevice, setSelectedDevice] = useState<DiscoveredCamera | null>(null)
  const [selectedProfile, setSelectedProfile] = useState<DiscoveredProfile | null>(null)
  const [addName, setAddName] = useState('')
  const [addRtspUrl, setAddRtspUrl] = useState('')
  const [addOnvifEndpoint, setAddOnvifEndpoint] = useState('')
  const [addUsername, setAddUsername] = useState('')
  const [addPassword, setAddPassword] = useState('')
  const [manualMode, setManualMode] = useState(false)
  const [probing, setProbing] = useState(false)
  const [probeError, setProbeError] = useState('')
  const [probedProfiles, setProbedProfiles] = useState<DiscoveredProfile[]>([])
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

  const handleDiscover = async () => {
    setDiscovering(true)
    setDiscovered([])
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
    }, 1000)
  }

  const handleSelectDevice = (dev: DiscoveredCamera) => {
    setSelectedDevice(dev)
    setAddName(`${dev.manufacturer} ${dev.model}`.trim() || 'Camera')
    setAddOnvifEndpoint(dev.xaddr)
    setManualMode(false)
    setProbeError('')
    setProbedProfiles([])
    setAddUsername('')
    setAddPassword('')

    if (dev.profiles && dev.profiles.length > 0) {
      setProbedProfiles(dev.profiles)
      setSelectedProfile(dev.profiles[0])
      setAddRtspUrl(dev.profiles[0].stream_uri)
    } else {
      setSelectedProfile(null)
      setAddRtspUrl('')
    }

    setShowAdd(true)
  }

  const handleProbe = async () => {
    if (!addOnvifEndpoint) return
    setProbing(true)
    setProbeError('')

    const res = await apiFetch('/cameras/probe', {
      method: 'POST',
      body: JSON.stringify({
        xaddr: addOnvifEndpoint,
        username: addUsername,
        password: addPassword,
      }),
    })

    if (res.ok) {
      const data = await res.json()
      const profiles: DiscoveredProfile[] = data.profiles || []
      setProbedProfiles(profiles)
      if (profiles.length > 0) {
        setSelectedProfile(profiles[0])
        setAddRtspUrl(profiles[0].stream_uri)
      }
    } else {
      const data = await res.json().catch(() => ({}))
      setProbeError(data.error || 'Failed to probe camera')
    }

    setProbing(false)
  }

  const handleSelectProfile = (p: DiscoveredProfile) => {
    setSelectedProfile(p)
    setAddRtspUrl(p.stream_uri)
  }

  const handleAddCamera = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()

    const res = await apiFetch('/cameras', {
      method: 'POST',
      body: JSON.stringify({
        name: addName,
        rtsp_url: addRtspUrl,
        onvif_endpoint: addOnvifEndpoint,
        onvif_username: addUsername,
        onvif_password: addPassword,
        onvif_profile_token: selectedProfile?.token || '',
      }),
    })
    if (res.ok) {
      resetForm()
      refresh()
    }
  }

  const resetForm = () => {
    setShowAdd(false)
    setSelectedDevice(null)
    setSelectedProfile(null)
    setAddName('')
    setAddRtspUrl('')
    setAddOnvifEndpoint('')
    setAddUsername('')
    setAddPassword('')
    setManualMode(false)
    setProbing(false)
    setProbeError('')
    setProbedProfiles([])
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

  if (loading) return <div className="text-nvr-text-secondary">Loading...</div>

  const hasProfiles = probedProfiles.length > 0

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-nvr-text-primary">Camera Management</h1>
        <div className="flex gap-2">
          <button
            onClick={handleDiscover}
            disabled={discovering}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
          >
            {discovering ? 'Scanning...' : 'Discover Cameras'}
          </button>
          <button
            onClick={() => { resetForm(); setManualMode(true); setShowAdd(true) }}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors"
          >
            Add Manually
          </button>
        </div>
      </div>

      {discovered.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5 mb-6">
          <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">Discovered Cameras</h3>
          {discovered.map((d, i) => (
            <div
              key={i}
              className="flex items-center justify-between px-3 py-2.5 mb-2 last:mb-0 border border-nvr-border rounded-lg bg-nvr-bg-tertiary hover:bg-nvr-border/30 transition-colors"
            >
              <div>
                <span className="font-medium text-nvr-text-primary">{d.manufacturer} {d.model}</span>
                {d.firmware && <span className="text-nvr-text-muted ml-2 text-sm">v{d.firmware}</span>}
                <div className="text-sm text-nvr-text-secondary">{d.xaddr}</div>
              </div>
              <button
                onClick={() => handleSelectDevice(d)}
                className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors shrink-0"
              >
                Add
              </button>
            </div>
          ))}
        </div>
      )}

      {showAdd && (
        <form onSubmit={handleAddCamera} className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5 mb-6">
          <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">
            {manualMode ? 'Add Camera Manually' : `Add ${selectedDevice?.manufacturer || ''} ${selectedDevice?.model || ''}`.trim()}
          </h3>

          <div className="mb-4">
            <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Camera Name</label>
            <input
              type="text"
              value={addName}
              onChange={e => setAddName(e.target.value)}
              required
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>

          {!manualMode && (
            <>
              <div className="mb-4">
                <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Camera Credentials</label>
                <div className="flex gap-2">
                  <input
                    type="text"
                    placeholder="Username"
                    value={addUsername}
                    onChange={e => setAddUsername(e.target.value)}
                    className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  />
                  <input
                    type="password"
                    placeholder="Password"
                    value={addPassword}
                    onChange={e => setAddPassword(e.target.value)}
                    className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  />
                  <button
                    type="button"
                    onClick={handleProbe}
                    disabled={probing || !addUsername}
                    className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 shrink-0"
                  >
                    {probing ? 'Connecting...' : 'Fetch Streams'}
                  </button>
                </div>
                {probeError && <p className="text-nvr-danger text-sm mt-1">{probeError}</p>}
              </div>

              {hasProfiles && (
                <div className="mb-4">
                  <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">Stream Profile</label>
                  <div className="flex gap-2 flex-wrap">
                    {probedProfiles.map(p => (
                      <button
                        key={p.token}
                        type="button"
                        onClick={() => handleSelectProfile(p)}
                        className={`px-3 py-1.5 rounded-lg text-sm transition-colors border ${
                          selectedProfile?.token === p.token
                            ? 'bg-nvr-accent/20 border-nvr-accent text-nvr-accent'
                            : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-secondary hover:bg-nvr-border/30'
                        }`}
                      >
                        {p.name} — {p.width}x{p.height} {p.video_codec}
                      </button>
                    ))}
                  </div>
                </div>
              )}

              {hasProfiles && selectedProfile?.stream_uri && (
                <div className="mb-4 text-sm text-nvr-text-muted break-all">
                  Stream URL: {selectedProfile.stream_uri.replace(/\/\/[^@]+@/, '//***:***@')}
                </div>
              )}
            </>
          )}

          {(manualMode || (!hasProfiles && !probing)) && (
            <div className="mb-4">
              <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">RTSP URL</label>
              <input
                type="text"
                value={addRtspUrl}
                onChange={e => setAddRtspUrl(e.target.value)}
                placeholder="rtsp://user:pass@192.168.1.100:554/stream"
                required={!hasProfiles}
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
              />
            </div>
          )}

          <div className="flex gap-2">
            <button
              type="submit"
              disabled={!addRtspUrl}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50"
            >
              Add Camera
            </button>
            <button
              type="button"
              onClick={resetForm}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {cameras.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-nvr-border">
                <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Name</th>
                <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Status</th>
                <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Recording</th>
                <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">RTSP URL</th>
                <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">PTZ</th>
                <th className="text-left text-xs font-semibold text-nvr-text-muted uppercase tracking-wider px-4 py-3">Actions</th>
              </tr>
            </thead>
            <tbody>
              {cameras.map(cam => (
                <tr
                  key={cam.id}
                  onClick={() => handleRowClick(cam)}
                  className={`border-b border-nvr-border/50 hover:bg-nvr-bg-tertiary/50 transition-colors cursor-pointer ${selectedCamera?.id === cam.id ? 'bg-nvr-accent/10' : ''}`}
                >
                  <td className="px-4 py-3 text-sm text-nvr-text-primary font-medium">{cam.name}</td>
                  <td className="px-4 py-3 text-sm">
                    <span className="flex items-center gap-1.5">
                      <span className={`inline-block w-2 h-2 rounded-full ${cam.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'}`} />
                      <span className={cam.status === 'online' ? 'text-nvr-success' : 'text-nvr-danger'}>{cam.status}</span>
                    </span>
                  </td>
                  <td className="px-4 py-3 text-sm"><RecordingModeBadge cameraId={cam.id} /></td>
                  <td className="px-4 py-3 text-xs text-nvr-text-muted font-mono truncate max-w-xs">{cam.rtsp_url}</td>
                  <td className="px-4 py-3 text-sm text-nvr-text-secondary">{cam.ptz_capable ? 'Yes' : 'No'}</td>
                  <td className="px-4 py-3 text-sm">
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDelete(cam.id) }}
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
      )}

      {selectedCamera && (
        <div className="mt-6 p-4 border border-nvr-border rounded-xl bg-nvr-bg-secondary">
          <div className="flex justify-between items-center mb-4">
            <h2 className="text-lg font-semibold text-nvr-text-primary">
              Recording Rules &mdash; {selectedCamera.name}
            </h2>
            <button
              onClick={() => setSelectedCamera(null)}
              className="text-nvr-text-muted hover:text-nvr-text-secondary text-lg bg-transparent border-none cursor-pointer"
            >
              &times;
            </button>
          </div>
          <RecordingRules cameraId={selectedCamera.id} />
        </div>
      )}

      {cameras.length === 0 && !showAdd && (
        <p className="text-center py-12 text-nvr-text-muted">No cameras configured. Discover cameras on your network or add one manually.</p>
      )}
    </div>
  )
}
