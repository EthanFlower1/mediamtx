import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

interface AudioCaps {
  has_backchannel: boolean
  audio_sources: number
  audio_outputs: number
}

interface Props {
  cameraId: string
}

export default function AudioIntercom({ cameraId }: Props) {
  const [caps, setCaps] = useState<AudioCaps | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    apiFetch(`/cameras/${cameraId}/audio/capabilities`)
      .then(res => res.ok ? res.json() : null)
      .then(data => {
        if (data) setCaps(data)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [cameraId])

  if (loading) {
    return (
      <div className="bg-nvr-bg-secondary rounded-lg p-3">
        <div className="text-xs text-nvr-text-muted animate-pulse">Checking audio capabilities...</div>
      </div>
    )
  }

  if (!caps || !caps.has_backchannel) {
    return (
      <div className="bg-nvr-bg-secondary rounded-lg p-3">
        <div className="flex items-center gap-2">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4 text-nvr-text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <line x1="1" y1="1" x2="23" y2="23" />
            <path d="M9 9v3a3 3 0 005.12 2.12M15 9.34V4a3 3 0 00-5.94-.6" />
            <path d="M17 16.95A7 7 0 015 12v-2m14 0v2c0 .76-.12 1.49-.34 2.18" />
            <line x1="12" y1="19" x2="12" y2="23" />
            <line x1="8" y1="23" x2="16" y2="23" />
          </svg>
          <span className="text-xs text-nvr-text-muted">Two-way audio not supported</span>
        </div>
      </div>
    )
  }

  return (
    <div className="bg-nvr-bg-secondary rounded-lg p-3">
      <div className="flex items-center gap-3">
        <div className="flex items-center justify-center w-10 h-10 rounded-full bg-nvr-accent/20 text-nvr-accent">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 1a3 3 0 00-3 3v8a3 3 0 006 0V4a3 3 0 00-3-3z" />
            <path d="M19 10v2a7 7 0 01-14 0v-2" />
            <line x1="12" y1="19" x2="12" y2="23" />
            <line x1="8" y1="23" x2="16" y2="23" />
          </svg>
        </div>
        <div className="flex flex-col">
          <span className="text-sm font-medium text-nvr-text-primary">Two-Way Audio Available</span>
          <span className="text-xs text-nvr-text-muted">
            {caps.audio_sources > 0 ? 'Mic + Speaker' : 'Speaker only'}
          </span>
        </div>
      </div>
    </div>
  )
}
