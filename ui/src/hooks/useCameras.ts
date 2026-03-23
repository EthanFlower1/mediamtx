import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

export interface Camera {
  id: string
  name: string
  rtsp_url: string
  mediamtx_path: string
  status: string
  ptz_capable: boolean
  onvif_endpoint?: string
  updated_at?: string
  retention_days?: number
  supports_imaging?: boolean
  supports_events?: boolean
  supports_relay?: boolean
  supports_audio_backchannel?: boolean
  snapshot_uri?: string
  supports_media2?: boolean
  supports_analytics?: boolean
  supports_edge_recording?: boolean
  motion_timeout_seconds?: number
}

function getRefreshInterval(): number {
  try {
    const saved = localStorage.getItem('nvr-refresh-interval')
    if (saved) {
      const val = parseInt(saved, 10)
      if (val >= 5 && val <= 60) return val * 1000
    }
  } catch {
    // ignore
  }
  return 15000
}

export function useCameras() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = useCallback(async () => {
    const res = await apiFetch('/cameras')
    if (res.ok) setCameras(await res.json())
    setLoading(false)
  }, [])

  useEffect(() => {
    refresh()
    const interval = setInterval(refresh, getRefreshInterval())
    return () => clearInterval(interval)
  }, [refresh])

  return { cameras, loading, refresh }
}
