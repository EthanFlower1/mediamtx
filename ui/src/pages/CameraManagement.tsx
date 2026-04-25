import { useState, useEffect, FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useCameras, Camera } from '../hooks/useCameras'
import { apiFetch } from '../api/client'
import CameraSettings from '../components/CameraSettings'
import DetectionZoneEditor from '../components/DetectionZoneEditor'
import AnalyticsConfig from '../components/AnalyticsConfig'
import RelayControls from '../components/RelayControls'
import ConfirmDialog from '../components/ConfirmDialog'

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

/** AI Detection toggle + sub stream URL for a camera. */
function AIDetectionPanel({ camera, onRefresh }: { camera: Camera; onRefresh: () => void }) {
  const [aiEnabled, setAiEnabled] = useState(camera.ai_enabled ?? false)
  const [subStreamUrl, setSubStreamUrl] = useState(camera.sub_stream_url ?? '')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  const handleToggleAI = async () => {
    const newEnabled = !aiEnabled
    setSaving(true)
    try {
      const res = await apiFetch(`/cameras/${camera.id}/ai`, {
        method: 'PUT',
        body: JSON.stringify({ ai_enabled: newEnabled, sub_stream_url: subStreamUrl }),
      })
      if (res.ok) {
        setAiEnabled(newEnabled)
        onRefresh()
      }
    } finally {
      setSaving(false)
    }
  }

  const handleSaveSubStream = async () => {
    setSaving(true)
    setSaved(false)
    try {
      const res = await apiFetch(`/cameras/${camera.id}/ai`, {
        method: 'PUT',
        body: JSON.stringify({ ai_enabled: aiEnabled, sub_stream_url: subStreamUrl }),
      })
      if (res.ok) {
        setSaved(true)
        onRefresh()
        setTimeout(() => setSaved(false), 2000)
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="mb-4 p-3 border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
      <h4 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide mb-1">AI Detection</h4>
      <p className="text-xs text-nvr-text-muted mb-3">Enable AI object detection on this camera's sub stream</p>

      {/* AI Toggle */}
      <div className="flex items-center justify-between mb-3">
        <span className="text-sm text-nvr-text-primary">Enable AI</span>
        <button
          onClick={handleToggleAI}
          disabled={saving}
          className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors shrink-0 ml-4 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
            aiEnabled ? 'bg-nvr-accent' : 'bg-nvr-bg-tertiary border border-nvr-border'
          }`}
          role="switch"
          aria-checked={aiEnabled}
        >
          <span
            className={`inline-block h-4 w-4 rounded-full bg-white transition-transform ${
              aiEnabled ? 'translate-x-6' : 'translate-x-1'
            }`}
          />
        </button>
      </div>

      {/* Sub Stream URL */}
      {aiEnabled && (
        <div className="mb-3">
          <label className="text-xs text-nvr-text-secondary mb-1 block">Detection Stream (Sub Stream)</label>
          <p className="text-xs text-nvr-text-muted mb-2">Select a low-resolution stream for AI processing. MJPEG recommended.</p>
          <input
            type="text"
            value={subStreamUrl}
            onChange={e => setSubStreamUrl(e.target.value)}
            placeholder="rtsp://camera-ip/sub_stream"
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-2.5 py-1.5 text-xs text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors mb-2"
          />
          <button
            onClick={handleSaveSubStream}
            disabled={saving}
            className="text-xs bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1 rounded transition-colors disabled:opacity-50"
          >
            {saving ? 'Saving...' : saved ? 'Saved!' : 'Save'}
          </button>
        </div>
      )}

      {aiEnabled && subStreamUrl && (
        <div className="text-xs text-nvr-success flex items-center gap-1">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
          </svg>
          AI detection active
        </div>
      )}
    </div>
  )
}

export default function CameraManagement() {
  const navigate = useNavigate()
  const { cameras, loading, refresh } = useCameras()

  // Page title
  useEffect(() => {
    document.title = 'Cameras — Raikada'
    return () => { document.title = 'Raikada' }
  }, [])

  // Search filter state
  const [searchQuery, setSearchQuery] = useState('')

  // Add camera modal state
  const [showAddModal, setShowAddModal] = useState(false)
  const [addName, setAddName] = useState('')
  const [addRtspUrl, setAddRtspUrl] = useState('')
  const [addOnvifEndpoint, setAddOnvifEndpoint] = useState('')
  const [addUsername, setAddUsername] = useState('')
  const [addPassword, setAddPassword] = useState('')

  // Clipboard state
  const [copiedCameraId, setCopiedCameraId] = useState<string | null>(null)

  // Camera detail state
  const [expandedCameraId, setExpandedCameraId] = useState<string | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)

  // Edit camera modal state
  const [editCamera, setEditCamera] = useState<Camera | null>(null)
  const [editName, setEditName] = useState('')
  const [editRtspUrl, setEditRtspUrl] = useState('')
  const [editOnvifEndpoint, setEditOnvifEndpoint] = useState('')
  const [editUsername, setEditUsername] = useState('')
  const [editPassword, setEditPassword] = useState('')
  const [editSaving, setEditSaving] = useState(false)
  const [editError, setEditError] = useState('')

  // Escape to close modals
  useEffect(() => {
    if (!showAddModal && !editCamera) return
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (editCamera) closeEditModal()
        else if (showAddModal) resetForm()
      }
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [showAddModal, editCamera])

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
      }),
    })
    if (res.ok) {
      resetForm()
      refresh()
    }
  }

  const resetForm = () => {
    setShowAddModal(false)
    setAddName('')
    setAddRtspUrl('')
    setAddOnvifEndpoint('')
    setAddUsername('')
    setAddPassword('')
  }

  const handleDelete = async (id: string) => {
    await apiFetch(`/cameras/${id}`, { method: 'DELETE' })
    if (expandedCameraId === id) setExpandedCameraId(null)
    setConfirmDeleteId(null)
    refresh()
  }

  const openEditModal = (cam: Camera) => {
    setEditCamera(cam)
    setEditName(cam.name)
    setEditRtspUrl(cam.rtsp_url)
    setEditOnvifEndpoint(cam.onvif_endpoint ?? '')
    setEditUsername('')
    setEditPassword('')
    setEditError('')
    setEditSaving(false)
  }

  const closeEditModal = () => {
    setEditCamera(null)
    setEditName('')
    setEditRtspUrl('')
    setEditOnvifEndpoint('')
    setEditUsername('')
    setEditPassword('')
    setEditError('')
    setEditSaving(false)
  }

  const handleEditSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!editCamera) return
    setEditSaving(true)
    setEditError('')

    const body: Record<string, string | boolean> = {
      name: editName,
      rtsp_url: editRtspUrl,
    }
    if (editOnvifEndpoint) body.onvif_endpoint = editOnvifEndpoint
    if (editUsername) body.onvif_username = editUsername
    if (editPassword) body.onvif_password = editPassword

    try {
      const res = await apiFetch(`/cameras/${editCamera.id}`, {
        method: 'PUT',
        body: JSON.stringify(body),
      })
      if (res.ok) {
        closeEditModal()
        refresh()
      } else {
        const data = await res.json().catch(() => ({}))
        setEditError(data.error || 'Failed to update camera')
      }
    } catch {
      setEditError('Network error')
    } finally {
      setEditSaving(false)
    }
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
            onClick={() => { resetForm(); setShowAddModal(true) }}
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
            Add a camera by entering its RTSP URL manually.
          </p>
          <button
            onClick={() => { resetForm(); setShowAddModal(true) }}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-5 py-2.5 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Add Camera
          </button>
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
                    <span
                      className={`absolute -bottom-0.5 -right-0.5 w-3 h-3 rounded-full border-2 border-nvr-bg-secondary ${
                        cam.status === 'online'
                          ? 'bg-nvr-success'
                          : cam.status === 'error'
                          ? 'bg-nvr-warning'
                          : 'bg-nvr-danger'
                      }`}
                      title={cam.status === 'online' ? 'Online' : cam.status === 'error' ? 'Error' : 'Offline'}
                    />
                  </div>

                  {/* Center: name, URL, badges */}
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-1">
                      <span className="text-sm font-medium text-nvr-text-primary">{cam.name}</span>
                      <span className={`inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold uppercase ${
                        cam.status === 'online'
                          ? 'bg-nvr-success/15 text-nvr-success'
                          : cam.status === 'error'
                          ? 'bg-nvr-warning/15 text-nvr-warning'
                          : 'bg-nvr-danger/15 text-nvr-danger'
                      }`}>
                        {cam.status === 'online' ? 'Online' : cam.status === 'error' ? 'Error' : 'Offline'}
                      </span>
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
                      {cam.status !== 'online' && cam.updated_at && (
                        <span className="text-nvr-text-muted text-[10px]" title={new Date(cam.updated_at).toLocaleString()}>
                          Last updated: {relativeTime(cam.updated_at)}
                        </span>
                      )}
                    </div>
                    {/* Capability badges */}
                    <div className="flex flex-wrap items-center gap-1.5 mt-2">
                      {cam.ptz_capable && (
                        <span className="text-[11px] bg-nvr-accent/10 text-nvr-accent px-2 py-0.5 rounded-full font-medium">PTZ</span>
                      )}
                      {cam.supports_audio_backchannel && (
                        <span className="text-[11px] bg-nvr-accent/10 text-nvr-accent px-2 py-0.5 rounded-full font-medium">Audio</span>
                      )}
                      {cam.supports_analytics && (
                        <span className="text-[11px] bg-amber-500/10 text-amber-400 px-2 py-0.5 rounded-full font-medium">Analytics</span>
                      )}
                      {cam.supports_edge_recording && (
                        <span className="text-[11px] bg-emerald-500/10 text-emerald-400 px-2 py-0.5 rounded-full font-medium">SD Card</span>
                      )}
                      {cam.supports_relay && (
                        <span className="text-[11px] bg-purple-500/10 text-purple-400 px-2 py-0.5 rounded-full font-medium">Relay</span>
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
                      onClick={() => openEditModal(cam)}
                      className="p-2 rounded-lg text-nvr-text-muted hover:text-nvr-accent hover:bg-nvr-accent/10 transition-colors min-w-[40px] min-h-[40px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      aria-label="Edit"
                      title="Edit"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                        <path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z" />
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
              </div>

              {/* Expanded inline detail */}
              {expandedCamera && expandedCameraId === cam.id && (
                <div className="mt-1 bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
                  {/* Header with optional image settings toggle */}
                  <div className="flex items-center gap-2 mb-4">
                    <span className="text-sm font-semibold text-nvr-text-primary">Camera Settings</span>
                    {(expandedCamera.onvif_endpoint && expandedCamera.supports_imaging) && (
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

                  {showSettings && expandedCamera.onvif_endpoint && expandedCamera.supports_imaging && (
                    <div className="mb-4 p-3 border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
                      <p className="text-xs text-nvr-text-muted mb-2">Adjust brightness, contrast, and other image parameters on the camera</p>
                      <CameraSettings
                        cameraId={expandedCamera.id}
                        onClose={() => setShowSettings(false)}
                      />
                    </div>
                  )}

                  {/* AI Detection Settings */}
                  <AIDetectionPanel camera={expandedCamera} onRefresh={refresh} />

                  {/* Relay Controls -- only visible when camera supports relay outputs */}
                  {expandedCamera.supports_relay && (
                    <div className="mt-4">
                      <div className="mb-1">
                        <p className="text-xs text-nvr-text-muted">Control physical outputs on your camera (sirens, lights, door locks)</p>
                      </div>
                      <RelayControls cameraId={expandedCamera.id} />
                    </div>
                  )}

                  {/* Motion Detection Zones */}
                  <div className="mb-4 p-3 border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
                    <h4 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide mb-1">Motion Detection Zones</h4>
                    {expandedCamera.supports_analytics ? (
                      <>
                        <p className="text-xs text-nvr-text-muted mb-3">Draw zones on the camera view to define where motion should be detected</p>
                        <DetectionZoneEditor
                          cameraId={expandedCamera.id}
                          snapshotUrl={expandedCamera.snapshot_uri || undefined}
                        />
                      </>
                    ) : (
                      <div className="py-3">
                        <p className="text-sm text-nvr-text-secondary mb-2">
                          This camera manages motion detection zones through its own web interface.
                        </p>
                        {expandedCamera.onvif_endpoint && (
                          <a
                            href={`http://${new URL(expandedCamera.onvif_endpoint).hostname}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="inline-flex items-center gap-1.5 text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors"
                          >
                            <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M18 13v6a2 2 0 01-2 2H5a2 2 0 01-2-2V8a2 2 0 012-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" y1="14" x2="21" y2="3"/></svg>
                            Open Camera Web Interface
                          </a>
                        )}
                        <p className="text-xs text-nvr-text-muted mt-2">
                          Motion events from this camera are still received and shown on the recordings timeline.
                        </p>
                      </div>
                    )}
                  </div>

                  {/* Analytics modules & rules -- only visible when camera supports analytics */}
                  {expandedCamera.supports_analytics && (
                    <div className="mb-4 p-3 border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
                      <h4 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide mb-1">Analytics</h4>
                      <p className="text-xs text-nvr-text-muted mb-3">Configure motion detection rules and analytics modules</p>
                      <AnalyticsConfig cameraId={expandedCamera.id} />
                    </div>
                  )}
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

            <form onSubmit={handleAddCamera} className="p-5">
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

              <div className="mb-4">
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">ONVIF Endpoint <span className="text-nvr-text-muted font-normal">(optional)</span></label>
                <input
                  type="text"
                  value={addOnvifEndpoint}
                  onChange={e => setAddOnvifEndpoint(e.target.value)}
                  placeholder="http://192.168.1.100:80/onvif/device_service"
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
                <p className="text-xs text-nvr-text-muted mt-1">Required for PTZ, imaging, and analytics features.</p>
              </div>

              <div className="mb-4">
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">ONVIF Credentials <span className="text-nvr-text-muted font-normal">(optional)</span></label>
                <div className="flex gap-2">
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
              </div>

              <button
                type="submit"
                disabled={!addRtspUrl || !addName}
                className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                Add Camera
              </button>
            </form>
          </div>
        </div>
      )}

      {/* Edit Camera Modal */}
      {editCamera && (
        <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={closeEditModal}>
          <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
          <div
            className="relative z-10 bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl w-full max-w-lg mx-4 max-h-[90vh] overflow-y-auto"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Modal header */}
            <div className="flex items-center justify-between p-5 border-b border-nvr-border">
              <h2 className="text-lg font-semibold text-nvr-text-primary">Edit Camera</h2>
              <div className="flex items-center gap-2">
                <span className="text-nvr-text-muted text-xs">Press Esc to close</span>
                <button
                  onClick={closeEditModal}
                  className="text-nvr-text-muted hover:text-nvr-text-secondary min-w-[40px] min-h-[40px] flex items-center justify-center rounded-lg hover:bg-nvr-bg-tertiary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
            </div>

            <form onSubmit={handleEditSubmit} className="p-5">
              {editError && (
                <div className="mb-4 p-3 bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg">
                  <p className="text-nvr-danger text-sm">{editError}</p>
                </div>
              )}

              <div className="mb-4">
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Camera Name</label>
                <input
                  type="text"
                  value={editName}
                  onChange={e => setEditName(e.target.value)}
                  required
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>

              <div className="mb-4">
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">RTSP URL</label>
                <input
                  type="text"
                  value={editRtspUrl}
                  onChange={e => setEditRtspUrl(e.target.value)}
                  placeholder="rtsp://user:pass@192.168.1.100:554/stream"
                  required
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
              </div>

              <div className="mb-4">
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">ONVIF Endpoint</label>
                <input
                  type="text"
                  value={editOnvifEndpoint}
                  onChange={e => setEditOnvifEndpoint(e.target.value)}
                  placeholder="http://192.168.1.100:80/onvif/device_service"
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                />
                <p className="text-xs text-nvr-text-muted mt-1">Optional. Required for PTZ, imaging, and analytics features.</p>
              </div>

              <div className="mb-4">
                <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">ONVIF Credentials</label>
                <p className="text-xs text-nvr-text-muted mb-2">Leave blank to keep existing credentials unchanged.</p>
                <div className="flex gap-2">
                  <input
                    type="text"
                    placeholder="Username"
                    value={editUsername}
                    onChange={e => setEditUsername(e.target.value)}
                    className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  />
                  <input
                    type="password"
                    placeholder="Password"
                    value={editPassword}
                    onChange={e => setEditPassword(e.target.value)}
                    className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  />
                </div>
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={closeEditModal}
                  className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={editSaving || !editName || !editRtspUrl}
                  className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  {editSaving ? 'Saving...' : 'Save Changes'}
                </button>
              </div>
            </form>
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
