import { useState, useMemo } from 'react'
import { useAuth } from '../auth/context'
import { useNotificationCenter, NotificationItem } from '../hooks/useNotificationCenter'

/* ------------------------------------------------------------------ */
/*  Severity badge                                                     */
/* ------------------------------------------------------------------ */
function SeverityBadge({ severity }: { severity: string }) {
  const colors: Record<string, string> = {
    critical: 'bg-red-500/20 text-red-400',
    warning: 'bg-yellow-500/20 text-yellow-400',
    info: 'bg-blue-500/20 text-blue-400',
  }
  return (
    <span className={`px-2 py-0.5 rounded text-[10px] font-semibold uppercase ${colors[severity] || colors.info}`}>
      {severity}
    </span>
  )
}

/* ------------------------------------------------------------------ */
/*  Type icon                                                          */
/* ------------------------------------------------------------------ */
function typeLabel(type: string): string {
  const labels: Record<string, string> = {
    motion: 'Motion',
    camera_offline: 'Camera Offline',
    camera_online: 'Camera Online',
    recording_started: 'Recording Started',
    recording_stopped: 'Recording Stopped',
    recording_stalled: 'Recording Stalled',
    recording_recovered: 'Recording Recovered',
    recording_failed: 'Recording Failed',
    tampering: 'Tampering',
    intrusion: 'Intrusion',
    line_crossing: 'Line Crossing',
    loitering: 'Loitering',
    ai_detection: 'AI Detection',
    object_count: 'Object Count',
  }
  return labels[type] || type
}

/* ------------------------------------------------------------------ */
/*  Relative time                                                      */
/* ------------------------------------------------------------------ */
function relativeTime(dateStr: string): string {
  const date = new Date(dateStr)
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return date.toLocaleDateString()
}

/* ------------------------------------------------------------------ */
/*  Notification row                                                   */
/* ------------------------------------------------------------------ */
function NotificationRow({
  notif,
  selected,
  onToggleSelect,
  onMarkRead,
  onMarkUnread,
  onArchive,
  onRestore,
}: {
  notif: NotificationItem
  selected: boolean
  onToggleSelect: () => void
  onMarkRead: () => void
  onMarkUnread: () => void
  onArchive: () => void
  onRestore: () => void
}) {
  const isRead = !!notif.read_at

  return (
    <div
      className={`flex items-center gap-3 px-4 py-3 border-b border-nvr-border transition-colors hover:bg-nvr-bg-tertiary/50 ${
        !isRead ? 'bg-nvr-bg-tertiary/30' : ''
      }`}
    >
      {/* Checkbox */}
      <input
        type="checkbox"
        checked={selected}
        onChange={onToggleSelect}
        className="w-4 h-4 rounded border-nvr-border bg-nvr-bg-tertiary text-nvr-accent focus:ring-nvr-accent/50 cursor-pointer"
      />

      {/* Unread indicator */}
      <div className="w-2 flex-shrink-0">
        {!isRead && <span className="block w-2 h-2 rounded-full bg-nvr-accent" />}
      </div>

      {/* Content */}
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 mb-0.5">
          <SeverityBadge severity={notif.severity} />
          <span className="text-xs font-medium text-nvr-text-primary">
            {typeLabel(notif.type)}
          </span>
          {notif.camera && (
            <span className="text-xs text-nvr-text-muted truncate">
              {notif.camera}
            </span>
          )}
        </div>
        <p className="text-sm text-nvr-text-secondary truncate">{notif.message}</p>
      </div>

      {/* Timestamp */}
      <span className="text-xs text-nvr-text-muted whitespace-nowrap flex-shrink-0">
        {relativeTime(notif.created_at)}
      </span>

      {/* Actions */}
      <div className="flex items-center gap-1 flex-shrink-0">
        {isRead ? (
          <button
            onClick={onMarkUnread}
            className="p-1.5 text-nvr-text-muted hover:text-nvr-text-primary transition-colors rounded"
            title="Mark unread"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
          </button>
        ) : (
          <button
            onClick={onMarkRead}
            className="p-1.5 text-nvr-text-muted hover:text-nvr-text-primary transition-colors rounded"
            title="Mark read"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 19v-8.93a2 2 0 01.89-1.664l7-4.666a2 2 0 012.22 0l7 4.666A2 2 0 0121 10.07V19M3 19a2 2 0 002 2h14a2 2 0 002-2M3 19l6.75-4.5M21 19l-6.75-4.5M3 10l6.75 4.5M21 10l-6.75 4.5" />
            </svg>
          </button>
        )}
        {notif.archived ? (
          <button
            onClick={onRestore}
            className="p-1.5 text-nvr-text-muted hover:text-nvr-accent transition-colors rounded"
            title="Restore from archive"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6" />
            </svg>
          </button>
        ) : (
          <button
            onClick={onArchive}
            className="p-1.5 text-nvr-text-muted hover:text-nvr-text-primary transition-colors rounded"
            title="Archive"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
            </svg>
          </button>
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Filter bar                                                         */
/* ------------------------------------------------------------------ */
const EVENT_TYPES = [
  'motion', 'camera_offline', 'camera_online',
  'recording_started', 'recording_stopped', 'recording_stalled',
  'recording_recovered', 'recording_failed',
  'tampering', 'intrusion', 'line_crossing', 'loitering',
  'ai_detection', 'object_count',
]
const SEVERITY_LEVELS = ['critical', 'warning', 'info']

/* ------------------------------------------------------------------ */
/*  Main page                                                          */
/* ------------------------------------------------------------------ */
export default function NotificationCenter() {
  const { isAuthenticated } = useAuth()
  const {
    notifications,
    total,
    offset,
    pageSize,
    loading,
    filters,
    setFilters,
    nextPage,
    prevPage,
    markRead,
    markUnread,
    markAllRead,
    archive,
    restore,
    deleteNotifications,
    refresh,
  } = useNotificationCenter(isAuthenticated)

  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [searchInput, setSearchInput] = useState(filters.q)

  // Derive "all selected" state
  const allSelected = notifications.length > 0 && notifications.every(n => selectedIds.has(n.id))

  const toggleSelect = (id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(notifications.map(n => n.id)))
    }
  }

  const selectedArray = useMemo(() => Array.from(selectedIds), [selectedIds])

  const handleBulkMarkRead = async () => {
    if (selectedArray.length === 0) return
    await markRead(selectedArray)
    setSelectedIds(new Set())
  }

  const handleBulkMarkUnread = async () => {
    if (selectedArray.length === 0) return
    await markUnread(selectedArray)
    setSelectedIds(new Set())
  }

  const handleBulkArchive = async () => {
    if (selectedArray.length === 0) return
    await archive(selectedArray)
    setSelectedIds(new Set())
  }

  const handleBulkRestore = async () => {
    if (selectedArray.length === 0) return
    await restore(selectedArray)
    setSelectedIds(new Set())
  }

  const handleBulkDelete = async () => {
    if (selectedArray.length === 0) return
    if (!confirm(`Permanently delete ${selectedArray.length} notification(s)?`)) return
    await deleteNotifications(selectedArray)
    setSelectedIds(new Set())
  }

  const handleSearch = () => {
    setFilters(prev => ({ ...prev, q: searchInput }))
  }

  const startItem = total === 0 ? 0 : offset + 1
  const endItem = Math.min(offset + pageSize, total)

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-xl font-bold text-nvr-text-primary">Notification Center</h1>
          <p className="text-sm text-nvr-text-muted mt-1">
            {total} notification{total !== 1 ? 's' : ''}
            {filters.archived ? ' in archive' : ''}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => {
              setFilters(prev => ({ ...prev, archived: !prev.archived }))
              setSelectedIds(new Set())
            }}
            className={`px-3 py-1.5 text-sm rounded-lg border transition-colors ${
              filters.archived
                ? 'bg-nvr-accent/20 text-nvr-accent border-nvr-accent/30'
                : 'bg-nvr-bg-tertiary text-nvr-text-secondary border-nvr-border hover:text-nvr-text-primary'
            }`}
          >
            {filters.archived ? 'Viewing Archive' : 'View Archive'}
          </button>
          {!filters.archived && (
            <button
              onClick={markAllRead}
              className="px-3 py-1.5 text-sm rounded-lg bg-nvr-bg-tertiary text-nvr-text-secondary border border-nvr-border hover:text-nvr-text-primary transition-colors"
            >
              Mark All Read
            </button>
          )}
          <button
            onClick={refresh}
            className="p-1.5 text-nvr-text-muted hover:text-nvr-text-primary transition-colors rounded"
            title="Refresh"
          >
            <svg className={`w-5 h-5 ${loading ? 'animate-spin' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </button>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-center gap-2 mb-4 p-3 bg-nvr-bg-secondary border border-nvr-border rounded-lg">
        {/* Search */}
        <div className="flex items-center gap-1">
          <input
            type="text"
            value={searchInput}
            onChange={e => setSearchInput(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSearch()}
            placeholder="Search notifications..."
            className="w-48 px-3 py-1.5 text-sm bg-nvr-bg-tertiary border border-nvr-border rounded text-nvr-text-primary placeholder:text-nvr-text-muted focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
          />
          <button
            onClick={handleSearch}
            className="p-1.5 text-nvr-text-muted hover:text-nvr-text-primary transition-colors"
            title="Search"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </button>
        </div>

        {/* Type filter */}
        <select
          value={filters.type}
          onChange={e => setFilters(prev => ({ ...prev, type: e.target.value }))}
          className="px-2 py-1.5 text-sm bg-nvr-bg-tertiary border border-nvr-border rounded text-nvr-text-primary focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
        >
          <option value="">All Types</option>
          {EVENT_TYPES.map(t => (
            <option key={t} value={t}>{typeLabel(t)}</option>
          ))}
        </select>

        {/* Severity filter */}
        <select
          value={filters.severity}
          onChange={e => setFilters(prev => ({ ...prev, severity: e.target.value }))}
          className="px-2 py-1.5 text-sm bg-nvr-bg-tertiary border border-nvr-border rounded text-nvr-text-primary focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
        >
          <option value="">All Severities</option>
          {SEVERITY_LEVELS.map(s => (
            <option key={s} value={s}>{s.charAt(0).toUpperCase() + s.slice(1)}</option>
          ))}
        </select>

        {/* Read filter */}
        <select
          value={filters.read}
          onChange={e => setFilters(prev => ({ ...prev, read: e.target.value }))}
          className="px-2 py-1.5 text-sm bg-nvr-bg-tertiary border border-nvr-border rounded text-nvr-text-primary focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
        >
          <option value="">Read & Unread</option>
          <option value="false">Unread Only</option>
          <option value="true">Read Only</option>
        </select>

        {/* Camera filter (text input) */}
        <input
          type="text"
          value={filters.camera}
          onChange={e => setFilters(prev => ({ ...prev, camera: e.target.value }))}
          placeholder="Camera..."
          className="w-32 px-2 py-1.5 text-sm bg-nvr-bg-tertiary border border-nvr-border rounded text-nvr-text-primary placeholder:text-nvr-text-muted focus:outline-none focus:ring-1 focus:ring-nvr-accent/50"
        />

        {/* Clear filters */}
        {(filters.type || filters.severity || filters.read || filters.camera || filters.q) && (
          <button
            onClick={() => {
              setFilters(prev => ({
                ...prev,
                type: '',
                severity: '',
                read: '',
                camera: '',
                q: '',
                since: '',
                until: '',
              }))
              setSearchInput('')
            }}
            className="px-2 py-1.5 text-xs text-nvr-text-muted hover:text-nvr-text-primary transition-colors"
          >
            Clear Filters
          </button>
        )}
      </div>

      {/* Bulk action bar */}
      {selectedIds.size > 0 && (
        <div className="flex items-center gap-2 mb-3 p-2 bg-nvr-accent/10 border border-nvr-accent/20 rounded-lg">
          <span className="text-sm text-nvr-accent font-medium px-2">
            {selectedIds.size} selected
          </span>
          <button onClick={handleBulkMarkRead} className="px-2 py-1 text-xs bg-nvr-bg-tertiary text-nvr-text-secondary border border-nvr-border rounded hover:text-nvr-text-primary transition-colors">
            Mark Read
          </button>
          <button onClick={handleBulkMarkUnread} className="px-2 py-1 text-xs bg-nvr-bg-tertiary text-nvr-text-secondary border border-nvr-border rounded hover:text-nvr-text-primary transition-colors">
            Mark Unread
          </button>
          {filters.archived ? (
            <button onClick={handleBulkRestore} className="px-2 py-1 text-xs bg-nvr-bg-tertiary text-nvr-accent border border-nvr-accent/30 rounded hover:bg-nvr-accent/10 transition-colors">
              Restore
            </button>
          ) : (
            <button onClick={handleBulkArchive} className="px-2 py-1 text-xs bg-nvr-bg-tertiary text-nvr-text-secondary border border-nvr-border rounded hover:text-nvr-text-primary transition-colors">
              Archive
            </button>
          )}
          <button onClick={handleBulkDelete} className="px-2 py-1 text-xs bg-red-500/10 text-red-400 border border-red-500/20 rounded hover:bg-red-500/20 transition-colors">
            Delete
          </button>
        </div>
      )}

      {/* Notification list */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg overflow-hidden">
        {/* Select-all header */}
        <div className="flex items-center gap-3 px-4 py-2 border-b border-nvr-border bg-nvr-bg-tertiary/50">
          <input
            type="checkbox"
            checked={allSelected}
            onChange={toggleSelectAll}
            className="w-4 h-4 rounded border-nvr-border bg-nvr-bg-tertiary text-nvr-accent focus:ring-nvr-accent/50 cursor-pointer"
          />
          <span className="text-xs text-nvr-text-muted">
            {allSelected ? 'Deselect all' : 'Select all'}
          </span>
        </div>

        {loading && notifications.length === 0 ? (
          <div className="px-4 py-12 text-center text-nvr-text-muted text-sm">
            Loading notifications...
          </div>
        ) : notifications.length === 0 ? (
          <div className="px-4 py-12 text-center text-nvr-text-muted text-sm">
            {filters.archived ? 'No archived notifications' : 'No notifications'}
          </div>
        ) : (
          notifications.map(notif => (
            <NotificationRow
              key={notif.id}
              notif={notif}
              selected={selectedIds.has(notif.id)}
              onToggleSelect={() => toggleSelect(notif.id)}
              onMarkRead={() => markRead([notif.id])}
              onMarkUnread={() => markUnread([notif.id])}
              onArchive={() => archive([notif.id])}
              onRestore={() => restore([notif.id])}
            />
          ))
        )}

        {/* Pagination */}
        {total > 0 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-nvr-border">
            <span className="text-xs text-nvr-text-muted">
              Showing {startItem}-{endItem} of {total}
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={prevPage}
                disabled={offset === 0}
                className="px-3 py-1 text-xs bg-nvr-bg-tertiary text-nvr-text-secondary border border-nvr-border rounded hover:text-nvr-text-primary transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Previous
              </button>
              <button
                onClick={nextPage}
                disabled={offset + pageSize >= total}
                className="px-3 py-1 text-xs bg-nvr-bg-tertiary text-nvr-text-secondary border border-nvr-border rounded hover:text-nvr-text-primary transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
