import { useState, useEffect, useCallback, useMemo, useRef } from 'react'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */
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

interface AuditResponse {
  entries: AuditEntry[]
  total: number
}

/* ------------------------------------------------------------------ */
/*  Constants                                                          */
/* ------------------------------------------------------------------ */
const PAGE_SIZE = 50
const ROW_HEIGHT = 48 // px per row for virtualization
const OVERSCAN = 5    // extra rows rendered above/below viewport

const ACTION_TYPES = [
  { value: '', label: 'All Actions' },
  { value: 'create', label: 'Create' },
  { value: 'update', label: 'Update' },
  { value: 'delete', label: 'Delete' },
  { value: 'login', label: 'Login' },
  { value: 'logout', label: 'Logout' },
  { value: 'login_failed', label: 'Login Failed' },
]

const RESOURCE_TYPES = [
  { value: '', label: 'All Resources' },
  { value: 'camera', label: 'Camera' },
  { value: 'user', label: 'User' },
  { value: 'recording_rule', label: 'Recording Rule' },
  { value: 'system', label: 'System' },
  { value: 'group', label: 'Group' },
  { value: 'role', label: 'Role' },
  { value: 'backup', label: 'Backup' },
]

const ACTION_COLORS: Record<string, string> = {
  create: 'bg-green-500/10 text-green-400',
  update: 'bg-blue-500/10 text-blue-400',
  delete: 'bg-red-500/10 text-red-400',
  login: 'bg-nvr-accent/10 text-nvr-accent',
  logout: 'bg-yellow-500/10 text-yellow-400',
  login_failed: 'bg-red-500/10 text-red-400',
}

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */
function formatTimestamp(ts: string): string {
  try {
    const d = new Date(ts.endsWith('Z') ? ts : ts + 'Z')
    return d.toLocaleString()
  } catch {
    return ts
  }
}

/** Returns YYYY-MM-DD for a Date object. */
function toDateStr(d: Date): string {
  return d.toISOString().slice(0, 10)
}

/** Default 30-day window: from 30 days ago to today. */
function defaultDateRange(): { from: string; to: string } {
  const now = new Date()
  const from = new Date(now)
  from.setDate(from.getDate() - 30)
  return { from: toDateStr(from), to: toDateStr(now) }
}

/** Build a search params string for the API from current filters. */
function buildParams(filters: {
  limit: number
  offset: number
  action: string
  resourceType: string
  userFilter: string
  searchQuery: string
  dateFrom: string
  dateTo: string
}): string {
  const p = new URLSearchParams()
  p.set('limit', String(filters.limit))
  p.set('offset', String(filters.offset))
  if (filters.action) p.set('action', filters.action)
  if (filters.resourceType) p.set('resource_type', filters.resourceType)
  if (filters.userFilter) p.set('user_id', filters.userFilter)
  if (filters.searchQuery.trim()) p.set('q', filters.searchQuery.trim())
  if (filters.dateFrom) p.set('from', filters.dateFrom)
  if (filters.dateTo) p.set('to', filters.dateTo)
  return p.toString()
}

/** Trigger a browser download from a URL. */
function downloadFromURL(url: string, filename: string) {
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
}

/** Generate a simple PDF from audit entries using browser print.
 *  Creates an HTML table in a hidden iframe and triggers print-to-PDF. */
function exportPDF(entries: AuditEntry[], dateRange: string) {
  const rows = entries.map(e =>
    `<tr>
      <td style="padding:4px 8px;border:1px solid #ddd;white-space:nowrap">${e.created_at}</td>
      <td style="padding:4px 8px;border:1px solid #ddd">${e.username || 'unknown'}</td>
      <td style="padding:4px 8px;border:1px solid #ddd">${e.action}</td>
      <td style="padding:4px 8px;border:1px solid #ddd">${e.resource_type}</td>
      <td style="padding:4px 8px;border:1px solid #ddd">${e.resource_id || '-'}</td>
      <td style="padding:4px 8px;border:1px solid #ddd;max-width:300px;overflow:hidden;text-overflow:ellipsis">${(e.details || '-').replace(/</g, '&lt;')}</td>
      <td style="padding:4px 8px;border:1px solid #ddd;font-family:monospace;font-size:12px">${e.ip_address || '-'}</td>
    </tr>`
  ).join('')

  const html = `<!DOCTYPE html>
<html><head>
<title>Audit Log Export</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif; margin: 20px; font-size: 12px; }
  h1 { font-size: 18px; margin-bottom: 4px; }
  p { color: #666; margin-bottom: 12px; }
  table { border-collapse: collapse; width: 100%; }
  th { padding: 6px 8px; border: 1px solid #999; background: #f0f0f0; text-align: left; font-weight: 600; }
  @media print { body { margin: 10px; } }
</style>
</head><body>
<h1>Audit Log Report</h1>
<p>${dateRange} &mdash; ${entries.length} entries</p>
<table>
  <thead><tr>
    <th>Timestamp</th><th>User</th><th>Action</th><th>Resource</th><th>Resource ID</th><th>Details</th><th>IP</th>
  </tr></thead>
  <tbody>${rows}</tbody>
</table>
</body></html>`

  const blob = new Blob([html], { type: 'text/html' })
  const url = URL.createObjectURL(blob)

  const iframe = document.createElement('iframe')
  iframe.style.position = 'fixed'
  iframe.style.left = '-9999px'
  iframe.style.width = '1px'
  iframe.style.height = '1px'
  document.body.appendChild(iframe)

  iframe.onload = () => {
    try {
      iframe.contentWindow?.print()
    } catch {
      // Fallback: open in new tab
      window.open(url, '_blank')
    }
    setTimeout(() => {
      document.body.removeChild(iframe)
      URL.revokeObjectURL(url)
    }, 1000)
  }
  iframe.src = url
}

/* ------------------------------------------------------------------ */
/*  Virtualized Table Component                                        */
/* ------------------------------------------------------------------ */
interface VirtualizedTableProps {
  entries: AuditEntry[]
  totalHeight: number
}

function VirtualizedTable({ entries }: VirtualizedTableProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [scrollTop, setScrollTop] = useState(0)
  const [containerHeight, setContainerHeight] = useState(600)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerHeight(entry.contentRect.height)
      }
    })
    observer.observe(el)
    setContainerHeight(el.clientHeight)
    return () => observer.disconnect()
  }, [])

  const handleScroll = useCallback(() => {
    if (containerRef.current) {
      setScrollTop(containerRef.current.scrollTop)
    }
  }, [])

  const totalHeight = entries.length * ROW_HEIGHT
  const startIndex = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN)
  const visibleCount = Math.ceil(containerHeight / ROW_HEIGHT) + 2 * OVERSCAN
  const endIndex = Math.min(entries.length, startIndex + visibleCount)
  const visibleEntries = entries.slice(startIndex, endIndex)
  const offsetY = startIndex * ROW_HEIGHT

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="overflow-auto"
      style={{ maxHeight: 'calc(100vh - 380px)', minHeight: '400px' }}
    >
      <table className="w-full text-sm">
        <thead className="sticky top-0 z-10">
          <tr className="border-b border-nvr-border bg-nvr-bg-tertiary">
            <th className="text-left px-4 py-3 font-medium text-nvr-text-muted">Timestamp</th>
            <th className="text-left px-4 py-3 font-medium text-nvr-text-muted">User</th>
            <th className="text-left px-4 py-3 font-medium text-nvr-text-muted">Action</th>
            <th className="text-left px-4 py-3 font-medium text-nvr-text-muted">Resource</th>
            <th className="text-left px-4 py-3 font-medium text-nvr-text-muted">Details</th>
            <th className="text-left px-4 py-3 font-medium text-nvr-text-muted">IP Address</th>
          </tr>
        </thead>
        <tbody>
          {/* Spacer row for items above viewport */}
          {offsetY > 0 && (
            <tr><td colSpan={6} style={{ height: offsetY, padding: 0, border: 'none' }} /></tr>
          )}
          {visibleEntries.map(entry => (
            <tr
              key={entry.id}
              className="border-b border-nvr-border/50 hover:bg-nvr-bg-tertiary/30 transition-colors"
              style={{ height: ROW_HEIGHT }}
            >
              <td className="px-4 py-3 text-nvr-text-secondary whitespace-nowrap">
                {formatTimestamp(entry.created_at)}
              </td>
              <td className="px-4 py-3 text-nvr-text-primary font-medium">
                {entry.username || 'unknown'}
              </td>
              <td className="px-4 py-3">
                <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${ACTION_COLORS[entry.action] || 'bg-nvr-bg-tertiary text-nvr-text-muted'}`}>
                  {entry.action}
                </span>
              </td>
              <td className="px-4 py-3 text-nvr-text-secondary">
                <span className="text-nvr-text-muted">{entry.resource_type}</span>
                {entry.resource_id && (
                  <span className="text-nvr-text-secondary ml-1 font-mono text-xs">
                    {entry.resource_id.length > 12 ? entry.resource_id.slice(0, 12) + '...' : entry.resource_id}
                  </span>
                )}
              </td>
              <td className="px-4 py-3 text-nvr-text-secondary max-w-xs truncate" title={entry.details}>
                {entry.details || '-'}
              </td>
              <td className="px-4 py-3 text-nvr-text-muted font-mono text-xs">
                {entry.ip_address || '-'}
              </td>
            </tr>
          ))}
          {/* Spacer row for items below viewport */}
          {endIndex < entries.length && (
            <tr><td colSpan={6} style={{ height: (entries.length - endIndex) * ROW_HEIGHT, padding: 0, border: 'none' }} /></tr>
          )}
        </tbody>
      </table>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main Component                                                     */
/* ------------------------------------------------------------------ */
export default function AuditLog() {
  const defaults = defaultDateRange()

  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // Filters — all pushed to server
  const [actionFilter, setActionFilter] = useState('')
  const [resourceTypeFilter, setResourceTypeFilter] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [dateFrom, setDateFrom] = useState(defaults.from)
  const [dateTo, setDateTo] = useState(defaults.to)

  // Debounced search: send server request after user stops typing
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [debouncedUser, setDebouncedUser] = useState('')
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(searchQuery), 300)
    return () => clearTimeout(t)
  }, [searchQuery])
  useEffect(() => {
    const t = setTimeout(() => setDebouncedUser(userFilter), 300)
    return () => clearTimeout(t)
  }, [userFilter])

  // Export state
  const [exporting, setExporting] = useState(false)

  // Page title
  useEffect(() => {
    document.title = 'Audit Log — MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  const fetchEntries = useCallback(async () => {
    setLoading(true)
    setError('')

    const qs = buildParams({
      limit: PAGE_SIZE,
      offset: page * PAGE_SIZE,
      action: actionFilter,
      resourceType: resourceTypeFilter,
      userFilter: debouncedUser,
      searchQuery: debouncedSearch,
      dateFrom,
      dateTo,
    })

    try {
      const res = await apiFetch(`/audit?${qs}`)
      if (!res.ok) {
        const data = await res.json().catch(() => ({}))
        setError(data.error || `Failed to load audit log (${res.status})`)
        return
      }
      const data: AuditResponse = await res.json()
      setEntries(data.entries || [])
      setTotal(data.total)
    } catch (err) {
      setError('Failed to load audit log')
    } finally {
      setLoading(false)
    }
  }, [page, actionFilter, resourceTypeFilter, debouncedUser, debouncedSearch, dateFrom, dateTo])

  useEffect(() => {
    fetchEntries()
  }, [fetchEntries])

  // Reset page when filters change
  useEffect(() => {
    setPage(0)
  }, [actionFilter, resourceTypeFilter, debouncedUser, debouncedSearch, dateFrom, dateTo])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const filtersActive = actionFilter || resourceTypeFilter || userFilter || searchQuery ||
    dateFrom !== defaults.from || dateTo !== defaults.to

  // Export via server-side endpoint
  const handleExport = async (format: 'csv' | 'pdf') => {
    if (!dateFrom || !dateTo) {
      setError('Date range is required for export')
      return
    }

    setExporting(true)
    setError('')

    try {
      if (format === 'pdf') {
        // For PDF: fetch all matching entries via server export (JSON), then generate PDF client-side
        const params = new URLSearchParams()
        params.set('format', 'json')
        params.set('from', dateFrom)
        params.set('to', dateTo)
        if (actionFilter) params.set('action', actionFilter)
        if (resourceTypeFilter) params.set('resource_type', resourceTypeFilter)
        if (debouncedUser) params.set('user_id', debouncedUser)
        if (debouncedSearch) params.set('q', debouncedSearch)

        const res = await apiFetch(`/audit/export?${params.toString()}`)
        if (!res.ok) {
          const data = await res.json().catch(() => ({}))
          setError(data.error || `Export failed (${res.status})`)
          return
        }
        const exportedEntries: AuditEntry[] = await res.json()
        exportPDF(exportedEntries, `${dateFrom} to ${dateTo}`)
      } else {
        // CSV: use server-side export endpoint and trigger download
        const params = new URLSearchParams()
        params.set('format', 'csv')
        params.set('from', dateFrom)
        params.set('to', dateTo)
        if (actionFilter) params.set('action', actionFilter)
        if (resourceTypeFilter) params.set('resource_type', resourceTypeFilter)
        if (debouncedUser) params.set('user_id', debouncedUser)
        if (debouncedSearch) params.set('q', debouncedSearch)

        const res = await apiFetch(`/audit/export?${params.toString()}`)
        if (!res.ok) {
          const data = await res.json().catch(() => ({}))
          setError(data.error || `Export failed (${res.status})`)
          return
        }
        const blob = await res.blob()
        const url = URL.createObjectURL(blob)
        downloadFromURL(url, `audit-log-${dateFrom}-to-${dateTo}.csv`)
        URL.revokeObjectURL(url)
      }
    } catch {
      setError('Export failed')
    } finally {
      setExporting(false)
    }
  }

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Audit Log</h1>
          <p className="text-sm text-nvr-text-muted mt-1">
            {total.toLocaleString()} total {total === 1 ? 'entry' : 'entries'}
            {dateFrom && dateTo && (
              <span className="ml-1">({dateFrom} to {dateTo})</span>
            )}
          </p>
        </div>

        {/* Export buttons */}
        <div className="flex items-center gap-2">
          <button
            onClick={() => handleExport('csv')}
            disabled={exporting || total === 0}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-2 rounded-lg border border-nvr-border transition-colors text-sm inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            CSV
          </button>
          <button
            onClick={() => handleExport('pdf')}
            disabled={exporting || total === 0}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-2 rounded-lg border border-nvr-border transition-colors text-sm inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z" />
            </svg>
            PDF
          </button>
          {exporting && (
            <span className="inline-block w-4 h-4 border-2 border-nvr-text-muted/30 border-t-nvr-text-muted rounded-full animate-spin" />
          )}
        </div>
      </div>

      {/* Filters */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 mb-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-6 gap-3">
          {/* Search */}
          <div className="lg:col-span-2">
            <label htmlFor="audit-search" className="block text-xs font-medium text-nvr-text-muted mb-1">Search</label>
            <div className="relative">
              <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
                <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <input
                id="audit-search"
                type="text"
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                placeholder="Search username, resource, details, IP..."
                className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg pl-9 pr-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
              />
            </div>
          </div>

          {/* Action type filter */}
          <div>
            <label htmlFor="audit-action" className="block text-xs font-medium text-nvr-text-muted mb-1">Action</label>
            <select
              id="audit-action"
              value={actionFilter}
              onChange={e => setActionFilter(e.target.value)}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            >
              {ACTION_TYPES.map(a => (
                <option key={a.value} value={a.value}>{a.label}</option>
              ))}
            </select>
          </div>

          {/* Resource type filter */}
          <div>
            <label className="block text-xs font-medium text-nvr-text-muted mb-1">Resource</label>
            <select
              value={resourceTypeFilter}
              onChange={e => setResourceTypeFilter(e.target.value)}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            >
              {RESOURCE_TYPES.map(r => (
                <option key={r.value} value={r.value}>{r.label}</option>
              ))}
            </select>
          </div>

          {/* Date from */}
          <div>
            <label htmlFor="audit-date-from" className="block text-xs font-medium text-nvr-text-muted mb-1">From</label>
            <input
              id="audit-date-from"
              type="date"
              value={dateFrom}
              onChange={e => setDateFrom(e.target.value)}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>

          {/* Date to */}
          <div>
            <label htmlFor="audit-date-to" className="block text-xs font-medium text-nvr-text-muted mb-1">To</label>
            <input
              id="audit-date-to"
              type="date"
              value={dateTo}
              onChange={e => setDateTo(e.target.value)}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
          </div>
        </div>

        {/* User filter */}
        <div className="mt-3">
          <label htmlFor="audit-user-id" className="block text-xs font-medium text-nvr-text-muted mb-1">User ID</label>
          <input
            id="audit-user-id"
            type="text"
            value={userFilter}
            onChange={e => setUserFilter(e.target.value)}
            placeholder="Filter by user ID..."
            className="w-full sm:w-64 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />
        </div>

        {/* Clear filters */}
        {filtersActive && (
          <button
            onClick={() => {
              setActionFilter('')
              setResourceTypeFilter('')
              setUserFilter('')
              setSearchQuery('')
              const d = defaultDateRange()
              setDateFrom(d.from)
              setDateTo(d.to)
              setPage(0)
            }}
            className="mt-3 text-sm text-nvr-accent hover:text-nvr-accent-hover transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Clear all filters
          </button>
        )}
      </div>

      {/* Error */}
      {error && (
        <div role="alert" className="bg-red-500/10 border border-red-500/20 rounded-lg px-4 py-3 mb-4">
          <p className="text-sm text-red-400">{error}</p>
        </div>
      )}

      {/* Table */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
        {loading ? (
          <div className="flex items-center justify-center py-16">
            <span className="inline-block w-6 h-6 border-2 border-nvr-text-muted/30 border-t-nvr-accent rounded-full animate-spin" />
          </div>
        ) : entries.length === 0 ? (
          <div className="text-center py-16">
            <svg className="w-10 h-10 mx-auto mb-3 text-nvr-text-muted/40" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            <p className="text-nvr-text-muted">No audit log entries found.</p>
          </div>
        ) : (
          <>
            {/* Desktop virtualized table */}
            <div className="hidden md:block">
              <VirtualizedTable entries={entries} totalHeight={entries.length * ROW_HEIGHT} />
            </div>

            {/* Mobile cards */}
            <div className="md:hidden divide-y divide-nvr-border/50">
              {entries.map(entry => (
                <div key={entry.id} className="p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${ACTION_COLORS[entry.action] || 'bg-nvr-bg-tertiary text-nvr-text-muted'}`}>
                      {entry.action}
                    </span>
                    <span className="text-xs text-nvr-text-muted">
                      {formatTimestamp(entry.created_at)}
                    </span>
                  </div>
                  <p className="text-sm text-nvr-text-primary font-medium">
                    {entry.username || 'unknown'}
                  </p>
                  <p className="text-xs text-nvr-text-muted mt-1">
                    {entry.resource_type}
                    {entry.resource_id && (
                      <span className="font-mono ml-1">{entry.resource_id.slice(0, 12)}</span>
                    )}
                  </p>
                  {entry.details && (
                    <p className="text-xs text-nvr-text-secondary mt-1 line-clamp-2">{entry.details}</p>
                  )}
                  {entry.ip_address && (
                    <p className="text-xs text-nvr-text-muted font-mono mt-1">{entry.ip_address}</p>
                  )}
                </div>
              ))}
            </div>
          </>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-4">
          <p className="text-sm text-nvr-text-muted">
            Page {page + 1} of {totalPages} ({total.toLocaleString()} entries)
          </p>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage(p => Math.max(0, p - 1))}
              disabled={page === 0}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Previous
            </button>
            <button
              onClick={() => setPage(p => Math.min(totalPages - 1, p + 1))}
              disabled={page >= totalPages - 1}
              className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-1.5 rounded-lg border border-nvr-border transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
