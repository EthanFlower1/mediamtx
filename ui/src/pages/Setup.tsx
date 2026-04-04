import { useState, useEffect, FormEvent, useMemo, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/context'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

type PasswordStrength = 'weak' | 'medium' | 'strong'

interface DiscoveredCamera {
  name: string
  xaddr: string
  hardware?: string
  manufacturer?: string
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function getPasswordStrength(pw: string): PasswordStrength {
  if (pw.length >= 12) return 'strong'
  if (pw.length >= 8) return 'medium'
  return 'weak'
}

const strengthConfig: Record<PasswordStrength, { label: string; color: string; width: string }> = {
  weak: { label: 'Weak', color: 'bg-nvr-danger', width: 'w-1/3' },
  medium: { label: 'Medium', color: 'bg-nvr-warning', width: 'w-2/3' },
  strong: { label: 'Strong', color: 'bg-nvr-success', width: 'w-full' },
}

const STEP_LABELS = ['Admin Account', 'Storage Path', 'Camera Discovery', 'Recording Schedule']

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

/* ------------------------------------------------------------------ */
/*  Step indicator                                                     */
/* ------------------------------------------------------------------ */

function StepIndicator({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center gap-2 mb-6">
      {Array.from({ length: total }, (_, i) => {
        const step = i + 1
        const isActive = step === current
        const isComplete = step < current
        return (
          <div key={step} className="flex items-center gap-2">
            <span
              className={`w-6 h-6 rounded-full text-xs font-bold flex items-center justify-center transition-colors ${
                isActive
                  ? 'bg-nvr-accent text-white'
                  : isComplete
                    ? 'bg-nvr-success text-white'
                    : 'bg-nvr-bg-tertiary text-nvr-text-muted'
              }`}
            >
              {isComplete ? (
                <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                </svg>
              ) : (
                step
              )}
            </span>
            {i < total - 1 && (
              <div
                className={`w-6 h-0.5 rounded-full transition-colors ${
                  isComplete ? 'bg-nvr-success' : 'bg-nvr-bg-tertiary'
                }`}
              />
            )}
          </div>
        )
      })}
      <span className="text-xs text-nvr-text-muted ml-2">
        Step {current} of {total} &mdash; {STEP_LABELS[current - 1]}
      </span>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 1: Admin Account                                              */
/* ------------------------------------------------------------------ */

function AdminAccountStep({
  onComplete,
}: {
  onComplete: (username: string, password: string) => void
}) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const strength = useMemo(() => getPasswordStrength(password), [password])
  const sc = strengthConfig[strength]

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')

    if (password !== confirm) {
      setError('Passwords do not match.')
      return
    }

    if (password.length < 6) {
      setError('Password must be at least 6 characters.')
      return
    }

    setLoading(true)
    try {
      const res = await fetch('/api/nvr/auth/setup', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      })
      if (!res.ok) {
        const data = await res.json().catch(() => null)
        setError(data?.error ?? 'Setup failed. Please try again.')
        setLoading(false)
        return
      }

      onComplete(username, password)
    } catch {
      setError('Setup failed. Please try again.')
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-3">
      <div>
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Username</label>
        <input
          type="text"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          required
          autoComplete="username"
          className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        />
      </div>

      <div>
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Password</label>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="Choose a strong password"
          required
          autoComplete="new-password"
          className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        />
        {password.length > 0 && (
          <div className="mt-2">
            <div className="h-1 rounded-full bg-nvr-bg-tertiary overflow-hidden">
              <div className={`h-full rounded-full ${sc.color} ${sc.width} transition-all duration-300`} />
            </div>
            <p
              className={`text-xs mt-1 ${
                strength === 'weak'
                  ? 'text-nvr-danger'
                  : strength === 'medium'
                    ? 'text-nvr-warning'
                    : 'text-nvr-success'
              }`}
            >
              {sc.label}
            </p>
          </div>
        )}
      </div>

      <div>
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Confirm Password</label>
        <input
          type="password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          placeholder="Re-enter password"
          required
          autoComplete="new-password"
          className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        />
      </div>

      {error && <ErrorBanner message={error} />}

      <button
        type="submit"
        disabled={loading}
        className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed mt-1 flex items-center justify-center gap-2"
      >
        {loading ? <Spinner label="Creating account..." /> : 'Create Admin Account'}
      </button>
    </form>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 2: Storage Path                                               */
/* ------------------------------------------------------------------ */

function StoragePathStep({
  onNext,
  onBack,
}: {
  onNext: (storagePath: string) => void
  onBack: (() => void) | undefined
}) {
  const [storagePath, setStoragePath] = useState('./recordings')
  const [checking, setChecking] = useState(false)
  const [diskInfo, setDiskInfo] = useState<{
    total_bytes: number
    free_bytes: number
    used_bytes: number
  } | null>(null)
  const [error, setError] = useState('')

  // Try to load current storage info
  useEffect(() => {
    apiFetch('/system/storage')
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setDiskInfo({
            total_bytes: data.total_bytes,
            free_bytes: data.free_bytes,
            used_bytes: data.used_bytes,
          })
        }
      })
      .catch(() => {})
  }, [])

  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
  }

  const handleNext = () => {
    if (!storagePath.trim()) {
      setError('Please enter a storage path.')
      return
    }
    setChecking(true)
    // We just validate non-empty and move on -- the path is configured server-side
    // via mediamtx.yml, and this step is informational/advisory.
    setTimeout(() => {
      setChecking(false)
      onNext(storagePath)
    }, 300)
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">
          Recording Storage Path
        </label>
        <input
          type="text"
          value={storagePath}
          onChange={(e) => {
            setStoragePath(e.target.value)
            setError('')
          }}
          placeholder="/mnt/recordings"
          className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-4 py-3 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors font-mono"
        />
        <p className="text-xs text-nvr-text-muted mt-1.5">
          Directory where video recordings will be stored. Ensure sufficient disk space.
        </p>
      </div>

      {diskInfo && (
        <div className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-3">
          <p className="text-xs font-medium text-nvr-text-secondary mb-2">Current Disk Usage</p>
          <div className="flex items-center gap-3 text-xs">
            <div className="flex-1">
              <div className="h-2 rounded-full bg-nvr-bg-primary overflow-hidden">
                <div
                  className="h-full rounded-full bg-nvr-accent transition-all"
                  style={{
                    width: `${Math.round((diskInfo.used_bytes / diskInfo.total_bytes) * 100)}%`,
                  }}
                />
              </div>
            </div>
            <span className="text-nvr-text-muted whitespace-nowrap">
              {formatBytes(diskInfo.free_bytes)} free of {formatBytes(diskInfo.total_bytes)}
            </span>
          </div>
        </div>
      )}

      {error && <ErrorBanner message={error} />}

      <div className="flex gap-3 mt-1">
        {onBack && (
          <button
            type="button"
            onClick={onBack}
            className="flex-1 bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-semibold text-sm px-4 py-3 rounded-lg transition-colors"
          >
            Back
          </button>
        )}
        <button
          type="button"
          onClick={handleNext}
          disabled={checking}
          className="flex-1 bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {checking ? <Spinner label="Checking..." /> : 'Next'}
        </button>
      </div>

      <button
        type="button"
        onClick={() => onNext(storagePath)}
        className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors text-center"
      >
        Skip this step
      </button>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 3: Camera Discovery                                           */
/* ------------------------------------------------------------------ */

function CameraDiscoveryStep({
  onNext,
  onBack,
}: {
  onNext: (addedCount: number) => void
  onBack: () => void
}) {
  const [discovering, setDiscovering] = useState(false)
  const [cameras, setCameras] = useState<DiscoveredCamera[]>([])
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [adding, setAdding] = useState(false)
  const [addedCount, setAddedCount] = useState(0)
  const [error, setError] = useState('')
  const [discovered, setDiscovered] = useState(false)
  const [onvifUser, setOnvifUser] = useState('admin')
  const [onvifPass, setOnvifPass] = useState('')
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [])

  const startDiscovery = useCallback(async () => {
    setDiscovering(true)
    setError('')
    setCameras([])
    setSelected(new Set())
    setDiscovered(false)

    try {
      const res = await apiFetch('/cameras/discover', { method: 'POST' })
      if (!res.ok) {
        const data = await res.json().catch(() => null)
        setError(data?.error ?? 'Discovery failed.')
        setDiscovering(false)
        return
      }

      // Poll for results
      pollRef.current = setInterval(async () => {
        try {
          const statusRes = await apiFetch('/cameras/discover/status')
          if (statusRes.ok) {
            const statusData = await statusRes.json()
            if (statusData.status === 'complete' || statusData.status === 'idle') {
              if (pollRef.current) clearInterval(pollRef.current)
              pollRef.current = null

              const resultsRes = await apiFetch('/cameras/discover/results')
              if (resultsRes.ok) {
                const resultsData = await resultsRes.json()
                const found = resultsData.cameras ?? resultsData ?? []
                setCameras(found)
                // Auto-select all found cameras
                setSelected(new Set(found.map((c: DiscoveredCamera) => c.xaddr)))
              }
              setDiscovering(false)
              setDiscovered(true)
            }
          }
        } catch {
          // ignore polling errors
        }
      }, 1500)
    } catch {
      setError('Network error during discovery.')
      setDiscovering(false)
    }
  }, [])

  const toggleCamera = (xaddr: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(xaddr)) next.delete(xaddr)
      else next.add(xaddr)
      return next
    })
  }

  const addSelected = async () => {
    if (selected.size === 0) {
      onNext(0)
      return
    }

    setAdding(true)
    setError('')
    let added = 0

    for (const cam of cameras) {
      if (!selected.has(cam.xaddr)) continue
      try {
        const res = await apiFetch('/cameras', {
          method: 'POST',
          body: JSON.stringify({
            name: cam.name || `Camera ${added + 1}`,
            onvif_endpoint: cam.xaddr,
            onvif_username: onvifUser,
            onvif_password: onvifPass,
          }),
        })
        if (res.ok) added++
      } catch {
        // continue with other cameras
      }
    }

    setAddedCount(added)
    setAdding(false)
    onNext(added)
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-nvr-text-secondary">
        Discover ONVIF cameras on your network. Enter credentials that will be used to connect to
        discovered cameras.
      </p>

      {/* ONVIF credentials */}
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">
            Camera Username
          </label>
          <input
            type="text"
            value={onvifUser}
            onChange={(e) => setOnvifUser(e.target.value)}
            placeholder="admin"
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2.5 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">
            Camera Password
          </label>
          <input
            type="password"
            value={onvifPass}
            onChange={(e) => setOnvifPass(e.target.value)}
            placeholder="password"
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2.5 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />
        </div>
      </div>

      {/* Discover button */}
      <button
        type="button"
        onClick={startDiscovery}
        disabled={discovering}
        className="w-full bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-primary font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 flex items-center justify-center gap-2 border border-nvr-border"
      >
        {discovering ? (
          <Spinner label="Scanning network..." />
        ) : (
          <>
            <svg
              className="w-4 h-4"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"
              />
            </svg>
            {discovered ? 'Scan Again' : 'Discover Cameras'}
          </>
        )}
      </button>

      {/* Results */}
      {discovered && cameras.length === 0 && (
        <div className="text-center py-4">
          <p className="text-sm text-nvr-text-muted">No cameras found on the network.</p>
          <p className="text-xs text-nvr-text-muted mt-1">
            You can add cameras manually later from the Cameras page.
          </p>
        </div>
      )}

      {cameras.length > 0 && (
        <div className="space-y-2 max-h-48 overflow-y-auto">
          {cameras.map((cam) => (
            <label
              key={cam.xaddr}
              className={`flex items-center gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                selected.has(cam.xaddr)
                  ? 'border-nvr-accent bg-nvr-accent/5'
                  : 'border-nvr-border bg-nvr-bg-tertiary hover:bg-nvr-bg-tertiary/80'
              }`}
            >
              <input
                type="checkbox"
                checked={selected.has(cam.xaddr)}
                onChange={() => toggleCamera(cam.xaddr)}
                className="w-4 h-4 rounded border-nvr-border text-nvr-accent focus:ring-nvr-accent bg-nvr-bg-input"
              />
              <div className="flex-1 min-w-0">
                <p className="text-sm font-medium text-nvr-text-primary truncate">
                  {cam.name || 'Unknown Camera'}
                </p>
                <p className="text-xs text-nvr-text-muted truncate">
                  {cam.manufacturer && `${cam.manufacturer} - `}
                  {cam.xaddr}
                </p>
              </div>
            </label>
          ))}
        </div>
      )}

      {error && <ErrorBanner message={error} />}

      <div className="flex gap-3 mt-1">
        <button
          type="button"
          onClick={onBack}
          className="flex-1 bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-semibold text-sm px-4 py-3 rounded-lg transition-colors"
        >
          Back
        </button>
        <button
          type="button"
          onClick={addSelected}
          disabled={adding}
          className="flex-1 bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {adding ? (
            <Spinner label="Adding cameras..." />
          ) : cameras.length > 0 ? (
            `Add ${selected.size} Camera${selected.size !== 1 ? 's' : ''} & Continue`
          ) : (
            'Next'
          )}
        </button>
      </div>

      <button
        type="button"
        onClick={() => onNext(0)}
        className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors text-center"
      >
        Skip this step
      </button>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 4: Recording Schedule                                         */
/* ------------------------------------------------------------------ */

function RecordingScheduleStep({
  onNext,
  onBack,
  camerasAdded,
}: {
  onNext: () => void
  onBack: () => void
  camerasAdded: number
}) {
  const [mode, setMode] = useState<'always' | 'events' | 'off'>('always')
  const [selectedDays, setSelectedDays] = useState<number[]>([0, 1, 2, 3, 4, 5, 6])
  const [startTime, setStartTime] = useState('00:00')
  const [endTime, setEndTime] = useState('23:59')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const toggleDay = (day: number) => {
    setSelectedDays((prev) =>
      prev.includes(day) ? prev.filter((d) => d !== day) : [...prev, day].sort()
    )
  }

  const handleFinish = async () => {
    if (mode === 'off' || camerasAdded === 0) {
      onNext()
      return
    }

    setSaving(true)
    setError('')

    try {
      // Get all cameras to apply the default rule
      const camRes = await apiFetch('/cameras')
      if (!camRes.ok) {
        setError('Failed to fetch cameras.')
        setSaving(false)
        return
      }
      const allCameras = await camRes.json()

      // Create a recording rule for each camera
      for (const cam of allCameras) {
        try {
          await apiFetch(`/cameras/${cam.id}/recording-rules`, {
            method: 'POST',
            body: JSON.stringify({
              name: 'Default Schedule',
              mode,
              days: selectedDays,
              start_time: startTime,
              end_time: endTime,
              post_event_seconds: mode === 'events' ? 30 : 0,
              enabled: true,
            }),
          })
        } catch {
          // continue with other cameras
        }
      }

      setSaving(false)
      onNext()
    } catch {
      setError('Failed to create recording schedule.')
      setSaving(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {camerasAdded === 0 ? (
        <div className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-3">
          <p className="text-xs text-nvr-text-muted">
            No cameras were added. You can configure recording schedules later from the Cameras page.
          </p>
        </div>
      ) : (
        <>
          <p className="text-xs text-nvr-text-secondary">
            Choose a default recording schedule for your {camerasAdded} camera
            {camerasAdded !== 1 ? 's' : ''}. You can customize per-camera schedules later.
          </p>

          {/* Recording mode */}
          <div>
            <label className="block text-xs font-medium text-nvr-text-secondary mb-2">
              Recording Mode
            </label>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  {
                    value: 'always',
                    label: 'Always',
                    desc: 'Record continuously',
                    color: 'nvr-accent',
                  },
                  {
                    value: 'events',
                    label: 'Events Only',
                    desc: 'Record on motion',
                    color: 'nvr-warning',
                  },
                  { value: 'off', label: 'Off', desc: 'No recording', color: 'nvr-text-muted' },
                ] as const
              ).map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setMode(opt.value)}
                  className={`flex flex-col items-center p-3 rounded-lg border text-center transition-colors ${
                    mode === opt.value
                      ? `border-${opt.color} bg-${opt.color}/10`
                      : 'border-nvr-border bg-nvr-bg-tertiary hover:bg-nvr-bg-tertiary/80'
                  }`}
                >
                  <span
                    className={`text-sm font-semibold ${
                      mode === opt.value ? `text-${opt.color}` : 'text-nvr-text-primary'
                    }`}
                  >
                    {opt.label}
                  </span>
                  <span className="text-xs text-nvr-text-muted mt-0.5">{opt.desc}</span>
                </button>
              ))}
            </div>
          </div>

          {/* Schedule details (only if not off) */}
          {mode !== 'off' && (
            <>
              {/* Days */}
              <div>
                <label className="block text-xs font-medium text-nvr-text-secondary mb-2">
                  Active Days
                </label>
                <div className="flex gap-1.5">
                  {DAY_NAMES.map((name, i) => (
                    <button
                      key={i}
                      type="button"
                      onClick={() => toggleDay(i)}
                      className={`w-9 h-9 rounded-lg text-xs font-semibold transition-colors ${
                        selectedDays.includes(i)
                          ? 'bg-nvr-accent text-white'
                          : 'bg-nvr-bg-tertiary text-nvr-text-muted hover:bg-nvr-border'
                      }`}
                    >
                      {name.charAt(0)}
                    </button>
                  ))}
                </div>
              </div>

              {/* Time range */}
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">
                    Start Time
                  </label>
                  <input
                    type="time"
                    value={startTime}
                    onChange={(e) => setStartTime(e.target.value)}
                    className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2.5 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">
                    End Time
                  </label>
                  <input
                    type="time"
                    value={endTime}
                    onChange={(e) => setEndTime(e.target.value)}
                    className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2.5 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  />
                </div>
              </div>
            </>
          )}
        </>
      )}

      {error && <ErrorBanner message={error} />}

      <div className="flex gap-3 mt-1">
        <button
          type="button"
          onClick={onBack}
          className="flex-1 bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-semibold text-sm px-4 py-3 rounded-lg transition-colors"
        >
          Back
        </button>
        <button
          type="button"
          onClick={handleFinish}
          disabled={saving}
          className="flex-1 bg-nvr-success hover:bg-nvr-success/90 text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 flex items-center justify-center gap-2"
        >
          {saving ? <Spinner label="Saving..." /> : 'Finish Setup'}
        </button>
      </div>

      <button
        type="button"
        onClick={onNext}
        className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors text-center"
      >
        Skip this step
      </button>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Shared small components                                            */
/* ------------------------------------------------------------------ */

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-3 bg-nvr-danger/10 border-l-4 border-nvr-danger rounded-r-lg px-4 py-3">
      <svg
        className="w-4 h-4 text-nvr-danger mt-0.5 shrink-0"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth={2}
      >
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
      <p className="text-sm text-nvr-danger">{message}</p>
    </div>
  )
}

function Spinner({ label }: { label: string }) {
  return (
    <>
      <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
        <circle
          className="opacity-25"
          cx="12"
          cy="12"
          r="10"
          stroke="currentColor"
          strokeWidth="4"
        />
        <path
          className="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
        />
      </svg>
      {label}
    </>
  )
}

function SuccessScreen() {
  return (
    <div className="flex flex-col items-center gap-4 py-6">
      <div className="w-12 h-12 rounded-full bg-nvr-success/20 flex items-center justify-center">
        <svg
          className="w-6 h-6 text-nvr-success"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
        </svg>
      </div>
      <div className="text-center">
        <p className="text-sm font-semibold text-white">Setup complete!</p>
        <p className="text-xs text-nvr-text-muted mt-1">Redirecting to dashboard...</p>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main Wizard                                                        */
/* ------------------------------------------------------------------ */

export default function Setup() {
  const [step, setStep] = useState(1)
  const [complete, setComplete] = useState(false)
  const [storagePath, setStoragePath] = useState('./recordings')
  const [camerasAdded, setCamerasAdded] = useState(0)
  const navigate = useNavigate()
  const { login } = useAuth()

  // Credentials stored from step 1 to auto-login
  const credentialsRef = useRef<{ username: string; password: string } | null>(null)

  // Page title
  useEffect(() => {
    document.title = 'Setup — MediaMTX NVR'
    return () => {
      document.title = 'MediaMTX NVR'
    }
  }, [])

  const handleAccountCreated = async (username: string, password: string) => {
    credentialsRef.current = { username, password }
    // Log in immediately so subsequent API calls are authenticated
    await login(username, password)
    setStep(2)
  }

  const handleStorageNext = (path: string) => {
    setStoragePath(path)
    setStep(3)
  }

  const handleCameraDiscoveryNext = (addedCount: number) => {
    setCamerasAdded(addedCount)
    setStep(4)
  }

  const handleFinish = () => {
    setComplete(true)
    setTimeout(() => navigate('/live'), 1200)
  }

  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-nvr-bg-primary px-4">
      <div className="w-full max-w-md">
        {/* Brand */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-nvr-accent/15 mb-4">
            <svg
              className="w-6 h-6 text-nvr-accent"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z"
              />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-white tracking-tight">Welcome to MediaMTX NVR</h1>
          <p className="text-sm text-nvr-text-secondary mt-2">
            {step === 1
              ? 'Create your admin account to get started.'
              : `Let's finish setting up your NVR system.`}
          </p>
        </div>

        {/* Card */}
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-2xl p-6 shadow-2xl">
          <StepIndicator current={step} total={4} />

          {complete ? (
            <SuccessScreen />
          ) : step === 1 ? (
            <AdminAccountStep onComplete={handleAccountCreated} />
          ) : step === 2 ? (
            <StoragePathStep onNext={handleStorageNext} onBack={undefined} />
          ) : step === 3 ? (
            <CameraDiscoveryStep
              onNext={handleCameraDiscoveryNext}
              onBack={() => setStep(2)}
            />
          ) : (
            <RecordingScheduleStep
              onNext={handleFinish}
              onBack={() => setStep(3)}
              camerasAdded={camerasAdded}
            />
          )}
        </div>

        {/* Footer */}
        <p className="text-center text-xs text-nvr-text-muted mt-6">Powered by MediaMTX</p>
      </div>
    </div>
  )
}
