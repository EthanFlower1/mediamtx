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
    const interval = setInterval(refresh, 15000)
    return () => clearInterval(interval)
  }, [refresh])

  return { cameras, loading, refresh }
}
