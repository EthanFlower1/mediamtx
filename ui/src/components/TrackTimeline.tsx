import { useState, useEffect, useMemo, useCallback } from 'react'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface Sighting {
  id: number
  track_id: number
  camera_id: string
  camera_name: string
  timestamp: string
  end_time?: string
  confidence: number
  thumbnail?: string
}

interface Track {
  id: number
  label: string
  status: string
  created_at: string
  updated_at: string
  detection_id: number
  sightings: Sighting[]
  camera_count: number
}

interface TrackTimelineProps {
  trackId?: number
  detectionId?: number
  onPlayback?: (cameraId: string, timestamp: string) => void
}

/* ------------------------------------------------------------------ */
/*  Confidence badge                                                    */
/* ------------------------------------------------------------------ */

function ConfidenceBadge({ value }: { value: number }) {
  const pct = Math.round(value * 100)
  let color = 'bg-red-500/20 text-red-400'
  if (pct >= 90) color = 'bg-green-500/20 text-green-400'
  else if (pct >= 75) color = 'bg-yellow-500/20 text-yellow-400'

  return (
    <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${color}`}>
      {pct}%
    </span>
  )
}

/* ------------------------------------------------------------------ */
/*  Beta badge                                                          */
/* ------------------------------------------------------------------ */

function BetaBadge() {
  return (
    <span className="inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold bg-purple-500/20 text-purple-400 border border-purple-500/30">
      BETA
    </span>
  )
}

/* ------------------------------------------------------------------ */
/*  Camera lane with sighting markers                                  */
/* ------------------------------------------------------------------ */

interface CameraLaneProps {
  cameraId: string
  cameraName: string
  sightings: Sighting[]
  timeRange: { start: number; end: number }
  onSightingClick: (sighting: Sighting) => void
}

function CameraLane({ cameraName, sightings, timeRange, onSightingClick }: CameraLaneProps) {
  const range = timeRange.end - timeRange.start || 1

  return (
    <div className="flex items-center gap-3 py-2 border-b border-nvr-border/30 last:border-b-0">
      <div className="w-32 shrink-0 text-sm text-nvr-text-secondary truncate" title={cameraName}>
        {cameraName}
      </div>
      <div className="flex-1 relative h-8 bg-nvr-bg-tertiary rounded overflow-hidden">
        {sightings.map((s) => {
          const t = new Date(s.timestamp).getTime()
          const left = ((t - timeRange.start) / range) * 100
          const width = Math.max(2, 3) // minimum visible width

          return (
            <button
              key={s.id}
              className="absolute top-1 bottom-1 rounded cursor-pointer bg-nvr-accent hover:bg-nvr-accent/80 transition-colors group"
              style={{ left: `${Math.min(left, 98)}%`, width: `${width}%` }}
              onClick={() => onSightingClick(s)}
              title={`${new Date(s.timestamp).toLocaleTimeString()} - ${Math.round(s.confidence * 100)}% confidence`}
            >
              <span className="absolute -top-6 left-1/2 -translate-x-1/2 hidden group-hover:block text-xs text-nvr-text-primary bg-nvr-bg-secondary px-1 py-0.5 rounded shadow whitespace-nowrap">
                {new Date(s.timestamp).toLocaleTimeString()}
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Transition arrow between cameras                                   */
/* ------------------------------------------------------------------ */

interface TransitionProps {
  from: Sighting
  to: Sighting
}

function CameraTransition({ from, to }: TransitionProps) {
  const fromTime = new Date(from.timestamp)
  const toTime = new Date(to.timestamp)
  const diffSec = Math.round((toTime.getTime() - fromTime.getTime()) / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffStr = diffMin > 0 ? `${diffMin}m ${diffSec % 60}s` : `${diffSec}s`

  return (
    <div className="flex items-center gap-2 px-4 py-1 text-xs text-nvr-text-tertiary">
      <span>{from.camera_name}</span>
      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 7l5 5m0 0l-5 5m5-5H6" />
      </svg>
      <span>{to.camera_name}</span>
      <span className="ml-1 text-nvr-text-tertiary/60">({diffStr})</span>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main TrackTimeline component                                       */
/* ------------------------------------------------------------------ */

export default function TrackTimeline({ trackId, detectionId, onPlayback }: TrackTimelineProps) {
  const [track, setTrack] = useState<Track | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)

  // Start tracking from a detection.
  const startTracking = useCallback(async () => {
    if (!detectionId) return
    setStarting(true)
    setError(null)
    try {
      const res = await apiFetch(`/detections/${detectionId}/track`, { method: 'POST' })
      if (!res.ok) {
        const data = await res.json()
        throw new Error(data.error || 'Failed to start tracking')
      }
      const data = await res.json()
      setTrack(data.track)
    } catch (err: any) {
      setError(err.message)
    } finally {
      setStarting(false)
    }
  }, [detectionId])

  // Fetch existing track by ID.
  useEffect(() => {
    if (!trackId) return
    setLoading(true)
    setError(null)

    apiFetch(`/tracks/${trackId}`)
      .then(async (res) => {
        if (!res.ok) {
          const data = await res.json()
          throw new Error(data.error || 'Failed to load track')
        }
        const data = await res.json()
        setTrack(data.track)
      })
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false))
  }, [trackId])

  // Compute time range from sightings.
  const timeRange = useMemo(() => {
    if (!track?.sightings?.length) return { start: 0, end: 0 }
    const times = track.sightings.map((s) => new Date(s.timestamp).getTime())
    const padding = 60000 // 1 min padding
    return {
      start: Math.min(...times) - padding,
      end: Math.max(...times) + padding,
    }
  }, [track])

  // Group sightings by camera.
  const cameraLanes = useMemo(() => {
    if (!track?.sightings?.length) return []
    const map = new Map<string, { cameraName: string; sightings: Sighting[] }>()
    for (const s of track.sightings) {
      if (!map.has(s.camera_id)) {
        map.set(s.camera_id, { cameraName: s.camera_name, sightings: [] })
      }
      map.get(s.camera_id)!.sightings.push(s)
    }
    return Array.from(map.entries()).map(([cameraId, data]) => ({
      cameraId,
      ...data,
    }))
  }, [track])

  // Build camera transitions (sorted sightings, transitions between different cameras).
  const transitions = useMemo(() => {
    if (!track?.sightings?.length) return []
    const sorted = [...track.sightings].sort(
      (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
    )
    const result: TransitionProps[] = []
    for (let i = 1; i < sorted.length; i++) {
      if (sorted[i].camera_id !== sorted[i - 1].camera_id) {
        result.push({ from: sorted[i - 1], to: sorted[i] })
      }
    }
    return result
  }, [track])

  const handleSightingClick = (sighting: Sighting) => {
    onPlayback?.(sighting.camera_id, sighting.timestamp)
  }

  /* -- Render: no track yet, show start button -- */
  if (!trackId && detectionId && !track) {
    return (
      <div className="rounded-lg bg-nvr-bg-secondary border border-nvr-border p-4">
        <div className="flex items-center gap-2 mb-3">
          <h3 className="text-sm font-medium text-nvr-text-primary">Cross-Camera Tracking</h3>
          <BetaBadge />
        </div>
        <p className="text-sm text-nvr-text-secondary mb-3">
          Track this person across all cameras using visual re-identification.
        </p>
        {error && (
          <div className="text-sm text-red-400 mb-2">{error}</div>
        )}
        <button
          onClick={startTracking}
          disabled={starting}
          className="px-4 py-2 bg-nvr-accent text-white text-sm rounded hover:bg-nvr-accent/80 disabled:opacity-50 transition-colors"
        >
          {starting ? 'Analyzing...' : 'Track This Person'}
        </button>
      </div>
    )
  }

  /* -- Render: loading -- */
  if (loading) {
    return (
      <div className="rounded-lg bg-nvr-bg-secondary border border-nvr-border p-4">
        <div className="flex items-center gap-2">
          <svg className="w-4 h-4 animate-spin text-nvr-accent" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
          </svg>
          <span className="text-sm text-nvr-text-secondary">Loading track...</span>
        </div>
      </div>
    )
  }

  /* -- Render: error -- */
  if (error && !track) {
    return (
      <div className="rounded-lg bg-nvr-bg-secondary border border-red-500/30 p-4">
        <div className="text-sm text-red-400">{error}</div>
      </div>
    )
  }

  /* -- Render: no track -- */
  if (!track) return null

  /* -- Render: full timeline -- */
  return (
    <div className="rounded-lg bg-nvr-bg-secondary border border-nvr-border p-4">
      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-medium text-nvr-text-primary">{track.label}</h3>
          <BetaBadge />
          <span className={`inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium ${
            track.status === 'active'
              ? 'bg-green-500/20 text-green-400'
              : 'bg-gray-500/20 text-gray-400'
          }`}>
            {track.status}
          </span>
        </div>
        <div className="text-xs text-nvr-text-tertiary">
          {track.camera_count} camera{track.camera_count !== 1 ? 's' : ''} &middot;{' '}
          {track.sightings?.length || 0} sighting{(track.sightings?.length || 0) !== 1 ? 's' : ''}
        </div>
      </div>

      {/* Timeline lanes */}
      <div className="mb-4">
        {cameraLanes.map((lane) => (
          <CameraLane
            key={lane.cameraId}
            cameraId={lane.cameraId}
            cameraName={lane.cameraName}
            sightings={lane.sightings}
            timeRange={timeRange}
            onSightingClick={handleSightingClick}
          />
        ))}
      </div>

      {/* Transitions */}
      {transitions.length > 0 && (
        <div className="border-t border-nvr-border/30 pt-3">
          <div className="text-xs font-medium text-nvr-text-secondary mb-2">Camera Transitions</div>
          {transitions.map((t, i) => (
            <CameraTransition key={i} from={t.from} to={t.to} />
          ))}
        </div>
      )}

      {/* Sighting detail list */}
      <div className="border-t border-nvr-border/30 pt-3 mt-3">
        <div className="text-xs font-medium text-nvr-text-secondary mb-2">Sighting Details</div>
        <div className="space-y-1">
          {(track.sightings || []).map((s) => (
            <button
              key={s.id}
              onClick={() => handleSightingClick(s)}
              className="w-full flex items-center justify-between px-2 py-1.5 rounded text-xs hover:bg-nvr-bg-tertiary transition-colors text-left"
            >
              <div className="flex items-center gap-2">
                <span className="text-nvr-text-primary font-medium">{s.camera_name}</span>
                <span className="text-nvr-text-tertiary">
                  {new Date(s.timestamp).toLocaleTimeString()}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <ConfidenceBadge value={s.confidence} />
                <svg className="w-3 h-3 text-nvr-text-tertiary" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  )
}
