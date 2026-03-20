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
  warning: boolean
  critical: boolean
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

type TabId = 'system' | 'storage' | 'config' | 'audit' | 'performance'

const TABS: { id: TabId; label: string }[] = [
  { id: 'system', label: 'System' },
  { id: 'storage', label: 'Storage' },
  { id: 'config', label: 'Configuration' },
  { id: 'audit', label: 'Audit Log' },
  { id: 'performance', label: 'Performance' },
]

const AUDIT_ACTIONS = ['create', 'update', 'delete', 'login', 'login_failed']

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
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const parts: string[] = []
  if (d > 0) parts.push(`${d}d`)
  if (h > 0) parts.push(`${h}h`)
  if (m > 0 || parts.length === 0) parts.push(`${m}m`)
  return parts.join(' ')
}

function relativeTime(ts: string): string {
  try {
    const now = Date.now()
    const then = new Date(ts).getTime()
    const diff = Math.floor((now - then) / 1000)
    if (diff < 60) return 'just now'
    if (diff < 3600) return `${Math.floor(diff / 60)} min ago`
    if (diff < 86400) return `${Math.floor(diff / 3600)} hour${Math.floor(diff / 3600) !== 1 ? 's' : ''} ago`
    if (diff < 604800) return `${Math.floor(diff / 86400)} day${Math.floor(diff / 86400) !== 1 ? 's' : ''} ago`
    return new Date(ts).toLocaleDateString()
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

// -- Stat card icon SVGs --
function IconTasks() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12" /></svg>
}
function IconMemory() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><rect x="4" y="4" width="16" height="16" rx="2" /><rect x="9" y="9" width="6" height="6" /><line x1="9" y1="1" x2="9" y2="4" /><line x1="15" y1="1" x2="15" y2="4" /><line x1="9" y1="20" x2="9" y2="23" /><line x1="15" y1="20" x2="15" y2="23" /><line x1="20" y1="9" x2="23" y2="9" /><line x1="20" y1="14" x2="23" y2="14" /><line x1="1" y1="9" x2="4" y2="9" /><line x1="1" y1="14" x2="4" y2="14" /></svg>
}
function IconCleanup() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="23 4 23 10 17 10" /><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10" /></svg>
}
function IconCamera() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M23 19a2 2 0 01-2 2H3a2 2 0 01-2-2V8a2 2 0 012-2h4l2-3h6l2 3h4a2 2 0 012 2z" /><circle cx="12" cy="13" r="4" /></svg>
}
function IconClock() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><polyline points="12 6 12 12 16 14" /></svg>
}

export default function Settings() {
  const [activeTab, setActiveTab] = useState<TabId>('system')
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
  const [isDragging, setIsDragging] = useState(false)

  useEffect(() => {
    apiFetch('/system/info').then(async res => {
      if (res.ok) setSystemInfo(await res.json())
    })
  }, [])

  const fetchStorage = useCallback(() => {
    apiFetch('/system/storage').then(async res => {
      if (res.ok) setStorage(await res.json())
      setStorageLoading(false)
    }).catch(() => setStorageLoading(false))
  }, [])

  useEffect(() => {
    fetchStorage()
    const interval = setInterval(fetchStorage, 30000)
    return () => clearInterval(interval)
  }, [fetchStorage])

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

  const processFile = (file: File) => {
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

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) processFile(file)
  }

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setIsDragging(false)
    const file = e.dataTransfer.files[0]
    if (file && file.name.endsWith('.json')) {
      processFile(file)
    } else {
      setImportError('Please drop a .json file')
    }
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

  // Find max per-camera bytes for bar chart scaling
  const maxCameraBytes = storage?.per_camera
    ? Math.max(...storage.per_camera.map(c => c.total_bytes), 1)
    : 1

  return (
    <div>
      <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary mb-4 md:mb-6">Settings</h1>

      {/* Tab navigation */}
      <div className="flex gap-1 mb-6 overflow-x-auto pb-1 -mx-1 px-1">
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors whitespace-nowrap ${
              activeTab === tab.id
                ? 'bg-nvr-accent text-white'
                : 'text-nvr-text-secondary hover:bg-nvr-bg-tertiary hover:text-nvr-text-primary'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* ===== SYSTEM TAB ===== */}
      {activeTab === 'system' && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
          <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">System Information</h2>
          {systemInfo ? (
            <div className="space-y-0">
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
            <div className="flex items-center gap-2 py-4">
              <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
              <span className="text-nvr-text-muted text-sm">Loading...</span>
            </div>
          )}
        </div>
      )}

      {/* ===== STORAGE TAB ===== */}
      {activeTab === 'storage' && (
        <div className="space-y-6">
          {storageLoading ? (
            <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 flex items-center justify-center py-12">
              <span className="inline-block w-5 h-5 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-3" />
              <span className="text-nvr-text-muted">Loading storage info...</span>
            </div>
          ) : storage ? (
            <>
              {/* Main usage card */}
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                <div className="flex items-center justify-between mb-4">
                  <h2 className="text-lg font-semibold text-nvr-text-primary">Disk Usage</h2>
                  <button
                    onClick={fetchStorage}
                    className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors"
                  >
                    Refresh
                  </button>
                </div>

                {/* Large usage bar */}
                <div className="mb-4">
                  <div className={`w-full h-6 rounded-full overflow-hidden flex ${
                    usedPercent > 85 ? 'bg-amber-500/10' : 'bg-nvr-bg-primary'
                  }`}>
                    <div
                      className={`h-full transition-all duration-500 ${
                        usedPercent > 85 ? 'bg-amber-500' : 'bg-nvr-accent'
                      }`}
                      style={{ width: `${usedPercent}%` }}
                    />
                  </div>
                  <div className="flex justify-between mt-2">
                    <span className="text-sm text-nvr-text-secondary">{usedPercent}% used</span>
                    <span className={`text-sm font-medium ${
                      usedPercent > 85 ? 'text-amber-400' : 'text-nvr-text-primary'
                    }`}>
                      Free: {formatBytes(storage.free_bytes)}
                    </span>
                  </div>
                </div>

                {/* Quick stats */}
                <div className="grid grid-cols-3 gap-3">
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">Total</p>
                    <p className="text-sm font-semibold text-nvr-text-primary">{formatBytes(storage.total_bytes)}</p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">Recordings</p>
                    <p className="text-sm font-semibold text-nvr-accent">{formatBytes(storage.recordings_bytes)}</p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">Other</p>
                    <p className="text-sm font-semibold text-nvr-text-primary">{formatBytes(storage.used_bytes - storage.recordings_bytes)}</p>
                  </div>
                </div>

                {usedPercent > 85 && (
                  <div className="mt-4 bg-amber-500/10 border border-amber-500/20 rounded-lg p-3 flex items-center gap-2">
                    <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4 text-amber-400 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" /><line x1="12" y1="9" x2="12" y2="13" /><line x1="12" y1="17" x2="12.01" y2="17" /></svg>
                    <span className="text-sm text-amber-400">Disk usage is above 85%. Consider increasing retention cleanup or adding storage.</span>
                  </div>
                )}
              </div>

              {/* Per-camera breakdown */}
              {storage.per_camera.length > 0 && (
                <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                  <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Per-Camera Storage</h2>
                  <div className="space-y-3">
                    {storage.per_camera.map(cam => {
                      const pct = Math.round((cam.total_bytes / maxCameraBytes) * 100)
                      return (
                        <div key={cam.camera_id}>
                          <div className="flex items-center justify-between mb-1">
                            <span className="text-sm text-nvr-text-primary">{cam.camera_name || cam.camera_id}</span>
                            <span className="text-xs text-nvr-text-secondary font-mono">{formatBytes(cam.total_bytes)}</span>
                          </div>
                          <div className="w-full h-2.5 bg-nvr-bg-primary rounded-full overflow-hidden">
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
              )}

              {storage.per_camera.length === 0 && (
                <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 text-center">
                  <p className="text-nvr-text-muted text-sm">No recordings found.</p>
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

      {/* ===== CONFIGURATION TAB ===== */}
      {activeTab === 'config' && (
        <div className="space-y-6">
          {/* Recording Defaults */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-2">Recording Defaults</h2>
            <p className="text-xs text-nvr-text-muted mb-4">
              Configured in mediamtx.yml under pathDefaults. Per-camera retention can be set on each camera.
            </p>
            <div className="space-y-0">
              <div className="flex justify-between py-3 border-b border-nvr-border/50">
                <span className="text-sm text-nvr-text-secondary">Global Retention Period</span>
                <span className="text-sm text-nvr-text-primary font-mono">1d</span>
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

          {/* Export / Import */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Backup & Restore</h2>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Export */}
              <div>
                <h3 className="text-sm font-medium text-nvr-text-secondary mb-2">Export</h3>
                <p className="text-xs text-nvr-text-muted mb-3">
                  Download a JSON backup of cameras, recording rules, and user accounts (without passwords).
                </p>
                <button
                  onClick={handleExport}
                  disabled={exporting}
                  className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2.5 rounded-lg transition-colors disabled:opacity-50 text-sm inline-flex items-center gap-2 w-full justify-center"
                >
                  {exporting ? (
                    <>
                      <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                      Exporting...
                    </>
                  ) : (
                    <>
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
                      Download Configuration
                    </>
                  )}
                </button>
              </div>

              {/* Import */}
              <div>
                <h3 className="text-sm font-medium text-nvr-text-secondary mb-2">Import</h3>
                <p className="text-xs text-nvr-text-muted mb-3">
                  Upload a previously exported config. Existing items are skipped. Users are never imported.
                </p>

                {/* Drop zone */}
                <div
                  onDragOver={e => { e.preventDefault(); setIsDragging(true) }}
                  onDragLeave={() => setIsDragging(false)}
                  onDrop={handleDrop}
                  onClick={() => fileInputRef.current?.click()}
                  className={`border-2 border-dashed rounded-lg p-4 text-center cursor-pointer transition-colors ${
                    isDragging
                      ? 'border-nvr-accent bg-nvr-accent/5'
                      : 'border-nvr-border hover:border-nvr-text-muted'
                  }`}
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="w-6 h-6 mx-auto mb-2 text-nvr-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="17 8 12 3 7 8" /><line x1="12" y1="3" x2="12" y2="15" /></svg>
                  <p className="text-sm text-nvr-text-secondary">Drop JSON file here or click to browse</p>
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".json"
                    onChange={handleFileSelect}
                    className="hidden"
                  />
                </div>
              </div>
            </div>

            {/* Import preview */}
            {importFile && (
              <div className="mt-4 p-4 bg-nvr-bg-tertiary border border-nvr-border rounded-lg">
                <p className="text-sm font-medium text-nvr-text-primary mb-2">Preview import:</p>
                <ul className="text-xs text-nvr-text-secondary space-y-0.5 mb-3">
                  <li>{importFile.cameras?.length ?? 0} camera(s)</li>
                  <li>{importFile.recording_rules?.length ?? 0} recording rule(s)</li>
                  <li>{importFile.users?.length ?? 0} user(s) (will be skipped)</li>
                </ul>
                <div className="flex gap-2">
                  <button
                    onClick={handleImport}
                    disabled={importing}
                    className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 text-sm inline-flex items-center gap-2"
                  >
                    {importing && <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
                    {importing ? 'Importing...' : 'Confirm Import'}
                  </button>
                  <button
                    onClick={() => { setImportFile(null); if (fileInputRef.current) fileInputRef.current.value = '' }}
                    className="text-nvr-text-secondary hover:text-nvr-text-primary text-sm px-3 py-2 transition-colors"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            )}

            {importError && (
              <p className="text-nvr-danger text-sm mt-3">{importError}</p>
            )}

            {importResult && (
              <div className="mt-4 p-4 bg-green-500/5 border border-green-500/20 rounded-lg">
                <p className="text-sm font-medium text-green-400 mb-1">Import Complete</p>
                <ul className="text-xs text-nvr-text-secondary space-y-0.5">
                  <li>{importResult.cameras_imported} camera(s) imported, {importResult.cameras_skipped} skipped</li>
                  <li>{importResult.rules_imported} rule(s) imported, {importResult.rules_skipped} skipped</li>
                  <li>{importResult.users_skipped} user(s) skipped</li>
                </ul>
                {importResult.errors && importResult.errors.length > 0 && (
                  <div className="mt-2">
                    {importResult.errors.map((err, i) => (
                      <p key={i} className="text-xs text-nvr-danger">{err}</p>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {/* ===== AUDIT LOG TAB ===== */}
      {activeTab === 'audit' && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
          <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Audit Log</h2>

          {/* Action filter chips */}
          <div className="flex flex-wrap gap-2 mb-4">
            <button
              onClick={() => setAuditFilterAction('')}
              className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors ${
                auditFilterAction === ''
                  ? 'bg-nvr-accent text-white'
                  : 'bg-nvr-bg-primary text-nvr-text-secondary hover:bg-nvr-bg-tertiary border border-nvr-border'
              }`}
            >
              All
            </button>
            {AUDIT_ACTIONS.map(action => (
              <button
                key={action}
                onClick={() => setAuditFilterAction(action === auditFilterAction ? '' : action)}
                className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors capitalize ${
                  auditFilterAction === action
                    ? 'bg-nvr-accent text-white'
                    : 'bg-nvr-bg-primary text-nvr-text-secondary hover:bg-nvr-bg-tertiary border border-nvr-border'
                }`}
              >
                {action.replace('_', ' ')}
              </button>
            ))}
          </div>

          {/* Audit table */}
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
                    <td colSpan={6} className="py-8 text-center text-nvr-text-muted text-sm">
                      No audit entries found.
                    </td>
                  </tr>
                )}
                {auditEntries.map(entry => (
                  <tr key={entry.id} className="border-b border-nvr-border/30 hover:bg-nvr-bg-tertiary/30 transition-colors">
                    <td className="py-2.5 text-nvr-text-secondary whitespace-nowrap text-xs" title={new Date(entry.created_at).toLocaleString()}>
                      {relativeTime(entry.created_at)}
                    </td>
                    <td className="py-2.5 text-nvr-text-primary">{entry.username}</td>
                    <td className="py-2.5">
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${actionBadgeColor(entry.action)}`}>
                        {entry.action}
                      </span>
                    </td>
                    <td className="py-2.5 text-nvr-text-secondary hidden md:table-cell">
                      {entry.resource_type}
                      {entry.resource_id && (
                        <span className="text-nvr-text-muted ml-1 font-mono text-xs">
                          {entry.resource_id.substring(0, 8)}
                        </span>
                      )}
                    </td>
                    <td className="py-2.5 text-nvr-text-muted text-xs hidden lg:table-cell max-w-xs truncate">
                      {entry.details}
                    </td>
                    <td className="py-2.5 text-nvr-text-muted font-mono text-xs hidden lg:table-cell">
                      {entry.ip_address}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {auditLoading && (
            <div className="flex items-center justify-center py-4">
              <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-2" />
              <span className="text-nvr-text-muted text-sm">Loading...</span>
            </div>
          )}

          {!auditLoading && auditEntries.length < auditTotal && (
            <div className="text-center mt-4">
              <button
                onClick={loadMoreAudit}
                className="px-4 py-2 text-sm bg-nvr-bg-primary border border-nvr-border rounded-lg text-nvr-text-secondary hover:text-nvr-text-primary hover:border-nvr-accent transition-colors"
              >
                Load more ({auditEntries.length} of {auditTotal})
              </button>
            </div>
          )}
        </div>
      )}

      {/* ===== PERFORMANCE TAB ===== */}
      {activeTab === 'performance' && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-nvr-text-primary">Performance</h2>
            <span className="text-xs text-nvr-text-muted">Auto-refreshes every 10s</span>
          </div>
          {metrics ? (
            <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
              <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                  <IconTasks />
                  <p className="text-xs">Active Tasks</p>
                </div>
                <p className="text-xl font-semibold text-nvr-text-primary font-mono">{metrics.cpu_goroutines}</p>
              </div>
              <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                  <IconMemory />
                  <p className="text-xs">Memory Used</p>
                </div>
                <p className="text-xl font-semibold text-nvr-text-primary font-mono">{formatBytes(metrics.mem_alloc_bytes)}</p>
              </div>
              <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                  <IconMemory />
                  <p className="text-xs">Memory Reserved</p>
                </div>
                <p className="text-xl font-semibold text-nvr-text-primary font-mono">{formatBytes(metrics.mem_sys_bytes)}</p>
              </div>
              <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                  <IconCleanup />
                  <p className="text-xs">Cleanup Cycles</p>
                </div>
                <p className="text-xl font-semibold text-nvr-text-primary font-mono">{metrics.mem_gc_count}</p>
              </div>
              <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                  <IconCamera />
                  <p className="text-xs">Cameras</p>
                </div>
                <p className="text-xl font-semibold text-nvr-text-primary font-mono">{metrics.camera_count}</p>
              </div>
              <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                  <IconClock />
                  <p className="text-xs">Uptime</p>
                </div>
                <p className="text-xl font-semibold text-nvr-text-primary font-mono">{formatUptimeSeconds(metrics.uptime_seconds)}</p>
              </div>
            </div>
          ) : (
            <div className="flex items-center gap-2 py-8 justify-center">
              <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
              <span className="text-nvr-text-muted text-sm">Loading metrics...</span>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
