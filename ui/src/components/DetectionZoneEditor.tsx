import { useState, useRef, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

interface Zone {
  name: string
  points: { x: number; y: number; w: number; h: number } // Normalized 0-1
}

interface Props {
  cameraId: string
  snapshotUrl?: string
  onSave?: () => void
}

interface AnalyticsRule {
  name: string
  type: string
  parameters: Record<string, string>
}

const ZONE_COLORS = [
  'rgba(59, 130, 246, 0.3)',   // blue
  'rgba(16, 185, 129, 0.3)',   // emerald
  'rgba(245, 158, 11, 0.3)',   // amber
  'rgba(239, 68, 68, 0.3)',    // red
  'rgba(139, 92, 246, 0.3)',   // violet
  'rgba(236, 72, 153, 0.3)',   // pink
]

const ZONE_BORDER_COLORS = [
  'rgba(59, 130, 246, 0.8)',
  'rgba(16, 185, 129, 0.8)',
  'rgba(245, 158, 11, 0.8)',
  'rgba(239, 68, 68, 0.8)',
  'rgba(139, 92, 246, 0.8)',
  'rgba(236, 72, 153, 0.8)',
]

export default function DetectionZoneEditor({ cameraId, snapshotUrl, onSave }: Props) {
  const [zones, setZones] = useState<Zone[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [notSupported, setNotSupported] = useState(false)
  const [addMode, setAddMode] = useState(false)
  const [dirty, setDirty] = useState(false)

  // Drawing state
  const [drawing, setDrawing] = useState(false)
  const [drawStart, setDrawStart] = useState<{ x: number; y: number } | null>(null)
  const [drawCurrent, setDrawCurrent] = useState<{ x: number; y: number } | null>(null)

  const containerRef = useRef<HTMLDivElement>(null)

  // Fetch existing rules from the API
  const fetchRules = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await apiFetch(`/cameras/${cameraId}/analytics/rules`)
      if (res.status === 404 || res.status === 501) {
        setNotSupported(true)
        setLoading(false)
        return
      }
      if (!res.ok) {
        // Analytics not supported or other error
        const data = await res.json().catch(() => ({}))
        if (data.error?.includes('analytics') || res.status === 400) {
          setNotSupported(true)
        } else {
          setError(data.error || 'Failed to load analytics rules')
        }
        setLoading(false)
        return
      }

      const rules: AnalyticsRule[] = await res.json()
      // Convert rules to zones — extract bounding box parameters
      const parsedZones: Zone[] = rules
        .filter(r => r.parameters)
        .map(r => {
          const x = parseFloat(r.parameters['ActiveCellsX'] || r.parameters['Left'] || r.parameters['x'] || '0')
          const y = parseFloat(r.parameters['ActiveCellsY'] || r.parameters['Top'] || r.parameters['y'] || '0')
          const w = parseFloat(r.parameters['ActiveCellsW'] || r.parameters['Right'] || r.parameters['w'] || '0.5')
          const h = parseFloat(r.parameters['ActiveCellsH'] || r.parameters['Bottom'] || r.parameters['h'] || '0.5')
          return {
            name: r.name,
            points: {
              x: Math.min(1, Math.max(0, x)),
              y: Math.min(1, Math.max(0, y)),
              w: Math.min(1, Math.max(0, w)),
              h: Math.min(1, Math.max(0, h)),
            },
          }
        })
      setZones(parsedZones)
      setDirty(false)
    } catch {
      setError('Failed to connect to analytics service')
    }
    setLoading(false)
  }, [cameraId])

  useEffect(() => {
    fetchRules()
  }, [fetchRules])

  // Convert mouse event to normalized coordinates (0-1)
  const toNormalized = (e: React.MouseEvent): { x: number; y: number } => {
    const rect = containerRef.current?.getBoundingClientRect()
    if (!rect) return { x: 0, y: 0 }
    return {
      x: Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)),
      y: Math.max(0, Math.min(1, (e.clientY - rect.top) / rect.height)),
    }
  }

  const handleMouseDown = (e: React.MouseEvent) => {
    if (!addMode) return
    e.preventDefault()
    const pos = toNormalized(e)
    setDrawing(true)
    setDrawStart(pos)
    setDrawCurrent(pos)
  }

  const handleMouseMove = (e: React.MouseEvent) => {
    if (!drawing) return
    e.preventDefault()
    setDrawCurrent(toNormalized(e))
  }

  const handleMouseUp = (e: React.MouseEvent) => {
    if (!drawing || !drawStart) return
    e.preventDefault()
    const end = toNormalized(e)

    const x = Math.min(drawStart.x, end.x)
    const y = Math.min(drawStart.y, end.y)
    const w = Math.abs(end.x - drawStart.x)
    const h = Math.abs(end.y - drawStart.y)

    // Ignore tiny accidental clicks (less than 2% of canvas)
    if (w > 0.02 && h > 0.02) {
      const newZone: Zone = {
        name: `Zone ${zones.length + 1}`,
        points: { x, y, w, h },
      }
      setZones(prev => [...prev, newZone])
      setDirty(true)
    }

    setDrawing(false)
    setDrawStart(null)
    setDrawCurrent(null)
    setAddMode(false)
  }

  const handleDeleteZone = (index: number) => {
    setZones(prev => prev.filter((_, i) => i !== index))
    setDirty(true)
  }

  const handleSave = async () => {
    setSaving(true)
    setError(null)
    try {
      // Send each zone as a rule. We POST each zone as a new rule.
      // The API expects individual rule creation.
      const res = await apiFetch(`/cameras/${cameraId}/analytics/rules`, {
        method: 'POST',
        body: JSON.stringify({
          name: 'MotionDetection',
          type: 'tt:CellMotionDetector',
          parameters: zones.reduce((acc, zone, i) => {
            acc[`Zone${i}_x`] = String(zone.points.x)
            acc[`Zone${i}_y`] = String(zone.points.y)
            acc[`Zone${i}_w`] = String(zone.points.w)
            acc[`Zone${i}_h`] = String(zone.points.h)
            acc[`Zone${i}_name`] = zone.name
            return acc
          }, {} as Record<string, string>),
        }),
      })

      if (res.ok) {
        setDirty(false)
        onSave?.()
      } else {
        const data = await res.json().catch(() => ({}))
        setError(data.error || 'Failed to save rules')
      }
    } catch {
      setError('Failed to save analytics rules')
    }
    setSaving(false)
  }

  // Compute the drawing preview rectangle
  const previewRect = drawing && drawStart && drawCurrent
    ? {
        x: Math.min(drawStart.x, drawCurrent.x),
        y: Math.min(drawStart.y, drawCurrent.y),
        w: Math.abs(drawCurrent.x - drawStart.x),
        h: Math.abs(drawCurrent.y - drawStart.y),
      }
    : null

  if (notSupported) {
    return (
      <div className="p-4 text-center border border-nvr-border rounded-lg bg-nvr-bg-tertiary">
        <p className="text-sm text-nvr-text-muted">
          Analytics not supported on this camera.
        </p>
        <p className="text-xs text-nvr-text-muted mt-1">
          The camera does not advertise an analytics service via ONVIF.
        </p>
      </div>
    )
  }

  if (loading) {
    return (
      <div className="p-4 text-center border border-nvr-border rounded-lg bg-nvr-bg-tertiary animate-pulse">
        <div className="h-4 bg-nvr-bg-input rounded w-1/3 mx-auto" />
      </div>
    )
  }

  return (
    <div className="border border-nvr-border rounded-lg bg-nvr-bg-tertiary overflow-hidden">
      {/* Step-by-step instructions */}
      <div className="bg-nvr-bg-tertiary/50 border-b border-nvr-border p-3">
        <h4 className="text-sm font-medium text-nvr-text-primary mb-1">How to set up motion zones</h4>
        <ol className="text-xs text-nvr-text-secondary space-y-1 list-decimal list-inside">
          <li>Click "Add Zone" to start drawing</li>
          <li>Click and drag on the image to draw a rectangle</li>
          <li>Give the zone a name and click Save</li>
          <li>The camera will only detect motion inside your zones</li>
        </ol>
      </div>

      {/* Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-nvr-border">
        <h4 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide">
          Motion Detection Zones
        </h4>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setAddMode(!addMode)}
            className={`text-xs font-medium px-2.5 py-1 rounded-md border transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
              addMode
                ? 'bg-nvr-accent/20 border-nvr-accent text-nvr-accent'
                : 'bg-nvr-bg-input border-nvr-border text-nvr-text-secondary hover:bg-nvr-border/30'
            }`}
          >
            {addMode ? 'Drawing...' : 'Add Zone'}
          </button>
          {dirty && (
            <button
              onClick={handleSave}
              disabled={saving}
              className="text-xs font-medium bg-nvr-accent hover:bg-nvr-accent-hover text-white px-3 py-1 rounded-md transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
          )}
        </div>
      </div>

      {/* Canvas area */}
      <div
        ref={containerRef}
        onMouseDown={handleMouseDown}
        onMouseMove={handleMouseMove}
        onMouseUp={handleMouseUp}
        onMouseLeave={() => {
          if (drawing) {
            setDrawing(false)
            setDrawStart(null)
            setDrawCurrent(null)
          }
        }}
        className={`relative w-full aspect-video select-none ${
          addMode ? 'cursor-crosshair' : 'cursor-default'
        }`}
        style={{
          background: snapshotUrl
            ? `url(${snapshotUrl}) center / cover no-repeat`
            : 'linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%)',
        }}
      >
        {/* Grid overlay for reference */}
        {addMode && (
          <div className="absolute inset-0 pointer-events-none" style={{
            backgroundImage: 'linear-gradient(rgba(255,255,255,0.05) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.05) 1px, transparent 1px)',
            backgroundSize: '10% 10%',
          }} />
        )}

        {/* Existing zones */}
        {zones.map((zone, i) => (
          <div
            key={i}
            className="absolute border-2 rounded-sm group/zone"
            style={{
              left: `${zone.points.x * 100}%`,
              top: `${zone.points.y * 100}%`,
              width: `${zone.points.w * 100}%`,
              height: `${zone.points.h * 100}%`,
              backgroundColor: ZONE_COLORS[i % ZONE_COLORS.length],
              borderColor: ZONE_BORDER_COLORS[i % ZONE_BORDER_COLORS.length],
            }}
          >
            {/* Zone label */}
            <div className="absolute -top-5 left-0 bg-black/70 text-white text-[10px] px-1.5 py-0.5 rounded whitespace-nowrap">
              {zone.name}
            </div>
            {/* Delete button */}
            <button
              onClick={(e) => {
                e.stopPropagation()
                handleDeleteZone(i)
              }}
              className="absolute -top-2 -right-2 w-5 h-5 bg-nvr-danger rounded-full text-white text-xs flex items-center justify-center opacity-0 group-hover/zone:opacity-100 transition-opacity hover:bg-nvr-danger-hover focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              title="Delete zone"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
              </svg>
            </button>
          </div>
        ))}

        {/* Drawing preview */}
        {previewRect && (
          <div
            className="absolute border-2 border-dashed border-white/70 rounded-sm pointer-events-none"
            style={{
              left: `${previewRect.x * 100}%`,
              top: `${previewRect.y * 100}%`,
              width: `${previewRect.w * 100}%`,
              height: `${previewRect.h * 100}%`,
              backgroundColor: 'rgba(255, 255, 255, 0.15)',
            }}
          />
        )}

        {/* Empty state */}
        {zones.length === 0 && !addMode && !drawing && (
          <div className="absolute inset-0 flex items-center justify-center">
            <p className="text-sm text-white/60 bg-black/40 px-3 py-1.5 rounded">
              No detection zones configured
            </p>
          </div>
        )}

        {/* Add mode instruction */}
        {addMode && !drawing && (
          <div className="absolute bottom-2 left-1/2 -translate-x-1/2">
            <p className="text-xs text-white/80 bg-black/60 px-3 py-1 rounded whitespace-nowrap">
              Click and drag to draw a detection zone
            </p>
          </div>
        )}
      </div>

      {/* Zone list */}
      {zones.length > 0 && (
        <div className="px-3 py-2 border-t border-nvr-border">
          <div className="flex flex-wrap gap-2">
            {zones.map((zone, i) => (
              <div
                key={i}
                className="flex items-center gap-1.5 bg-nvr-bg-input border border-nvr-border rounded px-2 py-1"
              >
                <div
                  className="w-2.5 h-2.5 rounded-sm shrink-0"
                  style={{ backgroundColor: ZONE_BORDER_COLORS[i % ZONE_BORDER_COLORS.length] }}
                />
                <span className="text-xs text-nvr-text-primary">{zone.name}</span>
                <button
                  onClick={() => handleDeleteZone(i)}
                  className="text-nvr-text-muted hover:text-nvr-danger ml-1 transition-colors"
                  title="Delete zone"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor">
                    <path fillRule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clipRule="evenodd" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Error message */}
      {error && (
        <div className="px-3 py-2 border-t border-nvr-border">
          <p className="text-xs text-nvr-danger">{error}</p>
        </div>
      )}
    </div>
  )
}
