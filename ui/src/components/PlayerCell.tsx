import { useRef, useEffect, useState, useCallback } from 'react'
import { Camera } from '../hooks/useCameras'

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'failed'

interface Props {
  camera: Camera
  onSelect?: () => void
}

export default function PlayerCell({ camera, onSelect }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const cancelledRef = useRef(false)
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting')

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
        } else if (state === 'disconnected' || state === 'failed') {
          setConnectionState('disconnected')
          retryTimerRef.current = setTimeout(() => {
            if (!cancelledRef.current) {
              cleanup()
              startConnection()
            }
          }, 5000)
        }
      }

      const offer = await pc.createOffer()
      await pc.setLocalDescription(offer)

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

      const whepUrl = `${window.location.protocol}//${window.location.hostname}:8889/${camera.mediamtx_path}/whep`
      const res = await fetch(whepUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/sdp' },
        body: pc.localDescription!.sdp,
      })

      if (!res.ok || cancelledRef.current) {
        if (!cancelledRef.current) {
          setConnectionState('disconnected')
          retryTimerRef.current = setTimeout(() => {
            if (!cancelledRef.current) {
              cleanup()
              startConnection()
            }
          }, 5000)
        }
        return
      }

      const answer = await res.text()
      await pc.setRemoteDescription({ type: 'answer', sdp: answer })
    } catch {
      if (!cancelledRef.current) {
        setConnectionState('disconnected')
        retryTimerRef.current = setTimeout(() => {
          if (!cancelledRef.current) {
            cleanup()
            startConnection()
          }
        }, 5000)
      }
    }
  }, [camera.mediamtx_path, cleanup])

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
      className="relative bg-black aspect-video cursor-pointer rounded-lg overflow-hidden border border-nvr-border group hover:scale-[1.02] hover:ring-2 ring-nvr-accent/50 hover:brightness-105 transition-all duration-200"
    >
      <video
        ref={videoRef}
        autoPlay
        muted
        playsInline
        className={`w-full h-full object-contain transition-opacity duration-300 ${
          connectionState === 'connected' ? 'opacity-100' : 'opacity-0'
        }`}
      />

      {/* Connecting: centered spinner */}
      {connectionState === 'connecting' && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/70 z-10 transition-opacity duration-300">
          <div className="w-8 h-8 border-2 border-nvr-accent border-t-transparent rounded-full animate-spin" />
        </div>
      )}

      {/* Offline: gray overlay with badge, auto-retrying silently */}
      {connectionState === 'disconnected' && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/70 z-10 transition-opacity duration-300">
          <span className="bg-nvr-danger/20 text-nvr-danger text-xs font-semibold px-3 py-1 rounded-full">
            Offline
          </span>
        </div>
      )}

      {/* LIVE badge — top-right when connected */}
      {connectionState === 'connected' && camera.status === 'online' && (
        <div className="absolute top-2 right-2 flex items-center gap-1 bg-black/60 rounded px-1.5 py-0.5 z-10">
          <span className="w-1.5 h-1.5 rounded-full bg-red-500 animate-pulse" />
          <span className="text-[10px] text-white font-medium">LIVE</span>
        </div>
      )}

      {/* Bottom bar: name + status dot */}
      <div className="absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 to-transparent group-hover:from-black/90 px-3 py-2 flex items-center gap-2 transition-all duration-200">
        <span
          className={`w-2 h-2 rounded-full shrink-0 ${
            camera.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'
          }`}
        />
        <span className="text-xs font-medium text-white truncate">{camera.name}</span>
        {camera.ptz_capable && (
          <span className="ml-auto text-[10px] font-semibold tracking-wide text-white/70 bg-white/10 rounded px-1.5 py-0.5 uppercase">
            PTZ
          </span>
        )}
      </div>
    </div>
  )
}
