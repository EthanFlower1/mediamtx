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
    <div onClick={handleClick} style={{
      position: 'relative', width: '100%', height: 40,
      background: '#222', borderRadius: 4, cursor: 'crosshair',
      overflow: 'hidden',
    }}>
      {ranges.map((r, i) => {
        const start = new Date(r.start).getTime() - dayStart.getTime()
        const end = new Date(r.end).getTime() - dayStart.getTime()
        const left = (start / dayMs) * 100
        const width = ((end - start) / dayMs) * 100
        return (
          <div key={i} style={{
            position: 'absolute', top: 0, bottom: 0,
            left: `${left}%`, width: `${width}%`,
            background: '#4a9eff', opacity: 0.7,
          }} />
        )
      })}
      {Array.from({ length: 24 }, (_, h) => (
        <div key={h} style={{
          position: 'absolute', top: 0, bottom: 0,
          left: `${(h / 24) * 100}%`,
          borderLeft: '1px solid rgba(255,255,255,0.2)',
          fontSize: 9, color: '#888', paddingLeft: 2,
        }}>
          {h}:00
        </div>
      ))}
    </div>
  )
}
