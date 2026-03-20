import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../api/client'

interface SystemInfo {
  version: string
  platform: string
  uptime: string
}

interface CameraStorageInfo {
  camera_id: string
  camera_name: string
  total_bytes: number
  segment_count: number
}

interface StorageInfo {
  total_bytes: number
  used_bytes: number
  free_bytes: number
  recordings_bytes: number
  per_camera: CameraStorageInfo[]
}

interface MetricsData {
  cpu_goroutines: number
  mem_alloc_bytes: number
  mem_sys_bytes: number
  mem_gc_count: number
  uptime_seconds: number
  camera_count: number
}

interface AuditEntry {
  id: number
  user_id: string
  username: string
  action: string
  resource_type: string
  resource_id: string
  details: string
  ip_address: string
  created_at: string
}

interface ImportResult {
  cameras_imported: number
  cameras_skipped: number
  rules_imported: number
  rules_skipped: number
  users_skipped: number
  errors?: string[]
}

interface ConfigExport {
  version: string
  exported_at: string
  cameras: unknown[]
  recording_rules: unknown[]
  users: unknown[]
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

function formatUptime(uptime: string): string {
  const match = uptime.match(/(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?/)
  if (!match) return uptime
  const hours = match[1] ? parseInt(match[1]) : 0
  const minutes = match[2] ? parseInt(match[2]) : 0
  const seconds = match[3] ? Math.floor(parseFloat(match[3])) : 0

  const parts: string[] = []
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0) parts.push(`${minutes}m`)
  if (seconds > 0 || parts.length === 0) parts.push(`${seconds}s`)
  return parts.join(' ')
}

function formatUptimeSeconds(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  const parts: string[] = []
  if (h > 0) parts.push(`${h}h`)
  if (m > 0) parts.push(`${m}m`)
  if (s > 0 || parts.length === 0) parts.push(`${s}s`)
  return parts.join(' ')
}

function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts)
    return d.toLocaleString()
  } catch {
    return ts
  }
}

function actionBadgeColor(action: string): string {
  switch (action) {
    case 'create': return 'bg-green-500/20 text-green-400'
    case 'update': return 'bg-blue-500/20 text-blue-400'
    case 'delete': return 'bg-red-500/20 text-red-400'
    case 'login': return 'bg-nvr-accent/20 text-nvr-accent'
    case 'login_failed': return 'bg-orange-500/20 text-orange-400'
    case 'logout': return 'bg-nvr-text-muted/20 text-nvr-text-muted'
    default: return 'bg-nvr-text-muted/20 text-nvr-text-secondary'
  }
}

export default function Settings() {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [storage, setStorage] = useState<StorageInfo | null>(null)
  const [storageLoading, setStorageLoading] = useState(true)
  const [metrics, setMetrics] = useState<MetricsData | null>(null)
  const [auditEntries, setAuditEntries] = useState<AuditEntry[]>([])
  const [auditTotal, setAuditTotal] = useState(0)
  const [auditLoading, setAuditLoading] = useState(false)
  const [auditOffset, setAuditOffset] = useState(0)
  const [auditFilterAction, setAuditFilterAction] = useState('')
  const auditLimit = 25
  const metricsInterval = useRef<ReturnType<typeof setInterval> | null>(null)
  const [exporting, setExporting] = useState(false)
  const [importFile, setImportFile] = useState<ConfigExport | null>(null)
  const [importing, setImporting] = useState(false)
  const [importResult, setImportResult] = useState<ImportResult | null>(null)
  const [importError, setImportError] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    apiFetch('/system/info').then(async res => {
      if (res.ok) setSystemInfo(await res.json())
    })
  }, [])

  const fetchStorage = useCallback(() => {
    apiFetch('/system/storage').then(async res => {
      if (res.ok) {
        setStorage(await res.json())
      }
      setStorageLoading(false)
    }).catch(() => setStorageLoading(false))
  }, [])

  useEffect(() => {
    fetchStorage()
    const interval = setInterval(fetchStorage, 30000)
    return () => clearInterval(interval)
  }, [fetchStorage])

  // Metrics auto-refresh every 10s
  const fetchMetrics = useCallback(() => {
    apiFetch('/system/metrics').then(async res => {
      if (res.ok) setMetrics(await res.json())
    }).catch(() => {})
  }, [])

  useEffect(() => {
    fetchMetrics()
    metricsInterval.current = setInterval(fetchMetrics, 10000)
    return () => {
      if (metricsInterval.current) clearInterval(metricsInterval.current)
    }
  }, [fetchMetrics])

  // Audit log
  const fetchAudit = useCallback((offset: number, action: string) => {
    setAuditLoading(true)
    const params = new URLSearchParams({ limit: String(auditLimit), offset: String(offset) })
    if (action) params.set('action', action)
    apiFetch(`/audit?${params.toString()}`).then(async res => {
      if (res.ok) {
        const data = await res.json()
        if (offset === 0) {
          setAuditEntries(data.entries || [])
        } else {
          setAuditEntries(prev => [...prev, ...(data.entries || [])])
        }
        setAuditTotal(data.total || 0)
      }
      setAuditLoading(false)
    }).catch(() => setAuditLoading(false))
  }, [auditLimit])

  useEffect(() => {
    fetchAudit(0, auditFilterAction)
    setAuditOffset(0)
  }, [fetchAudit, auditFilterAction])

  const loadMoreAudit = () => {
    const newOffset = auditOffset + auditLimit
    setAuditOffset(newOffset)
    fetchAudit(newOffset, auditFilterAction)
  }

  const usedPercent = storage && storage.total_bytes > 0
    ? Math.round((storage.used_bytes / storage.total_bytes) * 100)
    : 0

  const recordingsPercent = storage && storage.total_bytes > 0
    ? Math.round((storage.recordings_bytes / storage.total_bytes) * 100)
    : 0

  const otherUsedPercent = usedPercent - recordingsPercent

  const handleExport = async () => {
    setExporting(true)
    try {
      const res = await apiFetch('/system/config/export')
      if (res.ok) {
        const data = await res.json()
        const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `nvr-config-${new Date().toISOString().slice(0, 10)}.json`
        a.click()
        URL.revokeObjectURL(url)
      }
    } finally {
      setExporting(false)
    }
  }

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setImportResult(null)
    setImportError('')

    const reader = new FileReader()
    reader.onload = (ev) => {
      try {
        const data = JSON.parse(ev.target?.result as string) as ConfigExport
        if (!data.version || !data.cameras) {
          setImportError('Invalid config file format')
          return
        }
        setImportFile(data)
      } catch {
        setImportError('Failed to parse JSON file')
      }
    }
    reader.readAsText(file)
  }

  const handleImport = async () => {
    if (!importFile) return
    setImporting(true)
    setImportError('')
    setImportResult(null)

    try {
      const res = await apiFetch('/system/config/import', {
        method: 'POST',
        body: JSON.stringify(importFile),
      })
      if (res.ok) {
        const result: ImportResult = await res.json()
        setImportResult(result)
        setImportFile(null)
        if (fileInputRef.current) fileInputRef.current.value = ''
      } else {
        const data = await res.json().catch(() => ({}))
        setImportError(data.error || 'Import failed')
      }
    } catch {
      setImportError('Network error during import')
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="space-y-4 md:space-y-6">
      <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Settings</h1>

      {/* System Information */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
        <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">System Information</h2>
        {systemInfo ? (
          <div>
            <div className="flex justify-between py-3 border-b border-nvr-border/50">
              <span className="text-sm text-nvr-text-secondary">Version</span>
              <span className="text-sm text-nvr-text-primary font-mono">{systemInfo.version}</span>
            </div>
            <div className="flex justify-between py-3 border-b border-nvr-border/50">
              <span className="text-sm text-nvr-text-secondary">Platform</span>
              <span className="text-sm text-nvr-text-primary font-mono">{systemInfo.platform}</span>
            </div>
            <div className="flex justify-between py-3">
              <span className="text-sm text-nvr-text-secondary">Uptime</span>
              <span className="text-sm text-nvr-text-primary">{formatUptime(systemInfo.uptime)}</span>
            </div>
          </div>
        ) : (
          <p className="text-nvr-text-muted text-sm">Loading...</p>
        )}
      </div>

      {/* Performance Metrics */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-nvr-text-primary">Performance</h2>
          <span className="text-xs text-nvr-text-muted">Auto-refreshes every 10s</span>
        </div>
        {metrics ? (
          <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
            <div className="bg-nvr-bg-primary rounded-lg p-3 border border-nvr-border/50">
              <p className="text-xs text-nvr-text-muted mb-1">Goroutines</p>
              <p className="text-lg font-semibold text-nvr-text-primary font-mono">{metrics.cpu_goroutines}</p>
            </div>
            <div className="bg-nvr-bg-primary rounded-lg p-3 border border-nvr-border/50">
              <p className="text-xs text-nvr-text-muted mb-1">Memory (Alloc)</p>
              <p className="text-lg font-semibold text-nvr-text-primary font-mono">{formatBytes(metrics.mem_alloc_bytes)}</p>
            </div>
            <div className="bg-nvr-bg-primary rounded-lg p-3 border border-nvr-border/50">
              <p className="text-xs text-nvr-text-muted mb-1">Memory (System)</p>
              <p className="text-lg font-semibold text-nvr-text-primary font-mono">{formatBytes(metrics.mem_sys_bytes)}</p>
            </div>
            <div className="bg-nvr-bg-primary rounded-lg p-3 border border-nvr-border/50">
              <p className="text-xs text-nvr-text-muted mb-1">GC Cycles</p>
              <p className="text-lg font-semibold text-nvr-text-primary font-mono">{metrics.mem_gc_count}</p>
            </div>
            <div className="bg-nvr-bg-primary rounded-lg p-3 border border-nvr-border/50">
              <p className="text-xs text-nvr-text-muted mb-1">Cameras</p>
              <p className="text-lg font-semibold text-nvr-text-primary font-mono">{metrics.camera_count}</p>
            </div>
            <div className="bg-nvr-bg-primary rounded-lg p-3 border border-nvr-border/50">
              <p className="text-xs text-nvr-text-muted mb-1">Uptime</p>
              <p className="text-lg font-semibold text-nvr-text-primary font-mono">{formatUptimeSeconds(metrics.uptime_seconds)}</p>
            </div>
          </div>
        ) : (
          <p className="text-nvr-text-muted text-sm">Loading metrics...</p>
        )}
      </div>

      {/* Storage Overview */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-nvr-text-primary">Storage Overview</h2>
          <button
            onClick={fetchStorage}
            className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors"
          >
            Refresh
          </button>
        </div>

        {storageLoading ? (
          <p className="text-nvr-text-muted text-sm">Loading storage info...</p>
        ) : storage ? (
          <div className="space-y-5">
            {/* Disk usage bar */}
            <div>
              <div className="flex justify-between text-sm mb-2">
                <span className="text-nvr-text-secondary">Disk Usage</span>
                <span className="text-nvr-text-primary">
                  {formatBytes(storage.used_bytes)} / {formatBytes(storage.total_bytes)} ({usedPercent}%)
                </span>
              </div>
              <div className="w-full h-4 bg-nvr-bg-primary rounded-full overflow-hidden flex">
                {recordingsPercent > 0 && (
                  <div
                    className="h-full bg-nvr-accent transition-all duration-500"
                    style={{ width: `${recordingsPercent}%` }}
                    title={`Recordings: ${formatBytes(storage.recordings_bytes)} (${recordingsPercent}%)`}
                  />
                )}
                {otherUsedPercent > 0 && (
                  <div
                    className="h-full bg-nvr-text-muted transition-all duration-500"
                    style={{ width: `${otherUsedPercent}%` }}
                    title={`Other: ${formatBytes(storage.used_bytes - storage.recordings_bytes)} (${otherUsedPercent}%)`}
                  />
                )}
              </div>
              <div className="flex flex-wrap gap-2 md:gap-4 mt-2 text-xs text-nvr-text-muted">
                <span className="flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm bg-nvr-accent" />
                  Recordings ({formatBytes(storage.recordings_bytes)})
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm bg-nvr-text-muted" />
                  Other ({formatBytes(storage.used_bytes - storage.recordings_bytes)})
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm bg-nvr-bg-primary border border-nvr-border" />
                  Free ({formatBytes(storage.free_bytes)})
                </span>
              </div>
            </div>

            {storage.per_camera.length > 0 && (
              <div>
                <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">Per-Camera Breakdown</h3>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-nvr-text-muted border-b border-nvr-border/50">
                        <th className="text-left py-2 font-medium">Camera</th>
                        <th className="text-right py-2 font-medium">Segments</th>
                        <th className="text-right py-2 font-medium">Size</th>
                        <th className="text-right py-2 font-medium">% of Recordings</th>
                      </tr>
                    </thead>
                    <tbody>
                      {storage.per_camera.map(cam => (
                        <tr key={cam.camera_id} className="border-b border-nvr-border/30">
                          <td className="py-2 text-nvr-text-primary">{cam.camera_name || cam.camera_id}</td>
                          <td className="py-2 text-right text-nvr-text-secondary">{cam.segment_count}</td>
                          <td className="py-2 text-right text-nvr-text-primary font-mono">{formatBytes(cam.total_bytes)}</td>
                          <td className="py-2 text-right text-nvr-text-secondary">
                            {storage.recordings_bytes > 0
                              ? Math.round((cam.total_bytes / storage.recordings_bytes) * 100)
                              : 0}%
                          </td>
                        </tr>
                      ))}
                    </tbody>
                    <tfoot>
                      <tr className="border-t border-nvr-border">
                        <td className="py-2 font-medium text-nvr-text-primary">Total</td>
                        <td className="py-2 text-right font-medium text-nvr-text-primary">
                          {storage.per_camera.reduce((sum, c) => sum + c.segment_count, 0)}
                        </td>
                        <td className="py-2 text-right font-medium text-nvr-text-primary font-mono">
                          {formatBytes(storage.recordings_bytes)}
                        </td>
                        <td className="py-2 text-right font-medium text-nvr-text-primary">100%</td>
                      </tr>
                    </tfoot>
                  </table>
                </div>
              </div>
            )}

            {storage.per_camera.length === 0 && (
              <p className="text-nvr-text-muted text-sm">No recordings found.</p>
            )}
          </div>
        ) : (
          <p className="text-nvr-text-muted text-sm">Unable to load storage information.</p>
        )}
      </div>

      {/* Recording Defaults */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
        <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Recording Defaults</h2>
        <p className="text-xs text-nvr-text-muted mb-4">
          These values are configured in mediamtx.yml under pathDefaults. Per-camera retention can be set on each camera.
        </p>
        <div>
          <div className="flex justify-between py-3 border-b border-nvr-border/50">
            <span className="text-sm text-nvr-text-secondary">Global Retention Period</span>
            <span className="text-sm text-nvr-text-primary font-mono">1d (recordDeleteAfter)</span>
          </div>
          <div className="flex justify-between py-3 border-b border-nvr-border/50">
            <span className="text-sm text-nvr-text-secondary">Recording Format</span>
            <span className="text-sm text-nvr-text-primary font-mono">fmp4</span>
          </div>
          <div className="flex justify-between py-3">
            <span className="text-sm text-nvr-text-secondary">Segment Duration</span>
            <span className="text-sm text-nvr-text-primary font-mono">1h</span>
          </div>
        </div>
      </div>

      {/* Config Management */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
        <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Configuration Management</h2>

        {/* Export */}
        <div className="mb-6">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-2">Export Configuration</h3>
          <p className="text-xs text-nvr-text-muted mb-3">
            Download a JSON backup of all cameras, recording rules, and user accounts (without passwords).
          </p>
          <button
            onClick={handleExport}
            disabled={exporting}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 text-sm min-h-[44px]"
          >
            {exporting ? 'Exporting...' : 'Export Configuration'}
          </button>
        </div>

        {/* Import */}
        <div>
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-2">Import Configuration</h3>
          <p className="text-xs text-nvr-text-muted mb-3">
            Upload a previously exported config file. Cameras and rules that already exist (by name) will be skipped. Users are never imported for security.
          </p>

          <input
            ref={fileInputRef}
            type="file"
            accept=".json"
            onChange={handleFileSelect}
            className="block w-full text-sm text-nvr-text-secondary file:mr-4 file:py-2 file:px-4 file:rounded-lg file:border file:border-nvr-border file:text-sm file:font-medium file:bg-nvr-bg-tertiary file:text-nvr-text-secondary hover:file:bg-nvr-border file:transition-colors file:cursor-pointer file:min-h-[44px] mb-3"
          />

          {importFile && (
            <div className="mb-3 p-3 bg-nvr-bg-tertiary border border-nvr-border rounded-lg">
              <p className="text-sm text-nvr-text-primary mb-1">Preview:</p>
              <ul className="text-xs text-nvr-text-secondary space-y-0.5">
                <li>{importFile.cameras?.length ?? 0} camera(s)</li>
                <li>{importFile.recording_rules?.length ?? 0} recording rule(s)</li>
                <li>{importFile.users?.length ?? 0} user(s) (will be skipped)</li>
              </ul>
              <button
                onClick={handleImport}
                disabled={importing}
                className="mt-3 bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 text-sm min-h-[44px]"
              >
                {importing ? 'Importing...' : 'Confirm Import'}
              </button>
            </div>
          )}

          {importError && (
            <p className="text-nvr-danger text-sm mt-2">{importError}</p>
          )}

          {importResult && (
            <div className="mt-3 p-3 bg-nvr-bg-tertiary border border-nvr-border rounded-lg">
              <p className="text-sm font-medium text-nvr-text-primary mb-1">Import Complete</p>
              <ul className="text-xs text-nvr-text-secondary space-y-0.5">
                <li>{importResult.cameras_imported} camera(s) imported, {importResult.cameras_skipped} skipped</li>
                <li>{importResult.rules_imported} rule(s) imported, {importResult.rules_skipped} skipped</li>
                <li>{importResult.users_skipped} user(s) skipped</li>
              </ul>
              {importResult.errors && importResult.errors.length > 0 && (
                <div className="mt-2">
                  <p className="text-xs text-nvr-danger">Errors:</p>
                  {importResult.errors.map((err, i) => (
                    <p key={i} className="text-xs text-nvr-danger">{err}</p>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>

      {/* Audit Log */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
          <h2 className="text-lg font-semibold text-nvr-text-primary">Audit Log</h2>
          <div className="flex items-center gap-2">
            <label className="text-xs text-nvr-text-muted">Filter:</label>
            <select
              value={auditFilterAction}
              onChange={e => setAuditFilterAction(e.target.value)}
              className="bg-nvr-bg-primary border border-nvr-border rounded px-2 py-1 text-xs text-nvr-text-primary focus:outline-none focus:border-nvr-accent"
            >
              <option value="">All actions</option>
              <option value="create">Create</option>
              <option value="update">Update</option>
              <option value="delete">Delete</option>
              <option value="login">Login</option>
              <option value="login_failed">Login failed</option>
            </select>
          </div>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-nvr-text-muted border-b border-nvr-border/50">
                <th className="text-left py-2 font-medium">Time</th>
                <th className="text-left py-2 font-medium">User</th>
                <th className="text-left py-2 font-medium">Action</th>
                <th className="text-left py-2 font-medium hidden md:table-cell">Resource</th>
                <th className="text-left py-2 font-medium hidden lg:table-cell">Details</th>
                <th className="text-left py-2 font-medium hidden lg:table-cell">IP</th>
              </tr>
            </thead>
            <tbody>
              {auditEntries.length === 0 && !auditLoading && (
                <tr>
                  <td colSpan={6} className="py-6 text-center text-nvr-text-muted text-sm">
                    No audit entries found.
                  </td>
                </tr>
              )}
              {auditEntries.map(entry => (
                <tr key={entry.id} className="border-b border-nvr-border/30">
                  <td className="py-2 text-nvr-text-secondary whitespace-nowrap text-xs">
                    {formatTimestamp(entry.created_at)}
                  </td>
                  <td className="py-2 text-nvr-text-primary">{entry.username}</td>
                  <td className="py-2">
                    <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${actionBadgeColor(entry.action)}`}>
                      {entry.action}
                    </span>
                  </td>
                  <td className="py-2 text-nvr-text-secondary hidden md:table-cell">
                    {entry.resource_type}
                    {entry.resource_id && (
                      <span className="text-nvr-text-muted ml-1 font-mono text-xs">
                        {entry.resource_id.substring(0, 8)}
                      </span>
                    )}
                  </td>
                  <td className="py-2 text-nvr-text-muted text-xs hidden lg:table-cell max-w-xs truncate">
                    {entry.details}
                  </td>
                  <td className="py-2 text-nvr-text-muted font-mono text-xs hidden lg:table-cell">
                    {entry.ip_address}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {auditLoading && (
          <p className="text-nvr-text-muted text-sm text-center py-3">Loading...</p>
        )}

        {!auditLoading && auditEntries.length < auditTotal && (
          <div className="text-center mt-3">
            <button
              onClick={loadMoreAudit}
              className="px-4 py-2 text-xs bg-nvr-bg-primary border border-nvr-border rounded-lg text-nvr-text-secondary hover:text-nvr-text-primary hover:border-nvr-accent transition-colors"
            >
              Load more ({auditEntries.length} of {auditTotal})
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
