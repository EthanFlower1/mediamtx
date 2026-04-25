import { useState, useEffect, useCallback, useRef } from 'react'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface Camera {
  id: string
  name: string
  status: string
  [key: string]: unknown
}

interface Recorder {
  ID: string
  Hostname: string
  HealthStatus: string
  LastCheckinAt: string
}

interface CloudStatus {
  enabled: boolean
  url: string
  site_alias: string
}

interface HealthStatus {
  status: string
  mode: string
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function statusDotClass(status: string): string {
  const s = status.toLowerCase()
  if (s === 'online' || s === 'healthy' || s === 'ok') return 'bg-nvr-success'
  if (s === 'degraded' || s === 'warning') return 'bg-nvr-warning'
  if (s === 'offline' || s === 'error' || s === 'unhealthy') return 'bg-nvr-danger'
  return 'bg-nvr-text-muted'
}

function statusTextClass(status: string): string {
  const s = status.toLowerCase()
  if (s === 'online' || s === 'healthy' || s === 'ok') return 'text-nvr-success'
  if (s === 'degraded' || s === 'warning') return 'text-nvr-warning'
  if (s === 'offline' || s === 'error' || s === 'unhealthy') return 'text-nvr-danger'
  return 'text-nvr-text-muted'
}

/* ------------------------------------------------------------------ */
/*  Summary card component                                             */
/* ------------------------------------------------------------------ */

function SummaryCard({
  label,
  value,
  subtext,
  icon,
  accent,
}: {
  label: string
  value: string
  subtext?: string
  icon: React.ReactNode
  accent?: 'success' | 'warning' | 'danger'
}) {
  const accentBorder =
    accent === 'danger'
      ? 'border-nvr-danger/50'
      : accent === 'warning'
        ? 'border-nvr-warning/50'
        : 'border-nvr-border'

  return (
    <div className={`bg-nvr-bg-secondary border ${accentBorder} rounded-xl p-4 flex flex-col gap-2`}>
      <div className="flex items-center gap-2 text-nvr-text-secondary text-xs font-medium uppercase tracking-wider">
        {icon}
        {label}
      </div>
      <div className="text-2xl font-bold text-nvr-text-primary">{value}</div>
      {subtext && <div className="text-xs text-nvr-text-muted">{subtext}</div>}
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
  const [health, setHealth] = useState<HealthStatus | null>(null)
  const [cameras, setCameras] = useState<Camera[]>([])
  const [recorders, setRecorders] = useState<Recorder[]>([])
  const [cloud, setCloud] = useState<CloudStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refreshInterval, setRefreshInterval] = useState(10000)
  const [lastRefresh, setLastRefresh] = useState<Date | null>(null)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const fetchAll = useCallback(async () => {
    try {
      const [healthRes, camerasRes, recordersRes, cloudRes] = await Promise.all([
        fetch('/api/nvr/system/health').catch(() => null),
        fetch('/api/v1/cameras').catch(() => null),
        fetch('/api/v1/recorders').catch(() => null),
        fetch('/api/v1/admin/cloud').catch(() => null),
      ])

      if (healthRes && healthRes.ok) {
        setHealth(await healthRes.json())
      }
      if (camerasRes && camerasRes.ok) {
        const data = await camerasRes.json()
        setCameras(data.items ?? [])
      }
      if (recordersRes && recordersRes.ok) {
        const data = await recordersRes.json()
        setRecorders(data.items ?? [])
      }
      if (cloudRes && cloudRes.ok) {
        setCloud(await cloudRes.json())
      }

      setLastRefresh(new Date())
      setError(null)
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed to fetch dashboard data'
      setError(msg)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchAll()
  }, [fetchAll])

  useEffect(() => {
    if (timerRef.current) clearInterval(timerRef.current)
    if (refreshInterval > 0) {
      timerRef.current = setInterval(fetchAll, refreshInterval)
    }
    return () => {
      if (timerRef.current) clearInterval(timerRef.current)
    }
  }, [refreshInterval, fetchAll])

  // Camera counts
  const camerasOnline = cameras.filter(
    (c) => c.status?.toLowerCase() === 'online',
  ).length
  const camerasOffline = cameras.length - camerasOnline

  // Recorder counts
  const recordersHealthy = recorders.filter(
    (r) => r.HealthStatus?.toLowerCase() === 'healthy',
  ).length
  const recordersOffline = recorders.length - recordersHealthy

  if (loading) {
    return (
      <div className="flex items-center justify-center py-20" role="status" aria-label="Loading dashboard">
        <svg
          className="w-8 h-8 text-nvr-accent animate-spin"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
          aria-hidden="true"
        >
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-nvr-text-primary">Dashboard</h1>
          <p className="text-sm text-nvr-text-muted mt-0.5">
            {health
              ? `Mode: ${health.mode} -- Status: ${health.status}`
              : 'Connecting...'}
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
            className="text-xs text-nvr-accent hover:text-nvr-accent/80 transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded p-1"
            aria-label="Refresh dashboard data"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
          <label htmlFor="refresh-interval" className="sr-only">Auto-refresh interval</label>
          <select
            id="refresh-interval"
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
        <div role="alert" className="bg-nvr-danger/10 border border-nvr-danger/30 text-nvr-danger text-sm rounded-lg px-4 py-3">
          {error}
        </div>
      )}

      {/* Summary cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <SummaryCard
          label="System"
          value={health?.status ?? 'Unknown'}
          subtext={health ? `Mode: ${health.mode}` : undefined}
          accent={
            health?.status?.toLowerCase() === 'ok' || health?.status?.toLowerCase() === 'healthy'
              ? undefined
              : 'warning'
          }
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
            </svg>
          }
        />
        <SummaryCard
          label="Cameras"
          value={`${camerasOnline} / ${cameras.length}`}
          subtext={`${camerasOnline} online, ${camerasOffline} offline`}
          accent={camerasOffline > 0 ? 'warning' : undefined}
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
            </svg>
          }
        />
        <SummaryCard
          label="Recorders"
          value={`${recordersHealthy} / ${recorders.length}`}
          subtext={`${recordersHealthy} healthy, ${recordersOffline} offline`}
          accent={recordersOffline > 0 ? 'warning' : undefined}
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
            </svg>
          }
        />
        <SummaryCard
          label="Cloud"
          value={cloud?.enabled ? 'Connected' : 'Disconnected'}
          subtext={cloud?.enabled ? `Site: ${cloud.site_alias}` : 'Not configured'}
          accent={cloud?.enabled ? undefined : undefined}
          icon={
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 15a4 4 0 004 4h9a5 5 0 10-.1-9.999 5.002 5.002 0 10-9.78 2.096A4.001 4.001 0 003 15z" />
            </svg>
          }
        />
      </div>

      {/* Camera list */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
        <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">Cameras</h3>
        {cameras.length === 0 ? (
          <p className="text-xs text-nvr-text-muted">No cameras configured</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs text-nvr-text-muted border-b border-nvr-border">
                  <th scope="col" className="pb-2 pr-4 font-medium">Name</th>
                  <th scope="col" className="pb-2 pr-4 font-medium">Status</th>
                  <th scope="col" className="pb-2 font-medium">ID</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-nvr-border/50">
                {cameras.map((cam) => (
                  <tr key={cam.id}>
                    <td className="py-2 pr-4 text-nvr-text-primary font-medium">
                      {cam.name || cam.id}
                    </td>
                    <td className="py-2 pr-4">
                      <span className="flex items-center gap-1.5">
                        <span className={`w-2 h-2 rounded-full ${statusDotClass(cam.status)}`} />
                        <span className={`capitalize ${statusTextClass(cam.status)}`}>
                          {cam.status}
                        </span>
                      </span>
                    </td>
                    <td className="py-2 text-nvr-text-muted text-xs font-mono">
                      {cam.id}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Recorder list */}
      {recorders.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">Recorders</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-xs text-nvr-text-muted border-b border-nvr-border">
                  <th scope="col" className="pb-2 pr-4 font-medium">Hostname</th>
                  <th scope="col" className="pb-2 pr-4 font-medium">Health</th>
                  <th scope="col" className="pb-2 font-medium">Last Check-in</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-nvr-border/50">
                {recorders.map((rec) => (
                  <tr key={rec.ID}>
                    <td className="py-2 pr-4 text-nvr-text-primary font-medium">
                      {rec.Hostname}
                    </td>
                    <td className="py-2 pr-4">
                      <span className="flex items-center gap-1.5">
                        <span className={`w-2 h-2 rounded-full ${statusDotClass(rec.HealthStatus)}`} />
                        <span className={`capitalize ${statusTextClass(rec.HealthStatus)}`}>
                          {rec.HealthStatus}
                        </span>
                      </span>
                    </td>
                    <td className="py-2 text-nvr-text-muted text-xs">
                      {rec.LastCheckinAt
                        ? new Date(rec.LastCheckinAt).toLocaleString()
                        : '--'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  )
}
