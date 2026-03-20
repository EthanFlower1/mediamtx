import { TimeRange } from '../hooks/useRecordings'

interface Props {
  ranges: TimeRange[]
  date: string
  onSeek?: (time: Date) => void
}

export default function Timeline({ ranges, date, onSeek }: Props) {
  const dayStart = new Date(date + 'T00:00:00Z')
  const dayMs = 24 * 60 * 60 * 1000

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const pct = (e.clientX - rect.left) / rect.width
    const time = new Date(dayStart.getTime() + pct * dayMs)
    onSeek?.(time)
  }

  return (
    <div
      onClick={handleClick}
      className="relative w-full h-10 bg-nvr-bg-input rounded-lg cursor-crosshair overflow-hidden border border-nvr-border"
    >
      {ranges.map((r, i) => {
        const start = new Date(r.start).getTime() - dayStart.getTime()
        const end = new Date(r.end).getTime() - dayStart.getTime()
        const left = (start / dayMs) * 100
        const width = ((end - start) / dayMs) * 100
        return (
          <div
            key={i}
            className="absolute top-0 bottom-0 bg-nvr-accent/60"
            style={{ left: `${left}%`, width: `${width}%` }}
          />
        )
      })}
      {Array.from({ length: 24 }, (_, h) => (
        <div
          key={h}
          className="absolute top-0 bottom-0 border-l border-white/10 text-[9px] text-nvr-text-muted pl-0.5"
          style={{ left: `${(h / 24) * 100}%` }}
        >
          {h}:00
        </div>
      ))}
    </div>
  )
}
