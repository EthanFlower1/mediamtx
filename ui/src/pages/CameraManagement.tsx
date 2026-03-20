import { useState, useEffect, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useCameras, Camera } from '../hooks/useCameras'
import { apiFetch } from '../api/client'
import RecordingRules from '../components/RecordingRules'
import CameraSettings from '../components/CameraSettings'
import ConfirmDialog from '../components/ConfirmDialog'

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

  if (mode === null) return <span className="text-nvr-text-muted">--</span>

  const styles: Record<string, string> = {
    always: 'bg-nvr-accent/15 text-nvr-accent',
    events: 'bg-nvr-warning/15 text-nvr-warning',
    off: 'bg-nvr-text-muted/15 text-nvr-text-muted',
  }

  const label: Record<string, string> = {
    always: 'Always',
    events: 'Events',
    off: 'Off',
  }

  return (
    <span className={`inline-block px-2 py-0.5 rounded-full text-xs font-semibold ${styles[mode] ?? styles.off}`}>
      {label[mode] ?? 'Off'}
    </span>
  )
}

/** Thin timeline strip showing today's recording coverage for a camera. */
function RecordingMinimap({ mediamtxPath }: { mediamtxPath: string }) {
  const [ranges, setRanges] = useState<{ startPct: number; widthPct: number }[]>([])

  useEffect(() => {
    if (!mediamtxPath) return

    const today = new Date()
    const dayStart = new Date(today.getFullYear(), today.getMonth(), today.getDate()).getTime()
    const dayMs = 24 * 60 * 60 * 1000

    fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${mediamtxPath}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: { segments?: { start: string }[] } | null) => {
        if (!data?.segments) return

        const dayEnd = dayStart + dayMs
        const filtered = data.segments.filter(s => {
          const t = new Date(s.start).getTime()
          return t >= dayStart && t < dayEnd
        })

        if (filtered.length === 0) return

        // Build merged ranges
        const merged: { start: number; end: number }[] = []
        for (let i = 0; i < filtered.length; i++) {
          const segStart = new Date(filtered[i].start).getTime()
          const segEnd = i + 1 < filtered.length
            ? new Date(filtered[i + 1].start).getTime()
            : segStart + 5 * 60 * 1000

          if (merged.length > 0 && segStart - merged[merged.length - 1].end < 10000) {
            merged[merged.length - 1].end = segEnd
          } else {
            merged.push({ start: segStart, end: segEnd })
          }
        }

        setRanges(merged.map(r => ({
          startPct: ((r.start - dayStart) / dayMs) * 100,
          widthPct: Math.max(((r.end - r.start) / dayMs) * 100, 0.5),
        })))
      })
      .catch(() => {})
  }, [mediamtxPath])

  if (ranges.length === 0) return null

  return (
    <div className="w-full h-[2px] bg-nvr-bg-tertiary rounded-full overflow-hidden mt-3 relative">
      {ranges.map((r, i) => (
        <div
          key={i}
          className="absolute top-0 h-full bg-nvr-accent rounded-full"
          style={{ left: `${r.startPct}%`, width: `${r.widthPct}%` }}
        />
      ))}
    </div>
  )
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

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return 'just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export default function CameraManagement() {
  const navigate = useNavigate()
  const { cameras, loading, refresh } = useCameras()

  // Page title
  useEffect(() => {
    document.title = 'Cameras — MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  // Search filter state
  const [searchQuery, setSearchQuery] = useState('')

  // Add camera modal state
  const [showAddModal, setShowAddModal] = useState(false)
  const [addTab, setAddTab] = useState<'discover' | 'manual'>('discover')
  const [discovering, setDiscovering] = useState(false)
  const [discovered, setDiscovered] = useState<DiscoveredCamera[]>([])
  const [selectedDevice, setSelectedDevice] = useState<DiscoveredCamera | null>(null)
  const [selectedProfile, setSelectedProfile] = useState<DiscoveredProfile | null>(null)
  const [addName, setAddName] = useState('')
  const [addRtspUrl, setAddRtspUrl] = useState('')
  const [addOnvifEndpoint, setAddOnvifEndpoint] = useState('')
  const [addUsername, setAddUsername] = useState('')
  const [addPassword, setAddPassword] = useState('')
  const [probing, setProbing] = useState(false)
  const [probeError, setProbeError] = useState('')
  const [probedProfiles, setProbedProfiles] = useState<DiscoveredProfile[]>([])

  // Clipboard state
  const [copiedCameraId, setCopiedCameraId] = useState<string | null>(null)

  // Camera detail state
  const [expandedCameraId, setExpandedCameraId] = useState<string | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)

  // Escape to close modal
  useEffect(() => {
    if (!showAddModal) return
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') resetForm()
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [showAddModal])

  const handleDiscover = async () => {
    setDiscovering(true)
    setDiscovered([])
    setSelectedDevice(null)
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
    setShowAddModal(false)
    setAddTab('discover')
    setSelectedDevice(null)
    setSelectedProfile(null)
    setAddName('')
    setAddRtspUrl('')
    setAddOnvifEndpoint('')
    setAddUsername('')
    setAddPassword('')
    setProbing(false)
    setProbeError('')
    setProbedProfiles([])
    setDiscovered([])
    setDiscovering(false)
  }

  const handleDelete = async (id: string) => {
    await apiFetch(`/cameras/${id}`, { method: 'DELETE' })
    if (expandedCameraId === id) setExpandedCameraId(null)
    setConfirmDeleteId(null)
    refresh()
  }

  const toggleExpanded = (cam: Camera) => {
    if (expandedCameraId === cam.id) {
      setExpandedCameraId(null)
      setShowSettings(false)
    } else {
      setExpandedCameraId(cam.id)
      setShowSettings(false)
    }
  }

  const openAddModal = (tab: 'discover' | 'manual') => {
    resetForm()
    setAddTab(tab)
    setShowAddModal(true)
  }

  if (loading) {
    return (
      <div>
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Cameras</h1>
        </div>
        <div className="flex flex-col gap-3">
          {Array.from({ length: 3 }, (_, i) => (
            <div key={i} className="bg-nvr-bg-secondary rounded-xl p-4 animate-pulse">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-full bg-nvr-bg-tertiary" />
                <div className="flex-1 space-y-2">
                  <div className="h-4 bg-nvr-bg-tertiary rounded w-1/3" />
                  <div className="h-3 bg-nvr-bg-tertiary rounded w-2/3" />
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    )
  }

  const hasProfiles = probedProfiles.length > 0
  const expandedCamera = cameras.find(c => c.id === expandedCameraId) ?? null
  const filteredCameras = searchQuery
    ? cameras.filter(c => c.name.toLowerCase().includes(searchQuery.toLowerCase()))
    : cameras

  return (
    <div>
      {/* Page header */}
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Cameras</h1>
        {cameras.length > 0 && (
          <button
            onClick={() => openAddModal('discover')}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Add Camera
          </button>
        )}
      </div>

      {/* Empty state */}
      {cameras.length === 0 && !showAddModal && (
        <div className="flex flex-col items-center justify-center h-80 text-center px-4">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-16 h-16 text-nvr-text-muted mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
          </svg>
          <h2 className="text-xl font-semibold text-nvr-text-primary mb-2">No cameras configured</h2>
          <p className="text-sm text-nvr-text-muted mb-6 max-w-md">
            Scan your network for ONVIF cameras or enter an RTSP URL manually.
          </p>
          <div className="flex gap-3">
            <button
              onClick={() => openAddModal('discover')}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-5 py-2.5 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Discover Cameras
            </button>
            <button
              onClick={() => openAddModal('manual')}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-5 py-2.5 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Add Manually
            </button>
          </div>
        </div>
      )}

      {/* Camera search and cards */}
      {cameras.length > 0 && (
        <div className="flex flex-col gap-3">
          {/* Search input */}
          <input
            type="text"
            placeholder="Search cameras..."
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />

          {/* No results */}
          {filteredCameras.length === 0 && searchQuery && (
            <p className="text-sm text-nvr-text-muted text-center py-6">No cameras match "{searchQuery}"</p>
          )}

          {filteredCameras.map(cam => (
            <div key={cam.id}>
              {/* Camera card */}
              <div
                onClick={() => toggleExpanded(cam)}
                className={`bg-nvr-bg-secondary border rounded-xl p-4 cursor-pointer transition-colors ${
                  expandedCameraId === cam.id
                    ? 'border-nvr-accent/40 bg-nvr-accent/5'
                    : 'border-nvr-border hover:bg-nvr-bg-tertiary/50'
                }`}
              >
                <div className="flex items-center gap-4">
                  {/* Left: icon + status dot */}
                  <div className="relative shrink-0">
                    <div className="w-12 h-12 rounded-lg bg-nvr-bg-tertiary flex items-center justify-center">
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-6 h-6 text-nvr-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
                      </svg>
                    </div>
                    <span className={`absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full border-2 border-nvr-bg-secondary ${
                      cam.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'
                    }`} />
                  </div>

                  {/* Center: name, URL, badges */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-sm font-medium text-nvr-text-primary">{cam.name}</span>
                      {cam.ptz_capable && (
                        <span className="bg-nvr-accent/10 text-nvr-accent px-1.5 py-0.5 rounded text-[10px] font-semibold uppercase">PTZ</span>
                      )}
                    </div>
                    <div className="flex items-center gap-3 text-xs text-nvr-text-muted">
                      <span className="truncate max-w-[280px] font-mono">{cam.rtsp_url}</span>
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          navigator.clipboard.writeText(cam.rtsp_url)
                          setCopiedCameraId(cam.id)
                          setTimeout(() => setCopiedCameraId(prev => prev === cam.id ? null : prev), 2000)
                        }}
                        className="shrink-0 p-1 rounded hover:bg-nvr-bg-tertiary transition-colors"
                        title="Copy RTSP URL"
                      >
                        {copiedCameraId === cam.id ? (
                          <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5 text-nvr-success" viewBox="0 0 20 20" fill="currentColor">
                            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                          </svg>
                        ) : (
                          <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                            <path d="M8 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z" />
                            <path d="M6 3a2 2 0 00-2 2v11a2 2 0 002 2h8a2 2 0 002-2V5a2 2 0 00-2-2 3 3 0 01-3 3H9a3 3 0 01-3-3z" />
                          </svg>
                        )}
                      </button>
                      <RecordingModeBadge cameraId={cam.id} />
                      {cam.status !== 'online' && cam.updated_at && (
                        <span className="text-nvr-text-muted text-[10px]" title={new Date(cam.updated_at).toLocaleString()}>
                          Last updated: {relativeTime(cam.updated_at)}
                        </span>
                      )}
                    </div>
                  </div>

                  {/* Right: action buttons */}
                  <div className="flex items-center gap-1 shrink-0" onClick={(e) => e.stopPropagation()}>
                    <button
                      onClick={() => navigate('/live')}
                      className="p-2 rounded-lg text-nvr-text-muted hover:text-nvr-accent hover:bg-nvr-accent/10 transition-colors min-w-[40px] min-h-[40px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      aria-label="View Live"
                      title="View Live"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <circle cx="12" cy="12" r="3" />
                        <path strokeLinecap="round" strokeLinejoin="round" d="M16.24 7.76a6 6 0 010 8.49m-8.48-.01a6 6 0 010-8.49" />
                      </svg>
                    </button>
                    <button
                      onClick={() => {
                        localStorage.setItem('nvr-recordings-camera', cam.id)
                        navigate('/recordings')
                      }}
                      className="p-2 rounded-lg text-nvr-text-muted hover:text-nvr-accent hover:bg-nvr-accent/10 transition-colors min-w-[40px] min-h-[40px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      aria-label="Recordings"
                      title="Recordings"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
                      </svg>
                    </button>
                    <button
                      onClick={() => toggleExpanded(cam)}
                      className="p-2 rounded-lg text-nvr-text-muted hover:text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-w-[40px] min-h-[40px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      aria-label="Settings"
                      title="Settings"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                        <path fillRule="evenodd" d="M11.49 3.17c-.38-1.56-2.6-1.56-2.98 0a1.532 1.532 0 01-2.286.948c-1.372-.836-2.942.734-2.106 2.106.54.886.061 2.042-.947 2.287-1.561.379-1.561 2.6 0 2.978a1.532 1.532 0 01.947 2.287c-.836 1.372.734 2.942 2.106 2.106a1.532 1.532 0 012.287.947c.379 1.561 2.6 1.561 2.978 0a1.533 1.533 0 012.287-.947c1.372.836 2.942-.734 2.106-2.106a1.533 1.533 0 01.947-2.287c1.561-.379 1.561-2.6 0-2.978a1.532 1.532 0 01-.947-2.287c.836-1.372-.734-2.942-2.106-2.106a1.532 1.532 0 01-2.287-.947zM10 13a3 3 0 100-6 3 3 0 000 6z" clipRule="evenodd" />
                      </svg>
                    </button>
                    <button
                      onClick={() => setConfirmDeleteId(cam.id)}
                      className="p-2 rounded-lg text-nvr-text-muted hover:text-nvr-danger hover:bg-nvr-danger/10 transition-colors min-w-[40px] min-h-[40px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      aria-label="Delete"
                      title="Delete"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                        <path fillRule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clipRule="evenodd" />
                      </svg>
                    </button>
                  </div>
                </div>

                {/* Recording timeline minimap — today's coverage */}
                {cam.mediamtx_path && <RecordingMinimap mediamtxPath={cam.mediamtx_path} />}
              </div>

              {/* Expanded inline detail */}
              {expandedCamera && expandedCameraId === cam.id && (
                <div className="mt-1 bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
                  {/* Tabs for Recording Rules / Camera Settings */}
                  <div className="flex items-center gap-2 mb-4">
                    <span className="text-sm font-semibold text-nvr-text-primary">Recording Rules</span>
                    {expandedCamera.onvif_endpoint && (
                      <button
                        onClick={() => setShowSettings(!showSettings)}
                        className={`text-xs font-medium px-2.5 py-1 rounded-md border transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                          showSettings
                            ? 'bg-nvr-accent/20 border-nvr-accent text-nvr-accent'
                            : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-secondary hover:bg-nvr-border/30'
                        }`}
                      >
                        Image Settings
                      </button>
                    )}
                  </div>

                  {showSettings && expandedCamera.onvif_endpoint && (
                    <div className="mb-4 p-3 border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
                      <CameraSettings
                        cameraId={expandedCamera.id}
                        onClose={() => setShowSettings(false)}
                      />
                    </div>
                  )}

                  <RecordingRules cameraId={expandedCamera.id} />
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Add Camera Modal */}
      {showAddModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={resetForm}>
          <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
          <div
            className="relative z-10 bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal header */}
            <div className="flex items-center justify-between p-5 border-b border-nvr-border">
              <h2 className="text-lg font-semibold text-nvr-text-primary">Add Camera</h2>
              <div className="flex items-center gap-2">
                <span className="text-nvr-text-muted text-xs">Press Esc to close</span>
                <button
                  onClick={resetForm}
                  className="text-nvr-text-muted hover:text-nvr-text-secondary min-w-[40px] min-h-[40px] flex items-center justify-center rounded-lg hover:bg-nvr-bg-tertiary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
            </div>

            {/* Tabs */}
            <div className="flex border-b border-nvr-border">
              <button
                onClick={() => { setAddTab('discover'); setSelectedDevice(null); setProbedProfiles([]); setAddRtspUrl(''); setAddName('') }}
                className={`flex-1 py-3 text-sm font-medium text-center transition-colors border-b-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                  addTab === 'discover'
                    ? 'border-nvr-accent text-nvr-accent'
                    : 'border-transparent text-nvr-text-muted hover:text-nvr-text-secondary'
                }`}
              >
                Discover
              </button>
              <button
                onClick={() => { setAddTab('manual'); setSelectedDevice(null); setProbedProfiles([]); setAddRtspUrl(''); setAddName(''); setAddOnvifEndpoint('') }}
                className={`flex-1 py-3 text-sm font-medium text-center transition-colors border-b-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                  addTab === 'manual'
                    ? 'border-nvr-accent text-nvr-accent'
                    : 'border-transparent text-nvr-text-muted hover:text-nvr-text-secondary'
                }`}
              >
                Manual
              </button>
            </div>

            <div className="p-5">
              {/* Discover tab */}
              {addTab === 'discover' && !selectedDevice && (
                <div>
                  <button
                    onClick={handleDiscover}
                    disabled={discovering}
                    className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm mb-4 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    {discovering ? (
                      <span className="flex items-center justify-center gap-2">
                        <span className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                        Scanning network...
                      </span>
                    ) : 'Scan for Cameras'}
                  </button>

                  {discovered.length > 0 && (
                    <div className="flex flex-col gap-2">
                      <p className="text-xs text-nvr-text-muted mb-1">
                        Found {discovered.length} camera{discovered.length !== 1 ? 's' : ''}
                      </p>
                      {discovered.map((d, i) => (
                        <button
                          key={i}
                          onClick={() => handleSelectDevice(d)}
                          className="w-full text-left p-3 border border-nvr-border rounded-lg hover:bg-nvr-bg-tertiary/50 hover:border-nvr-accent/30 transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                        >
                          <div className="font-medium text-sm text-nvr-text-primary">
                            {d.manufacturer} {d.model}
                          </div>
                          <div className="text-xs text-nvr-text-muted truncate mt-0.5">{d.xaddr}</div>
                        </button>
                      ))}
                    </div>
                  )}

                  {!discovering && discovered.length === 0 && (
                    <p className="text-xs text-nvr-text-muted text-center py-4">
                      Click scan to discover ONVIF cameras on your network.
                    </p>
                  )}
                </div>
              )}

              {/* Discover tab: selected device form */}
              {addTab === 'discover' && selectedDevice && (
                <form onSubmit={handleAddCamera}>
                  <div className="flex items-center gap-2 mb-4">
                    <button
                      type="button"
                      onClick={() => { setSelectedDevice(null); setProbedProfiles([]); setAddRtspUrl('') }}
                      className="text-nvr-text-muted hover:text-nvr-text-secondary text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
                    >
                      &larr; Back
                    </button>
                    <span className="text-sm font-medium text-nvr-text-primary">
                      {selectedDevice.manufacturer} {selectedDevice.model}
                    </span>
                  </div>

                  <div className="mb-4">
                    <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Camera Name</label>
                    <input
                      type="text"
                      value={addName}
                      onChange={e => setAddName(e.target.value)}
                      required
                      className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                    />
                  </div>

                  <div className="mb-4">
                    <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Credentials</label>
                    <div className="flex gap-2 mb-2">
                      <input
                        type="text"
                        placeholder="Username"
                        value={addUsername}
                        onChange={e => setAddUsername(e.target.value)}
                        className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                      />
                      <input
                        type="password"
                        placeholder="Password"
                        value={addPassword}
                        onChange={e => setAddPassword(e.target.value)}
                        className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                      />
                    </div>
                    <button
                      type="button"
                      onClick={handleProbe}
                      disabled={probing || !addUsername}
                      className="w-full bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium py-2 rounded-lg border border-nvr-border transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                    >
                      {probing ? 'Fetching...' : 'Fetch Streams'}
                    </button>
                    {probeError && <p className="text-nvr-danger text-xs mt-2">{probeError}</p>}
                  </div>

                  {hasProfiles && (
                    <div className="mb-4">
                      <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Stream Profile</label>
                      <div className="flex gap-2 flex-wrap">
                        {probedProfiles.map(p => (
                          <button
                            key={p.token}
                            type="button"
                            onClick={() => handleSelectProfile(p)}
                            className={`px-3 py-1.5 rounded-lg text-xs transition-colors border focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                              selectedProfile?.token === p.token
                                ? 'bg-nvr-accent/20 border-nvr-accent text-nvr-accent'
                                : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-secondary hover:bg-nvr-border/30'
                            }`}
                          >
                            {p.name} - {p.width}x{p.height} {p.video_codec}
                          </button>
                        ))}
                      </div>
                    </div>
                  )}

                  <button
                    type="submit"
                    disabled={!addRtspUrl}
                    className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    Add Camera
                  </button>
                </form>
              )}

              {/* Manual tab */}
              {addTab === 'manual' && (
                <form onSubmit={handleAddCamera}>
                  <div className="mb-4">
                    <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Camera Name</label>
                    <input
                      type="text"
                      value={addName}
                      onChange={e => setAddName(e.target.value)}
                      placeholder="e.g. Front Door"
                      required
                      className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                    />
                  </div>

                  <div className="mb-4">
                    <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">RTSP URL</label>
                    <input
                      type="text"
                      value={addRtspUrl}
                      onChange={e => setAddRtspUrl(e.target.value)}
                      placeholder="rtsp://user:pass@192.168.1.100:554/stream"
                      required
                      className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                    />
                  </div>

                  <button
                    type="submit"
                    disabled={!addRtspUrl || !addName}
                    className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    Add Camera
                  </button>
                </form>
              )}
            </div>
          </div>
        </div>
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Camera"
        message="Are you sure you want to delete this camera? This action cannot be undone."
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={() => confirmDeleteId && handleDelete(confirmDeleteId)}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </div>
  )
}
