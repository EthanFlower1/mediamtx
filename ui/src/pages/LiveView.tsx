import { useState, useEffect, useRef } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import CameraGrid from '../components/CameraGrid'
import VideoPlayer from '../components/VideoPlayer'
import PTZControls from '../components/PTZControls'

/** Expanded single-camera view with VideoPlayer + PTZ overlay. */
function ExpandedCameraView({ camera, onBack }: { camera: Camera; onBack: () => void }) {
  const [stream, setStream] = useState<MediaStream | undefined>(undefined)
  const pcRef = useRef<RTCPeerConnection | null>(null)

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

      // Wait for ICE gathering to complete (or timeout).
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

  return (
    <div>
      <button
        onClick={onBack}
        className="mb-2 px-4 py-2 min-h-[44px] rounded-lg border border-nvr-border bg-nvr-bg-tertiary text-nvr-text-secondary hover:bg-nvr-bg-input hover:text-nvr-text-primary transition-colors text-sm md:text-base"
      >
        Back to Grid
      </button>
      <h2 className="mt-0 text-lg font-semibold text-nvr-text-primary">{camera.name}</h2>
      <div className="relative w-full">
        <VideoPlayer stream={stream} live />
        {camera.ptz_capable && <PTZControls cameraId={camera.id} />}
      </div>
    </div>
  )
}

export default function LiveView() {
  const { cameras, loading } = useCameras()
  const [layout, setLayout] = useState(2)
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

  if (loading) return <div className="flex items-center justify-center h-64 text-nvr-text-muted">Loading cameras...</div>
  if (cameras.length === 0) return <div className="flex items-center justify-center h-64 text-nvr-text-muted">No cameras configured. Go to Camera Management to add cameras.</div>

  if (selectedCamera) {
    return (
      <ExpandedCameraView
        camera={selectedCamera}
        onBack={() => setSelectedCamera(null)}
      />
    )
  }

  return (
    <div>
      <div className="mb-2 inline-flex rounded-lg border border-nvr-border overflow-hidden">
        <span className="flex items-center px-2 md:px-3 text-xs md:text-sm text-nvr-text-secondary">Layout</span>
        {[1, 2, 3, 4].map(n => (
          <button
            key={n}
            onClick={() => setLayout(n)}
            className={`px-2.5 md:px-3 py-1.5 text-xs md:text-sm font-medium border transition-colors min-h-[44px] ${
              layout === n
                ? 'bg-nvr-accent/20 border-nvr-accent text-nvr-accent'
                : 'bg-nvr-bg-input border-nvr-border text-nvr-text-secondary hover:border-nvr-text-muted'
            }`}
          >
            {n}x{n}
          </button>
        ))}
      </div>
      <CameraGrid cameras={cameras} layout={layout} onSelectCamera={setSelectedCamera} />
    </div>
  )
}
