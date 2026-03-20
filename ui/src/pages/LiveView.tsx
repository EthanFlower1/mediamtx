import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import CameraGrid from '../components/CameraGrid'
import VideoPlayer from '../components/VideoPlayer'
import PTZControls from '../components/PTZControls'
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts'

/** Full-screen modal overlay for a single camera with video + PTZ. */
function CameraModal({ camera, onClose }: { camera: Camera; onClose: () => void }) {
  const [stream, setStream] = useState<MediaStream | undefined>(undefined)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const modalVideoRef = useRef<HTMLVideoElement | null>(null)

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handleKey)
    return () => document.removeEventListener('keydown', handleKey)
  }, [onClose])

  useEffect(() => {
    if (!camera.mediamtx_path) return

    let cancelled = false
    let pc: RTCPeerConnection | null = null

    const start = async () => {
      pc = new RTCPeerConnection({
        iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      })
      pcRef.current = pc

      pc.addTransceiver('video', { direction: 'recvonly' })
      pc.addTransceiver('audio', { direction: 'recvonly' })

      pc.ontrack = (evt) => {
        if (!cancelled) setStream(evt.streams[0])
      }

      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)

      await new Promise<void>((resolve) => {
        if (pc!.iceGatheringState === 'complete') { resolve(); return }
        const check = () => {
          if (pc!.iceGatheringState === 'complete') {
            pc!.removeEventListener('icegatheringstatechange', check)
            resolve()
          }
        }
        pc!.addEventListener('icegatheringstatechange', check)
        setTimeout(resolve, 2000)
      })

      if (cancelled) return

      const whepUrl = `${window.location.protocol}//${window.location.hostname}:8889/${camera.mediamtx_path}/whep`
      const res = await fetch(whepUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/sdp' },
        body: pc.localDescription!.sdp,
      })

      if (!res.ok || cancelled) return

      const answer = await res.text()
      await pc.setRemoteDescription({ type: 'answer', sdp: answer })
    }

    start().catch(() => {})

    return () => {
      cancelled = true
      if (pc) { pc.close(); pcRef.current = null }
      setStream(undefined)
    }
  }, [camera.mediamtx_path])

  const handleRetry = useCallback(() => {
    const pc = pcRef.current
    if (pc) { pc.close(); pcRef.current = null }
    setStream(undefined)
  }, [])

  const handleScreenshot = useCallback(() => {
    const video = modalVideoRef.current
    if (!video) return
    const canvas = document.createElement('canvas')
    canvas.width = video.videoWidth
    canvas.height = video.videoHeight
    canvas.getContext('2d')!.drawImage(video, 0, 0)
    const link = document.createElement('a')
    link.download = `snapshot_${camera.name.replace(/\s+/g, '_')}_${new Date().toISOString().replace(/[:.]/g, '-')}.png`
    link.href = canvas.toDataURL('image/png')
    link.click()
  }, [camera.name])

  const handleVideoRef = useCallback((el: HTMLVideoElement | null) => {
    modalVideoRef.current = el
  }, [])

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/80 backdrop-blur-sm" />
      <div
        className="relative z-10 w-full max-w-5xl mx-4"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Close button */}
        <button
          onClick={onClose}
          className="absolute -top-10 right-0 text-white/70 hover:text-white transition-colors text-sm font-medium flex items-center gap-1.5 min-h-[44px] min-w-[44px] justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded-lg"
          aria-label="Close"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
          </svg>
          <span className="hidden sm:inline">Close</span>
          <span className="hidden sm:inline text-nvr-text-muted text-xs ml-1">Press Esc to close</span>
        </button>

        {/* Camera name, status, and screenshot button */}
        <div className="flex items-center gap-3 mb-3">
          <h2 className="text-lg font-semibold text-white">{camera.name}</h2>
          <span className={`flex items-center gap-1.5 text-xs font-medium ${
            camera.status === 'online' ? 'text-nvr-success' : 'text-nvr-danger'
          }`}>
            <span className={`w-2 h-2 rounded-full ${
              camera.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'
            }`} />
            {camera.status === 'online' ? 'Online' : 'Offline'}
          </span>
          <div className="flex-1" />
          <button
            onClick={handleScreenshot}
            className="flex items-center gap-1.5 text-white/70 hover:text-white bg-white/10 hover:bg-white/20 rounded-lg px-3 py-1.5 transition-colors text-xs font-medium"
            title="Save screenshot"
          >
            <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
              <path d="M23 19a2 2 0 01-2 2H3a2 2 0 01-2-2V8a2 2 0 012-2h4l2-3h6l2 3h4a2 2 0 012 2z" />
              <circle cx="12" cy="13" r="4" />
            </svg>
            Screenshot
          </button>
        </div>

        {/* Video player with PTZ overlay */}
        <div className="relative rounded-lg overflow-hidden">
          <VideoPlayer stream={stream} live onRetry={handleRetry} onVideoRef={handleVideoRef} />
          {camera.ptz_capable && <PTZControls cameraId={camera.id} />}
        </div>
      </div>
    </div>
  )
}

const gridColsMap: Record<number, string> = {
  1: 'grid-cols-1',
  2: 'grid-cols-1 sm:grid-cols-2',
  3: 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3',
  4: 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4',
}

export default function LiveView() {
  const { cameras, loading } = useCameras()
  const [layout, setLayout] = useState(() => {
    const saved = localStorage.getItem('nvr-live-layout')
    return saved ? parseInt(saved, 10) : 2
  })
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

  // Page title
  useEffect(() => {
    document.title = 'Live View — MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  const onlineCount = cameras.filter(c => c.status === 'online').length
  const offlineCount = cameras.length - onlineCount

  const handleLayoutChange = (n: number) => {
    setLayout(n)
    localStorage.setItem('nvr-live-layout', String(n))
  }

  // Keyboard shortcuts: 1-4 for layout, F for fullscreen
  const shortcuts = useMemo(() => [
    { key: '1', handler: () => handleLayoutChange(1), description: '1x1 layout' },
    { key: '2', handler: () => handleLayoutChange(2), description: '2x2 layout' },
    { key: '3', handler: () => handleLayoutChange(3), description: '3x3 layout' },
    { key: '4', handler: () => handleLayoutChange(4), description: '4x4 layout' },
    {
      key: 'f',
      handler: () => {
        if (selectedCamera) {
          if (document.fullscreenElement) {
            document.exitFullscreen()
          } else {
            document.documentElement.requestFullscreen().catch(() => {})
          }
        }
      },
      description: 'Toggle fullscreen',
    },
  ], [selectedCamera])
  useKeyboardShortcuts(shortcuts)

  if (loading) {
    return (
      <div>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Live View</h1>
          </div>
          <div className="flex rounded-lg overflow-hidden border border-nvr-border">
            {[1, 2, 3, 4].map(n => (
              <button
                key={n}
                disabled
                className={`px-3 py-1.5 text-xs font-medium transition-colors min-h-[36px] opacity-50 pointer-events-none ${
                  layout === n
                    ? 'bg-nvr-accent text-white'
                    : 'bg-nvr-bg-tertiary text-nvr-text-secondary'
                }`}
              >
                {n}x{n}
              </button>
            ))}
          </div>
        </div>
        <div className={`grid ${gridColsMap[layout]} gap-2 w-full`}>
          {Array.from({ length: layout * layout }, (_, i) => (
            <div key={i} className="aspect-video bg-nvr-bg-secondary rounded-lg animate-pulse" />
          ))}
        </div>
      </div>
    )
  }

  if (cameras.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-96 text-center px-4">
        <svg xmlns="http://www.w3.org/2000/svg" className="w-16 h-16 text-nvr-text-muted mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" />
        </svg>
        <h2 className="text-xl font-semibold text-nvr-text-primary mb-2">No cameras yet</h2>
        <p className="text-sm text-nvr-text-muted mb-6 max-w-md">
          Add cameras to start viewing live feeds. You can discover ONVIF cameras on your network or add them manually.
        </p>
        <a
          href="/cameras"
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-5 py-2.5 rounded-lg transition-colors text-sm"
        >
          Add your first camera
        </a>
      </div>
    )
  }

  return (
    <div>
      {/* Header: title + count badge + layout selector */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Live View</h1>
          <span className="bg-nvr-bg-tertiary text-nvr-text-secondary text-xs font-medium px-2.5 py-1 rounded-full">
            {cameras.length} camera{cameras.length !== 1 ? 's' : ''}
          </span>
        </div>

        {/* Layout pill buttons */}
        <div className="flex rounded-lg overflow-hidden border border-nvr-border">
          {[1, 2, 3, 4].map(n => (
            <button
              key={n}
              onClick={() => handleLayoutChange(n)}
              aria-label={`${n}x${n} grid layout`}
              className={`px-3 py-1.5 text-xs font-medium transition-colors min-h-[36px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                layout === n
                  ? 'bg-nvr-accent text-white'
                  : 'bg-nvr-bg-tertiary text-nvr-text-secondary hover:bg-nvr-bg-input hover:text-nvr-text-primary'
              }`}
            >
              {n}x{n}
            </button>
          ))}
        </div>
      </div>

      {/* Dashboard stats bar */}
      {cameras.length > 1 && (
        <div className="flex items-center gap-4 text-sm text-nvr-text-secondary mb-4">
          <span>{cameras.length} cameras</span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-nvr-success" />
            {onlineCount} online
          </span>
          <span className="flex items-center gap-1">
            <span className="w-2 h-2 rounded-full bg-nvr-danger" />
            {offlineCount} offline
          </span>
        </div>
      )}

      <CameraGrid cameras={cameras} layout={layout} onSelectCamera={setSelectedCamera} />

      {/* Camera modal overlay */}
      {selectedCamera && (
        <CameraModal
          camera={selectedCamera}
          onClose={() => setSelectedCamera(null)}
        />
      )}
    </div>
  )
}
