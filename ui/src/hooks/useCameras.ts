import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

export interface Camera {
  id: string
  name: string
  rtsp_url: string
  mediamtx_path: string
  status: string
  ptz_capable: boolean
  onvif_endpoint?: string
}

export function useCameras() {
  const [cameras, setCameras] = useState<Camera[]>([])
  const [loading, setLoading] = useState(true)

  const refresh = async () => {
    const res = await apiFetch('/cameras')
    if (res.ok) setCameras(await res.json())
    setLoading(false)
  }

  useEffect(() => { refresh() }, [])

  return { cameras, loading, refresh }
}
