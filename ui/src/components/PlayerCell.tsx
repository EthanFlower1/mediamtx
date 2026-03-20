import { useRef, useEffect, useState, useCallback } from 'react'
import { Camera } from '../hooks/useCameras'
import PTZControls from './PTZControls'

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'failed'

interface Props {
  camera: Camera
  onSelect?: () => void
  expanded?: boolean
}

export default function PlayerCell({ camera, onSelect, expanded }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const cancelledRef = useRef(false)
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')

  const startConnection = useCallback(async () => {
    const video = videoRef.current
    if (!video || !camera.mediamtx_path) return

    cancelledRef.current = false
    setConnectionState('connecting')

    try {
      const pc = new RTCPeerConnection({
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

      pc.onconnectionstatechange = () => {
        const state = pc.connectionState
        if (state === 'connected') {
          setConnectionState('connected')
        } else if (state === 'disconnected') {
          setConnectionState('disconnected')
          // Auto-retry after 5 seconds on disconnect
          retryTimerRef.current = setTimeout(() => {
            if (!cancelledRef.current) {
              cleanup()
              startConnection()
            }
          }, 5000)
        } else if (state === 'failed') {
          setConnectionState('failed')
        }
      }

      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)

      // Wait for ICE gathering to complete (or timeout).
      await new Promise<void>((resolve) => {
        if (pc.iceGatheringState === 'complete') {
          resolve()
          return
        }
        const check = () => {
          if (pc.iceGatheringState === 'complete') {
            pc.removeEventListener('icegatheringstatechange', check)
            resolve()
          }
        }
        pc.addEventListener('icegatheringstatechange', check)
        setTimeout(resolve, 2000)
      })

      if (cancelledRef.current) return

      // Send offer to MediaMTX WHEP endpoint (WebRTC server on port 8889).
      const whepUrl = `${window.location.protocol}//${window.location.hostname}:8889/${camera.mediamtx_path}/whep`
      const res = await fetch(whepUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/sdp' },
        body: pc.localDescription!.sdp,
      })

      if (!res.ok || cancelledRef.current) {
        if (!cancelledRef.current) setConnectionState('failed')
        return
      }

      const answer = await res.text()
      await pc.setRemoteDescription({ type: 'answer', sdp: answer })
    } catch {
      if (!cancelledRef.current) setConnectionState('failed')
    }
  }, [camera.mediamtx_path])

  const cleanup = useCallback(() => {
    if (retryTimerRef.current) {
      clearTimeout(retryTimerRef.current)
      retryTimerRef.current = null
    }
    const pc = pcRef.current
    if (pc) {
      pc.onconnectionstatechange = null
      pc.close()
      pcRef.current = null
    }
    const video = videoRef.current
    if (video && video.srcObject) {
      video.srcObject = null
    }
  }, [])

  const handleRetry = useCallback(() => {
    cleanup()
    startConnection()
  }, [cleanup, startConnection])

  useEffect(() => {
    startConnection()
    return () => {
      cancelledRef.current = true
      cleanup()
    }
  }, [startConnection, cleanup])

  return (
    <div
      onClick={onSelect}
      className="relative bg-black aspect-video cursor-pointer rounded-lg overflow-hidden group hover:ring-2 ring-nvr-accent/50 transition-all"
    >
      <video ref={videoRef} autoPlay muted playsInline className="w-full h-full object-contain" />

      {/* Connecting spinner overlay */}
      {connectionState === 'connecting' && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/70 z-10">
          <div className="w-8 h-8 border-2 border-nvr-accent border-t-transparent rounded-full animate-spin mb-2" />
          <span className="text-xs text-white/80">Connecting...</span>
        </div>
      )}

      {/* Disconnected overlay — auto-reconnecting */}
      {connectionState === 'disconnected' && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/70 z-10">
          <div className="w-8 h-8 border-2 border-amber-400 border-t-transparent rounded-full animate-spin mb-2" />
          <span className="text-xs text-amber-400">Connection lost. Reconnecting...</span>
        </div>
      )}

      {/* Failed overlay */}
      {connectionState === 'failed' && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/80 z-10">
          <span className="text-sm text-nvr-danger mb-2">Offline</span>
          <button
            onClick={(e) => { e.stopPropagation(); handleRetry() }}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-xs min-h-[36px]"
          >
            Retry
          </button>
        </div>
      )}

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
