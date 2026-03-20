import { useState } from 'react'
import { apiFetch } from '../api/client'

export interface TimeRange {
  start: string
  end: string
}

export function useTimeline(cameraId: string | null, date: string) {
  const [ranges, setRanges] = useState<TimeRange[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    if (!cameraId || !date) return
    setLoading(true)
    const res = await apiFetch(`/timeline?camera_id=${cameraId}&date=${date}`)
    if (res.ok) setRanges(await res.json())
    setLoading(false)
  }

  return { ranges, loading, load }
}
