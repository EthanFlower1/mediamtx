import { useState } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import CameraGrid from '../components/CameraGrid'

export default function LiveView() {
  const { cameras, loading } = useCameras()
  const [layout, setLayout] = useState(2)
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

  if (loading) return <div>Loading cameras...</div>
  if (cameras.length === 0) return <div>No cameras configured. Go to Camera Management to add cameras.</div>

  if (selectedCamera) {
    return (
      <div>
        <button onClick={() => setSelectedCamera(null)}>Back to Grid</button>
        <h2>{selectedCamera.name}</h2>
        <div style={{ maxWidth: '100%', aspectRatio: '16/9' }}>
          <video autoPlay muted playsInline style={{ width: '100%', height: '100%' }} />
        </div>
      </div>
    )
  }

  return (
    <div>
      <div style={{ marginBottom: 8 }}>
        <span>Layout: </span>
        {[1, 2, 3, 4].map(n => (
          <button key={n} onClick={() => setLayout(n)}
            style={{ fontWeight: layout === n ? 'bold' : 'normal', marginRight: 4 }}>
            {n}x{n}
          </button>
        ))}
      </div>
      <CameraGrid cameras={cameras} layout={layout} onSelectCamera={setSelectedCamera} />
    </div>
  )
}
