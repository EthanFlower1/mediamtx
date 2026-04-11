import { useState, useEffect, useCallback, useMemo } from 'react'
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

const ACTION_TYPES = [
  { value: '', label: 'All Actions' },
  { value: 'create', label: 'Create' },
  { value: 'update', label: 'Update' },
  { value: 'delete', label: 'Delete' },
  { value: 'login', label: 'Login' },
  { value: 'logout', label: 'Logout' },
  { value: 'login_failed', label: 'Login Failed' },
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

function toCSV(entries: AuditEntry[]): string {
  const headers = ['ID', 'Timestamp', 'Username', 'Action', 'Resource Type', 'Resource ID', 'Details', 'IP Address']
  const rows = entries.map(e => [
    e.id,
    e.created_at,
    e.username,
    e.action,
    e.resource_type,
    e.resource_id,
    `"${(e.details || '').replace(/"/g, '""')}"`,
    e.ip_address,
  ].join(','))
  return [headers.join(','), ...rows].join('\n')
}

function downloadBlob(content: string, filename: string, mime: string) {
  const blob = new Blob([content], { type: mime })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

/* ------------------------------------------------------------------ */
/*  Main Component                                                     */
/* ------------------------------------------------------------------ */
export default function AuditLog() {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // Filters
  const [actionFilter, setActionFilter] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const [searchQuery, setSearchQuery] = useState('')
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')

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

    const params = new URLSearchParams()
    params.set('limit', String(PAGE_SIZE))
    params.set('offset', String(page * PAGE_SIZE))
    if (actionFilter) params.set('action', actionFilter)
    if (userFilter) params.set('user_id', userFilter)

    try {
      const res = await apiFetch(`/audit?${params.toString()}`)
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
  }, [page, actionFilter, userFilter])

  useEffect(() => {
    fetchEntries()
  }, [fetchEntries])

  // Reset page when filters change
  useEffect(() => {
    setPage(0)
  }, [actionFilter, userFilter])

  // Client-side filtering for search query and date range (applied on top of server results)
  const filteredEntries = useMemo(() => {
    let result = entries

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase()
      result = result.filter(e =>
        e.username.toLowerCase().includes(q) ||
        e.action.toLowerCase().includes(q) ||
        e.resource_type.toLowerCase().includes(q) ||
        e.resource_id.toLowerCase().includes(q) ||
        (e.details || '').toLowerCase().includes(q) ||
        e.ip_address.toLowerCase().includes(q)
      )
    }

    if (dateFrom) {
      const from = new Date(dateFrom)
      result = result.filter(e => {
        const d = new Date(e.created_at.endsWith('Z') ? e.created_at : e.created_at + 'Z')
        return d >= from
      })
    }

    if (dateTo) {
      const to = new Date(dateTo)
      to.setDate(to.getDate() + 1) // include the entire "to" day
      result = result.filter(e => {
        const d = new Date(e.created_at.endsWith('Z') ? e.created_at : e.created_at + 'Z')
        return d < to
      })
    }

    return result
  }, [entries, searchQuery, dateFrom, dateTo])

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  // Export all matching entries (fetches all pages)
  const handleExport = async (format: 'csv' | 'json') => {
    setExporting(true)
    try {
      // Fetch all entries matching current filters
      const allEntries: AuditEntry[] = []
      let offset = 0
      const batchSize = 200
      while (true) {
        const params = new URLSearchParams()
        params.set('limit', String(batchSize))
        params.set('offset', String(offset))
        if (actionFilter) params.set('action', actionFilter)
        if (userFilter) params.set('user_id', userFilter)

        const res = await apiFetch(`/audit?${params.toString()}`)
        if (!res.ok) break
        const data: AuditResponse = await res.json()
        allEntries.push(...(data.entries || []))
        if (allEntries.length >= data.total || (data.entries || []).length < batchSize) break
        offset += batchSize
      }

      // Apply client-side filters
      let toExport = allEntries
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase()
        toExport = toExport.filter(e =>
          e.username.toLowerCase().includes(q) ||
          e.action.toLowerCase().includes(q) ||
          e.resource_type.toLowerCase().includes(q) ||
          e.resource_id.toLowerCase().includes(q) ||
          (e.details || '').toLowerCase().includes(q) ||
          e.ip_address.toLowerCase().includes(q)
        )
      }
      if (dateFrom) {
        const from = new Date(dateFrom)
        toExport = toExport.filter(e => new Date(e.created_at.endsWith('Z') ? e.created_at : e.created_at + 'Z') >= from)
      }
      if (dateTo) {
        const to = new Date(dateTo)
        to.setDate(to.getDate() + 1)
        toExport = toExport.filter(e => new Date(e.created_at.endsWith('Z') ? e.created_at : e.created_at + 'Z') < to)
      }

      const timestamp = new Date().toISOString().slice(0, 10)
      if (format === 'csv') {
        downloadBlob(toCSV(toExport), `audit-log-${timestamp}.csv`, 'text/csv')
      } else {
        downloadBlob(JSON.stringify(toExport, null, 2), `audit-log-${timestamp}.json`, 'application/json')
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
            {total} total {total === 1 ? 'entry' : 'entries'}
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
            onClick={() => handleExport('json')}
            disabled={exporting || total === 0}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-3 py-2 rounded-lg border border-nvr-border transition-colors text-sm inline-flex items-center gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 10v6m0 0l-3-3m3 3l3-3m2 8H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            JSON
          </button>
          {exporting && (
            <span className="inline-block w-4 h-4 border-2 border-nvr-text-muted/30 border-t-nvr-text-muted rounded-full animate-spin" />
          )}
        </div>
      </div>

      {/* Filters */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 mb-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5 gap-3">
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
                placeholder="Search by keyword..."
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

        {/* User filter (text input since we don't have a users list here) */}
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
        {(actionFilter || userFilter || searchQuery || dateFrom || dateTo) && (
          <button
            onClick={() => {
              setActionFilter('')
              setUserFilter('')
              setSearchQuery('')
              setDateFrom('')
              setDateTo('')
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
        ) : filteredEntries.length === 0 ? (
          <div className="text-center py-16">
            <svg className="w-10 h-10 mx-auto mb-3 text-nvr-text-muted/40" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            <p className="text-nvr-text-muted">No audit log entries found.</p>
          </div>
        ) : (
          <>
            {/* Desktop table */}
            <div className="hidden md:block overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-nvr-border bg-nvr-bg-tertiary/50">
                    <th scope="col" className="text-left px-4 py-3 font-medium text-nvr-text-muted">Timestamp</th>
                    <th scope="col" className="text-left px-4 py-3 font-medium text-nvr-text-muted">User</th>
                    <th scope="col" className="text-left px-4 py-3 font-medium text-nvr-text-muted">Action</th>
                    <th scope="col" className="text-left px-4 py-3 font-medium text-nvr-text-muted">Resource</th>
                    <th scope="col" className="text-left px-4 py-3 font-medium text-nvr-text-muted">Details</th>
                    <th scope="col" className="text-left px-4 py-3 font-medium text-nvr-text-muted">IP Address</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredEntries.map(entry => (
                    <tr key={entry.id} className="border-b border-nvr-border/50 hover:bg-nvr-bg-tertiary/30 transition-colors">
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
                </tbody>
              </table>
            </div>

            {/* Mobile cards */}
            <div className="md:hidden divide-y divide-nvr-border/50">
              {filteredEntries.map(entry => (
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
            Page {page + 1} of {totalPages}
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
