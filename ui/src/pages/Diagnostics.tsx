import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface RecorderStatus {
  camera_id: string
  camera_name: string
  status: string
  last_segment_at: string | null
  error_message: string | null
  restart_count: number
  uptime_seconds: number
}

interface LogEntry {
  timestamp: string
  level: string
  module: string
  message: string
  fields?: Record<string, unknown>
}

interface ProbeResult {
  target: string
  port: number
  reachable: boolean
  latency_ms: number
  error?: string
}

interface Bundle {
  id: string
  status: 'pending' | 'building' | 'ready' | 'failed' | 'expired'
  created_at: string
  expires_at: string
  size_bytes: number
  error?: string
}

/* ------------------------------------------------------------------ */
/*  Utility                                                            */
/* ------------------------------------------------------------------ */

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

function formatUptime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

function statusColor(status: string): string {
  switch (status) {
    case 'recording':
      return 'text-nvr-success'
    case 'idle':
      return 'text-nvr-text-muted'
    case 'stalled':
      return 'text-nvr-warning'
    case 'failed':
      return 'text-nvr-danger'
    default:
      return 'text-nvr-text-secondary'
  }
}

function statusDot(status: string): string {
  switch (status) {
    case 'recording':
      return 'bg-nvr-success'
    case 'idle':
      return 'bg-nvr-text-muted'
    case 'stalled':
      return 'bg-nvr-warning'
    case 'failed':
      return 'bg-nvr-danger'
    default:
      return 'bg-nvr-text-muted'
  }
}

function levelBadge(level: string): string {
  switch (level.toLowerCase()) {
    case 'error':
      return 'bg-red-500/20 text-red-400'
    case 'warn':
      return 'bg-yellow-500/20 text-yellow-400'
    case 'info':
      return 'bg-blue-500/20 text-blue-400'
    case 'debug':
      return 'bg-gray-500/20 text-gray-400'
    default:
      return 'bg-gray-500/20 text-gray-400'
  }
}

/* ------------------------------------------------------------------ */
/*  DiagnosticsDashboard                                               */
/* ------------------------------------------------------------------ */

function DiagnosticsDashboard() {
  const [recorders, setRecorders] = useState<RecorderStatus[]>([])
  const [loading, setLoading] = useState(true)

  const fetchRecorders = useCallback(() => {
    apiFetch('/diagnostics/recorders')
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setRecorders(data.recorders || [])
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    fetchRecorders()
    const id = setInterval(fetchRecorders, 15000)
    return () => clearInterval(id)
  }, [fetchRecorders])

  const summary = {
    total: recorders.length,
    recording: recorders.filter((r) => r.status === 'recording').length,
    stalled: recorders.filter((r) => r.status === 'stalled').length,
    failed: recorders.filter((r) => r.status === 'failed').length,
  }

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg">
      <div className="px-4 py-3 border-b border-nvr-border flex items-center justify-between">
        <h3 className="text-sm font-semibold text-nvr-text-primary">Recorder Status</h3>
        <button
          onClick={fetchRecorders}
          className="text-xs text-nvr-text-muted hover:text-nvr-text-primary transition-colors"
        >
          Refresh
        </button>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 p-4">
        <div className="bg-nvr-bg-tertiary rounded-lg px-3 py-2">
          <div className="text-xs text-nvr-text-muted">Total</div>
          <div className="text-lg font-bold text-nvr-text-primary">{summary.total}</div>
        </div>
        <div className="bg-nvr-bg-tertiary rounded-lg px-3 py-2">
          <div className="text-xs text-nvr-text-muted">Recording</div>
          <div className="text-lg font-bold text-nvr-success">{summary.recording}</div>
        </div>
        <div className="bg-nvr-bg-tertiary rounded-lg px-3 py-2">
          <div className="text-xs text-nvr-text-muted">Stalled</div>
          <div className="text-lg font-bold text-nvr-warning">{summary.stalled}</div>
        </div>
        <div className="bg-nvr-bg-tertiary rounded-lg px-3 py-2">
          <div className="text-xs text-nvr-text-muted">Failed</div>
          <div className="text-lg font-bold text-nvr-danger">{summary.failed}</div>
        </div>
      </div>

      {/* Camera table */}
      {loading ? (
        <div className="px-4 pb-4 text-sm text-nvr-text-muted">Loading...</div>
      ) : recorders.length === 0 ? (
        <div className="px-4 pb-4 text-sm text-nvr-text-muted">No cameras configured.</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-nvr-text-muted text-xs uppercase border-t border-nvr-border">
                <th className="text-left px-4 py-2">Camera</th>
                <th className="text-left px-4 py-2">Status</th>
                <th className="text-left px-4 py-2 hidden sm:table-cell">Last Segment</th>
                <th className="text-left px-4 py-2 hidden md:table-cell">Uptime</th>
                <th className="text-left px-4 py-2 hidden md:table-cell">Restarts</th>
              </tr>
            </thead>
            <tbody>
              {recorders.map((r) => (
                <tr key={r.camera_id} className="border-t border-nvr-border/50">
                  <td className="px-4 py-2.5 text-nvr-text-primary font-medium">{r.camera_name}</td>
                  <td className="px-4 py-2.5">
                    <span className="inline-flex items-center gap-1.5">
                      <span className={`w-2 h-2 rounded-full ${statusDot(r.status)}`} />
                      <span className={`capitalize ${statusColor(r.status)}`}>{r.status}</span>
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-nvr-text-muted hidden sm:table-cell">
                    {r.last_segment_at
                      ? new Date(r.last_segment_at).toLocaleTimeString()
                      : '--'}
                  </td>
                  <td className="px-4 py-2.5 text-nvr-text-muted hidden md:table-cell">
                    {formatUptime(r.uptime_seconds)}
                  </td>
                  <td className="px-4 py-2.5 text-nvr-text-muted hidden md:table-cell">
                    {r.restart_count}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  LogViewer                                                          */
/* ------------------------------------------------------------------ */

function LogViewer() {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [level, setLevel] = useState('')
  const [after, setAfter] = useState('')
  const [before, setBefore] = useState('')
  const [offset, setOffset] = useState(0)
  const limit = 50
  const searchRef = useRef<HTMLInputElement>(null)

  const fetchLogs = useCallback(() => {
    setLoading(true)
    const params = new URLSearchParams()
    if (search) params.set('search', search)
    if (level) params.set('level', level)
    if (after) params.set('after', new Date(after).toISOString())
    if (before) params.set('before', new Date(before).toISOString())
    params.set('limit', limit.toString())
    params.set('offset', offset.toString())

    apiFetch(`/diagnostics/logs?${params.toString()}`)
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setEntries(data.entries || [])
          setTotal(data.total || 0)
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [search, level, after, before, offset])

  useEffect(() => {
    fetchLogs()
  }, [fetchLogs])

  const handleSearch = () => {
    setOffset(0)
    fetchLogs()
  }

  const totalPages = Math.ceil(total / limit)
  const currentPage = Math.floor(offset / limit) + 1

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg">
      <div className="px-4 py-3 border-b border-nvr-border">
        <h3 className="text-sm font-semibold text-nvr-text-primary mb-3">Log Viewer</h3>

        <div className="flex flex-wrap gap-2">
          {/* Search */}
          <div className="flex-1 min-w-[200px]">
            <input
              ref={searchRef}
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              placeholder="Search logs..."
              className="w-full bg-nvr-bg-tertiary border border-nvr-border rounded px-3 py-1.5 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
            />
          </div>

          {/* Level filter */}
          <select
            value={level}
            onChange={(e) => { setLevel(e.target.value); setOffset(0) }}
            className="bg-nvr-bg-tertiary border border-nvr-border rounded px-3 py-1.5 text-sm text-nvr-text-primary focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
          >
            <option value="">All Levels</option>
            <option value="debug">Debug</option>
            <option value="info">Info</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>

          {/* Time filters */}
          <input
            type="datetime-local"
            value={after}
            onChange={(e) => { setAfter(e.target.value); setOffset(0) }}
            className="bg-nvr-bg-tertiary border border-nvr-border rounded px-3 py-1.5 text-sm text-nvr-text-primary focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
            title="After"
          />
          <input
            type="datetime-local"
            value={before}
            onChange={(e) => { setBefore(e.target.value); setOffset(0) }}
            className="bg-nvr-bg-tertiary border border-nvr-border rounded px-3 py-1.5 text-sm text-nvr-text-primary focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
            title="Before"
          />

          <button
            onClick={handleSearch}
            className="bg-nvr-accent text-white px-3 py-1.5 rounded text-sm font-medium hover:bg-nvr-accent/80 transition-colors"
          >
            Search
          </button>
        </div>
      </div>

      {/* Log entries */}
      <div className="max-h-[400px] overflow-y-auto">
        {loading ? (
          <div className="px-4 py-6 text-sm text-nvr-text-muted text-center">Loading logs...</div>
        ) : entries.length === 0 ? (
          <div className="px-4 py-6 text-sm text-nvr-text-muted text-center">No log entries found.</div>
        ) : (
          <div className="divide-y divide-nvr-border/30">
            {entries.map((entry, i) => (
              <div key={i} className="px-4 py-2 hover:bg-nvr-bg-tertiary/50 transition-colors">
                <div className="flex items-center gap-2 mb-0.5">
                  <span className={`text-[10px] uppercase font-bold px-1.5 py-0.5 rounded ${levelBadge(entry.level)}`}>
                    {entry.level}
                  </span>
                  <span className="text-xs text-nvr-text-muted font-mono">
                    {entry.timestamp ? new Date(entry.timestamp).toLocaleString() : '--'}
                  </span>
                  {entry.module && (
                    <span className="text-xs text-nvr-accent">[{entry.module}]</span>
                  )}
                </div>
                <div className="text-sm text-nvr-text-primary font-mono break-all">{entry.message}</div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Pagination */}
      {total > limit && (
        <div className="px-4 py-2 border-t border-nvr-border flex items-center justify-between">
          <span className="text-xs text-nvr-text-muted">
            {total} entries, page {currentPage} of {totalPages}
          </span>
          <div className="flex gap-1">
            <button
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - limit))}
              className="px-2 py-1 text-xs bg-nvr-bg-tertiary rounded text-nvr-text-secondary hover:text-nvr-text-primary disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Prev
            </button>
            <button
              disabled={offset + limit >= total}
              onClick={() => setOffset(offset + limit)}
              className="px-2 py-1 text-xs bg-nvr-bg-tertiary rounded text-nvr-text-secondary hover:text-nvr-text-primary disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  NetworkProbes                                                      */
/* ------------------------------------------------------------------ */

function NetworkProbes() {
  const [results, setResults] = useState<ProbeResult[]>([])
  const [loading, setLoading] = useState(false)
  const [lastRun, setLastRun] = useState<string | null>(null)

  const runProbes = useCallback(() => {
    setLoading(true)
    apiFetch('/diagnostics/network-probe', { method: 'POST' })
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setResults(data.results || [])
          setLastRun(new Date().toLocaleTimeString())
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg">
      <div className="px-4 py-3 border-b border-nvr-border flex items-center justify-between">
        <h3 className="text-sm font-semibold text-nvr-text-primary">Network Probes</h3>
        <div className="flex items-center gap-3">
          {lastRun && (
            <span className="text-xs text-nvr-text-muted">Last run: {lastRun}</span>
          )}
          <button
            onClick={runProbes}
            disabled={loading}
            className="bg-nvr-accent text-white px-3 py-1.5 rounded text-xs font-medium hover:bg-nvr-accent/80 disabled:opacity-50 transition-colors"
          >
            {loading ? 'Probing...' : 'Run Probes'}
          </button>
        </div>
      </div>

      {results.length === 0 ? (
        <div className="px-4 py-6 text-sm text-nvr-text-muted text-center">
          Click "Run Probes" to test network connectivity.
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-nvr-text-muted text-xs uppercase">
                <th className="text-left px-4 py-2">Target</th>
                <th className="text-left px-4 py-2">Port</th>
                <th className="text-left px-4 py-2">Status</th>
                <th className="text-left px-4 py-2">Latency</th>
              </tr>
            </thead>
            <tbody>
              {results.map((r, i) => (
                <tr key={i} className="border-t border-nvr-border/50">
                  <td className="px-4 py-2 text-nvr-text-primary font-mono">{r.target}</td>
                  <td className="px-4 py-2 text-nvr-text-muted">{r.port}</td>
                  <td className="px-4 py-2">
                    {r.reachable ? (
                      <span className="inline-flex items-center gap-1 text-nvr-success">
                        <span className="w-2 h-2 rounded-full bg-nvr-success" />
                        Reachable
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-nvr-danger">
                        <span className="w-2 h-2 rounded-full bg-nvr-danger" />
                        Unreachable
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-2 text-nvr-text-muted">
                    {r.reachable ? `${r.latency_ms.toFixed(1)} ms` : '--'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  BundleGenerator + BundleDownloadCard                               */
/* ------------------------------------------------------------------ */

function BundleDownloadCard({ bundle, onRefresh }: { bundle: Bundle; onRefresh: () => void }) {
  const [copied, setCopied] = useState(false)

  const copyId = () => {
    navigator.clipboard.writeText(bundle.id).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  const downloadBundle = () => {
    apiFetch(`/diagnostics/bundles/${bundle.id}/download`)
      .then(async (res) => {
        if (res.ok) {
          const blob = await res.blob()
          const url = URL.createObjectURL(blob)
          const a = document.createElement('a')
          a.href = url
          a.download = `support-bundle-${bundle.id}.zip`
          a.click()
          URL.revokeObjectURL(url)
        }
      })
      .catch(() => {})
  }

  const statusBadge = () => {
    switch (bundle.status) {
      case 'pending':
      case 'building':
        return 'bg-blue-500/20 text-blue-400'
      case 'ready':
        return 'bg-green-500/20 text-green-400'
      case 'failed':
        return 'bg-red-500/20 text-red-400'
      case 'expired':
        return 'bg-gray-500/20 text-gray-400'
      default:
        return 'bg-gray-500/20 text-gray-400'
    }
  }

  return (
    <div className="bg-nvr-bg-tertiary rounded-lg px-4 py-3 flex flex-col sm:flex-row sm:items-center gap-3">
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-1">
          <button
            onClick={copyId}
            className="font-mono text-sm text-nvr-text-primary hover:text-nvr-accent transition-colors cursor-pointer"
            title="Click to copy bundle ID"
          >
            {bundle.id}
          </button>
          {copied && <span className="text-xs text-nvr-success">Copied!</span>}
          <span className={`text-[10px] uppercase font-bold px-1.5 py-0.5 rounded ${statusBadge()}`}>
            {bundle.status}
          </span>
        </div>
        <div className="text-xs text-nvr-text-muted flex gap-4">
          <span>Created: {new Date(bundle.created_at).toLocaleString()}</span>
          {bundle.size_bytes > 0 && <span>{formatBytes(bundle.size_bytes)}</span>}
          {bundle.expires_at && (
            <span>Expires: {new Date(bundle.expires_at).toLocaleString()}</span>
          )}
        </div>
        {bundle.error && (
          <div className="text-xs text-nvr-danger mt-1">{bundle.error}</div>
        )}
      </div>

      <div className="flex gap-2 shrink-0">
        {(bundle.status === 'pending' || bundle.status === 'building') && (
          <button
            onClick={onRefresh}
            className="px-3 py-1.5 text-xs bg-nvr-bg-secondary border border-nvr-border rounded text-nvr-text-secondary hover:text-nvr-text-primary transition-colors"
          >
            Refresh
          </button>
        )}
        {bundle.status === 'ready' && (
          <button
            onClick={downloadBundle}
            className="px-3 py-1.5 text-xs bg-nvr-accent text-white rounded font-medium hover:bg-nvr-accent/80 transition-colors"
          >
            Download
          </button>
        )}
      </div>
    </div>
  )
}

function BundleGenerator() {
  const [bundles, setBundles] = useState<Bundle[]>([])
  const [generating, setGenerating] = useState(false)
  const pollRef = useRef<number | null>(null)

  const fetchBundles = useCallback(() => {
    apiFetch('/diagnostics/bundles')
      .then(async (res) => {
        if (res.ok) {
          const data = await res.json()
          setBundles(data.bundles || [])
        }
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    fetchBundles()
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [fetchBundles])

  // Auto-poll while any bundle is building.
  useEffect(() => {
    const hasPending = bundles.some(
      (b) => b.status === 'pending' || b.status === 'building'
    )
    if (hasPending && !pollRef.current) {
      pollRef.current = window.setInterval(fetchBundles, 3000)
    } else if (!hasPending && pollRef.current) {
      clearInterval(pollRef.current)
      pollRef.current = null
    }
  }, [bundles, fetchBundles])

  const handleGenerate = () => {
    setGenerating(true)
    apiFetch('/diagnostics/bundles', { method: 'POST' })
      .then(async (res) => {
        if (res.ok) {
          // Refresh list to include new bundle.
          fetchBundles()
        }
      })
      .catch(() => {})
      .finally(() => setGenerating(false))
  }

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg">
      <div className="px-4 py-3 border-b border-nvr-border flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-nvr-text-primary">Support Bundles</h3>
          <p className="text-xs text-nvr-text-muted mt-0.5">
            Generate diagnostic bundles containing logs, system info, and health data.
          </p>
        </div>
        <button
          onClick={handleGenerate}
          disabled={generating}
          className="bg-nvr-accent text-white px-4 py-2 rounded text-sm font-medium hover:bg-nvr-accent/80 disabled:opacity-50 transition-colors whitespace-nowrap"
        >
          {generating ? 'Generating...' : 'Generate Support Bundle'}
        </button>
      </div>

      <div className="p-4 space-y-2">
        {bundles.length === 0 ? (
          <div className="text-sm text-nvr-text-muted text-center py-4">
            No bundles generated yet.
          </div>
        ) : (
          bundles.map((b) => (
            <BundleDownloadCard key={b.id} bundle={b} onRefresh={fetchBundles} />
          ))
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main Diagnostics Page                                              */
/* ------------------------------------------------------------------ */

export default function Diagnostics() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-nvr-text-primary">Remote Diagnostics</h1>
        <p className="text-sm text-nvr-text-muted mt-1">
          Read-only diagnostic tools for troubleshooting recorder health, reviewing logs,
          and testing network connectivity.
        </p>
      </div>

      <DiagnosticsDashboard />
      <LogViewer />
      <NetworkProbes />
      <BundleGenerator />
    </div>
  )
}
