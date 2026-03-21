import { useState, useCallback, useRef, useEffect } from 'react'
import { TimeRange } from '../hooks/useRecordings'

const ZOOM_LEVELS = [24, 12, 6, 3, 1] as const
type ZoomLevel = typeof ZOOM_LEVELS[number]

export interface MotionEvent {
  started_at: string
  ended_at?: string | null
}

interface Props {
  ranges: TimeRange[]
  date: string
  onSeek?: (time: Date) => void
  playbackTime?: Date | null

  // Motion events to display as markers
  events?: MotionEvent[]

  // Callback when a motion event marker on the timeline is clicked
  onEventClick?: (index: number) => void

  // Clip creation mode
  clipMode?: boolean
  clipStart?: Date | null
  clipEnd?: Date | null
  onClipStartChange?: (time: Date | null) => void
  onClipEndChange?: (time: Date | null) => void

  // Compact mode for multi-camera "All Cameras" view
  compact?: boolean
}

export default function Timeline({
  ranges,
  date,
  onSeek,
  playbackTime,
  events,
  onEventClick,
  clipMode,
  clipStart,
  clipEnd,
  onClipStartChange,
  onClipEndChange,
  compact,
}: Props) {
  const dayStart = new Date(date + 'T00:00:00')
  const dayMs = 24 * 60 * 60 * 1000
  const barRef = useRef<HTMLDivElement>(null)

  const [hoverX, setHoverX] = useState<number | null>(null)
  const [hoverTime, setHoverTime] = useState<string | null>(null)
  const [dragging, setDragging] = useState<'start' | 'end' | null>(null)
  const [hoveredEvent, setHoveredEvent] = useState<number | null>(null)

  // Zoom state
  const [zoomLevel, setZoomLevel] = useState<ZoomLevel>(24)
  const [viewStart, setViewStart] = useState(0) // start hour of visible window

  // Panning state
  const [isPanning, setIsPanning] = useState(false)
  const panStartXRef = useRef(0)
  const panStartViewRef = useRef(0)

  // Clamp viewStart so the window stays within 0..24
  const clampViewStart = useCallback((v: number, zoom: ZoomLevel) => {
    return Math.max(0, Math.min(24 - zoom, v))
  }, [])

  // Visible window in ms
  const viewStartMs = viewStart * 60 * 60 * 1000
  const viewEndMs = (viewStart + zoomLevel) * 60 * 60 * 1000

  // Convert a percentage of the visible window to an absolute Date
  const pctToTime = useCallback((pct: number) => {
    const ms = viewStartMs + pct * (viewEndMs - viewStartMs)
    return new Date(dayStart.getTime() + ms)
  }, [dayStart.getTime(), viewStartMs, viewEndMs])

  // Convert an absolute Date to percentage of the visible window
  const timeToPct = useCallback((time: Date) => {
    const ms = time.getTime() - dayStart.getTime()
    return ((ms - viewStartMs) / (viewEndMs - viewStartMs)) * 100
  }, [dayStart.getTime(), viewStartMs, viewEndMs])

  const pctFromEvent = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    return Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  }

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (dragging || isPanning) return

    const pct = pctFromEvent(e)
    const time = pctToTime(pct)

    if (clipMode) {
      // In clip mode: first click sets start, second click sets end
      if (!clipStart || (clipStart && clipEnd)) {
        // Starting fresh or resetting
        onClipStartChange?.(time)
        onClipEndChange?.(null)
      } else {
        // Have start, setting end — ensure start < end
        if (time.getTime() > clipStart.getTime()) {
          onClipEndChange?.(time)
        } else {
          // User clicked before start, swap
          onClipEndChange?.(clipStart)
          onClipStartChange?.(time)
        }
      }
    } else {
      onSeek?.(time)
    }
  }

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    // Handle panning
    if (isPanning && !compact) {
      const dx = e.clientX - panStartXRef.current
      const rect = e.currentTarget.getBoundingClientRect()
      const hoursPerPixel = zoomLevel / rect.width
      const newViewStart = panStartViewRef.current - dx * hoursPerPixel
      setViewStart(clampViewStart(newViewStart, zoomLevel))
      return
    }

    const rect = e.currentTarget.getBoundingClientRect()
    const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    setHoverX(e.clientX - rect.left)
    const t = pctToTime(pct)
    setHoverTime(t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }))

    // Handle dragging clip handles
    if (dragging && clipMode) {
      const time = pctToTime(pct)
      if (dragging === 'start') {
        onClipStartChange?.(time)
      } else {
        onClipEndChange?.(time)
      }
    }
  }

  const handleMouseLeave = () => {
    setHoverX(null)
    setHoverTime(null)
    setDragging(null)
    setIsPanning(false)
  }

  const handleMouseUp = () => {
    setDragging(null)
    setIsPanning(false)
  }

  const handleMouseDown = (e: React.MouseEvent<HTMLDivElement>) => {
    // Right-click or middle-click starts panning when zoomed in
    if (zoomLevel < 24 && !clipMode && !compact && e.button === 0 && e.shiftKey) {
      e.preventDefault()
      setIsPanning(true)
      panStartXRef.current = e.clientX
      panStartViewRef.current = viewStart
    }
  }

  // Mouse wheel zoom: centered on cursor position
  const handleWheel = useCallback((e: WheelEvent) => {
    if (compact) return
    e.preventDefault()

    const bar = barRef.current
    if (!bar) return

    const rect = bar.getBoundingClientRect()
    const cursorPct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    // The hour under cursor
    const cursorHour = viewStart + cursorPct * zoomLevel

    // Determine new zoom level
    const currentIdx = ZOOM_LEVELS.indexOf(zoomLevel)
    let newIdx: number
    if (e.deltaY > 0) {
      // Scroll down = zoom out
      newIdx = Math.max(0, currentIdx - 1)
    } else {
      // Scroll up = zoom in
      newIdx = Math.min(ZOOM_LEVELS.length - 1, currentIdx + 1)
    }
    const newZoom = ZOOM_LEVELS[newIdx]

    // Keep cursor hour at the same visual position
    const newViewStart = cursorHour - cursorPct * newZoom
    setZoomLevel(newZoom)
    setViewStart(clampViewStart(newViewStart, newZoom))
  }, [viewStart, zoomLevel, clampViewStart, compact])

  // Attach wheel listener with passive: false for preventDefault
  useEffect(() => {
    const bar = barRef.current
    if (!bar || compact) return
    bar.addEventListener('wheel', handleWheel, { passive: false })
    return () => bar.removeEventListener('wheel', handleWheel)
  }, [handleWheel, compact])

  // Zoom button handlers
  const handleZoomIn = () => {
    const currentIdx = ZOOM_LEVELS.indexOf(zoomLevel)
    if (currentIdx < ZOOM_LEVELS.length - 1) {
      const newZoom = ZOOM_LEVELS[currentIdx + 1]
      // Keep center
      const center = viewStart + zoomLevel / 2
      const newViewStart = center - newZoom / 2
      setZoomLevel(newZoom)
      setViewStart(clampViewStart(newViewStart, newZoom))
    }
  }

  const handleZoomOut = () => {
    const currentIdx = ZOOM_LEVELS.indexOf(zoomLevel)
    if (currentIdx > 0) {
      const newZoom = ZOOM_LEVELS[currentIdx - 1]
      const center = viewStart + zoomLevel / 2
      const newViewStart = center - newZoom / 2
      setZoomLevel(newZoom)
      setViewStart(clampViewStart(newViewStart, newZoom))
    }
  }

  const handleZoomReset = () => {
    setZoomLevel(24)
    setViewStart(0)
  }

  // Compute playback position percentage
  const playbackPct = playbackTime
    ? timeToPct(playbackTime)
    : null

  // Clip range percentages
  const clipStartPct = clipMode && clipStart ? timeToPct(clipStart) : null
  const clipEndPct = clipMode && clipEnd ? timeToPct(clipEnd) : null

  // Compute which hour labels to show within the visible range
  const visibleHours: number[] = []
  const startHour = Math.floor(viewStart)
  const endHour = Math.ceil(viewStart + zoomLevel)
  for (let h = startHour; h <= endHour && h <= 24; h++) {
    visibleHours.push(h)
  }

  // Format visible range label
  const formatHour = (h: number) => {
    const hClamped = Math.max(0, Math.min(24, h))
    const hh = Math.floor(hClamped)
    const mm = Math.round((hClamped - hh) * 60)
    return `${hh.toString().padStart(2, '0')}:${mm.toString().padStart(2, '0')}`
  }

  // Compact mode: reduced height, no zoom controls, no clip indicators
  if (compact) {
    return (
      <div className="w-full">
        {/* Timeline bar - compact */}
        <div
          ref={barRef}
          onClick={handleClick}
          onMouseMove={handleMouseMove}
          onMouseLeave={handleMouseLeave}
          onMouseUp={handleMouseUp}
          className="relative w-full h-8 bg-nvr-bg-input rounded cursor-crosshair overflow-hidden border border-nvr-border"
        >
          {/* Subtle hour grid lines */}
          {Array.from({ length: 24 }, (_, h) => (
            <div
              key={h}
              className="absolute top-0 bottom-0 border-l border-white/[0.06]"
              style={{ left: `${(h / 24) * 100}%` }}
            />
          ))}

          {/* Recording blocks */}
          {ranges.map((r, i) => {
            const start = new Date(r.start).getTime() - dayStart.getTime()
            const end = new Date(r.end).getTime() - dayStart.getTime()
            const left = (start / dayMs) * 100
            const width = ((end - start) / dayMs) * 100
            return (
              <div
                key={i}
                className="absolute top-0.5 bottom-0.5 bg-blue-500/60 rounded-sm transition-colors hover:bg-blue-500/80"
                style={{ left: `${left}%`, width: `${Math.max(width, 0.2)}%` }}
              />
            )
          })}

          {/* Playback position marker */}
          {playbackPct !== null && playbackPct >= 0 && playbackPct <= 100 && (
            <div
              className="absolute top-0 bottom-0 w-0.5 bg-white z-10 pointer-events-none"
              style={{ left: `${playbackPct}%` }}
            />
          )}

          {/* Hover indicator line */}
          {hoverX !== null && (
            <div
              className="absolute top-0 bottom-0 w-px bg-white/40 pointer-events-none z-20"
              style={{ left: `${hoverX}px` }}
            />
          )}

          {/* Hover tooltip */}
          {hoverX !== null && hoverTime && (
            <div
              className="absolute -top-7 -translate-x-1/2 bg-nvr-bg-secondary border border-nvr-border rounded px-1.5 py-0.5 text-[10px] text-nvr-text-primary pointer-events-none z-30 whitespace-nowrap shadow-lg"
              style={{ left: `${hoverX}px` }}
            >
              {hoverTime}
            </div>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="w-full">
      {/* Zoom controls */}
      {!compact && (
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center gap-1.5">
            <button
              onClick={handleZoomIn}
              disabled={zoomLevel === 1}
              className="bg-nvr-bg-input border border-nvr-border rounded px-2 py-1 text-xs text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors disabled:opacity-30 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              title="Zoom in"
              aria-label="Zoom in"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
            </button>
            <button
              onClick={handleZoomOut}
              disabled={zoomLevel === 24}
              className="bg-nvr-bg-input border border-nvr-border rounded px-2 py-1 text-xs text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors disabled:opacity-30 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              title="Zoom out"
              aria-label="Zoom out"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="5" y1="12" x2="19" y2="12" /></svg>
            </button>
            {zoomLevel < 24 && (
              <button
                onClick={handleZoomReset}
                className="bg-nvr-bg-input border border-nvr-border rounded px-2 py-1 text-xs text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                title="Reset to 24h view"
              >
                24h
              </button>
            )}
          </div>
          <div className="flex items-center gap-2">
            {zoomLevel < 24 && (
              <span className="text-xs text-nvr-text-muted">
                {formatHour(viewStart)} - {formatHour(viewStart + zoomLevel)} ({zoomLevel}h view)
              </span>
            )}
            {zoomLevel < 24 && (
              <span className="text-[10px] text-nvr-text-muted">
                Shift+drag to pan, scroll to zoom
              </span>
            )}
          </div>
        </div>
      )}

      {/* Hour labels above the bar */}
      <div className="relative w-full h-5 mb-1">
        {visibleHours.filter(h => h < 24).map(h => {
          const pct = ((h - viewStart) / zoomLevel) * 100
          // For narrow zoom levels, show every hour; for wider show every 2
          const showLabel = zoomLevel <= 6 || h % 2 === 0
          if (pct < -2 || pct > 102) return null
          return (
            <span
              key={h}
              className={`absolute text-[10px] text-nvr-text-muted -translate-x-1/2 select-none ${
                !showLabel ? 'hidden sm:inline' : ''
              }`}
              style={{ left: `${pct}%` }}
            >
              {h.toString().padStart(2, '0')}
              {zoomLevel <= 3 ? ':00' : ''}
            </span>
          )
        })}
      </div>

      {/* Timeline bar */}
      <div
        ref={barRef}
        onClick={handleClick}
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
        onMouseUp={handleMouseUp}
        onMouseDown={handleMouseDown}
        className={`relative w-full h-14 md:h-16 bg-nvr-bg-input rounded-lg overflow-hidden border border-nvr-border ${
          isPanning ? 'cursor-grabbing' : zoomLevel < 24 ? 'cursor-crosshair' : 'cursor-crosshair'
        }`}
      >
        {/* Subtle hour grid lines */}
        {visibleHours.map(h => {
          const pct = ((h - viewStart) / zoomLevel) * 100
          if (pct < 0 || pct > 100) return null
          return (
            <div
              key={h}
              className="absolute top-0 bottom-0 border-l border-white/[0.06]"
              style={{ left: `${pct}%` }}
            />
          )
        })}

        {/* Recording blocks — continuous blue bars showing where footage exists */}
        {ranges.map((r, i) => {
          const startMs = new Date(r.start).getTime() - dayStart.getTime()
          const endMs = new Date(r.end).getTime() - dayStart.getTime()
          // Convert to percentage of visible window
          const left = ((startMs - viewStartMs) / (viewEndMs - viewStartMs)) * 100
          const right = ((endMs - viewStartMs) / (viewEndMs - viewStartMs)) * 100
          const width = right - left
          // Skip if entirely outside visible range
          if (right < 0 || left > 100) return null
          return (
            <div
              key={i}
              className="absolute top-1 bottom-1 bg-blue-500/60 rounded-sm transition-colors hover:bg-blue-500/80"
              style={{
                left: `${Math.max(left, 0)}%`,
                width: `${Math.min(width, 100 - Math.max(left, 0))}%`,
                minWidth: '2px',
              }}
            />
          )
        })}

        {/* Motion event markers */}
        {events && events.map((ev, i) => {
          const evStart = new Date(ev.started_at)
          const evStartMs = evStart.getTime() - dayStart.getTime()
          const leftPct = ((evStartMs - viewStartMs) / (viewEndMs - viewStartMs)) * 100
          if (leftPct < -2 || leftPct > 102) return null

          const evEnd = ev.ended_at ? new Date(ev.ended_at) : null
          const startLabel = evStart.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
          const endLabel = evEnd
            ? evEnd.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
            : 'ongoing'
          const tooltip = `Motion ${startLabel} – ${endLabel}`

          return (
            <div
              key={i}
              className="absolute z-[8] cursor-pointer select-none group/marker"
              style={{ left: `${leftPct}%`, top: '0px', transform: 'translateX(-50%)' }}
              title={tooltip}
              onMouseEnter={() => setHoveredEvent(i)}
              onMouseLeave={() => setHoveredEvent(null)}
              onClick={(e) => {
                e.stopPropagation()
                onSeek?.(evStart)
                onEventClick?.(i)
              }}
            >
              <span className="text-[10px] leading-none drop-shadow-sm" role="img" aria-label="Motion event">
                {'\u{1F3C3}'}
              </span>
              {hoveredEvent === i && (
                <div className="absolute -top-7 left-1/2 -translate-x-1/2 bg-nvr-bg-secondary border border-nvr-border rounded px-2 py-0.5 text-[10px] text-nvr-text-primary pointer-events-none z-30 whitespace-nowrap shadow-lg">
                  {tooltip}
                </div>
              )}
            </div>
          )
        })}

        {/* Clip selection range highlight */}
        {clipMode && clipStartPct !== null && clipEndPct !== null && (
          <div
            className="absolute top-0 bottom-0 bg-emerald-500/25 border-l-2 border-r-2 border-emerald-400/70 z-[5]"
            style={{
              left: `${clipStartPct}%`,
              width: `${clipEndPct - clipStartPct}%`,
            }}
          />
        )}

        {/* Clip start handle */}
        {clipMode && clipStartPct !== null && (
          <div
            className="absolute top-0 bottom-0 z-[15] cursor-ew-resize group/handle"
            style={{ left: `${clipStartPct}%`, transform: 'translateX(-50%)', width: '12px' }}
            onMouseDown={(e) => { e.stopPropagation(); setDragging('start') }}
          >
            <div className="absolute top-0 bottom-0 left-1/2 -translate-x-1/2 w-0.5 bg-emerald-400" />
            <div className="absolute top-1/2 -translate-y-1/2 left-1/2 -translate-x-1/2 w-3 h-6 bg-emerald-400 rounded-sm shadow-lg" />
          </div>
        )}

        {/* Clip end handle */}
        {clipMode && clipEndPct !== null && (
          <div
            className="absolute top-0 bottom-0 z-[15] cursor-ew-resize group/handle"
            style={{ left: `${clipEndPct}%`, transform: 'translateX(-50%)', width: '12px' }}
            onMouseDown={(e) => { e.stopPropagation(); setDragging('end') }}
          >
            <div className="absolute top-0 bottom-0 left-1/2 -translate-x-1/2 w-0.5 bg-emerald-400" />
            <div className="absolute top-1/2 -translate-y-1/2 left-1/2 -translate-x-1/2 w-3 h-6 bg-emerald-400 rounded-sm shadow-lg" />
          </div>
        )}

        {/* Playback position marker */}
        {playbackPct !== null && playbackPct >= 0 && playbackPct <= 100 && (
          <div
            className="absolute top-0 bottom-0 w-0.5 bg-white z-10 pointer-events-none"
            style={{ left: `${playbackPct}%` }}
          />
        )}

        {/* Hover indicator line */}
        {hoverX !== null && (
          <div
            className="absolute top-0 bottom-0 w-px bg-white/40 pointer-events-none z-20"
            style={{ left: `${hoverX}px` }}
          />
        )}

        {/* Hover tooltip */}
        {hoverX !== null && hoverTime && (
          <div
            className="absolute -top-8 -translate-x-1/2 bg-nvr-bg-secondary border border-nvr-border rounded px-2 py-0.5 text-xs text-nvr-text-primary pointer-events-none z-30 whitespace-nowrap shadow-lg"
            style={{ left: `${hoverX}px` }}
          >
            {hoverTime}
          </div>
        )}
      </div>

      {/* Clip mode instruction text */}
      {clipMode && !clipStart && (
        <p className="text-xs text-nvr-text-muted mt-1.5">Click on the timeline to set clip start point</p>
      )}
      {clipMode && clipStart && !clipEnd && (
        <p className="text-xs text-nvr-text-muted mt-1.5">Click again to set clip end point</p>
      )}
      {clipMode && clipStart && clipEnd && (
        <p className="text-xs text-nvr-text-muted mt-1.5">Drag handles to adjust, or click to reset</p>
      )}
    </div>
  )
}
