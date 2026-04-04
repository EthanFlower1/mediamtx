import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface SystemInfo {
  version: string
  platform: string
  uptime: string
}

interface MetricsCurrent {
  cpu_percent: number
  mem_percent: number
  mem_alloc_mb: number
  mem_sys_mb: number
  goroutines: number
}

interface MetricsSample {
  timestamp: string
  cpu_percent: number
  mem_percent: number
}

interface MetricsData {
  cpu_goroutines: number
  mem_alloc_bytes: number
  mem_sys_bytes: number
  mem_gc_count: number
  uptime_seconds: number
  camera_count: number
  current: MetricsCurrent
  history: MetricsSample[] | null
}

interface StorageData {
  total_bytes: number
  used_bytes: number
  free_bytes: number
  recordings_bytes: number
  per_camera: { camera_id: string; camera_name: string; total_bytes: number; segment_count: number }[]
  warning: boolean
  critical: boolean
}

interface RecordingHealthEntry {
  camera_id: string
  camera_name: string
  status: string
  last_segment_time: string | null
  stall_detected_at?: string
  restart_attempts: number
  last_error?: string
}

interface ConnectionState {
  camera_id: string
  camera_name: string
  state: string
  since: string
  reconnect_attempts: number
  last_error: string
}

interface DiskIOPathStatus {
  path: string
  status: string
  avg_latency_ms: number
  p99_latency_ms: number
  warn_threshold_ms: number
  critical_threshold_ms: number
}

/* ------------------------------------------------------------------ */
/*  Refresh interval options                                           */
/* ------------------------------------------------------------------ */

const REFRESH_OPTIONS = [
  { label: '5s', value: 5000 },
  { label: '10s', value: 10000 },
  { label: '30s', value: 30000 },
  { label: '1m', value: 60000 },
  { label: 'Off', value: 0 },
]

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`
}

function formatUptime(uptimeStr: string): string {
  // Parse Go duration strings like "2h30m15.123456s"
  const hours = uptimeStr.match(/(\d+)h/)
  const minutes = uptimeStr.match(/(\d+)m(?!s)/)
  const seconds = uptimeStr.match(/([\d.]+)s/)

  const h = hours ? parseInt(hours[1]) : 0
  const m = minutes ? parseInt(minutes[1]) : 0
  const s = seconds ? Math.floor(parseFloat(seconds[1])) : 0

  if (h > 24) {
    const days = Math.floor(h / 24)
    const remainH = h % 24
    return `${days}d ${remainH}h ${m}m`
  }
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function statusColor(status: string): string {
  switch (status) {
    case 'healthy':
    case 'recording':
    case 'online':
    case 'ok':
      return 'text-nvr-success'
    case 'warning':
    case 'warn':
    case 'stalled':
      return 'text-nvr-warning'
    case 'error':
    case 'critical':
    case 'failed':
    case 'offline':
    case 'disconnected':
      return 'text-nvr-danger'
    default:
      return 'text-nvr-text-muted'
  }
}

function statusBg(status: string): string {
  switch (status) {
    case 'healthy':
    case 'recording':
    case 'online':
    case 'ok':
      return 'bg-nvr-success/10 border-nvr-success/30'
    case 'warning':
    case 'warn':
    case 'stalled':
      return 'bg-nvr-warning/10 border-nvr-warning/30'
    case 'error':
    case 'critical':
    case 'failed':
    case 'offline':
    case 'disconnected':
      return 'bg-nvr-danger/10 border-nvr-danger/30'
    default:
      return 'bg-nvr-bg-tertiary border-nvr-border'
  }
}

function statusDot(status: string): string {
  switch (status) {
    case 'healthy':
    case 'recording':
    case 'online':
    case 'ok':
      return 'bg-nvr-success'
    case 'warning':
    case 'warn':
    case 'stalled':
      return 'bg-nvr-warning'
    case 'error':
    case 'critical':
    case 'failed':
    case 'offline':
    case 'disconnected':
      return 'bg-nvr-danger'
    default:
      return 'bg-nvr-text-muted'
  }
}

function timeAgo(dateStr: string): string {
  const now = Date.now()
  const then = new Date(dateStr).getTime()
  const diffMs = now - then
  const diffSec = Math.floor(diffMs / 1000)
  if (diffSec < 60) return `${diffSec}s ago`
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDay = Math.floor(diffHr / 24)
  return `${diffDay}d ago`
}

/* ------------------------------------------------------------------ */
/*  Gauge component                                                    */
/* ------------------------------------------------------------------ */

function GaugeRing({ value, label, color }: { value: number; label: string; color: string }) {
  const radius = 36
  const circumference = 2 * Math.PI * radius
  const pct = Math.min(Math.max(value, 0), 100)
  const offset = circumference - (pct / 100) * circumference

  return (
    <div className="flex flex-col items-center gap-1">
      <div className="relative w-24 h-24">
        <svg className="w-full h-full -rotate-90" viewBox="0 0 80 80">
          <circle
            cx="40" cy="40" r={radius}
            fill="none" stroke="currentColor"
            className="text-nvr-bg-tertiary"
            strokeWidth="6"
          />
          <circle
            cx="40" cy="40" r={radius}
            fill="none"
            stroke={color}
            strokeWidth="6"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={offset}
            className="transition-all duration-500"
          />
        </svg>
        <div className="absolute inset-0 flex items-center justify-center">
          <span className="text-lg font-bold text-nvr-text-primary">{pct.toFixed(0)}%</span>
        </div>
      </div>
      <span className="text-xs text-nvr-text-secondary font-medium">{label}</span>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Mini spark bar (last N samples)                                    */
/* ------------------------------------------------------------------ */

function SparkBars({ data, color }: { data: number[]; color: string }) {
  if (data.length === 0) return null
  const max = Math.max(...data, 1)
  const barCount = Math.min(data.length, 30)
  const samples = data.slice(-barCount)

  return (
    <div className="flex items-end gap-px h-8">
      {samples.map((v, i) => (
        <div
          key={i}
          className="flex-1 min-w-[2px] max-w-[6px] rounded-t transition-all duration-300"
          style={{
            height: `${Math.max((v / max) * 100, 4)}%`,
            backgroundColor: color,
            opacity: 0.4 + (i / samples.length) * 0.6,
          }}
        />
      ))}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Stat card                                                          */
/* ------------------------------------------------------------------ */

function StatCard({
  title,
  value,
  subtitle,
  icon,
  valueColor,
}: {
  title: string
  value: string | number
  subtitle?: string
  icon: React.ReactNode
  valueColor?: string
}) {
  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg p-4 flex items-start gap-3">
      <div className="w-10 h-10 rounded-lg bg-nvr-bg-tertiary flex items-center justify-center flex-shrink-0 text-nvr-text-muted">
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-nvr-text-muted font-medium uppercase tracking-wider">{title}</p>
        <p className={`text-xl font-bold mt-0.5 ${valueColor || 'text-nvr-text-primary'}`}>{value}</p>
        {subtitle && <p className="text-xs text-nvr-text-muted mt-0.5">{subtitle}</p>}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main page component                                                */
/* ------------------------------------------------------------------ */

export default function SystemHealth() {
  const [refreshInterval, setRefreshInterval] = useState(10000)
  const [lastRefreshed, setLastRefreshed] = useState<Date>(new Date())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Data states
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [metrics, setMetrics] = useState<MetricsData | null>(null)
  const [storage, setStorage] = useState<StorageData | null>(null)
  const [recordingHealth, setRecordingHealth] = useState<RecordingHealthEntry[]>([])
  const [connections, setConnections] = useState<ConnectionState[]>([])
  const [diskIO, setDiskIO] = useState<DiskIOPathStatus[]>([])

  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchAll = useCallback(async () => {
    try {
      const results = await Promise.allSettled([
        apiFetch('/system/info').then(r => r.ok ? r.json() : null),
        apiFetch('/system/metrics').then(r => r.ok ? r.json() : null),
        apiFetch('/system/storage').then(r => r.ok ? r.json() : null),
        apiFetch('/recordings/health').then(r => r.ok ? r.json() : null),
        apiFetch('/connections').then(r => r.ok ? r.json() : null),
        apiFetch('/system/disk-io').then(r => r.ok ? r.json() : null),
      ])

      const getValue = <T,>(r: PromiseSettledResult<T>) =>
        r.status === 'fulfilled' ? r.value : null

      const infoData = getValue(results[0]) as SystemInfo | null
      const metricsData = getValue(results[1]) as MetricsData | null
      const storageData = getValue(results[2]) as StorageData | null
      const healthData = getValue(results[3]) as { cameras: RecordingHealthEntry[] } | null
      const connData = getValue(results[4]) as { cameras: ConnectionState[] } | null
      const diskData = getValue(results[5]) as { paths: Record<string, DiskIOPathStatus> } | null

      if (infoData) setInfo(infoData)
      if (metricsData) setMetrics(metricsData)
      if (storageData) setStorage(storageData)
      setRecordingHealth(healthData?.cameras ?? [])
      setConnections(connData?.cameras ?? [])

      // Flatten disk IO paths object into array
      if (diskData?.paths) {
        const entries = Object.entries(diskData.paths).map(([path, status]) => ({
          ...status,
          path,
        }))
        setDiskIO(entries)
      } else {
        setDiskIO([])
      }

      setLastRefreshed(new Date())
      setError(null)
      setLoading(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch system data')
      setLoading(false)
    }
  }, [])

  // Initial fetch
  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  // Auto-refresh
  useEffect(() => {
    if (timerRef.current) clearInterval(timerRef.current)
    if (refreshInterval > 0) {
      timerRef.current = setInterval(fetchAll, refreshInterval)
    }
    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [refreshInterval, fetchAll])

  // Compute derived values
  const cpuPercent = metrics?.current?.cpu_percent ?? 0
  const memPercent = metrics?.current?.mem_percent ?? 0
  const diskPercent = storage ? (storage.used_bytes / storage.total_bytes) * 100 : 0

  const cpuColor = cpuPercent > 90 ? '#ef4444' : cpuPercent > 70 ? '#f59e0b' : '#22c55e'
  const memColor = memPercent > 90 ? '#ef4444' : memPercent > 70 ? '#f59e0b' : '#22c55e'
  const diskColor = diskPercent > 95 ? '#ef4444' : diskPercent > 85 ? '#f59e0b' : '#22c55e'

  const cpuHistory = (metrics?.history ?? []).map(s => s.cpu_percent)
  const memHistory = (metrics?.history ?? []).map(s => s.mem_percent)

  const healthyCameras = recordingHealth.filter(c => c.status === 'recording' || c.status === 'healthy').length
  const stalledCameras = recordingHealth.filter(c => c.status === 'stalled').length
  const errorCameras = recordingHealth.filter(c => c.status === 'error' || c.status === 'failed').length

  const overallStatus = error ? 'error' :
    (storage?.critical || errorCameras > 0) ? 'critical' :
    (storage?.warning || stalledCameras > 0) ? 'warning' : 'healthy'

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <svg className="w-8 h-8 text-nvr-accent animate-spin" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* ---- Header ---- */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-nvr-text-primary">System Health</h1>
          <p className="text-sm text-nvr-text-muted mt-1">
            Real-time system monitoring and diagnostics
          </p>
        </div>
        <div className="flex items-center gap-3">
          {/* Overall status badge */}
          <div className={`flex items-center gap-2 px-3 py-1.5 rounded-full border text-xs font-medium ${statusBg(overallStatus)} ${statusColor(overallStatus)}`}>
            <span className={`w-2 h-2 rounded-full ${statusDot(overallStatus)} ${overallStatus === 'healthy' ? 'animate-pulse' : ''}`} />
            {overallStatus === 'healthy' ? 'All Systems Healthy' :
             overallStatus === 'warning' ? 'Warnings Detected' : 'Issues Detected'}
          </div>

          {/* Refresh interval selector */}
          <div className="flex items-center gap-1.5 bg-nvr-bg-secondary border border-nvr-border rounded-lg px-2 py-1">
            <svg className="w-3.5 h-3.5 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            <select
              value={refreshInterval}
              onChange={(e) => setRefreshInterval(Number(e.target.value))}
              className="bg-transparent text-xs text-nvr-text-secondary border-none outline-none cursor-pointer"
            >
              {REFRESH_OPTIONS.map(opt => (
                <option key={opt.value} value={opt.value}>{opt.label}</option>
              ))}
            </select>
          </div>

          {/* Manual refresh */}
          <button
            onClick={fetchAll}
            className="p-1.5 rounded-lg bg-nvr-bg-secondary border border-nvr-border text-nvr-text-secondary hover:text-nvr-text-primary transition-colors"
            title="Refresh now"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>
      </div>

      {/* Last refreshed */}
      <p className="text-xs text-nvr-text-muted -mt-4">
        Last updated: {lastRefreshed.toLocaleTimeString()}
      </p>

      {error && (
        <div className="bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg p-3 text-sm text-nvr-danger">
          {error}
        </div>
      )}

      {/* ---- Resource Gauges ---- */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg p-6">
        <h2 className="text-sm font-semibold text-nvr-text-primary mb-4 uppercase tracking-wider">Resource Usage</h2>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
          <div className="flex flex-col items-center gap-3">
            <GaugeRing value={cpuPercent} label="CPU" color={cpuColor} />
            {cpuHistory.length > 0 && <SparkBars data={cpuHistory} color={cpuColor} />}
          </div>
          <div className="flex flex-col items-center gap-3">
            <GaugeRing value={memPercent} label="Memory" color={memColor} />
            {memHistory.length > 0 && <SparkBars data={memHistory} color={memColor} />}
          </div>
          <div className="flex flex-col items-center gap-3">
            <GaugeRing value={diskPercent} label="Disk" color={diskColor} />
            <p className="text-xs text-nvr-text-muted">
              {formatBytes(storage?.free_bytes ?? 0)} free of {formatBytes(storage?.total_bytes ?? 0)}
            </p>
          </div>
        </div>
      </div>

      {/* ---- Summary stats ---- */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Uptime"
          value={info ? formatUptime(info.uptime) : '--'}
          subtitle={info?.version ? `v${info.version}` : undefined}
          icon={
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          }
        />
        <StatCard
          title="Cameras"
          value={metrics?.camera_count ?? 0}
          subtitle={`${healthyCameras} recording`}
          icon={
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
          }
        />
        <StatCard
          title="Goroutines"
          value={metrics?.cpu_goroutines ?? 0}
          subtitle={`${formatBytes(metrics?.mem_alloc_bytes ?? 0)} allocated`}
          icon={
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
          }
        />
        <StatCard
          title="Recordings Storage"
          value={formatBytes(storage?.recordings_bytes ?? 0)}
          subtitle={`${storage?.per_camera?.length ?? 0} cameras with data`}
          icon={
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
            </svg>
          }
        />
      </div>

      {/* ---- Recording Pipeline Status ---- */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg overflow-hidden">
        <div className="px-4 py-3 border-b border-nvr-border flex items-center justify-between">
          <h2 className="text-sm font-semibold text-nvr-text-primary uppercase tracking-wider">
            Recording Pipeline
          </h2>
          {recordingHealth.length > 0 && (
            <div className="flex items-center gap-3 text-xs text-nvr-text-muted">
              <span className="flex items-center gap-1">
                <span className="w-2 h-2 rounded-full bg-nvr-success" /> {healthyCameras} healthy
              </span>
              {stalledCameras > 0 && (
                <span className="flex items-center gap-1">
                  <span className="w-2 h-2 rounded-full bg-nvr-warning" /> {stalledCameras} stalled
                </span>
              )}
              {errorCameras > 0 && (
                <span className="flex items-center gap-1">
                  <span className="w-2 h-2 rounded-full bg-nvr-danger" /> {errorCameras} error
                </span>
              )}
            </div>
          )}
        </div>
        {recordingHealth.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-nvr-text-muted">
            No recording pipelines active
          </div>
        ) : (
          <div className="divide-y divide-nvr-border">
            {recordingHealth.map(cam => (
              <div key={cam.camera_id} className="px-4 py-3 flex items-center gap-3">
                <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${statusDot(cam.status)}`} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-nvr-text-primary truncate">{cam.camera_name || cam.camera_id}</p>
                  <div className="flex items-center gap-3 mt-0.5 text-xs text-nvr-text-muted">
                    <span className={`capitalize font-medium ${statusColor(cam.status)}`}>{cam.status}</span>
                    {cam.last_segment_time && (
                      <span>Last segment: {timeAgo(cam.last_segment_time)}</span>
                    )}
                    {cam.restart_attempts > 0 && (
                      <span className="text-nvr-warning">{cam.restart_attempts} restart(s)</span>
                    )}
                  </div>
                  {cam.last_error && (
                    <p className="text-xs text-nvr-danger mt-0.5 truncate">{cam.last_error}</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ---- Connection Status ---- */}
      {connections.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg overflow-hidden">
          <div className="px-4 py-3 border-b border-nvr-border">
            <h2 className="text-sm font-semibold text-nvr-text-primary uppercase tracking-wider">
              Camera Connections
            </h2>
          </div>
          <div className="divide-y divide-nvr-border">
            {connections.map(conn => (
              <div key={conn.camera_id} className="px-4 py-3 flex items-center gap-3">
                <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${statusDot(conn.state)}`} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-nvr-text-primary truncate">{conn.camera_name || conn.camera_id}</p>
                  <div className="flex items-center gap-3 mt-0.5 text-xs text-nvr-text-muted">
                    <span className={`capitalize font-medium ${statusColor(conn.state)}`}>{conn.state}</span>
                    {conn.since && <span>Since: {timeAgo(conn.since)}</span>}
                    {conn.reconnect_attempts > 0 && (
                      <span className="text-nvr-warning">{conn.reconnect_attempts} reconnect(s)</span>
                    )}
                  </div>
                  {conn.last_error && (
                    <p className="text-xs text-nvr-danger mt-0.5 truncate">{conn.last_error}</p>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ---- Disk I/O ---- */}
      {diskIO.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg overflow-hidden">
          <div className="px-4 py-3 border-b border-nvr-border">
            <h2 className="text-sm font-semibold text-nvr-text-primary uppercase tracking-wider">
              Disk I/O Performance
            </h2>
          </div>
          <div className="divide-y divide-nvr-border">
            {diskIO.map(dio => (
              <div key={dio.path} className="px-4 py-3 flex items-center gap-3">
                <span className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${statusDot(dio.status)}`} />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-nvr-text-primary font-mono truncate">{dio.path}</p>
                  <div className="flex items-center gap-3 mt-0.5 text-xs text-nvr-text-muted">
                    <span className={`capitalize font-medium ${statusColor(dio.status)}`}>{dio.status}</span>
                    <span>Avg: {dio.avg_latency_ms?.toFixed(1)}ms</span>
                    <span>P99: {dio.p99_latency_ms?.toFixed(1)}ms</span>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* ---- Per-Camera Storage Breakdown ---- */}
      {(storage?.per_camera?.length ?? 0) > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg overflow-hidden">
          <div className="px-4 py-3 border-b border-nvr-border">
            <h2 className="text-sm font-semibold text-nvr-text-primary uppercase tracking-wider">
              Storage by Camera
            </h2>
          </div>
          <div className="divide-y divide-nvr-border">
            {storage!.per_camera.map(cam => {
              const pct = storage!.recordings_bytes > 0
                ? (cam.total_bytes / storage!.recordings_bytes) * 100
                : 0
              return (
                <div key={cam.camera_id} className="px-4 py-3">
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-sm font-medium text-nvr-text-primary truncate">{cam.camera_name || cam.camera_id}</span>
                    <span className="text-xs text-nvr-text-muted ml-2 flex-shrink-0">
                      {formatBytes(cam.total_bytes)} ({cam.segment_count} segments)
                    </span>
                  </div>
                  <div className="w-full h-1.5 bg-nvr-bg-tertiary rounded-full overflow-hidden">
                    <div
                      className="h-full bg-nvr-accent rounded-full transition-all duration-500"
                      style={{ width: `${Math.max(pct, 0.5)}%` }}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
