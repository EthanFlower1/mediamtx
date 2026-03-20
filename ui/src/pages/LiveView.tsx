import { useState } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import CameraGrid from '../components/CameraGrid'
import PlayerCell from '../components/PlayerCell'

export default function LiveView() {
  const { cameras, loading } = useCameras()
  const [layout, setLayout] = useState(2)
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null)

  if (loading) return <div className="flex items-center justify-center h-64 text-nvr-text-muted">Loading cameras...</div>
  if (cameras.length === 0) return <div className="flex items-center justify-center h-64 text-nvr-text-muted">No cameras configured. Go to Camera Management to add cameras.</div>

  if (selectedCamera) {
    return (
      <div>
        <button
          onClick={() => setSelectedCamera(null)}
          className="mb-2 px-4 py-2 rounded-lg border border-nvr-border bg-nvr-bg-tertiary text-nvr-text-secondary hover:bg-nvr-bg-input hover:text-nvr-text-primary transition-colors"
        >
          Back to Grid
        </button>
        <h2 className="mt-0 text-lg font-semibold text-nvr-text-primary">{selectedCamera.name}</h2>
        <div className="max-w-full">
          <PlayerCell camera={selectedCamera} />
        </div>
      </div>
    )
  }

  return (
    <div>
      <div className="mb-2 inline-flex rounded-lg border border-nvr-border overflow-hidden">
        <span className="flex items-center px-3 text-sm text-nvr-text-secondary">Layout</span>
        {[1, 2, 3, 4].map(n => (
          <button
            key={n}
            onClick={() => setLayout(n)}
            className={`px-3 py-1.5 text-sm font-medium border transition-colors ${
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
