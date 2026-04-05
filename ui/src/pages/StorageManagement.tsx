import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface CameraStorageInfo {
  camera_id: string
  camera_name: string
  total_bytes: number
  segment_count: number
}

interface DatabaseStats {
  file_size_bytes: number
  tables: Record<string, { row_count: number }>
}

interface StorageInfo {
  total_bytes: number
  used_bytes: number
  free_bytes: number
  recordings_bytes: number
  per_camera: CameraStorageInfo[]
  database: DatabaseStats | null
  warning: boolean
  critical: boolean
}

interface QuotaStatus {
  camera_id?: string
  camera_name?: string
  quota_bytes: number
  used_bytes: number
  used_percent: number
  status: string // "ok" | "warning" | "critical" | "exceeded"
  warning_percent: number
  critical_percent: number
}

interface QuotaStatusResponse {
  global: QuotaStatus | null
  cameras: QuotaStatus[]
}

interface CameraFull {
  id: string
  name: string
  retention_days: number
  event_retention_days: number
  detection_retention_days: number
  quota_bytes: number
  quota_warning_percent: number
  quota_critical_percent: number
}

interface CleanupResult {
  deleted_count: number
  files_removed: number
  bytes_freed: number
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatBytesInput(bytes: number): { value: number; unit: string } {
  if (bytes === 0) return { value: 0, unit: 'GB' }
  const gb = bytes / (1024 * 1024 * 1024)
  if (gb >= 1024) return { value: parseFloat((gb / 1024).toFixed(1)), unit: 'TB' }
  return { value: parseFloat(gb.toFixed(1)), unit: 'GB' }
}

function toBytes(value: number, unit: string): number {
  if (unit === 'TB') return Math.round(value * 1024 * 1024 * 1024 * 1024)
  return Math.round(value * 1024 * 1024 * 1024)
}

function statusColor(status: string): string {
  switch (status) {
    case 'critical':
    case 'exceeded':
      return 'text-red-400'
    case 'warning':
      return 'text-amber-400'
    default:
      return 'text-green-400'
  }
}

function statusBg(status: string): string {
  switch (status) {
    case 'critical':
    case 'exceeded':
      return 'bg-red-500'
    case 'warning':
      return 'bg-amber-500'
    default:
      return 'bg-nvr-accent'
  }
}

/* ------------------------------------------------------------------ */
/*  Sub-tab type                                                       */
/* ------------------------------------------------------------------ */

type SubTab = 'overview' | 'retention' | 'quotas' | 'cleanup'

const SUB_TABS: { id: SubTab; label: string }[] = [
  { id: 'overview', label: 'Disk Usage' },
  { id: 'retention', label: 'Retention Policies' },
  { id: 'quotas', label: 'Quotas' },
  { id: 'cleanup', label: 'Cleanup' },
]

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

export default function StorageManagement() {
  const [activeTab, setActiveTab] = useState<SubTab>('overview')
  const [storage, setStorage] = useState<StorageInfo | null>(null)
  const [storageLoading, setStorageLoading] = useState(true)
  const [cameras, setCameras] = useState<CameraFull[]>([])
  const [camerasLoading, setCamerasLoading] = useState(true)
  const [quotaStatus, setQuotaStatus] = useState<QuotaStatusResponse | null>(null)
  const [quotaLoading, setQuotaLoading] = useState(true)
  const [savingId, setSavingId] = useState<string | null>(null)
  const [saveSuccess, setSaveSuccess] = useState<string | null>(null)
  const [saveError, setSaveError] = useState<string | null>(null)

  // Global quota form state
  const [globalQuotaEnabled, setGlobalQuotaEnabled] = useState(false)
  const [globalQuotaValue, setGlobalQuotaValue] = useState(0)
  const [globalQuotaUnit, setGlobalQuotaUnit] = useState('GB')
  const [globalQuotaWarn, setGlobalQuotaWarn] = useState(80)
  const [globalQuotaCrit, setGlobalQuotaCrit] = useState(90)

  // Cleanup state
  const [cleanupCameraId, setCleanupCameraId] = useState('')
  const [cleanupDate, setCleanupDate] = useState('')
  const [cleanupDryRun, setCleanupDryRun] = useState(true)
  const [cleanupLoading, setCleanupLoading] = useState(false)
  const [cleanupResult, setCleanupResult] = useState<CleanupResult | null>(null)
  const [cleanupError, setCleanupError] = useState<string | null>(null)

  /* ---- Data fetching ---- */

  const fetchStorage = useCallback(async () => {
    try {
      const res = await apiFetch('/system/storage')
      if (res.ok) setStorage(await res.json())
    } catch { /* ignore */ }
    setStorageLoading(false)
  }, [])

  const fetchCameras = useCallback(async () => {
    try {
      const res = await apiFetch('/cameras')
      if (res.ok) setCameras(await res.json())
    } catch { /* ignore */ }
    setCamerasLoading(false)
  }, [])

  const fetchQuotas = useCallback(async () => {
    try {
      const res = await apiFetch('/quotas/status')
      if (res.ok) {
        const data: QuotaStatusResponse = await res.json()
        setQuotaStatus(data)
        if (data.global) {
          setGlobalQuotaEnabled(true)
          const parsed = formatBytesInput(data.global.quota_bytes)
          setGlobalQuotaValue(parsed.value)
          setGlobalQuotaUnit(parsed.unit)
          setGlobalQuotaWarn(data.global.warning_percent)
          setGlobalQuotaCrit(data.global.critical_percent)
        }
      }
    } catch { /* ignore */ }
    setQuotaLoading(false)
  }, [])

  useEffect(() => {
    fetchStorage()
    fetchCameras()
    fetchQuotas()
  }, [fetchStorage, fetchCameras, fetchQuotas])

  /* ---- Retention save ---- */

  const handleSaveRetention = async (cam: CameraFull, retDays: number, eventRetDays: number, detRetDays: number) => {
    setSavingId(cam.id)
    setSaveSuccess(null)
    setSaveError(null)
    try {
      const res = await apiFetch(`/cameras/${cam.id}/retention`, {
        method: 'PUT',
        body: JSON.stringify({
          retention_days: retDays,
          event_retention_days: eventRetDays,
          detection_retention_days: detRetDays,
        }),
      })
      if (res.ok) {
        setSaveSuccess(cam.id)
        fetchCameras()
        setTimeout(() => setSaveSuccess(null), 3000)
      } else {
        const data = await res.json().catch(() => ({ error: 'Failed' }))
        setSaveError(data.error || 'Failed to save')
      }
    } catch {
      setSaveError('Network error')
    } finally {
      setSavingId(null)
    }
  }

  /* ---- Quota save ---- */

  const handleSaveGlobalQuota = async () => {
    setSavingId('global')
    setSaveSuccess(null)
    setSaveError(null)
    try {
      const quotaBytes = globalQuotaEnabled ? toBytes(globalQuotaValue, globalQuotaUnit) : 0
      const res = await apiFetch('/quotas/global', {
        method: 'PUT',
        body: JSON.stringify({
          quota_bytes: quotaBytes || 1, // API requires > 0
          warning_percent: globalQuotaWarn,
          critical_percent: globalQuotaCrit,
          enabled: globalQuotaEnabled,
        }),
      })
      if (res.ok) {
        setSaveSuccess('global')
        fetchQuotas()
        setTimeout(() => setSaveSuccess(null), 3000)
      } else {
        const data = await res.json().catch(() => ({ error: 'Failed' }))
        setSaveError(data.error || 'Failed to save')
      }
    } catch {
      setSaveError('Network error')
    } finally {
      setSavingId(null)
    }
  }

  const handleSaveCameraQuota = async (camId: string, quotaBytes: number, warnPct: number, critPct: number) => {
    setSavingId(camId)
    setSaveSuccess(null)
    setSaveError(null)
    try {
      const res = await apiFetch(`/cameras/${camId}/quota`, {
        method: 'PUT',
        body: JSON.stringify({
          quota_bytes: quotaBytes,
          warning_percent: warnPct,
          critical_percent: critPct,
        }),
      })
      if (res.ok) {
        setSaveSuccess(camId)
        fetchCameras()
        fetchQuotas()
        setTimeout(() => setSaveSuccess(null), 3000)
      } else {
        const data = await res.json().catch(() => ({ error: 'Failed' }))
        setSaveError(data.error || 'Failed to save')
      }
    } catch {
      setSaveError('Network error')
    } finally {
      setSavingId(null)
    }
  }

  /* ---- Cleanup ---- */

  const handleCleanup = async (dryRun: boolean) => {
    if (!cleanupCameraId || !cleanupDate) return
    setCleanupLoading(true)
    setCleanupResult(null)
    setCleanupError(null)
    try {
      if (dryRun) {
        // For dry-run: query recordings count before the date
        const res = await apiFetch(`/recordings?camera_id=${cleanupCameraId}&before=${cleanupDate}T00:00:00Z&limit=0`)
        if (res.ok) {
          const data = await res.json()
          const count = data.total ?? data.recordings?.length ?? 0
          setCleanupResult({
            deleted_count: count,
            files_removed: count,
            bytes_freed: 0, // dry-run cannot know exact bytes
          })
        } else {
          // Fallback: just show that dry-run estimation is not available
          setCleanupResult({
            deleted_count: -1,
            files_removed: 0,
            bytes_freed: 0,
          })
        }
      } else {
        const res = await apiFetch('/recordings/cleanup', {
          method: 'DELETE',
          body: JSON.stringify({
            camera_id: cleanupCameraId,
            before: `${cleanupDate}T00:00:00Z`,
          }),
        })
        if (res.ok) {
          const data: CleanupResult = await res.json()
          setCleanupResult(data)
          fetchStorage()
        } else {
          const data = await res.json().catch(() => ({ error: 'Cleanup failed' }))
          setCleanupError(data.error || 'Cleanup failed')
        }
      }
    } catch {
      setCleanupError('Network error')
    } finally {
      setCleanupLoading(false)
    }
  }

  /* ---- Computed ---- */

  const usedPercent = storage && storage.total_bytes > 0
    ? Math.round((storage.used_bytes / storage.total_bytes) * 100)
    : 0

  const maxCameraBytes = storage?.per_camera
    ? Math.max(...storage.per_camera.map(c => c.total_bytes), 1)
    : 1

  /* ---- Render ---- */

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-nvr-text-primary">Storage Management</h1>
        <p className="text-sm text-nvr-text-secondary mt-1">
          Monitor disk usage, configure retention policies, set quotas, and manage cleanup.
        </p>
      </div>

      {/* Sub-tabs */}
      <div className="flex gap-1 border-b border-nvr-border overflow-x-auto">
        {SUB_TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-2.5 text-sm font-medium whitespace-nowrap transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
              activeTab === tab.id
                ? 'text-white border-b-2 border-nvr-accent -mb-px'
                : 'text-nvr-text-secondary hover:text-nvr-text-primary border-b-2 border-transparent -mb-px'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* ===== DISK USAGE TAB ===== */}
      {activeTab === 'overview' && (
        <div className="space-y-6">
          {storageLoading ? (
            <LoadingCard message="Loading storage info..." />
          ) : storage ? (
            <>
              {/* Main usage card */}
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                <div className="flex items-center justify-between mb-4">
                  <h2 className="text-lg font-semibold text-nvr-text-primary">Disk Usage</h2>
                  <button
                    onClick={fetchStorage}
                    className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded px-2 py-1"
                  >
                    Refresh
                  </button>
                </div>

                {/* Large usage bar */}
                <div className="mb-4">
                  <div className={`w-full h-6 rounded-full overflow-hidden flex ${
                    usedPercent > 85 ? 'bg-amber-500/10' : 'bg-nvr-bg-primary'
                  }`}>
                    {/* Recordings portion */}
                    <div
                      className="h-full bg-nvr-accent transition-all duration-500"
                      style={{ width: `${storage.total_bytes > 0 ? Math.round((storage.recordings_bytes / storage.total_bytes) * 100) : 0}%` }}
                      title={`Recordings: ${formatBytes(storage.recordings_bytes)}`}
                    />
                    {/* Other used portion */}
                    <div
                      className="h-full bg-nvr-text-muted/40 transition-all duration-500"
                      style={{ width: `${storage.total_bytes > 0 ? Math.round(((storage.used_bytes - storage.recordings_bytes) / storage.total_bytes) * 100) : 0}%` }}
                      title={`Other: ${formatBytes(storage.used_bytes - storage.recordings_bytes)}`}
                    />
                  </div>
                  <div className="flex justify-between mt-2">
                    <div className="flex items-center gap-4">
                      <span className="text-sm text-nvr-text-secondary">{usedPercent}% used</span>
                      <div className="flex items-center gap-1.5 text-xs text-nvr-text-muted">
                        <span className="w-2.5 h-2.5 rounded-sm bg-nvr-accent inline-block" /> Recordings
                        <span className="w-2.5 h-2.5 rounded-sm bg-nvr-text-muted/40 inline-block ml-2" /> Other
                      </div>
                    </div>
                    <span className={`text-sm font-medium ${usedPercent > 85 ? 'text-amber-400' : 'text-nvr-text-primary'}`}>
                      Free: {formatBytes(storage.free_bytes)}
                    </span>
                  </div>
                </div>

                {/* Quick stats */}
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                  <StatCard label="Total" value={formatBytes(storage.total_bytes)} />
                  <StatCard label="Used" value={formatBytes(storage.used_bytes)} />
                  <StatCard label="Recordings" value={formatBytes(storage.recordings_bytes)} accent />
                  <StatCard label="Free" value={formatBytes(storage.free_bytes)} />
                </div>

                {usedPercent > 85 && (
                  <div className="mt-4 bg-amber-500/10 border border-amber-500/20 rounded-lg p-3 flex items-center gap-2">
                    <WarningIcon />
                    <span className="text-sm text-amber-400">
                      Disk usage is above 85%. Consider adjusting retention policies or running a cleanup.
                    </span>
                  </div>
                )}
              </div>

              {/* Per-camera breakdown */}
              {storage.per_camera.length > 0 ? (
                <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                  <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Per-Camera Storage</h2>
                  <div className="space-y-4">
                    {storage.per_camera
                      .sort((a, b) => b.total_bytes - a.total_bytes)
                      .map(cam => {
                        const pct = Math.round((cam.total_bytes / maxCameraBytes) * 100)
                        const diskPct = storage.total_bytes > 0
                          ? (cam.total_bytes / storage.total_bytes * 100).toFixed(1)
                          : '0'
                        return (
                          <div key={cam.camera_id}>
                            <div className="flex items-center justify-between mb-1">
                              <span className="text-sm text-nvr-text-primary font-medium">{cam.camera_name || cam.camera_id}</span>
                              <div className="flex items-center gap-3">
                                <span className="text-xs text-nvr-text-muted">{diskPct}% of disk</span>
                                <span className="text-xs text-nvr-text-secondary font-mono">{formatBytes(cam.total_bytes)}</span>
                              </div>
                            </div>
                            <div className="w-full h-3 bg-nvr-bg-primary rounded-full overflow-hidden">
                              <div
                                className="h-full bg-nvr-accent rounded-full transition-all duration-500"
                                style={{ width: `${pct}%` }}
                              />
                            </div>
                            <p className="text-xs text-nvr-text-muted mt-0.5">{cam.segment_count} segments</p>
                          </div>
                        )
                      })}
                  </div>
                </div>
              ) : (
                <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 text-center">
                  <p className="text-nvr-text-muted text-sm">No recordings found.</p>
                </div>
              )}

              {/* Database stats */}
              {storage.database && (
                <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                  <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Database</h2>
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                    <StatCard label="DB Size" value={formatBytes(storage.database.file_size_bytes)} />
                    {Object.entries(storage.database.tables)
                      .filter(([name]) => ['recordings', 'motion_events', 'detections'].includes(name))
                      .map(([name, stats]) => (
                        <StatCard key={name} label={name.replace('_', ' ')} value={stats.row_count.toLocaleString()} sublabel="rows" />
                      ))}
                  </div>
                </div>
              )}
            </>
          ) : (
            <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 text-center">
              <p className="text-nvr-text-muted text-sm">Unable to load storage information.</p>
            </div>
          )}
        </div>
      )}

      {/* ===== RETENTION POLICIES TAB ===== */}
      {activeTab === 'retention' && (
        <div className="space-y-4">
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Retention Policies</h2>
            <p className="text-sm text-nvr-text-muted mb-4">
              Configure how long recordings, events, and detection data are kept for each camera.
              Set to 0 to use the system default.
            </p>

            {camerasLoading ? (
              <LoadingCard message="Loading cameras..." />
            ) : cameras.length === 0 ? (
              <p className="text-nvr-text-muted text-sm text-center py-8">No cameras configured.</p>
            ) : (
              <div className="space-y-4">
                {cameras.map(cam => (
                  <RetentionEditor
                    key={cam.id}
                    camera={cam}
                    saving={savingId === cam.id}
                    success={saveSuccess === cam.id}
                    onSave={(ret, eventRet, detRet) => handleSaveRetention(cam, ret, eventRet, detRet)}
                  />
                ))}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ===== QUOTAS TAB ===== */}
      {activeTab === 'quotas' && (
        <div className="space-y-6">
          {/* Global quota */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Global Quota</h2>
            <p className="text-sm text-nvr-text-muted mb-4">
              Limit total storage usage across all cameras. Warnings are shown when thresholds are exceeded.
            </p>

            {quotaLoading ? (
              <LoadingCard message="Loading quota status..." />
            ) : (
              <>
                {/* Current status */}
                {quotaStatus?.global && (
                  <div className="mb-4 bg-nvr-bg-primary border border-nvr-border/50 rounded-lg p-3 flex items-center gap-3">
                    <div className={`w-2.5 h-2.5 rounded-full ${statusBg(quotaStatus.global.status)}`} />
                    <span className="text-sm text-nvr-text-primary">
                      {formatBytes(quotaStatus.global.used_bytes)} / {formatBytes(quotaStatus.global.quota_bytes)}
                      <span className={`ml-2 font-medium ${statusColor(quotaStatus.global.status)}`}>
                        ({quotaStatus.global.used_percent.toFixed(1)}% - {quotaStatus.global.status})
                      </span>
                    </span>
                  </div>
                )}

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                  <label className="flex items-center gap-3">
                    <input
                      type="checkbox"
                      checked={globalQuotaEnabled}
                      onChange={e => setGlobalQuotaEnabled(e.target.checked)}
                      className="accent-nvr-accent w-4 h-4"
                    />
                    <span className="text-sm text-nvr-text-primary">Enable global quota</span>
                  </label>
                </div>

                {globalQuotaEnabled && (
                  <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mt-4">
                    <div>
                      <label className="block text-xs text-nvr-text-muted mb-1">Quota Size</label>
                      <div className="flex gap-2">
                        <input
                          type="number"
                          min={1}
                          value={globalQuotaValue}
                          onChange={e => setGlobalQuotaValue(parseFloat(e.target.value) || 0)}
                          className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                        />
                        <select
                          value={globalQuotaUnit}
                          onChange={e => setGlobalQuotaUnit(e.target.value)}
                          className="bg-nvr-bg-input border border-nvr-border rounded-lg px-2 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                        >
                          <option value="GB">GB</option>
                          <option value="TB">TB</option>
                        </select>
                      </div>
                    </div>
                    <div>
                      <label className="block text-xs text-nvr-text-muted mb-1">Warning at (%)</label>
                      <input
                        type="number"
                        min={1}
                        max={99}
                        value={globalQuotaWarn}
                        onChange={e => setGlobalQuotaWarn(parseInt(e.target.value) || 80)}
                        className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-nvr-text-muted mb-1">Critical at (%)</label>
                      <input
                        type="number"
                        min={1}
                        max={99}
                        value={globalQuotaCrit}
                        onChange={e => setGlobalQuotaCrit(parseInt(e.target.value) || 90)}
                        className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                      />
                    </div>
                  </div>
                )}

                <div className="mt-4 flex items-center gap-3">
                  <button
                    onClick={handleSaveGlobalQuota}
                    disabled={savingId === 'global'}
                    className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    {savingId === 'global' ? 'Saving...' : 'Save Global Quota'}
                  </button>
                  {saveSuccess === 'global' && (
                    <span className="text-sm text-green-400">Saved</span>
                  )}
                  {saveError && savingId === null && (
                    <span className="text-sm text-red-400">{saveError}</span>
                  )}
                </div>
              </>
            )}
          </div>

          {/* Per-camera quotas */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Per-Camera Quotas</h2>
            <p className="text-sm text-nvr-text-muted mb-4">
              Set individual storage limits for each camera. Leave at 0 to use only the global quota.
            </p>

            {camerasLoading ? (
              <LoadingCard message="Loading cameras..." />
            ) : cameras.length === 0 ? (
              <p className="text-nvr-text-muted text-sm text-center py-8">No cameras configured.</p>
            ) : (
              <div className="space-y-4">
                {cameras.map(cam => {
                  const qs = quotaStatus?.cameras.find(q => q.camera_id === cam.id)
                  return (
                    <CameraQuotaEditor
                      key={cam.id}
                      camera={cam}
                      quotaStatus={qs}
                      saving={savingId === cam.id}
                      success={saveSuccess === cam.id}
                      onSave={(bytes, warn, crit) => handleSaveCameraQuota(cam.id, bytes, warn, crit)}
                    />
                  )
                })}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ===== CLEANUP TAB ===== */}
      {activeTab === 'cleanup' && (
        <div className="space-y-6">
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Manual Cleanup</h2>
            <p className="text-sm text-nvr-text-muted mb-4">
              Delete recordings older than a specified date for a camera. Use the dry-run option to preview what would be deleted before committing.
            </p>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
              <div>
                <label className="block text-xs text-nvr-text-muted mb-1">Camera</label>
                <select
                  value={cleanupCameraId}
                  onChange={e => {
                    setCleanupCameraId(e.target.value)
                    setCleanupResult(null)
                    setCleanupError(null)
                  }}
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                >
                  <option value="">Select a camera</option>
                  {cameras.map(cam => (
                    <option key={cam.id} value={cam.id}>{cam.name || cam.id}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs text-nvr-text-muted mb-1">Delete recordings before</label>
                <input
                  type="date"
                  value={cleanupDate}
                  onChange={e => {
                    setCleanupDate(e.target.value)
                    setCleanupResult(null)
                    setCleanupError(null)
                  }}
                  className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                />
              </div>
            </div>

            <div className="flex items-center gap-4 mb-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={cleanupDryRun}
                  onChange={e => {
                    setCleanupDryRun(e.target.checked)
                    setCleanupResult(null)
                  }}
                  className="accent-nvr-accent w-4 h-4"
                />
                <span className="text-sm text-nvr-text-primary">Dry run (preview only)</span>
              </label>
            </div>

            <div className="flex items-center gap-3">
              <button
                onClick={() => handleCleanup(cleanupDryRun)}
                disabled={cleanupLoading || !cleanupCameraId || !cleanupDate}
                className={`font-medium px-4 py-2 rounded-lg transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                  cleanupDryRun
                    ? 'bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-primary border border-nvr-border'
                    : 'bg-nvr-danger hover:bg-nvr-danger-hover text-white'
                }`}
              >
                {cleanupLoading
                  ? 'Processing...'
                  : cleanupDryRun
                    ? 'Preview Cleanup'
                    : 'Delete Recordings'}
              </button>
            </div>

            {/* Results */}
            {cleanupResult && (
              <div className={`mt-4 rounded-lg p-4 border ${
                cleanupDryRun
                  ? 'bg-nvr-bg-primary border-nvr-border'
                  : 'bg-green-500/10 border-green-500/20'
              }`}>
                {cleanupDryRun ? (
                  <div>
                    <p className="text-sm font-medium text-nvr-text-primary mb-1">Dry Run Preview</p>
                    {cleanupResult.deleted_count >= 0 ? (
                      <p className="text-sm text-nvr-text-secondary">
                        Approximately <strong className="text-nvr-text-primary">{cleanupResult.deleted_count}</strong> recording(s) would be deleted.
                      </p>
                    ) : (
                      <p className="text-sm text-nvr-text-secondary">
                        Dry-run estimation unavailable. Uncheck "Dry run" to proceed with actual deletion.
                      </p>
                    )}
                    <button
                      onClick={() => {
                        setCleanupDryRun(false)
                        setCleanupResult(null)
                      }}
                      className="mt-2 text-sm text-nvr-accent hover:text-nvr-accent-hover font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                    >
                      Proceed with actual cleanup
                    </button>
                  </div>
                ) : (
                  <p className="text-sm text-green-400">
                    Deleted {cleanupResult.deleted_count} recording(s),
                    removed {cleanupResult.files_removed} file(s),
                    freed {formatBytes(cleanupResult.bytes_freed)}.
                  </p>
                )}
              </div>
            )}

            {cleanupError && (
              <div className="mt-4 bg-red-500/10 border border-red-500/20 rounded-lg p-3">
                <p className="text-sm text-red-400">{cleanupError}</p>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Global save error display */}
      {saveError && savingId === null && activeTab !== 'quotas' && (
        <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-3">
          <p className="text-sm text-red-400">{saveError}</p>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Sub-components                                                     */
/* ------------------------------------------------------------------ */

function LoadingCard({ message }: { message: string }) {
  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 flex items-center justify-center py-12">
      <span className="inline-block w-5 h-5 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-3" />
      <span className="text-nvr-text-muted">{message}</span>
    </div>
  )
}

function StatCard({ label, value, accent, sublabel }: { label: string; value: string; accent?: boolean; sublabel?: string }) {
  return (
    <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
      <p className="text-xs text-nvr-text-muted mb-1">{label}</p>
      <p className={`text-sm font-semibold ${accent ? 'text-nvr-accent' : 'text-nvr-text-primary'}`}>{value}</p>
      {sublabel && <p className="text-xs text-nvr-text-muted mt-0.5">{sublabel}</p>}
    </div>
  )
}

function WarningIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4 text-amber-400 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
      <line x1="12" y1="9" x2="12" y2="13" />
      <line x1="12" y1="17" x2="12.01" y2="17" />
    </svg>
  )
}

/* ---- Retention Editor ---- */

function RetentionEditor({
  camera,
  saving,
  success,
  onSave,
}: {
  camera: CameraFull
  saving: boolean
  success: boolean
  onSave: (ret: number, eventRet: number, detRet: number) => void
}) {
  const [retDays, setRetDays] = useState(camera.retention_days ?? 0)
  const [eventRetDays, setEventRetDays] = useState(camera.event_retention_days ?? 0)
  const [detRetDays, setDetRetDays] = useState(camera.detection_retention_days ?? 0)
  const [expanded, setExpanded] = useState(false)

  const hasChanges =
    retDays !== (camera.retention_days ?? 0) ||
    eventRetDays !== (camera.event_retention_days ?? 0) ||
    detRetDays !== (camera.detection_retention_days ?? 0)

  return (
    <div className="bg-nvr-bg-primary border border-nvr-border/50 rounded-lg p-4">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between text-left focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
      >
        <div>
          <span className="text-sm font-medium text-nvr-text-primary">{camera.name || camera.id}</span>
          <span className="text-xs text-nvr-text-muted ml-3">
            {camera.retention_days ? `${camera.retention_days}d` : 'default'}
            {camera.event_retention_days ? ` / events: ${camera.event_retention_days}d` : ''}
          </span>
        </div>
        <svg
          className={`w-4 h-4 text-nvr-text-muted transition-transform ${expanded ? 'rotate-180' : ''}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {expanded && (
        <div className="mt-4 space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label className="block text-xs text-nvr-text-muted mb-1">Retention (days)</label>
              <input
                type="number"
                min={0}
                value={retDays}
                onChange={e => setRetDays(parseInt(e.target.value) || 0)}
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                placeholder="0 = system default"
              />
              <p className="text-xs text-nvr-text-muted mt-1">How long to keep non-event recordings</p>
            </div>
            <div>
              <label className="block text-xs text-nvr-text-muted mb-1">Event Retention (days)</label>
              <input
                type="number"
                min={0}
                value={eventRetDays}
                onChange={e => setEventRetDays(parseInt(e.target.value) || 0)}
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                placeholder="0 = system default"
              />
              <p className="text-xs text-nvr-text-muted mt-1">How long to keep motion event recordings</p>
            </div>
            <div>
              <label className="block text-xs text-nvr-text-muted mb-1">Detection Retention (days)</label>
              <input
                type="number"
                min={0}
                value={detRetDays}
                onChange={e => setDetRetDays(parseInt(e.target.value) || 0)}
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                placeholder="0 = system default"
              />
              <p className="text-xs text-nvr-text-muted mt-1">How long to keep AI detection data</p>
            </div>
          </div>

          <div className="flex items-center gap-3 pt-1">
            <button
              onClick={() => onSave(retDays, eventRetDays, detRetDays)}
              disabled={saving || !hasChanges}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
            {success && <span className="text-sm text-green-400">Saved</span>}
          </div>
        </div>
      )}
    </div>
  )
}

/* ---- Camera Quota Editor ---- */

function CameraQuotaEditor({
  camera,
  quotaStatus,
  saving,
  success,
  onSave,
}: {
  camera: CameraFull
  quotaStatus?: QuotaStatus
  saving: boolean
  success: boolean
  onSave: (bytes: number, warnPct: number, critPct: number) => void
}) {
  const initialParsed = formatBytesInput(camera.quota_bytes ?? 0)
  const [quotaValue, setQuotaValue] = useState(initialParsed.value)
  const [quotaUnit, setQuotaUnit] = useState(initialParsed.unit)
  const [warnPct, setWarnPct] = useState(camera.quota_warning_percent || 80)
  const [critPct, setCritPct] = useState(camera.quota_critical_percent || 90)
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="bg-nvr-bg-primary border border-nvr-border/50 rounded-lg p-4">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center justify-between text-left focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
      >
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-nvr-text-primary">{camera.name || camera.id}</span>
          {quotaStatus && (
            <span className={`text-xs font-medium ${statusColor(quotaStatus.status)}`}>
              {quotaStatus.used_percent.toFixed(0)}% used
            </span>
          )}
          {!quotaStatus && camera.quota_bytes > 0 && (
            <span className="text-xs text-nvr-text-muted">{formatBytes(camera.quota_bytes)} quota</span>
          )}
          {!quotaStatus && (!camera.quota_bytes || camera.quota_bytes === 0) && (
            <span className="text-xs text-nvr-text-muted">No quota set</span>
          )}
        </div>
        <svg
          className={`w-4 h-4 text-nvr-text-muted transition-transform ${expanded ? 'rotate-180' : ''}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {expanded && (
        <div className="mt-4 space-y-3">
          {/* Quota status bar */}
          {quotaStatus && quotaStatus.quota_bytes > 0 && (
            <div className="mb-3">
              <div className="w-full h-2.5 bg-nvr-bg-secondary rounded-full overflow-hidden">
                <div
                  className={`h-full rounded-full transition-all duration-500 ${statusBg(quotaStatus.status)}`}
                  style={{ width: `${Math.min(quotaStatus.used_percent, 100)}%` }}
                />
              </div>
              <p className="text-xs text-nvr-text-muted mt-1">
                {formatBytes(quotaStatus.used_bytes)} / {formatBytes(quotaStatus.quota_bytes)}
              </p>
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label className="block text-xs text-nvr-text-muted mb-1">Quota Size</label>
              <div className="flex gap-2">
                <input
                  type="number"
                  min={0}
                  value={quotaValue}
                  onChange={e => setQuotaValue(parseFloat(e.target.value) || 0)}
                  className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                  placeholder="0 = no limit"
                />
                <select
                  value={quotaUnit}
                  onChange={e => setQuotaUnit(e.target.value)}
                  className="bg-nvr-bg-input border border-nvr-border rounded-lg px-2 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
                >
                  <option value="GB">GB</option>
                  <option value="TB">TB</option>
                </select>
              </div>
            </div>
            <div>
              <label className="block text-xs text-nvr-text-muted mb-1">Warning at (%)</label>
              <input
                type="number"
                min={1}
                max={99}
                value={warnPct}
                onChange={e => setWarnPct(parseInt(e.target.value) || 80)}
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-xs text-nvr-text-muted mb-1">Critical at (%)</label>
              <input
                type="number"
                min={1}
                max={99}
                value={critPct}
                onChange={e => setCritPct(parseInt(e.target.value) || 90)}
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
              />
            </div>
          </div>

          <div className="flex items-center gap-3 pt-1">
            <button
              onClick={() => onSave(toBytes(quotaValue, quotaUnit), warnPct, critPct)}
              disabled={saving}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              {saving ? 'Saving...' : 'Save Quota'}
            </button>
            {success && <span className="text-sm text-green-400">Saved</span>}
          </div>
        </div>
      )}
    </div>
  )
}
