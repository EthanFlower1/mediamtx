import { useState, useCallback, useRef } from 'react'
import { TimeRange } from '../hooks/useRecordings'

interface Props {
  ranges: TimeRange[]
  date: string
  onSeek?: (time: Date) => void
  playbackTime?: Date | null
}

export default function Timeline({ ranges, date, onSeek, playbackTime }: Props) {
  const dayStart = new Date(date + 'T00:00:00')
  const dayMs = 24 * 60 * 60 * 1000
  const barRef = useRef<HTMLDivElement>(null)

  const [hoverX, setHoverX] = useState<number | null>(null)
  const [hoverTime, setHoverTime] = useState<string | null>(null)

  const pctToTime = useCallback((pct: number) => {
    return new Date(dayStart.getTime() + pct * dayMs)
  }, [dayStart, dayMs])

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    const time = pctToTime(pct)
    onSeek?.(time)
  }

  const handleMouseMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    setHoverX(e.clientX - rect.left)
    const t = pctToTime(pct)
    setHoverTime(t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }))
  }

  const handleMouseLeave = () => {
    setHoverX(null)
    setHoverTime(null)
  }

  // Compute playback position percentage
  const playbackPct = playbackTime
    ? ((playbackTime.getTime() - dayStart.getTime()) / dayMs) * 100
    : null

  return (
    <div className="w-full">
      {/* Hour labels above the bar */}
      <div className="relative w-full h-5 mb-1">
        {Array.from({ length: 25 }, (_, h) => {
          if (h === 24) return null
          // Show every 2 hours on small screens, every hour otherwise
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

        {/* Recording blocks */}
        {ranges.map((r, i) => {
          const start = new Date(r.start).getTime() - dayStart.getTime()
          const end = new Date(r.end).getTime() - dayStart.getTime()
          const left = (start / dayMs) * 100
          const width = ((end - start) / dayMs) * 100
          return (
            <div
              key={i}
              className="absolute top-1 bottom-1 bg-nvr-accent/50 rounded-sm transition-colors hover:bg-nvr-accent/70"
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
            className="absolute -top-8 -translate-x-1/2 bg-nvr-bg-secondary border border-nvr-border rounded px-2 py-0.5 text-xs text-nvr-text-primary pointer-events-none z-30 whitespace-nowrap shadow-lg"
            style={{ left: `${hoverX}px` }}
          >
            {hoverTime}
          </div>
        )}
      </div>
    </div>
  )
}
