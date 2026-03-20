import { useEffect, useRef, useState, useCallback } from 'react'
import { pushToast, ToastMessage } from '../components/Toast'

export interface Notification {
  id: string
  type: 'motion' | 'camera_offline' | 'camera_online' | 'recording_started' | 'recording_stopped'
  camera: string
  message: string
  time: Date
  read: boolean
}

const MAX_HISTORY = 20
const RECONNECT_DELAY_MS = 3000

function eventTypeToToastType(eventType: string): ToastMessage['type'] {
  switch (eventType) {
    case 'motion':
      return 'warning'
    case 'camera_offline':
      return 'error'
    case 'camera_online':
      return 'success'
    case 'recording_started':
    case 'recording_stopped':
      return 'info'
    default:
      return 'info'
  }
}

function eventTypeToTitle(eventType: string): string {
  switch (eventType) {
    case 'motion':
      return 'Motion Detected'
    case 'camera_offline':
      return 'Camera Offline'
    case 'camera_online':
      return 'Camera Online'
    case 'recording_started':
      return 'Recording Started'
    case 'recording_stopped':
      return 'Recording Stopped'
    default:
      return 'Notification'
  }
}

export function useNotifications(isAuthenticated: boolean) {
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const eventSourceRef = useRef<EventSource | null>(null)
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const addNotification = useCallback((notif: Notification) => {
    setNotifications(prev => {
      const next = [notif, ...prev]
      if (next.length > MAX_HISTORY) {
        return next.slice(0, MAX_HISTORY)
      }
      return next
    })
    setUnreadCount(prev => prev + 1)

    // Push a toast notification.
    pushToast({
      id: notif.id,
      type: eventTypeToToastType(notif.type),
      title: eventTypeToTitle(notif.type),
      message: notif.message,
      timestamp: notif.time,
    })
  }, [])

  const markAllRead = useCallback(() => {
    setNotifications(prev => prev.map(n => ({ ...n, read: true })))
    setUnreadCount(0)
  }, [])

  const connect = useCallback(() => {
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
    }

    const es = new EventSource('/api/nvr/system/events')
    eventSourceRef.current = es

    es.addEventListener('notification', (e: MessageEvent) => {
      try {
        const data = JSON.parse(e.data)
        const notif: Notification = {
          id: crypto.randomUUID(),
          type: data.type,
          camera: data.camera,
          message: data.message,
          time: new Date(data.time),
          read: false,
        }
        addNotification(notif)
      } catch {
        // Ignore malformed events.
      }
    })

    es.onerror = () => {
      es.close()
      eventSourceRef.current = null
      // Auto-reconnect after a delay.
      reconnectTimerRef.current = setTimeout(() => {
        if (isAuthenticated) {
          connect()
        }
      }, RECONNECT_DELAY_MS)
    }
  }, [addNotification, isAuthenticated])

  useEffect(() => {
    if (!isAuthenticated) {
      // Close connection when not authenticated.
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
      return
    }

    connect()

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
    }
  }, [isAuthenticated, connect])

  return { notifications, unreadCount, markAllRead }
}
