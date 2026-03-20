import { useState, useCallback, useRef } from 'react'
import { TimeRange } from '../hooks/useRecordings'

interface Props {
  ranges: TimeRange[]
  date: string
  onSeek?: (time: Date) => void
  playbackTime?: Date | null

  // Clip creation mode
  clipMode?: boolean
  clipStart?: Date | null
  clipEnd?: Date | null
  onClipStartChange?: (time: Date | null) => void
  onClipEndChange?: (time: Date | null) => void
}

export default function Timeline({
  ranges,
  date,
  onSeek,
  playbackTime,
  clipMode,
  clipStart,
  clipEnd,
  onClipStartChange,
  onClipEndChange,
}: Props) {
  const dayStart = new Date(date + 'T00:00:00')
  const dayMs = 24 * 60 * 60 * 1000
  const barRef = useRef<HTMLDivElement>(null)

  const [hoverX, setHoverX] = useState<number | null>(null)
  const [hoverTime, setHoverTime] = useState<string | null>(null)
  const [dragging, setDragging] = useState<'start' | 'end' | null>(null)

  const pctToTime = useCallback((pct: number) => {
    return new Date(dayStart.getTime() + pct * dayMs)
  }, [dayStart.getTime(), dayMs])

  const timeToPct = useCallback((time: Date) => {
    return ((time.getTime() - dayStart.getTime()) / dayMs) * 100
  }, [dayStart.getTime(), dayMs])

  const pctFromEvent = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    return Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
  }

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (dragging) return

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
  }

  const handleMouseUp = () => {
    setDragging(null)
  }

  // Compute playback position percentage
  const playbackPct = playbackTime
    ? timeToPct(playbackTime)
    : null

  // Clip range percentages
  const clipStartPct = clipMode && clipStart ? timeToPct(clipStart) : null
  const clipEndPct = clipMode && clipEnd ? timeToPct(clipEnd) : null

  return (
    <div className="w-full">
      {/* Hour labels above the bar */}
      <div className="relative w-full h-5 mb-1">
        {Array.from({ length: 24 }, (_, h) => {
          const showLabel = h % 2 === 0
          return (
            <span
              key={h}
              className={`absolute text-[10px] text-nvr-text-muted -translate-x-1/2 select-none ${
                !showLabel ? 'hidden sm:inline' : ''
              }`}
              style={{ left: `${(h / 24) * 100}%` }}
            >
              {h.toString().padStart(2, '0')}
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
        className="relative w-full h-14 md:h-16 bg-nvr-bg-input rounded-lg cursor-crosshair overflow-hidden border border-nvr-border"
      >
        {/* Subtle hour grid lines */}
        {Array.from({ length: 24 }, (_, h) => (
          <div
            key={h}
            className="absolute top-0 bottom-0 border-l border-white/[0.06]"
            style={{ left: `${(h / 24) * 100}%` }}
          />
        ))}

        {/* Recording blocks — continuous blue bars showing where footage exists */}
        {ranges.map((r, i) => {
          const start = new Date(r.start).getTime() - dayStart.getTime()
          const end = new Date(r.end).getTime() - dayStart.getTime()
          const left = (start / dayMs) * 100
          const width = ((end - start) / dayMs) * 100
          return (
            <div
              key={i}
              className="absolute top-1 bottom-1 bg-blue-500/60 rounded-sm transition-colors hover:bg-blue-500/80"
              style={{ left: `${left}%`, width: `${Math.max(width, 0.2)}%` }}
            />
          )
        })}

        {/* TODO: Motion event markers — amber triangles/dots above footage bars.
            When per-event motion data is stored in the recording_status API,
            render small amber markers here at each motion event timestamp. */}

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
