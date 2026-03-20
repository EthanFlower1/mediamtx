import { useRef, useEffect } from 'react'
import { Camera } from '../hooks/useCameras'
import PTZControls from './PTZControls'

interface Props {
  camera: Camera
  onSelect?: () => void
  expanded?: boolean
}

export default function PlayerCell({ camera, onSelect, expanded }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)

  useEffect(() => {
    const video = videoRef.current
    if (!video || !camera.mediamtx_path) return

    let pc: RTCPeerConnection | null = null
    let cancelled = false

    const start = async () => {
      pc = new RTCPeerConnection({
        iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      })
      pcRef.current = pc

      pc.addTransceiver('video', { direction: 'recvonly' })
      pc.addTransceiver('audio', { direction: 'recvonly' })

      pc.ontrack = (evt) => {
        if (video.srcObject !== evt.streams[0]) {
          video.srcObject = evt.streams[0]
        }
      }

      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)

      // Wait for ICE gathering to complete (or timeout).
      await new Promise<void>((resolve) => {
        if (pc!.iceGatheringState === 'complete') {
          resolve()
          return
        }
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

      // Send offer to MediaMTX WHEP endpoint (WebRTC server on port 8889).
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
      if (pc) {
        pc.close()
        pcRef.current = null
      }
      if (video.srcObject) {
        video.srcObject = null
      }
    }
  }, [camera.mediamtx_path])

  return (
    <div
      onClick={onSelect}
      className="relative bg-black aspect-video cursor-pointer rounded-lg overflow-hidden group hover:ring-2 ring-nvr-accent/50 transition-all"
    >
      <video ref={videoRef} autoPlay muted playsInline className="w-full h-full object-contain" />
      <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent px-3 py-2 flex items-center">
        <span className="text-xs font-medium text-white">{camera.name}</span>
        <span
          className={`ml-2 w-1.5 h-1.5 rounded-full inline-block ${
            camera.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'
          }`}
        />
        {/* PTZ badge in grid view */}
        {!expanded && camera.ptz_capable && (
          <span className="ml-auto text-[10px] font-semibold tracking-wide text-white/70 bg-white/10 rounded px-1.5 py-0.5 uppercase">
            PTZ
          </span>
        )}
      </div>
      {/* PTZ overlay in expanded single-camera view */}
      {expanded && camera.ptz_capable && (
        <PTZControls cameraId={camera.id} />
      )}
    </div>
  )
}
