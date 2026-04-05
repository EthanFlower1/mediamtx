import { useState, useEffect, FormEvent, useMemo, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/context'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

type PasswordStrength = 'weak' | 'medium' | 'strong'
type Step = 1 | 2 | 3 | 4

interface DiscoveredDevice {
  xaddr: string
  name: string
  manufacturer: string
  model: string
  firmware_version: string
  hardware_id: string
  existing_camera_id?: string
  profiles?: { name: string; token: string; width: number; height: number; stream_uri: string }[]
}

interface ScanStatus {
  scan_id: string
  status: 'scanning' | 'complete' | 'error'
  devices_found: number
  error?: string
}

interface StorageInfo {
  total_bytes: number
  used_bytes: number
  free_bytes: number
  recordings_bytes: number
  warning: boolean
  critical: boolean
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

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${sizes[i]}`
}

const STEP_LABELS: Record<Step, string> = {
  1: 'Admin Account',
  2: 'Storage',
  3: 'Camera Discovery',
  4: 'Recording Schedule',
}

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

/* ------------------------------------------------------------------ */
/*  Step Indicator                                                     */
/* ------------------------------------------------------------------ */

function StepIndicator({ current }: { current: Step }) {
  const steps: Step[] = [1, 2, 3, 4]
  return (
    <div className="flex items-center gap-1.5 mb-6">
      {steps.map((s) => (
        <div key={s} className="flex items-center gap-1.5">
          <span
            className={`w-7 h-7 rounded-full text-xs font-bold flex items-center justify-center transition-colors ${
              s === current
                ? 'bg-nvr-accent text-white'
                : s < current
                  ? 'bg-nvr-success/20 text-nvr-success'
                  : 'bg-nvr-bg-tertiary text-nvr-text-muted'
            }`}
          >
            {s < current ? (
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={3}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            ) : (
              s
            )}
          </span>
          {s < 4 && (
            <div
              className={`w-6 h-0.5 rounded-full transition-colors ${
                s < current ? 'bg-nvr-success/40' : 'bg-nvr-bg-tertiary'
              }`}
            />
          )}
        </div>
      ))}
      <span className="text-xs text-nvr-text-muted ml-2">
        Step {current} of 4 &mdash; {STEP_LABELS[current]}
      </span>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Error Alert                                                        */
/* ------------------------------------------------------------------ */

function ErrorAlert({ message }: { message: string }) {
  return (
    <div className="flex items-start gap-3 bg-nvr-danger/10 border-l-4 border-nvr-danger rounded-r-lg px-4 py-3">
      <svg className="w-4 h-4 text-nvr-danger mt-0.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
      <p className="text-sm text-nvr-danger">{message}</p>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 1: Admin Account                                              */
/* ------------------------------------------------------------------ */

function StepAdminAccount({
  onComplete,
}: {
  onComplete: () => void
}) {
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [success, setSuccess] = useState(false)
  const { login } = useAuth()

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

      setSuccess(true)
      await login(username, password)
      setTimeout(() => onComplete(), 800)
    } catch {
      setError('Setup failed. Please try again.')
      setLoading(false)
    }
  }

  if (success) {
    return (
      <div className="flex flex-col items-center gap-4 py-6">
        <div className="w-12 h-12 rounded-full bg-nvr-success/20 flex items-center justify-center">
          <svg className="w-6 h-6 text-nvr-success" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <div className="text-center">
          <p className="text-sm font-semibold text-white">Account created!</p>
          <p className="text-xs text-nvr-text-muted mt-1">Continuing setup...</p>
        </div>
      </div>
    )
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
            <p className={`text-xs mt-1 ${
              strength === 'weak' ? 'text-nvr-danger' :
              strength === 'medium' ? 'text-nvr-warning' : 'text-nvr-success'
            }`}>
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

      {error && <ErrorAlert message={error} />}

      <button
        type="submit"
        disabled={loading}
        className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed mt-1 flex items-center justify-center gap-2"
      >
        {loading ? (
          <>
            <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
            Creating account...
          </>
        ) : (
          'Create Admin Account'
        )}
      </button>
    </form>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 2: Storage Path                                               */
/* ------------------------------------------------------------------ */

function StepStorage({
  onNext,
  onBack,
}: {
  onNext: () => void
  onBack: () => void
}) {
  const [storage, setStorage] = useState<StorageInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setLoading(true)
    apiFetch('/system/storage')
      .then(async (res) => {
        if (res.ok) {
          setStorage(await res.json())
        } else {
          setError('Could not load storage information.')
        }
      })
      .catch(() => setError('Could not load storage information.'))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-3 py-8">
        <svg className="w-6 h-6 text-nvr-accent animate-spin" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        <span className="text-sm text-nvr-text-muted">Checking storage...</span>
      </div>
    )
  }

  const usedPct = storage ? Math.round((storage.used_bytes / storage.total_bytes) * 100) : 0

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-nvr-text-secondary">
        Recordings will be stored on the server's local disk. Review your available storage below.
      </p>

      {error && <ErrorAlert message={error} />}

      {storage && (
        <div className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-4 space-y-3">
          {/* Usage bar */}
          <div>
            <div className="flex justify-between text-xs text-nvr-text-muted mb-1.5">
              <span>Disk Usage</span>
              <span>{usedPct}%</span>
            </div>
            <div className="h-2 rounded-full bg-nvr-bg-primary overflow-hidden">
              <div
                className={`h-full rounded-full transition-all ${
                  storage.critical ? 'bg-nvr-danger' : storage.warning ? 'bg-nvr-warning' : 'bg-nvr-accent'
                }`}
                style={{ width: `${usedPct}%` }}
              />
            </div>
          </div>

          {/* Stats */}
          <div className="grid grid-cols-3 gap-3 text-center">
            <div>
              <p className="text-xs text-nvr-text-muted">Total</p>
              <p className="text-sm font-semibold text-nvr-text-primary">{formatBytes(storage.total_bytes)}</p>
            </div>
            <div>
              <p className="text-xs text-nvr-text-muted">Used</p>
              <p className="text-sm font-semibold text-nvr-text-primary">{formatBytes(storage.used_bytes)}</p>
            </div>
            <div>
              <p className="text-xs text-nvr-text-muted">Free</p>
              <p className="text-sm font-semibold text-nvr-text-primary">{formatBytes(storage.free_bytes)}</p>
            </div>
          </div>

          {storage.critical && (
            <div className="flex items-center gap-2 text-xs text-nvr-danger bg-nvr-danger/10 rounded-lg px-3 py-2">
              <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
              </svg>
              <span>Storage is critically low. Consider freeing up disk space before recording.</span>
            </div>
          )}

          {storage.warning && !storage.critical && (
            <div className="flex items-center gap-2 text-xs text-nvr-warning bg-nvr-warning/10 rounded-lg px-3 py-2">
              <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
              </svg>
              <span>Storage is running low. Recordings may be affected.</span>
            </div>
          )}
        </div>
      )}

      <p className="text-xs text-nvr-text-muted">
        Storage path and retention policies can be adjusted later in Settings.
      </p>

      {/* Navigation */}
      <div className="flex justify-between mt-2">
        <button
          onClick={onBack}
          className="text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors px-4 py-2 rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Back
        </button>
        <button
          onClick={onNext}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-6 py-2.5 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Next
        </button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 3: Camera Discovery                                           */
/* ------------------------------------------------------------------ */

function StepCameraDiscovery({
  onNext,
  onBack,
  onCamerasAdded,
}: {
  onNext: () => void
  onBack: () => void
  onCamerasAdded: (ids: string[]) => void
}) {
  const [scanStatus, setScanStatus] = useState<ScanStatus | null>(null)
  const [devices, setDevices] = useState<DiscoveredDevice[]>([])
  const [error, setError] = useState('')
  const [scanning, setScanning] = useState(false)
  const [addingDevice, setAddingDevice] = useState<string | null>(null)
  const [addedDevices, setAddedDevices] = useState<Set<string>>(new Set())
  const [credentials, setCredentials] = useState<Record<string, { username: string; password: string }>>({})
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const stopPolling = useCallback(() => {
    if (pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [])

  useEffect(() => {
    return () => stopPolling()
  }, [stopPolling])

  const pollStatus = useCallback(async () => {
    try {
      const statusRes = await apiFetch('/cameras/discover/status')
      if (statusRes.ok) {
        const status: ScanStatus = await statusRes.json()
        setScanStatus(status)

        if (status.status === 'complete' || status.status === 'error') {
          stopPolling()
          setScanning(false)

          if (status.status === 'complete') {
            const resultsRes = await apiFetch('/cameras/discover/results')
            if (resultsRes.ok) {
              const results = await resultsRes.json()
              setDevices(results ?? [])
            }
          }
          if (status.status === 'error') {
            setError(status.error ?? 'Discovery scan failed.')
          }
        }
      }
    } catch {
      // ignore poll errors
    }
  }, [stopPolling])

  const startScan = async () => {
    setError('')
    setScanning(true)
    setDevices([])
    setScanStatus(null)

    try {
      const res = await apiFetch('/cameras/discover', { method: 'POST' })
      if (res.status === 409) {
        // Scan already in progress, just start polling
      } else if (!res.ok) {
        const data = await res.json().catch(() => null)
        setError(data?.error ?? 'Failed to start discovery scan.')
        setScanning(false)
        return
      }

      // Poll for results
      pollRef.current = setInterval(pollStatus, 1500)
      pollStatus()
    } catch {
      setError('Failed to start discovery scan.')
      setScanning(false)
    }
  }

  const addCamera = async (device: DiscoveredDevice) => {
    const creds = credentials[device.xaddr] ?? { username: '', password: '' }
    setAddingDevice(device.xaddr)
    setError('')

    try {
      // Probe the device first to get profiles
      const probeRes = await apiFetch('/cameras/probe', {
        method: 'POST',
        body: JSON.stringify({
          xaddr: device.xaddr,
          username: creds.username,
          password: creds.password,
        }),
      })

      if (!probeRes.ok) {
        const data = await probeRes.json().catch(() => null)
        setError(data?.error ?? `Failed to probe ${device.name || device.xaddr}. Check credentials.`)
        setAddingDevice(null)
        return
      }

      const probeData = await probeRes.json()

      // Create the camera
      const createRes = await apiFetch('/cameras', {
        method: 'POST',
        body: JSON.stringify({
          name: device.name || device.model || 'Camera',
          onvif_endpoint: device.xaddr,
          onvif_username: creds.username,
          onvif_password: creds.password,
          rtsp_url: probeData.profiles?.[0]?.stream_uri ?? '',
          sub_stream_url: probeData.profiles?.length > 1 ? probeData.profiles[1].stream_uri : '',
        }),
      })

      if (!createRes.ok) {
        const data = await createRes.json().catch(() => null)
        setError(data?.error ?? `Failed to add camera ${device.name || device.xaddr}.`)
        setAddingDevice(null)
        return
      }

      const created = await createRes.json()
      setAddedDevices((prev) => new Set(prev).add(device.xaddr))
      onCamerasAdded([...(Array.from(addedDevices)), created.id])
    } catch {
      setError('Failed to add camera.')
    } finally {
      setAddingDevice(null)
    }
  }

  const updateCredentials = (xaddr: string, field: 'username' | 'password', value: string) => {
    setCredentials((prev) => ({
      ...prev,
      [xaddr]: { ...prev[xaddr] ?? { username: '', password: '' }, [field]: value },
    }))
  }

  const newDevices = devices.filter((d) => !d.existing_camera_id && !addedDevices.has(d.xaddr))
  const existingDevices = devices.filter((d) => d.existing_camera_id || addedDevices.has(d.xaddr))

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-nvr-text-secondary">
        Scan your network for ONVIF-compatible cameras. You can add cameras now or later from the Cameras page.
      </p>

      {/* Scan button */}
      {!scanning && devices.length === 0 && (
        <button
          onClick={startScan}
          className="w-full bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-4 py-3 rounded-lg transition-colors flex items-center justify-center gap-2"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          Scan Network for Cameras
        </button>
      )}

      {/* Scanning state */}
      {scanning && (
        <div className="flex flex-col items-center gap-3 py-6 bg-nvr-bg-tertiary border border-nvr-border rounded-lg">
          <svg className="w-6 h-6 text-nvr-accent animate-spin" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <span className="text-sm text-nvr-text-secondary">Scanning network...</span>
          {scanStatus && (
            <span className="text-xs text-nvr-text-muted">{scanStatus.devices_found} device(s) found so far</span>
          )}
        </div>
      )}

      {error && <ErrorAlert message={error} />}

      {/* Results */}
      {!scanning && devices.length > 0 && (
        <div className="space-y-3">
          {/* New devices */}
          {newDevices.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold text-nvr-text-muted uppercase tracking-wider mb-2">
                New Cameras ({newDevices.length})
              </h3>
              <div className="space-y-2">
                {newDevices.map((device) => {
                  const creds = credentials[device.xaddr] ?? { username: '', password: '' }
                  return (
                    <div
                      key={device.xaddr}
                      className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-3"
                    >
                      <div className="flex items-start justify-between gap-2 mb-2">
                        <div className="min-w-0">
                          <p className="text-sm font-medium text-nvr-text-primary truncate">
                            {device.name || device.model || 'Unknown Camera'}
                          </p>
                          <p className="text-xs text-nvr-text-muted">
                            {device.manufacturer} {device.model && `- ${device.model}`}
                          </p>
                          <p className="text-xs text-nvr-text-muted font-mono mt-0.5">{device.xaddr}</p>
                        </div>
                      </div>

                      {/* Credentials */}
                      <div className="grid grid-cols-2 gap-2 mb-2">
                        <input
                          type="text"
                          placeholder="Username"
                          value={creds.username}
                          onChange={(e) => updateCredentials(device.xaddr, 'username', e.target.value)}
                          className="bg-nvr-bg-input border border-nvr-border rounded px-2.5 py-1.5 text-xs text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                        />
                        <input
                          type="password"
                          placeholder="Password"
                          value={creds.password}
                          onChange={(e) => updateCredentials(device.xaddr, 'password', e.target.value)}
                          className="bg-nvr-bg-input border border-nvr-border rounded px-2.5 py-1.5 text-xs text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                        />
                      </div>

                      <button
                        onClick={() => addCamera(device)}
                        disabled={addingDevice === device.xaddr}
                        className="w-full bg-nvr-accent/15 hover:bg-nvr-accent/25 text-nvr-accent font-medium text-xs px-3 py-1.5 rounded transition-colors disabled:opacity-50 flex items-center justify-center gap-1.5"
                      >
                        {addingDevice === device.xaddr ? (
                          <>
                            <svg className="w-3 h-3 animate-spin" fill="none" viewBox="0 0 24 24">
                              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                            </svg>
                            Adding...
                          </>
                        ) : (
                          <>
                            <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
                            </svg>
                            Add Camera
                          </>
                        )}
                      </button>
                    </div>
                  )
                })}
              </div>
            </div>
          )}

          {/* Already added / existing */}
          {existingDevices.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold text-nvr-text-muted uppercase tracking-wider mb-2">
                Already Added ({existingDevices.length})
              </h3>
              <div className="space-y-2">
                {existingDevices.map((device) => (
                  <div
                    key={device.xaddr}
                    className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-3 opacity-60"
                  >
                    <div className="flex items-center gap-2">
                      <svg className="w-4 h-4 text-nvr-success shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                      </svg>
                      <div className="min-w-0">
                        <p className="text-sm font-medium text-nvr-text-primary truncate">
                          {device.name || device.model || 'Camera'}
                        </p>
                        <p className="text-xs text-nvr-text-muted font-mono">{device.xaddr}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Scan again */}
          <button
            onClick={startScan}
            className="text-xs text-nvr-text-secondary hover:text-nvr-accent transition-colors"
          >
            Scan again
          </button>
        </div>
      )}

      {/* No devices found */}
      {!scanning && scanStatus?.status === 'complete' && devices.length === 0 && (
        <div className="text-center py-6 bg-nvr-bg-tertiary border border-nvr-border rounded-lg">
          <svg className="w-8 h-8 text-nvr-text-muted mx-auto mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
          <p className="text-sm text-nvr-text-secondary">No cameras found on the network.</p>
          <p className="text-xs text-nvr-text-muted mt-1">You can add cameras manually later from the Cameras page.</p>
          <button
            onClick={startScan}
            className="text-xs text-nvr-accent hover:text-nvr-accent-hover mt-3 transition-colors"
          >
            Try scanning again
          </button>
        </div>
      )}

      {/* Navigation */}
      <div className="flex justify-between mt-2">
        <button
          onClick={onBack}
          className="text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors px-4 py-2 rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Back
        </button>
        <div className="flex items-center gap-3">
          <button
            onClick={onNext}
            className="text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors px-4 py-2 rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Skip
          </button>
          <button
            onClick={onNext}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-6 py-2.5 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Next
          </button>
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Step 4: Recording Schedule                                         */
/* ------------------------------------------------------------------ */

interface Camera {
  id: string
  name: string
}

function StepRecordingSchedule({
  onComplete,
  onBack,
  addedCameraIds,
}: {
  onComplete: () => void
  onBack: () => void
  addedCameraIds: string[]
}) {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)

  // Schedule settings
  const [mode, setMode] = useState<'always' | 'events'>('always')
  const [selectedDays, setSelectedDays] = useState<number[]>([0, 1, 2, 3, 4, 5, 6])
  const [startTime, setStartTime] = useState('00:00')
  const [endTime, setEndTime] = useState('23:59')
  const [selectedCameras, setSelectedCameras] = useState<Set<string>>(new Set())

  useEffect(() => {
    apiFetch('/cameras')
      .then(async (res) => {
        if (res.ok) {
          const data: Camera[] = await res.json()
          setCameras(data)
          // Pre-select cameras that were added during discovery
          if (addedCameraIds.length > 0) {
            setSelectedCameras(new Set(addedCameraIds.filter((id) => data.some((c) => c.id === id))))
          } else {
            setSelectedCameras(new Set(data.map((c) => c.id)))
          }
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [addedCameraIds])

  const toggleDay = (day: number) => {
    setSelectedDays((prev) =>
      prev.includes(day) ? prev.filter((d) => d !== day) : [...prev, day].sort()
    )
  }

  const toggleCamera = (id: string) => {
    setSelectedCameras((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleAllCameras = () => {
    if (selectedCameras.size === cameras.length) {
      setSelectedCameras(new Set())
    } else {
      setSelectedCameras(new Set(cameras.map((c) => c.id)))
    }
  }

  const handleSave = async () => {
    if (selectedCameras.size === 0) {
      setError('Select at least one camera.')
      return
    }
    if (selectedDays.length === 0) {
      setError('Select at least one day.')
      return
    }

    setSaving(true)
    setError('')

    try {
      const payload = {
        name: 'Default Schedule',
        mode,
        days: selectedDays,
        start_time: startTime,
        end_time: endTime,
        post_event_seconds: mode === 'events' ? 30 : 0,
        enabled: true,
      }

      const results = await Promise.allSettled(
        Array.from(selectedCameras).map((cameraId) =>
          apiFetch(`/cameras/${cameraId}/recording-rules`, {
            method: 'POST',
            body: JSON.stringify(payload),
          })
        )
      )

      const failures = results.filter((r) => r.status === 'rejected' || (r.status === 'fulfilled' && !r.value.ok))
      if (failures.length > 0 && failures.length === results.length) {
        setError('Failed to create recording rules. You can configure them later.')
        setSaving(false)
        return
      }

      setSuccess(true)
      setTimeout(() => onComplete(), 1000)
    } catch {
      setError('Failed to save recording schedule.')
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex flex-col items-center gap-3 py-8">
        <svg className="w-6 h-6 text-nvr-accent animate-spin" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
        <span className="text-sm text-nvr-text-muted">Loading cameras...</span>
      </div>
    )
  }

  if (success) {
    return (
      <div className="flex flex-col items-center gap-4 py-6">
        <div className="w-12 h-12 rounded-full bg-nvr-success/20 flex items-center justify-center">
          <svg className="w-6 h-6 text-nvr-success" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
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

  if (cameras.length === 0) {
    return (
      <div className="flex flex-col gap-4">
        <div className="text-center py-6 bg-nvr-bg-tertiary border border-nvr-border rounded-lg">
          <svg className="w-8 h-8 text-nvr-text-muted mx-auto mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
          <p className="text-sm text-nvr-text-secondary">No cameras configured yet.</p>
          <p className="text-xs text-nvr-text-muted mt-1">You can set up recording schedules after adding cameras.</p>
        </div>

        <div className="flex justify-between mt-2">
          <button
            onClick={onBack}
            className="text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors px-4 py-2 rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Back
          </button>
          <button
            onClick={onComplete}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-6 py-2.5 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Finish Setup
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-nvr-text-secondary">
        Set a default recording schedule for your cameras. You can customize per-camera schedules later.
      </p>

      {/* Recording mode */}
      <div>
        <label className="block text-xs font-medium text-nvr-text-secondary mb-2">Recording Mode</label>
        <div className="grid grid-cols-2 gap-2">
          <button
            onClick={() => setMode('always')}
            className={`px-3 py-2.5 rounded-lg text-sm font-medium border transition-colors ${
              mode === 'always'
                ? 'bg-nvr-accent/15 border-nvr-accent text-nvr-accent'
                : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-secondary hover:text-nvr-text-primary'
            }`}
          >
            <span className="block font-semibold">Always</span>
            <span className="block text-xs opacity-70 mt-0.5">Record continuously</span>
          </button>
          <button
            onClick={() => setMode('events')}
            className={`px-3 py-2.5 rounded-lg text-sm font-medium border transition-colors ${
              mode === 'events'
                ? 'bg-nvr-warning/15 border-nvr-warning text-nvr-warning'
                : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-secondary hover:text-nvr-text-primary'
            }`}
          >
            <span className="block font-semibold">Events Only</span>
            <span className="block text-xs opacity-70 mt-0.5">Record on motion</span>
          </button>
        </div>
      </div>

      {/* Days */}
      <div>
        <label className="block text-xs font-medium text-nvr-text-secondary mb-2">Days</label>
        <div className="flex gap-1">
          {DAY_NAMES.map((name, i) => (
            <button
              key={i}
              onClick={() => toggleDay(i)}
              className={`w-9 h-9 rounded-lg text-xs font-semibold transition-colors ${
                selectedDays.includes(i)
                  ? 'bg-nvr-accent text-white'
                  : 'bg-nvr-bg-tertiary text-nvr-text-muted hover:text-nvr-text-primary'
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
          <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Start Time</label>
          <input
            type="time"
            value={startTime}
            onChange={(e) => setStartTime(e.target.value)}
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
          />
        </div>
        <div>
          <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">End Time</label>
          <input
            type="time"
            value={endTime}
            onChange={(e) => setEndTime(e.target.value)}
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
          />
        </div>
      </div>

      {/* Camera selection */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <label className="text-xs font-medium text-nvr-text-secondary">Apply to Cameras</label>
          <button
            onClick={toggleAllCameras}
            className="text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors"
          >
            {selectedCameras.size === cameras.length ? 'Deselect All' : 'Select All'}
          </button>
        </div>
        <div className="space-y-1 max-h-32 overflow-y-auto">
          {cameras.map((cam) => (
            <label
              key={cam.id}
              className="flex items-center gap-2.5 px-3 py-2 rounded-lg hover:bg-nvr-bg-tertiary cursor-pointer transition-colors"
            >
              <input
                type="checkbox"
                checked={selectedCameras.has(cam.id)}
                onChange={() => toggleCamera(cam.id)}
                className="rounded border-nvr-border text-nvr-accent focus:ring-nvr-accent/50"
              />
              <span className="text-sm text-nvr-text-primary">{cam.name}</span>
            </label>
          ))}
        </div>
      </div>

      {error && <ErrorAlert message={error} />}

      {/* Navigation */}
      <div className="flex justify-between mt-2">
        <button
          onClick={onBack}
          className="text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors px-4 py-2 rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Back
        </button>
        <div className="flex items-center gap-3">
          <button
            onClick={onComplete}
            className="text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors px-4 py-2 rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Skip
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-semibold text-sm px-6 py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            {saving ? (
              <>
                <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Saving...
              </>
            ) : (
              'Finish Setup'
            )}
          </button>
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main Setup Wizard                                                  */
/* ------------------------------------------------------------------ */

export default function Setup() {
  const [step, setStep] = useState<Step>(1)
  const [addedCameraIds, setAddedCameraIds] = useState<string[]>([])
  const navigate = useNavigate()

  useEffect(() => {
    document.title = 'Setup - MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  const handleComplete = () => {
    navigate('/live')
  }

  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-nvr-bg-primary px-4">
      <div className="w-full max-w-md">
        {/* Brand */}
        <div className="text-center mb-8">
          <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-nvr-accent/15 mb-4">
            <svg className="w-6 h-6 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-white tracking-tight">Welcome to MediaMTX NVR</h1>
          <p className="text-sm text-nvr-text-secondary mt-2">
            {step === 1 && 'Create your admin account to get started.'}
            {step === 2 && 'Review your storage configuration.'}
            {step === 3 && 'Discover cameras on your network.'}
            {step === 4 && 'Set up your recording schedule.'}
          </p>
        </div>

        {/* Card */}
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-2xl p-6 shadow-2xl">
          <StepIndicator current={step} />

          {step === 1 && (
            <StepAdminAccount onComplete={() => setStep(2)} />
          )}

          {step === 2 && (
            <StepStorage
              onNext={() => setStep(3)}
              onBack={() => setStep(1)}
            />
          )}

          {step === 3 && (
            <StepCameraDiscovery
              onNext={() => setStep(4)}
              onBack={() => setStep(2)}
              onCamerasAdded={(ids) => setAddedCameraIds(ids)}
            />
          )}

          {step === 4 && (
            <StepRecordingSchedule
              onComplete={handleComplete}
              onBack={() => setStep(3)}
              addedCameraIds={addedCameraIds}
            />
          )}
        </div>

        {/* Footer */}
        <p className="text-center text-xs text-nvr-text-muted mt-6">Powered by MediaMTX</p>
      </div>
    </div>
  )
}
