import { useState, useEffect, useCallback, useRef } from 'react'
import { apiFetch } from '../api/client'

interface PTZPreset {
  token: string
  name: string
}

interface PTZNodeCaps {
  token: string
  name: string
  max_presets: number
  home_supported: boolean
}

interface Props {
  cameraId: string
}

export default function PTZControls({ cameraId }: Props) {
  const [presets, setPresets] = useState<PTZPreset[]>([])
  const [loadingPresets, setLoadingPresets] = useState(false)
  const [capabilities, setCapabilities] = useState<PTZNodeCaps | null>(null)
  const activeRef = useRef(false)

  useEffect(() => {
    setLoadingPresets(true)
    apiFetch(`/cameras/${cameraId}/ptz/presets`)
      .then(res => res.ok ? res.json() : null)
      .then(data => {
        if (data?.presets) setPresets(data.presets)
      })
      .catch(() => {})
      .finally(() => setLoadingPresets(false))
  }, [cameraId])

  useEffect(() => {
    apiFetch(`/cameras/${cameraId}/ptz/capabilities`)
      .then(res => res.ok ? res.json() : null)
      .then(data => {
        if (data?.nodes && data.nodes.length > 0) {
          setCapabilities(data.nodes[0])
        }
      })
      .catch(() => {})
  }, [cameraId])

  const homeSupported = capabilities === null || capabilities.home_supported

  const sendMove = useCallback((pan: number, tilt: number, zoom: number) => {
    activeRef.current = true
    apiFetch(`/cameras/${cameraId}/ptz`, {
      method: 'POST',
      body: JSON.stringify({ action: 'move', pan, tilt, zoom }),
    }).catch(() => {})
  }, [cameraId])

  const sendStop = useCallback(() => {
    if (!activeRef.current) return
    activeRef.current = false
    apiFetch(`/cameras/${cameraId}/ptz`, {
      method: 'POST',
      body: JSON.stringify({ action: 'stop' }),
    }).catch(() => {})
  }, [cameraId])

  const sendHome = useCallback(() => {
    apiFetch(`/cameras/${cameraId}/ptz`, {
      method: 'POST',
      body: JSON.stringify({ action: 'home' }),
    }).catch(() => {})
  }, [cameraId])

  const sendPreset = useCallback((token: string) => {
    apiFetch(`/cameras/${cameraId}/ptz`, {
      method: 'POST',
      body: JSON.stringify({ action: 'preset', preset_token: token }),
    }).catch(() => {})
  }, [cameraId])

  // Common press/release handlers for continuous move buttons.
  const dirHandlers = (pan: number, tilt: number, zoom: number) => ({
    onMouseDown: (e: React.MouseEvent) => { e.preventDefault(); sendMove(pan, tilt, zoom) },
    onMouseUp: sendStop,
    onMouseLeave: sendStop,
    onTouchStart: (e: React.TouchEvent) => { e.preventDefault(); sendMove(pan, tilt, zoom) },
    onTouchEnd: sendStop,
    onTouchCancel: sendStop,
  })

  const btnBase =
    'flex items-center justify-center w-11 h-11 sm:w-10 sm:h-10 rounded-lg bg-white/10 hover:bg-white/25 active:bg-white/35 text-white transition-colors select-none touch-none'

  return (
    <div
      className="absolute inset-0 pointer-events-none flex flex-col items-end justify-end p-3 gap-2"
      onClick={(e) => e.stopPropagation()}
    >
      {/* Directional pad */}
      <div className="pointer-events-auto bg-black/50 backdrop-blur-sm rounded-xl p-2 flex flex-col items-center gap-1">
        {/* Up */}
        <div className="flex justify-center">
          <button className={btnBase} {...dirHandlers(0, 0.5, 0)} aria-label="Tilt up">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
        {/* Left / Home / Right */}
        <div className="flex gap-1">
          <button className={btnBase} {...dirHandlers(-0.5, 0, 0)} aria-label="Pan left">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M12.707 5.293a1 1 0 010 1.414L9.414 10l3.293 3.293a1 1 0 01-1.414 1.414l-4-4a1 1 0 010-1.414l4-4a1 1 0 011.414 0z" clipRule="evenodd" />
            </svg>
          </button>
          {homeSupported && (
            <button
              className={`${btnBase} bg-white/15`}
              onClick={(e) => { e.stopPropagation(); sendHome() }}
              aria-label="Home position"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
                <path d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z" />
              </svg>
            </button>
          )}
          <button className={btnBase} {...dirHandlers(0.5, 0, 0)} aria-label="Pan right">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
        {/* Down */}
        <div className="flex justify-center">
          <button className={btnBase} {...dirHandlers(0, -0.5, 0)} aria-label="Tilt down">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
        {/* Zoom controls */}
        <div className="flex gap-1 mt-1 border-t border-white/10 pt-2">
          <button className={btnBase} {...dirHandlers(0, 0, -0.5)} aria-label="Zoom out">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z" clipRule="evenodd" />
              <path fillRule="evenodd" d="M5 8a1 1 0 011-1h4a1 1 0 110 2H6a1 1 0 01-1-1z" clipRule="evenodd" />
            </svg>
          </button>
          <button className={btnBase} {...dirHandlers(0, 0, 0.5)} aria-label="Zoom in">
            <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M8 4a4 4 0 100 8 4 4 0 000-8zM2 8a6 6 0 1110.89 3.476l4.817 4.817a1 1 0 01-1.414 1.414l-4.816-4.816A6 6 0 012 8z" clipRule="evenodd" />
              <path fillRule="evenodd" d="M8 5a1 1 0 011 1v1h1a1 1 0 110 2H9v1a1 1 0 11-2 0V9H6a1 1 0 010-2h1V6a1 1 0 011-1z" clipRule="evenodd" />
            </svg>
          </button>
        </div>
      </div>

      {/* Presets */}
      {!loadingPresets && presets.length > 0 && (
        <div className="pointer-events-auto bg-black/50 backdrop-blur-sm rounded-xl p-2 flex flex-wrap gap-1 max-w-[200px]">
          <span className="w-full text-[10px] uppercase tracking-wider text-white/60 px-1 mb-0.5">
            Presets{capabilities && capabilities.max_presets > 0 ? ` (max ${capabilities.max_presets})` : ''}
          </span>
          {presets.map(p => (
            <button
              key={p.token}
              onClick={(e) => { e.stopPropagation(); sendPreset(p.token) }}
              className="px-2.5 py-1 rounded-md bg-white/10 hover:bg-white/25 active:bg-white/35 text-white text-xs font-medium transition-colors"
              title={p.name || `Preset ${p.token}`}
            >
              {p.name || p.token}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
