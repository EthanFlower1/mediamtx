import { useState, useEffect, useCallback } from 'react'

interface RecorderInfo {
  ID: string
  Name: string
  Hostname: string
  InternalAPIAddr: string
  HealthStatus: string
  LastCheckinAt: string
}

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function timeAgo(dateStr: string): string {
  if (!dateStr) return 'never'
  const diff = Date.now() - new Date(dateStr).getTime()
  const seconds = Math.floor(diff / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

export default function Diagnostics() {
  const [healthOk, setHealthOk] = useState<boolean | null>(null)
  const [healthError, setHealthError] = useState<string | null>(null)
  const [recorders, setRecorders] = useState<RecorderInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [lastRefresh, setLastRefresh] = useState<string>('')

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const [healthRes, recRes] = await Promise.all([
        fetch('/healthz').catch(() => null),
        fetch('/api/v1/recorders').catch(() => null),
      ])

      if (healthRes) {
        setHealthOk(healthRes.ok)
        if (!healthRes.ok) {
          setHealthError(`HTTP ${healthRes.status}`)
        } else {
          setHealthError(null)
        }
      } else {
        setHealthOk(false)
        setHealthError('Unreachable')
      }

      if (recRes && recRes.ok) {
        const data = await recRes.json()
        setRecorders(data.items || [])
      } else {
        setRecorders([])
      }

      setLastRefresh(new Date().toLocaleTimeString())
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchAll()
    const id = setInterval(fetchAll, 15000)
    return () => clearInterval(id)
  }, [fetchAll])

  const effectiveStatus = (rec: RecorderInfo) => {
    if (!rec.LastCheckinAt) return 'offline'
    const diff = Date.now() - new Date(rec.LastCheckinAt).getTime()
    if (diff > 2 * 60 * 1000) return 'offline'
    return rec.HealthStatus || 'unknown'
  }

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-nvr-text-primary">Diagnostics</h1>
          <p className="text-sm text-nvr-text-tertiary mt-1">
            System health overview from available endpoints.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {lastRefresh && (
            <span className="text-xs text-nvr-text-tertiary">Updated {lastRefresh}</span>
          )}
          <button
            onClick={fetchAll}
            disabled={loading}
            className="px-3 py-1.5 bg-nvr-bg-secondary border border-nvr-border rounded-lg text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors disabled:opacity-50"
          >
            {loading ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>
      </div>

      {/* Health check */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <h2 className="text-sm font-semibold text-nvr-text-primary mb-3">System Health</h2>
        <div className="flex items-center gap-3">
          {healthOk === null ? (
            <span className="text-nvr-text-tertiary text-sm">Checking...</span>
          ) : healthOk ? (
            <>
              <span className="w-3 h-3 rounded-full bg-green-500" />
              <span className="text-green-400 text-sm font-medium">Healthy</span>
              <span className="text-nvr-text-tertiary text-xs">(/healthz responded OK)</span>
            </>
          ) : (
            <>
              <span className="w-3 h-3 rounded-full bg-red-500" />
              <span className="text-red-400 text-sm font-medium">Unhealthy</span>
              {healthError && (
                <span className="text-nvr-text-tertiary text-xs">{healthError}</span>
              )}
            </>
          )}
        </div>
      </div>

      {/* Recorders */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <h2 className="text-sm font-semibold text-nvr-text-primary mb-3">
          Recording Servers ({recorders.length})
        </h2>
        {recorders.length === 0 ? (
          <p className="text-nvr-text-tertiary text-sm py-4 text-center">
            No recording servers connected.
          </p>
        ) : (
          <div className="space-y-2">
            {recorders.map((rec) => {
              const status = effectiveStatus(rec)
              return (
                <div
                  key={rec.ID}
                  className="bg-nvr-bg-primary border border-nvr-border rounded-lg p-4 flex items-center justify-between"
                >
                  <div className="flex items-center gap-3">
                    <span
                      className={`w-3 h-3 rounded-full ${
                        status === 'healthy'
                          ? 'bg-green-500'
                          : status === 'degraded'
                            ? 'bg-amber-500'
                            : 'bg-red-500'
                      }`}
                    />
                    <div>
                      <div className="text-nvr-text-primary font-medium">
                        {rec.Name || rec.Hostname || rec.ID.slice(0, 12)}
                      </div>
                      <div className="text-nvr-text-secondary text-sm">
                        {rec.Hostname}
                        {rec.InternalAPIAddr && ` (${rec.InternalAPIAddr})`}
                      </div>
                    </div>
                  </div>
                  <div className="text-right">
                    <div
                      className={`text-sm font-medium ${
                        status === 'healthy'
                          ? 'text-green-400'
                          : status === 'degraded'
                            ? 'text-amber-400'
                            : 'text-red-400'
                      }`}
                    >
                      {status}
                    </div>
                    <div className="text-nvr-text-tertiary text-xs">
                      Last seen: {timeAgo(rec.LastCheckinAt)}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        )}
      </div>

      {/* Coming soon note */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-8 text-center">
        <p className="text-nvr-text-secondary">Extended diagnostics (log viewer, network probes, support bundles) will be available in a future update.</p>
      </div>
    </div>
  )
}
