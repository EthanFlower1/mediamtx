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

interface MetricsSample {
  t: number
  cpu: number
  mem: number
  alloc: number
  sys: number
  gr: number
}

interface MetricsData {
  cpu_goroutines: number
  mem_alloc_bytes: number
  mem_sys_bytes: number
  mem_gc_count: number
  uptime_seconds: number
  camera_count: number
  current: {
    cpu_percent: number
    mem_percent: number
    mem_alloc_mb: number
    mem_sys_mb: number
    goroutines: number
  }
  history: MetricsSample[]
}

interface StorageInfo {
  total_bytes: number
  used_bytes: number
  free_bytes: number
  recordings_bytes: number
  warning: boolean
  critical: boolean
}

interface DiskIOPath {
  path: string
  status: string
  avg_latency_ms: number
  p99_latency_ms: number
  warn_threshold_ms: number
  critical_threshold_ms: number
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

interface ConfigSummary {
  cameras: {
    total: number
    online: number
    recording: number
  }
  recording_rules: {
    total: number
    active: number
  }
}

interface CameraInfo {
  id: string
  name: string
  ai_enabled: boolean
  status: string
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h ${mins}m`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

function statusColor(status: string): string {
  switch (status) {
    case 'recording':
      return 'text-nvr-success'
    case 'stalled':
      return 'text-nvr-warning'
    case 'error':
    case 'failed':
      return 'text-nvr-danger'
    default:
      return 'text-nvr-text-muted'
  }
}

function statusDot(status: string): string {
  switch (status) {
    case 'recording':
      return 'bg-nvr-success'
    case 'stalled':
      return 'bg-nvr-warning'
    case 'error':
    case 'failed':
      return 'bg-nvr-danger'
    default:
      return 'bg-nvr-text-muted'
  }
}

/* ------------------------------------------------------------------ */
/*  Mini sparkline chart (SVG)                                         */
/* ------------------------------------------------------------------ */

function Sparkline({
  data,
  maxVal = 100,
  color = '#3b82f6',
  height = 40,
  width = 200,
}: {
  data: number[]
  maxVal?: number
  color?: string
  height?: number
  width?: number
}) {
  if (data.length < 2) return null
  const max = Math.max(maxVal, ...data, 1)
  const step = width / (data.length - 1)
  const points = data.map((v, i) => {
    const x = i * step
    const y = height - (v / max) * (height - 4) - 2
    return `${x},${y}`
  })
  const fillPoints = `0,${height} ${points.join(' ')} ${width},${height}`

  return (
    <svg width={width} height={height} className="overflow-visible">
      <polygon points={fillPoints} fill={color} opacity="0.1" />
      <polyline
        points={points.join(' ')}
        fill="none"
        stroke={color}
        strokeWidth="1.5"
        strokeLinejoin="round"
      />
    </svg>
  )
}

/* ------------------------------------------------------------------ */
/*  Metric card                                                        */
/* ------------------------------------------------------------------ */

function MetricCard({
  label,
  value,
  subtext,
  sparkData,
  sparkColor,
  icon,
  alert,
}: {
  label: string
  value: string
  subtext?: string
  sparkData?: number[]
  sparkColor?: string
  icon: React.ReactNode
  alert?: 'warning' | 'critical'
}) {
  const borderClass =
    alert === 'critical'
      ? 'border-nvr-danger/50'
      : alert === 'warning'
        ? 'border-nvr-warning/50'
        : 'border-nvr-border'

  return (
    <div
      className={`bg-nvr-bg-secondary border ${borderClass} rounded-xl p-4 flex flex-col gap-2`}
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2 text-nvr-text-secondary text-xs font-medium uppercase tracking-wider">
          {icon}
          {label}
        </div>
        {alert && (
          <span
            className={`text-xs font-medium px-2 py-0.5 rounded-full ${
              alert === 'critical'
                ? 'bg-nvr-danger/20 text-nvr-danger'
                : 'bg-nvr-warning/20 text-nvr-warning'
            }`}
          >
            {alert}
          </span>
        )}
      </div>
      <div className="text-2xl font-bold text-nvr-text-primary">{value}</div>
      {subtext && (
        <div className="text-xs text-nvr-text-muted">{subtext}</div>
      )}
      {sparkData && sparkData.length > 1 && (
        <div className="mt-1">
          <Sparkline data={sparkData} color={sparkColor ?? '#3b82f6'} width={200} height={36} />
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Refresh interval selector                                          */
/* ------------------------------------------------------------------ */

const REFRESH_OPTIONS = [
  { label: '5s', value: 5000 },
  { label: '10s', value: 10000 },
  { label: '30s', value: 30000 },
  { label: '1m', value: 60000 },
  { label: 'Off', value: 0 },
]

/* ------------------------------------------------------------------ */
/*  Dashboard page                                                     */
/* ------------------------------------------------------------------ */

export default function Dashboard() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [metrics, setMetrics] = useState<MetricsData | null>(null)
  const [storage, setStorage] = useState<StorageInfo | null>(null)
  const [diskIO, setDiskIO] = useState<DiskIOPath[]>([])
  const [recHealth, setRecHealth] = useState<RecordingHealthEntry[]>([])
  const [config, setConfig] = useState<ConfigSummary | null>(null)
  const [cameras, setCameras] = useState<CameraInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refreshInterval, setRefreshInterval] = useState(10000)
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchAll = useCallback(async () => {
    try {
      const [infoRes, metricsRes, storageRes, diskIORes, healthRes, configRes, camerasRes] =
        await Promise.all([
          apiFetch('/system/info'),
          apiFetch('/system/metrics'),
          apiFetch('/system/storage'),
          apiFetch('/system/disk-io'),
          apiFetch('/recordings/health').catch(() => null),
          apiFetch('/system/config').catch(() => null),
          apiFetch('/cameras').catch(() => null),
        ])

      if (infoRes.ok) setInfo(await infoRes.json())
      if (metricsRes.ok) setMetrics(await metricsRes.json())
      if (storageRes.ok) setStorage(await storageRes.json())
      if (diskIORes.ok) {
        const d = await diskIORes.json()
        const paths = d.paths ?? {}
        setDiskIO(
          Object.entries(paths).map(([path, v]: [string, any]) => ({
            path,
            status: v.status ?? 'unknown',
            avg_latency_ms: v.avg_latency_ms ?? 0,
            p99_latency_ms: v.p99_latency_ms ?? 0,
            warn_threshold_ms: v.warn_threshold_ms ?? 0,
            critical_threshold_ms: v.critical_threshold_ms ?? 0,
          })),
        )
      }
      if (healthRes && healthRes.ok) {
        const h = await healthRes.json()
        setRecHealth(h.cameras ?? [])
      }
      if (configRes && configRes.ok) setConfig(await configRes.json())
      if (camerasRes && camerasRes.ok) {
        const camData = await camerasRes.json()
        setCameras(camData ?? [])
      }

      setLastRefresh(new Date())
      setError(null)
    } catch (e: any) {
      setError(e.message ?? 'Failed to fetch dashboard data')
    } finally {
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

  // Extract sparkline data from history
  const cpuHistory = metrics?.history?.map((s) => s.cpu) ?? []
  const memHistory = metrics?.history?.map((s) => s.mem) ?? []
  const allocHistory = metrics?.history?.map((s) => s.alloc) ?? []
  const goroutineHistory = metrics?.history?.map((s) => s.gr) ?? []

  // Storage percent
  const storagePercent =
    storage && storage.total_bytes > 0
      ? ((storage.used_bytes / storage.total_bytes) * 100).toFixed(1)
      : '0'

  const storageAlert = storage?.critical
    ? 'critical' as const
    : storage?.warning
      ? 'warning' as const
      : undefined

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20">
        <svg
          className="w-8 h-8 text-nvr-accent animate-spin"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
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
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-nvr-text-primary">
            System Health
          </h1>
          <p className="text-sm text-nvr-text-muted mt-0.5">
            {info
              ? `MediaMTX ${info.version} -- ${info.platform} -- up ${formatUptime(metrics?.uptime_seconds ?? 0)}`
              : 'Loading system info...'}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {lastRefresh && (
            <span className="text-xs text-nvr-text-muted">
              Updated {lastRefresh.toLocaleTimeString()}
            </span>
          )}
          <button
            onClick={fetchAll}
            className="text-xs text-nvr-accent hover:text-nvr-accent/80 transition-colors"
            title="Refresh now"
          >
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
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
              />
            </svg>
          </button>
          <select
            value={refreshInterval}
            onChange={(e) => setRefreshInterval(Number(e.target.value))}
            className="bg-nvr-bg-secondary border border-nvr-border text-nvr-text-primary text-xs rounded-lg px-2 py-1.5 focus:ring-1 focus:ring-nvr-accent focus:outline-none"
          >
            {REFRESH_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      {error && (
        <div className="bg-nvr-danger/10 border border-nvr-danger/30 text-nvr-danger text-sm rounded-lg px-4 py-3">
          {error}
        </div>
      )}

      {/* Overview cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <MetricCard
          label="CPU"
          value={`${(metrics?.current?.cpu_percent ?? 0).toFixed(1)}%`}
          subtext={`${metrics?.current?.goroutines ?? 0} goroutines`}
          sparkData={cpuHistory}
          sparkColor="#3b82f6"
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
            </svg>
          }
        />
        <MetricCard
          label="Memory"
          value={`${(metrics?.current?.mem_percent ?? 0).toFixed(1)}%`}
          subtext={`${(metrics?.current?.mem_alloc_mb ?? 0).toFixed(0)} MB heap / ${(metrics?.current?.mem_sys_mb ?? 0).toFixed(0)} MB sys`}
          sparkData={memHistory}
          sparkColor="#8b5cf6"
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
            </svg>
          }
        />
        <MetricCard
          label="Disk"
          value={`${storagePercent}%`}
          subtext={
            storage
              ? `${formatBytes(storage.used_bytes)} / ${formatBytes(storage.total_bytes)}`
              : 'Loading...'
          }
          alert={storageAlert}
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
            </svg>
          }
        />
        <MetricCard
          label="Cameras"
          value={`${config?.cameras?.online ?? 0} / ${config?.cameras?.total ?? metrics?.camera_count ?? 0}`}
          subtext={`${config?.cameras?.recording ?? 0} recording, ${config?.recording_rules?.active ?? 0} active rules`}
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
          }
        />
      </div>

      {/* Charts row */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Go heap allocation chart */}
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">
            Go Heap Allocation (MB)
          </h3>
          {allocHistory.length > 1 ? (
            <Sparkline
              data={allocHistory}
              maxVal={Math.max(...allocHistory) * 1.2}
              color="#10b981"
              width={500}
              height={80}
            />
          ) : (
            <p className="text-xs text-nvr-text-muted">
              Collecting data...
            </p>
          )}
        </div>

        {/* Goroutine count chart */}
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">
            Goroutines
          </h3>
          {goroutineHistory.length > 1 ? (
            <Sparkline
              data={goroutineHistory}
              maxVal={Math.max(...goroutineHistory) * 1.2}
              color="#f59e0b"
              width={500}
              height={80}
            />
          ) : (
            <p className="text-xs text-nvr-text-muted">
              Collecting data...
            </p>
          )}
        </div>
      </div>

      {/* Recording pipeline health */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
        <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">
          Recording Pipeline Status
        </h3>
        {recHealth.length === 0 ? (
          <p className="text-xs text-nvr-text-muted">
            No active recording pipelines
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs text-nvr-text-muted border-b border-nvr-border">
                  <th className="pb-2 pr-4 font-medium">Camera</th>
                  <th className="pb-2 pr-4 font-medium">Status</th>
                  <th className="pb-2 pr-4 font-medium">Last Segment</th>
                  <th className="pb-2 pr-4 font-medium">Restarts</th>
                  <th className="pb-2 font-medium">Error</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-nvr-border/50">
                {recHealth.map((cam) => (
                  <tr key={cam.camera_id}>
                    <td className="py-2 pr-4 text-nvr-text-primary font-medium">
                      {cam.camera_name || cam.camera_id}
                    </td>
                    <td className="py-2 pr-4">
                      <span className="flex items-center gap-1.5">
                        <span
                          className={`w-2 h-2 rounded-full ${statusDot(cam.status)}`}
                        />
                        <span className={`capitalize ${statusColor(cam.status)}`}>
                          {cam.status}
                        </span>
                      </span>
                    </td>
                    <td className="py-2 pr-4 text-nvr-text-muted text-xs">
                      {cam.last_segment_time
                        ? new Date(cam.last_segment_time).toLocaleString()
                        : '--'}
                    </td>
                    <td className="py-2 pr-4 text-nvr-text-muted">
                      {cam.restart_attempts}
                    </td>
                    <td className="py-2 text-nvr-danger text-xs max-w-xs truncate">
                      {cam.last_error || '--'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Disk I/O status */}
      {diskIO.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">
            Disk I/O Health
          </h3>
          <div className="space-y-3">
            {diskIO.map((io) => {
              const ioAlert =
                io.status === 'critical'
                  ? 'critical'
                  : io.status === 'warn'
                    ? 'warning'
                    : null
              return (
                <div
                  key={io.path}
                  className="flex items-center justify-between gap-4"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <span
                      className={`w-2 h-2 rounded-full flex-shrink-0 ${
                        ioAlert === 'critical'
                          ? 'bg-nvr-danger'
                          : ioAlert === 'warning'
                            ? 'bg-nvr-warning'
                            : 'bg-nvr-success'
                      }`}
                    />
                    <span className="text-sm text-nvr-text-primary truncate">
                      {io.path}
                    </span>
                  </div>
                  <div className="flex items-center gap-4 text-xs text-nvr-text-muted flex-shrink-0">
                    <span>Avg: {io.avg_latency_ms.toFixed(1)}ms</span>
                    <span>P99: {io.p99_latency_ms.toFixed(1)}ms</span>
                    {ioAlert && (
                      <span
                        className={`font-medium px-2 py-0.5 rounded-full ${
                          ioAlert === 'critical'
                            ? 'bg-nvr-danger/20 text-nvr-danger'
                            : 'bg-nvr-warning/20 text-nvr-warning'
                        }`}
                      >
                        {ioAlert}
                      </span>
                    )}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* AI pipeline status */}
      {cameras.some((c) => c.ai_enabled) && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">
            AI Pipeline Status
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            {cameras
              .filter((c) => c.ai_enabled)
              .map((cam) => (
                <div
                  key={cam.id}
                  className="flex items-center gap-2 bg-nvr-bg-tertiary rounded-lg px-3 py-2"
                >
                  <span className="w-2 h-2 rounded-full bg-nvr-success flex-shrink-0" />
                  <span className="text-sm text-nvr-text-primary truncate">
                    {cam.name}
                  </span>
                  <span className="ml-auto text-xs text-nvr-text-muted">
                    AI active
                  </span>
                </div>
              ))}
          </div>
          <p className="text-xs text-nvr-text-muted mt-2">
            {cameras.filter((c) => c.ai_enabled).length} of {cameras.length}{' '}
            cameras with AI detection enabled
          </p>
        </div>
      )}

      {/* Storage breakdown bar */}
      {storage && storage.total_bytes > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">
            Storage Usage
          </h3>
          <div className="relative w-full h-4 bg-nvr-bg-tertiary rounded-full overflow-hidden">
            <div
              className={`absolute inset-y-0 left-0 rounded-full transition-all ${
                storage.critical
                  ? 'bg-nvr-danger'
                  : storage.warning
                    ? 'bg-nvr-warning'
                    : 'bg-nvr-accent'
              }`}
              style={{
                width: `${Math.min((storage.used_bytes / storage.total_bytes) * 100, 100)}%`,
              }}
            />
            {storage.recordings_bytes > 0 && (
              <div
                className="absolute inset-y-0 left-0 rounded-full bg-nvr-success/50"
                style={{
                  width: `${Math.min((storage.recordings_bytes / storage.total_bytes) * 100, 100)}%`,
                }}
              />
            )}
          </div>
          <div className="flex justify-between mt-2 text-xs text-nvr-text-muted">
            <span>
              Recordings: {formatBytes(storage.recordings_bytes)}
            </span>
            <span>
              Free: {formatBytes(storage.free_bytes)}
            </span>
          </div>
        </div>
      )}
    </div>
  )
}
