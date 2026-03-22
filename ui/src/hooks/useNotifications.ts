import { useEffect, useRef, useState, useCallback } from 'react'
import { pushToast, ToastMessage } from '../components/Toast'
import { getAccessToken } from '../api/client'

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

function getNotifPrefs() {
  try {
    return {
      enabled: localStorage.getItem('nvr-notif-enabled') !== 'false',
      motion: localStorage.getItem('nvr-notif-motion') !== 'false',
      offline: localStorage.getItem('nvr-notif-offline') !== 'false',
      sound: localStorage.getItem('nvr-notif-sound') === 'true',
    }
  } catch {
    return { enabled: true, motion: true, offline: true, sound: false }
  }
}

function eventTypeToToastType(eventType: string): ToastMessage['type'] {
  switch (eventType) {
    case 'motion': return 'warning'
    case 'camera_offline': return 'error'
    case 'camera_online': return 'success'
    default: return 'info'
  }
}

function eventTypeToTitle(eventType: string): string {
  switch (eventType) {
    case 'motion': return 'Motion Detected'
    case 'camera_offline': return 'Camera Offline'
    case 'camera_online': return 'Camera Online'
    case 'recording_started': return 'Recording Started'
    case 'recording_stopped': return 'Recording Stopped'
    default: return 'Notification'
  }
}

function playAlertSound() {
  try {
    const ctx = new AudioContext()
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.frequency.value = 880
    gain.gain.value = 0.15
    osc.start()
    osc.stop(ctx.currentTime + 0.15)
  } catch {
    // ignore
  }
}

export function useNotifications(isAuthenticated: boolean) {
  const [notifications, setNotifications] = useState<Notification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const reconnectDelayRef = useRef(RECONNECT_DELAY_MS)
  const MAX_RECONNECT_DELAY = 30000

  const addNotification = useCallback((notif: Notification) => {
    const prefs = getNotifPrefs()

    if (!prefs.enabled) {
      setNotifications(prev => [notif, ...prev].slice(0, MAX_HISTORY))
      setUnreadCount(prev => prev + 1)
      return
    }

    if (notif.type === 'motion' && !prefs.motion) return
    if ((notif.type === 'camera_offline' || notif.type === 'camera_online') && !prefs.offline) return

    setNotifications(prev => [notif, ...prev].slice(0, MAX_HISTORY))
    setUnreadCount(prev => prev + 1)

    pushToast({
      id: notif.id,
      type: eventTypeToToastType(notif.type),
      title: eventTypeToTitle(notif.type),
      message: notif.message,
      timestamp: notif.time,
    })

    if (prefs.sound) playAlertSound()
  }, [])

  const markAllRead = useCallback(() => {
    setNotifications(prev => prev.map(n => ({ ...n, read: true })))
    setUnreadCount(0)
  }, [])

  const connect = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close()
    }

    const token = getAccessToken()
    if (!token) return

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const apiPort = parseInt(window.location.port || '9997', 10)
    const wsPort = apiPort + 1
    const url = `${protocol}//${window.location.hostname}:${wsPort}/ws`

    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      reconnectDelayRef.current = RECONNECT_DELAY_MS // reset backoff on success
    }

    ws.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data)
        // Skip the initial "connected" message
        if (data.type === 'connected') return

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
        // ignore malformed messages
      }
    }

    ws.onclose = () => {
      wsRef.current = null
      reconnectTimerRef.current = setTimeout(() => {
        if (isAuthenticated) connect()
      }, reconnectDelayRef.current)
      // Exponential backoff
      reconnectDelayRef.current = Math.min(reconnectDelayRef.current * 2, MAX_RECONNECT_DELAY)
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [addNotification, isAuthenticated])

  useEffect(() => {
    if (!isAuthenticated) {
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
      return
    }

    connect()

    return () => {
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
      if (reconnectTimerRef.current) {
        clearTimeout(reconnectTimerRef.current)
        reconnectTimerRef.current = null
      }
    }
  }, [isAuthenticated, connect])

  return { notifications, unreadCount, markAllRead }
}
