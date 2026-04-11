import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../api/client'

export interface NotificationItem {
  id: string
  type: string
  severity: string
  camera: string
  message: string
  created_at: string
  read_at: string | null
  archived: boolean
}

export interface NotificationFilters {
  camera: string
  type: string
  severity: string
  read: string   // '' | 'true' | 'false'
  archived: boolean
  q: string
  since: string
  until: string
}

interface NotificationPage {
  notifications: NotificationItem[]
  total: number
  limit: number
  offset: number
}

const PAGE_SIZE = 30

export function useNotificationCenter(isAuthenticated: boolean) {
  const [notifications, setNotifications] = useState<NotificationItem[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState<NotificationFilters>({
    camera: '',
    type: '',
    severity: '',
    read: '',
    archived: false,
    q: '',
    since: '',
    until: '',
  })

  const fetchRef = useRef(0)

  const fetchNotifications = useCallback(async (newOffset = 0) => {
    if (!isAuthenticated) return

    const fetchId = ++fetchRef.current
    setLoading(true)

    const params = new URLSearchParams()
    params.set('limit', String(PAGE_SIZE))
    params.set('offset', String(newOffset))
    if (filters.archived) params.set('archived', 'true')
    if (filters.camera) params.set('camera', filters.camera)
    if (filters.type) params.set('type', filters.type)
    if (filters.severity) params.set('severity', filters.severity)
    if (filters.read) params.set('read', filters.read)
    if (filters.q) params.set('q', filters.q)
    if (filters.since) params.set('since', filters.since)
    if (filters.until) params.set('until', filters.until)

    try {
      const res = await apiFetch(`/notifications?${params.toString()}`)
      if (!res.ok) return

      const data: NotificationPage = await res.json()

      if (fetchId !== fetchRef.current) return

      setNotifications(data.notifications || [])
      setTotal(data.total)
      setOffset(newOffset)
    } catch {
      // ignore
    } finally {
      if (fetchId === fetchRef.current) setLoading(false)
    }
  }, [isAuthenticated, filters])

  useEffect(() => {
    fetchNotifications(0)
  }, [fetchNotifications])

  // Poll for new notifications every 30s (fallback; WebSocket is primary).
  useEffect(() => {
    if (!isAuthenticated) return
    const id = setInterval(() => fetchNotifications(offset), 30000)
    return () => clearInterval(id)
  }, [isAuthenticated, fetchNotifications, offset])

  // Real-time WebSocket updates — refresh the current page when a new
  // notification arrives so the list stays live across devices.
  useEffect(() => {
    if (!isAuthenticated) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const apiPort = parseInt(window.location.port || '9997', 10)
    const wsPort = apiPort + 1
    const url = `${protocol}//${window.location.hostname}:${wsPort}/ws`

    let ws: WebSocket | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let reconnectDelay = 3000
    let disposed = false

    const connect = () => {
      if (disposed) return
      ws = new WebSocket(url)

      ws.onopen = () => { reconnectDelay = 3000 }

      ws.onmessage = () => {
        // Any event (except connected) triggers a refresh of the current page.
        fetchNotifications(offset)
      }

      ws.onclose = () => {
        ws = null
        if (!disposed) {
          reconnectTimer = setTimeout(connect, reconnectDelay)
          reconnectDelay = Math.min(reconnectDelay * 2, 30000)
        }
      }

      ws.onerror = () => { ws?.close() }
    }

    connect()

    return () => {
      disposed = true
      ws?.close()
      if (reconnectTimer) clearTimeout(reconnectTimer)
    }
  }, [isAuthenticated, fetchNotifications, offset])

  const nextPage = useCallback(() => {
    if (offset + PAGE_SIZE < total) {
      fetchNotifications(offset + PAGE_SIZE)
    }
  }, [offset, total, fetchNotifications])

  const prevPage = useCallback(() => {
    if (offset > 0) {
      fetchNotifications(Math.max(0, offset - PAGE_SIZE))
    }
  }, [offset, fetchNotifications])

  const markRead = useCallback(async (ids: string[]) => {
    await apiFetch('/notifications/mark-read', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
    setNotifications(prev =>
      prev.map(n => ids.includes(n.id) ? { ...n, read_at: new Date().toISOString() } : n)
    )
  }, [])

  const markUnread = useCallback(async (ids: string[]) => {
    await apiFetch('/notifications/mark-unread', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
    setNotifications(prev =>
      prev.map(n => ids.includes(n.id) ? { ...n, read_at: null } : n)
    )
  }, [])

  const markAllRead = useCallback(async () => {
    await apiFetch('/notifications/mark-all-read', { method: 'POST' })
    setNotifications(prev => prev.map(n => ({ ...n, read_at: n.read_at || new Date().toISOString() })))
  }, [])

  const archive = useCallback(async (ids: string[]) => {
    await apiFetch('/notifications/archive', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
    setNotifications(prev => prev.filter(n => !ids.includes(n.id)))
    setTotal(prev => Math.max(0, prev - ids.length))
  }, [])

  const restore = useCallback(async (ids: string[]) => {
    await apiFetch('/notifications/restore', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
    setNotifications(prev => prev.filter(n => !ids.includes(n.id)))
    setTotal(prev => Math.max(0, prev - ids.length))
  }, [])

  const deleteNotifications = useCallback(async (ids: string[]) => {
    await apiFetch('/notifications', {
      method: 'DELETE',
      body: JSON.stringify({ ids }),
    })
    setNotifications(prev => prev.filter(n => !ids.includes(n.id)))
    setTotal(prev => Math.max(0, prev - ids.length))
  }, [])

  return {
    notifications,
    total,
    offset,
    pageSize: PAGE_SIZE,
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
    refresh: () => fetchNotifications(offset),
  }
}
