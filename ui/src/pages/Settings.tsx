import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../api/client'
import { useCameras } from '../hooks/useCameras'

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

interface ConfigSummary {
  recording: {
    enabled: boolean
    format: string
    segment_duration: string
    delete_after: string
    path: string
  }
  cameras: {
    total: number
    online: number
    recording: number
  }
  recording_rules: {
    total: number
    active: number
  }
  users: {
    total: number
    admins: number
  }
  server: {
    rtsp_port: string
    webrtc_port: string
    api_port: string
    hls_port: string
  }
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

interface NetworkProtocol {
  address: string
  port: string
  encryption: string
}

interface NetworkConfig {
  protocols: Record<string, NetworkProtocol>
}

interface TLSServiceInfo {
  encryption: boolean
  cert_path: string
  key_path: string
  cert_exists: boolean
  key_exists: boolean
}

interface TLSStatus {
  services: Record<string, TLSServiceInfo>
}

interface UpdateInfo {
  current_version: string
  update_available: boolean
  message: string
}

type TabId = 'system' | 'appearance' | 'notifications' | 'storage' | 'config' | 'audit' | 'performance' | 'ai' | 'sysconfig'

const TABS: { id: TabId; label: string }[] = [
  { id: 'system', label: 'System' },
  { id: 'sysconfig', label: 'System Config' },
  { id: 'storage', label: 'Storage' },
  { id: 'ai', label: 'AI Analytics' },
  { id: 'notifications', label: 'Notifications' },
  { id: 'appearance', label: 'Appearance' },
  { id: 'config', label: 'Configuration' },
  { id: 'audit', label: 'Audit Log' },
  { id: 'performance', label: 'Performance' },
]

const TAB_DESCRIPTIONS: Record<TabId, string> = {
  system: 'System version, uptime, and server information',
  sysconfig: 'Network settings, TLS certificates, database backup, and software updates',
  storage: 'Disk usage and per-camera storage breakdown',
  ai: 'AI-powered object detection, classification, and semantic search',
  notifications: 'Configure how you receive alerts for motion and camera events',
  appearance: 'Theme, default layout, and display preferences',
  config: 'Export and import your NVR configuration',
  audit: 'Activity log of all user actions',
  performance: 'Server resource usage and active connections',
}

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

function formatUptimeHuman(uptime: string): string {
  const match = uptime.match(/(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?/)
  if (!match) return uptime
  const totalHours = match[1] ? parseInt(match[1]) : 0
  const minutes = match[2] ? parseInt(match[2]) : 0

  const days = Math.floor(totalHours / 24)
  const hours = totalHours % 24

  const parts: string[] = []
  if (days > 0) parts.push(`${days} day${days !== 1 ? 's' : ''}`)
  if (hours > 0) parts.push(`${hours} hour${hours !== 1 ? 's' : ''}`)
  if (minutes > 0 && days === 0) parts.push(`${minutes} min${minutes !== 1 ? 's' : ''}`)
  if (parts.length === 0) parts.push('just started')
  return parts.join(', ')
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
function IconUsers() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4-4v2" /><circle cx="9" cy="7" r="4" /><path d="M23 21v-2a4 4 0 00-3-3.87" /><path d="M16 3.13a4 4 0 010 7.75" /></svg>
}
function IconRules() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" /><line x1="3" y1="9" x2="21" y2="9" /><line x1="9" y1="21" x2="9" y2="9" /></svg>
}
function IconServer() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><rect x="2" y="2" width="20" height="8" rx="2" ry="2" /><rect x="2" y="14" width="20" height="8" rx="2" ry="2" /><line x1="6" y1="6" x2="6.01" y2="6" /><line x1="6" y1="18" x2="6.01" y2="18" /></svg>
}
function IconRecord() {
  return <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10" /><circle cx="12" cy="12" r="3" fill="currentColor" /></svg>
}

// -- Toggle switch component --
function Toggle({ checked, onChange, label, description }: { checked: boolean; onChange: (v: boolean) => void; label: string; description?: string }) {
  return (
    <label className="flex items-center justify-between py-3 cursor-pointer group">
      <div>
        <p className="text-sm text-nvr-text-primary group-hover:text-white transition-colors">{label}</p>
        {description && <p className="text-xs text-nvr-text-muted mt-0.5">{description}</p>}
      </div>
      <button
        role="switch"
        aria-checked={checked}
        onClick={() => onChange(!checked)}
        className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors shrink-0 ml-4 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
          checked ? 'bg-nvr-accent' : 'bg-nvr-bg-tertiary border border-nvr-border'
        }`}
      >
        <span
          className={`inline-block h-4 w-4 rounded-full bg-white transition-transform ${
            checked ? 'translate-x-6' : 'translate-x-1'
          }`}
        />
      </button>
    </label>
  )
}

export default function Settings() {
  // Page title
  useEffect(() => {
    document.title = 'Settings — MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  const [activeTab, setActiveTab] = useState<TabId>('system')
  const { cameras: allCameras } = useCameras()
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
  const [configSummary, setConfigSummary] = useState<ConfigSummary | null>(null)
  const [configLoading, setConfigLoading] = useState(true)

  // System Config tab state
  const [networkConfig, setNetworkConfig] = useState<NetworkConfig | null>(null)
  const [networkLoading, setNetworkLoading] = useState(true)
  const [tlsStatus, setTlsStatus] = useState<TLSStatus | null>(null)
  const [tlsLoading, setTlsLoading] = useState(true)
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)
  const [updateLoading, setUpdateLoading] = useState(false)
  const [dbBackupLoading, setDbBackupLoading] = useState(false)
  const [dbRestoreLoading, setDbRestoreLoading] = useState(false)
  const [dbRestoreResult, setDbRestoreResult] = useState<string | null>(null)
  const [dbRestoreError, setDbRestoreError] = useState('')
  const dbRestoreInputRef = useRef<HTMLInputElement>(null)

  // Branding state
  const [brandingProductName, setBrandingProductName] = useState('MediaMTX NVR')
  const [brandingAccentColor, setBrandingAccentColor] = useState('#6366f1')
  const [brandingLogoURL, setBrandingLogoURL] = useState('')
  const [brandingSaving, setBrandingSaving] = useState(false)
  const [brandingLogoUploading, setBrandingLogoUploading] = useState(false)
  const logoInputRef = useRef<HTMLInputElement>(null)

  // Appearance state
  const [theme, setTheme] = useState(() => localStorage.getItem('nvr-theme') || 'dark')
  const [defaultLayout, setDefaultLayout] = useState(() => {
    const saved = localStorage.getItem('nvr-live-layout')
    return saved ? parseInt(saved, 10) : 2
  })
  const [refreshInterval, setRefreshInterval] = useState(() => {
    const saved = localStorage.getItem('nvr-refresh-interval')
    return saved ? parseInt(saved, 10) : 15
  })

  // Notification preferences state
  const [notifEnabled, setNotifEnabled] = useState(() => localStorage.getItem('nvr-notif-enabled') !== 'false')
  const [notifMotion, setNotifMotion] = useState(() => localStorage.getItem('nvr-notif-motion') !== 'false')
  const [notifOffline, setNotifOffline] = useState(() => localStorage.getItem('nvr-notif-offline') !== 'false')
  const [notifSound, setNotifSound] = useState(() => localStorage.getItem('nvr-notif-sound') === 'true')

  // WebRTC stream count for system tab
  const [webrtcStreams, setWebrtcStreams] = useState<number | null>(null)

  // Apply theme on mount and change
  useEffect(() => {
    if (theme === 'oled') {
      document.documentElement.classList.add('theme-oled')
    } else {
      document.documentElement.classList.remove('theme-oled')
    }
    localStorage.setItem('nvr-theme', theme)
  }, [theme])

  // Fetch branding config
  useEffect(() => {
    apiFetch('/system/branding').then(async res => {
      if (res.ok) {
        const data = await res.json()
        setBrandingProductName(data.product_name || 'MediaMTX NVR')
        setBrandingAccentColor(data.accent_color || '#6366f1')
        setBrandingLogoURL(data.logo_url || '')
      }
    }).catch(() => {})
  }, [])

  useEffect(() => {
    apiFetch('/system/info').then(async res => {
      if (res.ok) setSystemInfo(await res.json())
    })
  }, [])

  useEffect(() => {
    apiFetch('/system/config').then(async res => {
      if (res.ok) setConfigSummary(await res.json())
      setConfigLoading(false)
    }).catch(() => setConfigLoading(false))
  }, [])

  // Fetch network config and TLS status for System Config tab
  useEffect(() => {
    apiFetch('/system/network').then(async res => {
      if (res.ok) setNetworkConfig(await res.json())
      setNetworkLoading(false)
    }).catch(() => setNetworkLoading(false))

    apiFetch('/system/tls').then(async res => {
      if (res.ok) setTlsStatus(await res.json())
      setTlsLoading(false)
    }).catch(() => setTlsLoading(false))
  }, [])

  // Fetch WebRTC stream count
  useEffect(() => {
    const fetchStreams = () => {
      // MediaMTX v1 lists paths at /v3/paths/list
      fetch('/v3/paths/list')
        .then(async res => {
          if (res.ok) {
            const data = await res.json()
            const items = data.items || []
            let count = 0
            for (const item of items) {
              if (item.readers) count += item.readers.length
            }
            setWebrtcStreams(count)
          }
        })
        .catch(() => {
          setWebrtcStreams(null)
        })
    }
    fetchStreams()
    const id = setInterval(fetchStreams, 15000)
    return () => clearInterval(id)
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

  // Branding handlers
  const handleBrandingSave = async () => {
    setBrandingSaving(true)
    try {
      const res = await apiFetch('/system/branding', {
        method: 'PUT',
        body: JSON.stringify({
          product_name: brandingProductName,
          accent_color: brandingAccentColor,
        }),
      })
      if (res.ok) {
        const data = await res.json()
        setBrandingProductName(data.product_name)
        setBrandingAccentColor(data.accent_color)
        // Dispatch a custom event so the header can update immediately.
        window.dispatchEvent(new CustomEvent('branding-updated', { detail: data }))
      }
    } catch {
      // ignore
    } finally {
      setBrandingSaving(false)
    }
  }

  const handleLogoUpload = async (file: File) => {
    setBrandingLogoUploading(true)
    try {
      const formData = new FormData()
      formData.append('logo', file)

      const token = (await import('../api/client')).getAccessToken()
      const res = await fetch('/api/nvr/system/branding/logo', {
        method: 'POST',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        credentials: 'include',
        body: formData,
      })
      if (res.ok) {
        const data = await res.json()
        setBrandingLogoURL(data.logo_url)
        window.dispatchEvent(new CustomEvent('branding-updated', { detail: { logo_url: data.logo_url } }))
      }
    } catch {
      // ignore
    } finally {
      setBrandingLogoUploading(false)
    }
  }

  const handleLogoDelete = async () => {
    try {
      const res = await apiFetch('/system/branding/logo', { method: 'DELETE' })
      if (res.ok) {
        setBrandingLogoURL('')
        window.dispatchEvent(new CustomEvent('branding-updated', { detail: { logo_url: '' } }))
      }
    } catch {
      // ignore
    }
  }

  // Appearance handlers
  const handleThemeChange = (newTheme: string) => {
    setTheme(newTheme)
  }

  const handleDefaultLayoutChange = (n: number) => {
    setDefaultLayout(n)
    localStorage.setItem('nvr-live-layout', String(n))
  }

  const handleRefreshIntervalChange = (val: number) => {
    setRefreshInterval(val)
    localStorage.setItem('nvr-refresh-interval', String(val))
  }

  // Notification preference handlers
  const handleNotifEnabled = (v: boolean) => {
    setNotifEnabled(v)
    localStorage.setItem('nvr-notif-enabled', String(v))
  }
  const handleNotifMotion = (v: boolean) => {
    setNotifMotion(v)
    localStorage.setItem('nvr-notif-motion', String(v))
  }
  const handleNotifOffline = (v: boolean) => {
    setNotifOffline(v)
    localStorage.setItem('nvr-notif-offline', String(v))
  }
  const handleNotifSound = (v: boolean) => {
    setNotifSound(v)
    localStorage.setItem('nvr-notif-sound', String(v))
  }

  // System Config handlers
  const handleCheckForUpdates = async () => {
    setUpdateLoading(true)
    try {
      const res = await apiFetch('/system/updates/check')
      if (res.ok) setUpdateInfo(await res.json())
    } finally {
      setUpdateLoading(false)
    }
  }

  const handleDbBackup = async () => {
    setDbBackupLoading(true)
    try {
      const res = await apiFetch('/system/backup/database')
      if (res.ok) {
        const blob = await res.blob()
        const url = URL.createObjectURL(blob)
        const a = document.createElement('a')
        a.href = url
        a.download = `nvr-database-backup-${new Date().toISOString().slice(0, 10)}.db`
        a.click()
        URL.revokeObjectURL(url)
      }
    } finally {
      setDbBackupLoading(false)
    }
  }

  const handleDbRestoreSelect = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    setDbRestoreLoading(true)
    setDbRestoreResult(null)
    setDbRestoreError('')

    try {
      const formData = new FormData()
      formData.append('database', file)

      const headers = new Headers()
      const token = (await import('../api/client')).getAccessToken()
      if (token) headers.set('Authorization', `Bearer ${token}`)

      const res = await fetch('/api/nvr/system/backup/restore', {
        method: 'POST',
        headers,
        credentials: 'include',
        body: formData,
      })

      if (res.ok) {
        const data = await res.json()
        setDbRestoreResult(data.message)
      } else {
        const data = await res.json().catch(() => ({}))
        setDbRestoreError(data.error || 'Restore failed')
      }
    } catch {
      setDbRestoreError('Network error during restore')
    } finally {
      setDbRestoreLoading(false)
      if (dbRestoreInputRef.current) dbRestoreInputRef.current.value = ''
    }
  }

  // Cleanup dialog state
  const [cleanupCameraId, setCleanupCameraId] = useState<string | null>(null)
  const [cleanupCameraName, setCleanupCameraName] = useState('')
  const [cleanupDate, setCleanupDate] = useState('')
  const [cleanupLoading, setCleanupLoading] = useState(false)
  const [cleanupResult, setCleanupResult] = useState<{ deleted_count: number; bytes_freed: number } | null>(null)

  const handleOpenCleanup = (cameraId: string, cameraName: string) => {
    setCleanupCameraId(cameraId)
    setCleanupCameraName(cameraName)
    setCleanupDate(new Date().toISOString().slice(0, 10))
    setCleanupResult(null)
  }

  const handleCloseCleanup = () => {
    setCleanupCameraId(null)
    setCleanupCameraName('')
    setCleanupDate('')
    setCleanupResult(null)
  }

  const handleConfirmCleanup = async () => {
    if (!cleanupCameraId || !cleanupDate) return
    setCleanupLoading(true)
    try {
      const res = await apiFetch('/recordings/cleanup', {
        method: 'DELETE',
        body: JSON.stringify({
          camera_id: cleanupCameraId,
          before: `${cleanupDate}T00:00:00Z`,
        }),
      })
      if (res.ok) {
        const data = await res.json()
        setCleanupResult(data)
        fetchStorage()
      }
    } finally {
      setCleanupLoading(false)
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
            className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors whitespace-nowrap focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
              activeTab === tab.id
                ? 'bg-nvr-accent text-white'
                : 'text-nvr-text-secondary hover:bg-nvr-bg-tertiary hover:text-nvr-text-primary'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <p className="text-xs text-nvr-text-muted mb-4">{TAB_DESCRIPTIONS[activeTab]}</p>

      {/* ===== SYSTEM TAB ===== */}
      {activeTab === 'system' && (
        <div className="space-y-6">
          {/* Version hero card */}
          {systemInfo ? (
            <div className="bg-gradient-to-br from-nvr-accent/10 to-nvr-bg-secondary border border-nvr-accent/20 rounded-xl p-6">
              <div className="flex items-center gap-4">
                <div className="w-14 h-14 rounded-xl bg-nvr-accent/20 flex items-center justify-center shrink-0">
                  <IconServer />
                </div>
                <div className="flex-1 min-w-0">
                  <h2 className="text-lg font-bold text-nvr-text-primary">MediaMTX NVR</h2>
                  <p className="text-2xl font-mono font-semibold text-nvr-accent mt-0.5">v{systemInfo.version}</p>
                </div>
                <div className="text-right hidden sm:block">
                  <p className="text-xs text-nvr-text-muted">Platform</p>
                  <p className="text-sm text-nvr-text-primary font-mono">{systemInfo.platform}</p>
                </div>
              </div>
            </div>
          ) : (
            <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 flex items-center gap-2">
              <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
              <span className="text-nvr-text-muted text-sm">Loading system info...</span>
            </div>
          )}

          {/* System details */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">System Details</h2>
            {systemInfo ? (
              <div className="space-y-0">
                <div className="flex justify-between py-3 border-b border-nvr-border/50">
                  <span className="text-sm text-nvr-text-secondary">Uptime</span>
                  <div className="text-right">
                    <span className="text-sm text-nvr-text-primary">{formatUptimeHuman(systemInfo.uptime)}</span>
                    <span className="text-xs text-nvr-text-muted ml-2">({formatUptime(systemInfo.uptime)})</span>
                  </div>
                </div>
                <div className="flex justify-between py-3 border-b border-nvr-border/50">
                  <span className="text-sm text-nvr-text-secondary">Platform</span>
                  <span className="text-sm text-nvr-text-primary font-mono">{systemInfo.platform}</span>
                </div>
                <div className="flex justify-between py-3 border-b border-nvr-border/50">
                  <span className="text-sm text-nvr-text-secondary">Active Streams</span>
                  <span className="text-sm text-nvr-text-primary font-mono">
                    {webrtcStreams !== null ? webrtcStreams : '--'}
                  </span>
                </div>
                <div className="flex justify-between items-center py-3">
                  <span className="text-sm text-nvr-text-secondary">Server Controls</span>
                  <button
                    disabled
                    className="text-xs font-medium px-3 py-1.5 rounded-lg bg-nvr-bg-tertiary text-nvr-text-muted border border-nvr-border cursor-not-allowed"
                    title="Server restart will be available in a future update"
                  >
                    Restart Server (coming soon)
                  </button>
                </div>
              </div>
            ) : (
              <div className="flex items-center gap-2 py-4">
                <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                <span className="text-nvr-text-muted text-sm">Loading...</span>
              </div>
            )}
          </div>
        </div>
      )}

      {/* ===== APPEARANCE TAB ===== */}
      {activeTab === 'appearance' && (
        <div className="space-y-6">
          {/* Theme selector */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Theme</h2>
            <p className="text-xs text-nvr-text-muted mb-4">Choose a visual theme for the NVR interface.</p>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <button
                onClick={() => handleThemeChange('dark')}
                className={`relative rounded-xl p-4 text-left transition-all focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                  theme === 'dark'
                    ? 'bg-nvr-accent/10 border-2 border-nvr-accent'
                    : 'bg-nvr-bg-primary border-2 border-nvr-border hover:border-nvr-text-muted'
                }`}
              >
                <div className="flex items-center gap-3 mb-3">
                  <div className="w-8 h-8 rounded-lg bg-[#0f1117] border border-[#2d3140]" />
                  <div>
                    <p className="text-sm font-medium text-nvr-text-primary">Dark</p>
                    <p className="text-xs text-nvr-text-muted">Default dark theme</p>
                  </div>
                </div>
                {/* Preview swatches */}
                <div className="flex gap-1.5">
                  <div className="w-6 h-3 rounded bg-[#0f1117] border border-[#2d3140]" />
                  <div className="w-6 h-3 rounded bg-[#1a1d27]" />
                  <div className="w-6 h-3 rounded bg-[#242836]" />
                </div>
                {theme === 'dark' && (
                  <div className="absolute top-3 right-3">
                    <svg className="w-5 h-5 text-nvr-accent" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                    </svg>
                  </div>
                )}
              </button>
              <button
                onClick={() => handleThemeChange('oled')}
                className={`relative rounded-xl p-4 text-left transition-all focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                  theme === 'oled'
                    ? 'bg-nvr-accent/10 border-2 border-nvr-accent'
                    : 'bg-nvr-bg-primary border-2 border-nvr-border hover:border-nvr-text-muted'
                }`}
              >
                <div className="flex items-center gap-3 mb-3">
                  <div className="w-8 h-8 rounded-lg bg-[#000000] border border-[#1a1a1a]" />
                  <div>
                    <p className="text-sm font-medium text-nvr-text-primary">OLED Black</p>
                    <p className="text-xs text-nvr-text-muted">Pure black for AMOLED</p>
                  </div>
                </div>
                <div className="flex gap-1.5">
                  <div className="w-6 h-3 rounded bg-[#000000] border border-[#1a1a1a]" />
                  <div className="w-6 h-3 rounded bg-[#0a0a0a]" />
                  <div className="w-6 h-3 rounded bg-[#141414]" />
                </div>
                {theme === 'oled' && (
                  <div className="absolute top-3 right-3">
                    <svg className="w-5 h-5 text-nvr-accent" fill="currentColor" viewBox="0 0 20 20">
                      <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                    </svg>
                  </div>
                )}
              </button>
            </div>
          </div>

          {/* Grid default layout */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Default Live View Layout</h2>
            <p className="text-xs text-nvr-text-muted mb-4">Choose the default grid layout when opening the Live View page.</p>
            <div className="flex gap-2">
              {[1, 2, 3, 4].map(n => (
                <button
                  key={n}
                  onClick={() => handleDefaultLayoutChange(n)}
                  className={`flex-1 py-3 rounded-lg text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                    defaultLayout === n
                      ? 'bg-nvr-accent text-white'
                      : 'bg-nvr-bg-primary text-nvr-text-secondary border border-nvr-border hover:border-nvr-text-muted hover:text-nvr-text-primary'
                  }`}
                >
                  {n}x{n}
                </button>
              ))}
            </div>
          </div>

          {/* Auto-refresh interval */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Camera Status Refresh</h2>
            <p className="text-xs text-nvr-text-muted mb-4">
              How often to poll camera status. Lower values use more bandwidth. Takes effect on next page load.
            </p>
            <div className="flex items-center gap-4">
              <input
                type="range"
                min={5}
                max={60}
                step={5}
                value={refreshInterval}
                onChange={e => handleRefreshIntervalChange(parseInt(e.target.value, 10))}
                className="flex-1 accent-nvr-accent h-2 rounded-full appearance-none bg-nvr-bg-primary cursor-pointer"
              />
              <span className="text-sm font-mono text-nvr-text-primary w-12 text-right shrink-0">
                {refreshInterval}s
              </span>
            </div>
            <div className="flex justify-between mt-1">
              <span className="text-xs text-nvr-text-muted">5s</span>
              <span className="text-xs text-nvr-text-muted">60s</span>
            </div>
          </div>

          {/* Branding customization */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">Branding</h2>
            <p className="text-xs text-nvr-text-muted mb-4">
              Customize the product name, accent color, and logo shown throughout the NVR interface. Changes persist across restarts.
            </p>

            {/* Logo upload */}
            <div className="mb-5">
              <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Logo</label>
              <div className="flex items-center gap-4">
                {brandingLogoURL ? (
                  <div className="w-12 h-12 rounded-lg bg-nvr-bg-primary border border-nvr-border flex items-center justify-center overflow-hidden">
                    <img src={brandingLogoURL} alt="Logo" className="max-w-full max-h-full object-contain" />
                  </div>
                ) : (
                  <div className="w-12 h-12 rounded-lg bg-nvr-bg-primary border border-nvr-border flex items-center justify-center">
                    <svg className="w-6 h-6 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909M3.75 21h16.5A2.25 2.25 0 0022.5 18.75V5.25A2.25 2.25 0 0020.25 3H3.75A2.25 2.25 0 001.5 5.25v13.5A2.25 2.25 0 003.75 21z" />
                    </svg>
                  </div>
                )}
                <div className="flex gap-2">
                  <input
                    ref={logoInputRef}
                    type="file"
                    accept="image/png,image/jpeg,image/svg+xml,image/webp"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0]
                      if (file) handleLogoUpload(file)
                      e.target.value = ''
                    }}
                  />
                  <button
                    onClick={() => logoInputRef.current?.click()}
                    disabled={brandingLogoUploading}
                    className="px-3 py-1.5 text-xs font-medium bg-nvr-bg-primary border border-nvr-border rounded-lg text-nvr-text-secondary hover:text-nvr-text-primary hover:border-nvr-text-muted transition-colors disabled:opacity-50"
                  >
                    {brandingLogoUploading ? 'Uploading...' : 'Upload Logo'}
                  </button>
                  {brandingLogoURL && (
                    <button
                      onClick={handleLogoDelete}
                      className="px-3 py-1.5 text-xs font-medium text-nvr-danger border border-nvr-border rounded-lg hover:bg-nvr-danger/10 transition-colors"
                    >
                      Remove
                    </button>
                  )}
                </div>
              </div>
              <p className="text-xs text-nvr-text-muted mt-1.5">PNG, JPEG, SVG, or WebP. Max 2 MB.</p>
            </div>

            {/* Product name */}
            <div className="mb-5">
              <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Product Name</label>
              <input
                type="text"
                value={brandingProductName}
                onChange={(e) => setBrandingProductName(e.target.value)}
                maxLength={100}
                className="w-full max-w-sm px-3 py-2 bg-nvr-bg-primary border border-nvr-border rounded-lg text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent/30 outline-none transition-colors"
                placeholder="MediaMTX NVR"
              />
            </div>

            {/* Accent color */}
            <div className="mb-5">
              <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Accent Color</label>
              <div className="flex items-center gap-3">
                <input
                  type="color"
                  value={brandingAccentColor}
                  onChange={(e) => setBrandingAccentColor(e.target.value)}
                  className="w-10 h-10 rounded-lg border border-nvr-border cursor-pointer bg-transparent"
                />
                <input
                  type="text"
                  value={brandingAccentColor}
                  onChange={(e) => {
                    const v = e.target.value
                    if (/^#[0-9a-fA-F]{0,6}$/.test(v)) setBrandingAccentColor(v)
                  }}
                  maxLength={7}
                  className="w-28 px-3 py-2 bg-nvr-bg-primary border border-nvr-border rounded-lg text-sm text-nvr-text-primary font-mono focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent/30 outline-none transition-colors"
                  placeholder="#6366f1"
                />
                <div
                  className="w-24 h-8 rounded-lg border border-nvr-border"
                  style={{ backgroundColor: brandingAccentColor }}
                />
              </div>
              <div className="flex gap-2 mt-2">
                {['#6366f1', '#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899'].map(color => (
                  <button
                    key={color}
                    onClick={() => setBrandingAccentColor(color)}
                    className={`w-6 h-6 rounded-full border-2 transition-transform hover:scale-110 ${
                      brandingAccentColor === color ? 'border-white scale-110' : 'border-transparent'
                    }`}
                    style={{ backgroundColor: color }}
                    title={color}
                  />
                ))}
              </div>
            </div>

            {/* Save button */}
            <button
              onClick={handleBrandingSave}
              disabled={brandingSaving}
              className="px-4 py-2 text-sm font-medium bg-nvr-accent text-white rounded-lg hover:bg-nvr-accent/90 transition-colors disabled:opacity-50"
            >
              {brandingSaving ? 'Saving...' : 'Save Branding'}
            </button>
          </div>
        </div>
      )}

      {/* ===== NOTIFICATIONS TAB ===== */}
      {activeTab === 'notifications' && (
        <div className="space-y-6">
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">In-App Notifications</h2>
            <p className="text-xs text-nvr-text-muted mb-2">Control which notifications appear as toast pop-ups in the NVR interface.</p>
            <div className="divide-y divide-nvr-border/50">
              <Toggle
                checked={notifEnabled}
                onChange={handleNotifEnabled}
                label="Enable notifications"
                description="Master toggle for all in-app notification pop-ups"
              />
              <Toggle
                checked={notifMotion}
                onChange={handleNotifMotion}
                label="Motion alerts"
                description="Show a notification when motion is detected on a camera"
              />
              <Toggle
                checked={notifOffline}
                onChange={handleNotifOffline}
                label="Camera offline/online alerts"
                description="Notify when a camera goes offline or comes back online"
              />
              <Toggle
                checked={notifSound}
                onChange={handleNotifSound}
                label="Alert sound"
                description="Play a short tone when a notification appears"
              />
            </div>
          </div>
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
                    className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
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
                            <div className="flex items-center gap-2">
                              <span className="text-xs text-nvr-text-secondary font-mono">{formatBytes(cam.total_bytes)}</span>
                              <button
                                onClick={() => handleOpenCleanup(cam.camera_id, cam.camera_name || cam.camera_id)}
                                className="text-xs text-nvr-text-muted hover:text-nvr-danger transition-colors px-1.5 py-0.5 rounded hover:bg-nvr-danger/10 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                                title="Clean up old recordings"
                              >
                                Clean Up
                              </button>
                            </div>
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

          {/* Cleanup dialog */}
          {cleanupCameraId && (
            <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={handleCloseCleanup}>
              <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
              <div
                className="relative z-10 bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl w-full max-w-sm mx-4 p-5"
                onClick={(e) => e.stopPropagation()}
              >
                <h3 className="text-lg font-semibold text-nvr-text-primary mb-2">Clean Up Recordings</h3>
                <p className="text-sm text-nvr-text-secondary mb-4">
                  Delete recordings for <span className="font-medium text-nvr-text-primary">{cleanupCameraName}</span> older than:
                </p>

                {!cleanupResult ? (
                  <>
                    <input
                      type="date"
                      value={cleanupDate}
                      onChange={e => setCleanupDate(e.target.value)}
                      className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors mb-4"
                    />
                    <div className="flex justify-end gap-2">
                      <button
                        onClick={handleCloseCleanup}
                        className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      >
                        Cancel
                      </button>
                      <button
                        onClick={handleConfirmCleanup}
                        disabled={cleanupLoading || !cleanupDate}
                        className="bg-nvr-danger hover:bg-nvr-danger-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      >
                        {cleanupLoading ? 'Deleting...' : 'Delete Recordings'}
                      </button>
                    </div>
                  </>
                ) : (
                  <>
                    <div className="bg-nvr-success/10 border border-nvr-success/20 rounded-lg p-3 mb-4">
                      <p className="text-sm text-nvr-success">
                        Deleted {cleanupResult.deleted_count} recording{cleanupResult.deleted_count !== 1 ? 's' : ''}, freed {formatBytes(cleanupResult.bytes_freed)}.
                      </p>
                    </div>
                    <div className="flex justify-end">
                      <button
                        onClick={handleCloseCleanup}
                        className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                      >
                        Done
                      </button>
                    </div>
                  </>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {/* ===== CONFIGURATION TAB ===== */}
      {activeTab === 'config' && (
        <div className="space-y-6">
          {configLoading ? (
            <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 flex items-center justify-center py-12">
              <span className="inline-block w-5 h-5 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-3" />
              <span className="text-nvr-text-muted">Loading configuration...</span>
            </div>
          ) : configSummary ? (
            <>
              {/* NVR Overview */}
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">NVR Overview</h2>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                    <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                      <IconCamera />
                      <p className="text-xs">Cameras</p>
                    </div>
                    <p className="text-xl font-semibold text-nvr-text-primary font-mono">{configSummary.cameras.total}</p>
                    <p className="text-xs text-nvr-text-muted mt-1">
                      {configSummary.cameras.online} online, {configSummary.cameras.recording} recording
                    </p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                    <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                      <IconRules />
                      <p className="text-xs">Recording Rules</p>
                    </div>
                    <p className="text-xl font-semibold text-nvr-text-primary font-mono">{configSummary.recording_rules.total}</p>
                    <p className="text-xs text-nvr-text-muted mt-1">
                      {configSummary.recording_rules.active} active now
                    </p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                    <div className="flex items-center gap-2 mb-2 text-nvr-text-muted">
                      <IconUsers />
                      <p className="text-xs">Users</p>
                    </div>
                    <p className="text-xl font-semibold text-nvr-text-primary font-mono">{configSummary.users.total}</p>
                    <p className="text-xs text-nvr-text-muted mt-1">
                      {configSummary.users.admins} admin{configSummary.users.admins !== 1 ? 's' : ''}
                    </p>
                  </div>
                </div>
              </div>

              {/* Server Ports */}
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                <div className="flex items-center gap-2 mb-4">
                  <IconServer />
                  <h2 className="text-lg font-semibold text-nvr-text-primary">Server Ports</h2>
                </div>
                <p className="text-xs text-nvr-text-muted mb-4">
                  Useful for configuring firewall rules and client connections.
                </p>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">RTSP</p>
                    <p className="text-sm font-semibold text-nvr-text-primary font-mono">{configSummary.server.rtsp_port}</p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">WebRTC</p>
                    <p className="text-sm font-semibold text-nvr-text-primary font-mono">{configSummary.server.webrtc_port}</p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">API</p>
                    <p className="text-sm font-semibold text-nvr-text-primary font-mono">{configSummary.server.api_port}</p>
                  </div>
                  <div className="bg-nvr-bg-primary rounded-lg p-3 text-center border border-nvr-border/50">
                    <p className="text-xs text-nvr-text-muted mb-1">HLS</p>
                    <p className="text-sm font-semibold text-nvr-text-primary font-mono">{configSummary.server.hls_port}</p>
                  </div>
                </div>
              </div>

              {/* Recording Configuration */}
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
                <div className="flex items-center gap-2 mb-2">
                  <IconRecord />
                  <h2 className="text-lg font-semibold text-nvr-text-primary">Recording Configuration</h2>
                </div>
                <p className="text-xs text-nvr-text-muted mb-4">
                  Read from mediamtx.yml pathDefaults. Per-camera retention can be set on each camera.
                </p>
                <div className="space-y-0">
                  <div className="flex justify-between py-3 border-b border-nvr-border/50">
                    <span className="text-sm text-nvr-text-secondary">Recording Enabled</span>
                    <span className={`text-sm font-mono ${configSummary.recording.enabled ? 'text-green-400' : 'text-nvr-text-muted'}`}>
                      {configSummary.recording.enabled ? 'Yes' : 'No'}
                    </span>
                  </div>
                  <div className="flex justify-between py-3 border-b border-nvr-border/50">
                    <span className="text-sm text-nvr-text-secondary">Format</span>
                    <span className="text-sm text-nvr-text-primary font-mono">{configSummary.recording.format}</span>
                  </div>
                  <div className="flex justify-between py-3 border-b border-nvr-border/50">
                    <span className="text-sm text-nvr-text-secondary">Segment Duration</span>
                    <span className="text-sm text-nvr-text-primary font-mono">{configSummary.recording.segment_duration}</span>
                  </div>
                  <div className="flex justify-between py-3 border-b border-nvr-border/50">
                    <span className="text-sm text-nvr-text-secondary">Retention Period</span>
                    <span className="text-sm text-nvr-text-primary font-mono">{configSummary.recording.delete_after}</span>
                  </div>
                  <div className="flex justify-between py-3">
                    <span className="text-sm text-nvr-text-secondary">Recording Path</span>
                    <span className="text-sm text-nvr-text-primary font-mono text-right max-w-[60%] truncate" title={configSummary.recording.path}>
                      {configSummary.recording.path}
                    </span>
                  </div>
                </div>
              </div>
            </>
          ) : (
            <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 text-center">
              <p className="text-nvr-text-muted text-sm">Unable to load configuration. Admin access may be required.</p>
            </div>
          )}

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
                  className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 w-full justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
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
                    className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    {importing && <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
                    {importing ? 'Importing...' : 'Confirm Import'}
                  </button>
                  <button
                    onClick={() => { setImportFile(null); if (fileInputRef.current) fileInputRef.current.value = '' }}
                    className="text-nvr-text-secondary hover:text-nvr-text-primary text-sm px-3 py-2 transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
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
              className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
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
                className={`px-3 py-1.5 rounded-full text-xs font-medium transition-colors capitalize focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
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
                    <td colSpan={6} className="py-12 text-center">
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 mx-auto mb-3 text-nvr-text-muted/30" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" /><polyline points="14 2 14 8 20 8" /><line x1="16" y1="13" x2="8" y2="13" /><line x1="16" y1="17" x2="8" y2="17" /><polyline points="10 9 9 9 8 9" /></svg>
                      <p className="text-nvr-text-muted text-sm max-w-xs mx-auto">
                        No activity recorded yet. Actions like adding cameras, changing settings, and user logins will appear here.
                      </p>
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
                className="px-4 py-2 text-sm bg-nvr-bg-primary border border-nvr-border rounded-lg text-nvr-text-secondary hover:text-nvr-text-primary hover:border-nvr-accent transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                Load more ({auditEntries.length} of {auditTotal})
              </button>
            </div>
          )}
        </div>
      )}

      {/* ===== PERFORMANCE TAB ===== */}
      {/* ===== AI ANALYTICS TAB ===== */}
      {activeTab === 'ai' && (
        <div className="space-y-6">
          {/* Model Info */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">AI Models</h2>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="bg-nvr-bg-input rounded-lg p-3">
                <p className="text-sm font-medium text-nvr-text-primary">YOLOv8n</p>
                <p className="text-xs text-nvr-text-muted">Real-time detection</p>
                <p className="text-xs text-nvr-success mt-1">Loaded</p>
              </div>
              <div className="bg-nvr-bg-input rounded-lg p-3">
                <p className="text-sm font-medium text-nvr-text-primary">YOLOv8m</p>
                <p className="text-xs text-nvr-text-muted">High-accuracy refinement</p>
                <p className="text-xs text-nvr-success mt-1">Loaded</p>
              </div>
              <div className="bg-nvr-bg-input rounded-lg p-3">
                <p className="text-sm font-medium text-nvr-text-primary">CLIP ViT-B/32</p>
                <p className="text-xs text-nvr-text-muted">Semantic search</p>
                <p className="text-xs text-nvr-success mt-1">Loaded</p>
              </div>
            </div>
          </div>

          {/* Cameras with AI */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Active Cameras</h2>
            <p className="text-xs text-nvr-text-muted mb-3">Cameras with AI detection enabled</p>
            {(() => {
              const aiCameras = allCameras.filter(c => c.ai_enabled)
              if (aiCameras.length === 0) {
                return (
                  <div className="text-center py-6">
                    <p className="text-sm text-nvr-text-muted">No cameras have AI detection enabled.</p>
                    <p className="text-xs text-nvr-text-muted mt-1">Enable AI on individual cameras in the Cameras page.</p>
                  </div>
                )
              }
              return (
                <div className="space-y-2">
                  {aiCameras.map(cam => (
                    <div key={cam.id} className="flex items-center justify-between bg-nvr-bg-input rounded-lg p-3">
                      <div className="flex items-center gap-3">
                        <span className={`w-2 h-2 rounded-full ${cam.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'}`} />
                        <div>
                          <p className="text-sm font-medium text-nvr-text-primary">{cam.name}</p>
                          {cam.sub_stream_url && (
                            <p className="text-xs text-nvr-text-muted font-mono truncate max-w-[300px]">{cam.sub_stream_url}</p>
                          )}
                        </div>
                      </div>
                      <span className="text-xs text-nvr-success font-medium">AI Active</span>
                    </div>
                  ))}
                </div>
              )
            })()}
          </div>

          {/* How it works */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <h2 className="text-lg font-semibold text-nvr-text-primary mb-2">How AI Detection Works</h2>
            <div className="text-sm text-nvr-text-secondary space-y-2">
              <p>Each enabled camera's sub stream is analyzed frame-by-frame</p>
              <p>YOLOv8 detects people, vehicles, and animals in real-time</p>
              <p>CLIP generates visual embeddings for each detection</p>
              <p>Search across all detections using natural language</p>
            </div>
          </div>
        </div>
      )}

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

      {/* ===== SYSTEM CONFIG TAB ===== */}
      {activeTab === 'sysconfig' && (
        <div className="space-y-6">
          {/* Network Settings */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <div className="flex items-center gap-2 mb-4">
              <svg className="w-5 h-5 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" />
              </svg>
              <h2 className="text-lg font-semibold text-nvr-text-primary">Network Settings</h2>
            </div>
            <p className="text-xs text-nvr-text-muted mb-4">
              Bind addresses and ports for each protocol. These are read from mediamtx.yml and require a server restart to change.
            </p>

            {networkLoading ? (
              <div className="flex items-center gap-2 py-6 justify-center">
                <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                <span className="text-nvr-text-muted text-sm">Loading network config...</span>
              </div>
            ) : networkConfig ? (
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-nvr-text-muted border-b border-nvr-border/50">
                      <th className="text-left py-2 font-medium">Protocol</th>
                      <th className="text-left py-2 font-medium">Address</th>
                      <th className="text-left py-2 font-medium">Port</th>
                      <th className="text-left py-2 font-medium">Encryption</th>
                    </tr>
                  </thead>
                  <tbody>
                    {Object.entries(networkConfig.protocols).map(([name, proto]) => (
                      <tr key={name} className="border-b border-nvr-border/30 hover:bg-nvr-bg-tertiary/30 transition-colors">
                        <td className="py-2.5 text-nvr-text-primary font-medium uppercase">{name}</td>
                        <td className="py-2.5 text-nvr-text-secondary font-mono">{proto.address}</td>
                        <td className="py-2.5 text-nvr-text-primary font-mono font-semibold">{proto.port}</td>
                        <td className="py-2.5">
                          <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${
                            proto.encryption === 'yes' || proto.encryption === 'strict'
                              ? 'bg-green-500/20 text-green-400'
                              : proto.encryption === 'optional'
                                ? 'bg-yellow-500/20 text-yellow-400'
                                : 'bg-nvr-text-muted/20 text-nvr-text-muted'
                          }`}>
                            {proto.encryption}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <p className="text-nvr-text-muted text-sm text-center py-4">Unable to load network configuration.</p>
            )}
          </div>

          {/* TLS Certificates */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <div className="flex items-center gap-2 mb-4">
              <svg className="w-5 h-5 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z" />
              </svg>
              <h2 className="text-lg font-semibold text-nvr-text-primary">TLS Certificates</h2>
            </div>
            <p className="text-xs text-nvr-text-muted mb-4">
              TLS/SSL certificate status for encrypted protocols. Configure certificate paths in mediamtx.yml.
            </p>

            {tlsLoading ? (
              <div className="flex items-center gap-2 py-6 justify-center">
                <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                <span className="text-nvr-text-muted text-sm">Loading TLS status...</span>
              </div>
            ) : tlsStatus ? (
              <div className="space-y-3">
                {Object.entries(tlsStatus.services).map(([name, svc]) => (
                  <div key={name} className="bg-nvr-bg-primary rounded-lg p-4 border border-nvr-border/50">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-sm font-medium text-nvr-text-primary uppercase">{name}</span>
                      <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${
                        svc.encryption
                          ? 'bg-green-500/20 text-green-400'
                          : 'bg-nvr-text-muted/20 text-nvr-text-muted'
                      }`}>
                        {svc.encryption ? 'Encryption Enabled' : 'Encryption Disabled'}
                      </span>
                    </div>
                    <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-xs">
                      <div className="flex items-center gap-2">
                        <span className={`w-2 h-2 rounded-full shrink-0 ${svc.cert_exists ? 'bg-green-400' : 'bg-red-400'}`} />
                        <span className="text-nvr-text-secondary">Certificate:</span>
                        <span className="text-nvr-text-primary font-mono truncate" title={svc.cert_path}>{svc.cert_path}</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <span className={`w-2 h-2 rounded-full shrink-0 ${svc.key_exists ? 'bg-green-400' : 'bg-red-400'}`} />
                        <span className="text-nvr-text-secondary">Key:</span>
                        <span className="text-nvr-text-primary font-mono truncate" title={svc.key_path}>{svc.key_path}</span>
                      </div>
                    </div>
                    {svc.encryption && (!svc.cert_exists || !svc.key_exists) && (
                      <div className="mt-2 bg-red-500/10 border border-red-500/20 rounded-lg p-2 flex items-center gap-2">
                        <svg className="w-4 h-4 text-red-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" />
                        </svg>
                        <span className="text-xs text-red-400">
                          {!svc.cert_exists && !svc.key_exists
                            ? 'Certificate and key files not found. Encryption may fail.'
                            : !svc.cert_exists
                              ? 'Certificate file not found.'
                              : 'Key file not found.'}
                        </span>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <p className="text-nvr-text-muted text-sm text-center py-4">Unable to load TLS status.</p>
            )}
          </div>

          {/* Database Backup & Restore */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <div className="flex items-center gap-2 mb-4">
              <svg className="w-5 h-5 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
              </svg>
              <h2 className="text-lg font-semibold text-nvr-text-primary">Database Backup & Restore</h2>
            </div>
            <p className="text-xs text-nvr-text-muted mb-4">
              Back up the full SQLite database including cameras, users, recording rules, and all settings.
              For configuration-only backup, use the Configuration tab.
            </p>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              {/* Backup */}
              <div>
                <h3 className="text-sm font-medium text-nvr-text-secondary mb-2">Download Backup</h3>
                <p className="text-xs text-nvr-text-muted mb-3">
                  Download a complete copy of the NVR database file.
                </p>
                <button
                  onClick={handleDbBackup}
                  disabled={dbBackupLoading}
                  className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 w-full justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  {dbBackupLoading ? (
                    <>
                      <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                      Preparing...
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                        <polyline points="7 10 12 15 17 10" />
                        <line x1="12" y1="15" x2="12" y2="3" />
                      </svg>
                      Download Database
                    </>
                  )}
                </button>
              </div>

              {/* Restore */}
              <div>
                <h3 className="text-sm font-medium text-nvr-text-secondary mb-2">Restore from Backup</h3>
                <p className="text-xs text-nvr-text-muted mb-3">
                  Upload a previously backed up database file. A server restart is required after restore.
                </p>
                <button
                  onClick={() => dbRestoreInputRef.current?.click()}
                  disabled={dbRestoreLoading}
                  className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2.5 rounded-lg border border-nvr-border transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 w-full justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  {dbRestoreLoading ? (
                    <>
                      <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                      Restoring...
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                        <path strokeLinecap="round" strokeLinejoin="round" d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
                        <polyline points="17 8 12 3 7 8" />
                        <line x1="12" y1="3" x2="12" y2="15" />
                      </svg>
                      Upload Database File
                    </>
                  )}
                </button>
                <input
                  ref={dbRestoreInputRef}
                  type="file"
                  accept=".db,.sqlite,.sqlite3"
                  onChange={handleDbRestoreSelect}
                  className="hidden"
                />
              </div>
            </div>

            {dbRestoreResult && (
              <div className="mt-4 bg-amber-500/10 border border-amber-500/20 rounded-lg p-3 flex items-center gap-2">
                <svg className="w-4 h-4 text-amber-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <span className="text-sm text-amber-400">{dbRestoreResult}</span>
              </div>
            )}
            {dbRestoreError && (
              <p className="text-nvr-danger text-sm mt-3">{dbRestoreError}</p>
            )}
          </div>

          {/* Check for Updates */}
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-6">
            <div className="flex items-center gap-2 mb-4">
              <svg className="w-5 h-5 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
              <h2 className="text-lg font-semibold text-nvr-text-primary">Software Updates</h2>
            </div>

            <div className="flex items-center gap-4 mb-4">
              <div className="flex-1">
                <p className="text-sm text-nvr-text-secondary">
                  Current version: <span className="font-mono font-semibold text-nvr-text-primary">
                    {systemInfo ? `v${systemInfo.version}` : '--'}
                  </span>
                </p>
                {updateInfo && (
                  <p className="text-xs text-nvr-text-muted mt-1">{updateInfo.message}</p>
                )}
              </div>
              <button
                onClick={handleCheckForUpdates}
                disabled={updateLoading}
                className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors disabled:opacity-50 disabled:cursor-not-allowed text-sm inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                {updateLoading ? (
                  <>
                    <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                    Checking...
                  </>
                ) : (
                  'Check for Updates'
                )}
              </button>
            </div>

            {updateInfo && (
              <div className={`rounded-lg p-3 flex items-center gap-2 ${
                updateInfo.update_available
                  ? 'bg-nvr-accent/10 border border-nvr-accent/20'
                  : 'bg-green-500/10 border border-green-500/20'
              }`}>
                {updateInfo.update_available ? (
                  <>
                    <svg className="w-4 h-4 text-nvr-accent shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M9 19l3 3m0 0l3-3m-3 3V10" />
                    </svg>
                    <span className="text-sm text-nvr-accent">An update is available.</span>
                  </>
                ) : (
                  <>
                    <svg className="w-4 h-4 text-green-400 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                    <span className="text-sm text-green-400">{updateInfo.message}</span>
                  </>
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
