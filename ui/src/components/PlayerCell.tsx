import { useRef, useEffect } from 'react'
import { Camera } from '../hooks/useCameras'

interface Props {
  camera: Camera
  onSelect?: () => void
}

export default function PlayerCell({ camera, onSelect }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null)

  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.src = `/${camera.mediamtx_path}`
    }
  }, [camera.mediamtx_path])

  return (
    <div onClick={onSelect} style={{
      position: 'relative',
      background: '#000',
      aspectRatio: '16/9',
      cursor: 'pointer',
    }}>
      <video ref={videoRef} autoPlay muted playsInline style={{ width: '100%', height: '100%', objectFit: 'contain' }} />
      <div style={{
        position: 'absolute', bottom: 4, left: 4,
        background: 'rgba(0,0,0,0.6)', color: '#fff',
        padding: '2px 8px', fontSize: 12, borderRadius: 4,
      }}>
        {camera.name}
        <span style={{ marginLeft: 8, color: camera.status === 'online' ? '#4f4' : '#f44' }}>
          {camera.status}
        </span>
      </div>
    </div>
  )
}
