import { useRef, useCallback, useState, useEffect } from 'react'
import VideoPlayer from './VideoPlayer'
import Timeline from './Timeline'

interface CameraInfo {
  id: string
  name: string
  mediamtxPath: string
}

interface Props {
  cameras: CameraInfo[]
  startTime: Date
  date: string
  allCameraRanges: { cameraId: string; ranges: { start: string; end: string }[] }[]
  onClose: () => void
}

// Format a Date as RFC3339 with local timezone offset.
function toLocalRFC3339(d: Date): string {
  const pad = (n: number) => n.toString().padStart(2, '0')
  const offset = -d.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const absOffset = Math.abs(offset)
  const offH = pad(Math.floor(absOffset / 60))
  const offM = pad(absOffset % 60)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}${sign}${offH}:${offM}`
}

export default function MultiCameraPlayer({ cameras, startTime, date, allCameraRanges, onClose }: Props) {
  const videoRefs = useRef<(HTMLVideoElement | null)[]>([])
  const [currentTime, setCurrentTime] = useState<Date>(startTime)
  const startTimeRef = useRef<Date>(startTime)
  const seekingRef = useRef(false)

  // Build playback URLs for all cameras at the given start time
  const buildUrl = useCallback((mediamtxPath: string, time: Date) => {
    const startISO = toLocalRFC3339(time)
    return `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(mediamtxPath)}&start=${encodeURIComponent(startISO)}&duration=86400`
  }, [])

  const [urls, setUrls] = useState<string[]>(() =>
    cameras.map(c => buildUrl(c.mediamtxPath, startTime))
  )

  // When startTime prop changes, rebuild URLs
  useEffect(() => {
    startTimeRef.current = startTime
    setCurrentTime(startTime)
    setUrls(cameras.map(c => buildUrl(c.mediamtxPath, startTime)))
  }, [startTime, cameras, buildUrl])

  // Handle time update from first video player (used as sync source)
  const handleTimeUpdate = useCallback((videoSeconds: number) => {
    if (seekingRef.current) return
    const wallTime = new Date(startTimeRef.current.getTime() + videoSeconds * 1000)
    setCurrentTime(wallTime)
  }, [])

  // Handle seek from shared timeline
  const handleSeek = useCallback((time: Date) => {
    seekingRef.current = true
    startTimeRef.current = time
    setCurrentTime(time)
    setUrls(cameras.map(c => buildUrl(c.mediamtxPath, time)))
    // Allow time updates again after URLs reload
    setTimeout(() => { seekingRef.current = false }, 1000)
  }, [cameras, buildUrl])

  // Store video element refs
  const handleVideoRef = useCallback((index: number) => (el: HTMLVideoElement | null) => {
    videoRefs.current[index] = el
  }, [])

  // Grid layout: 2 cameras = 2x1, 3-4 cameras = 2x2
  const gridCols = cameras.length <= 2 ? 'grid-cols-2' : 'grid-cols-2'
  const gridRows = cameras.length <= 2 ? 'grid-rows-1' : 'grid-rows-2'

  // Merge all selected camera ranges for the shared timeline
  const mergedRanges: { start: string; end: string }[] = []
  for (const cam of cameras) {
    const camRanges = allCameraRanges.find(r => r.cameraId === cam.id)
    if (camRanges) {
      mergedRanges.push(...camRanges.ranges)
    }
  }
  // Deduplicate and sort
  mergedRanges.sort((a, b) => a.start.localeCompare(b.start))

  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-nvr-text-primary">
            Compare Mode
          </span>
          <span className="text-xs text-nvr-text-muted">
            {cameras.length} cameras synced at {currentTime.toLocaleTimeString()}
          </span>
        </div>
        <button
          onClick={onClose}
          className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-1.5 text-xs text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors inline-flex items-center gap-1.5 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
          Exit Compare
        </button>
      </div>

      {/* Camera grid */}
      <div className={`grid ${gridCols} ${gridRows} gap-2 mb-3`}>
        {cameras.map((cam, i) => (
          <div key={cam.id} className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
            {/* Camera name label */}
            <div className="bg-nvr-bg-tertiary px-3 py-1.5 border-b border-nvr-border">
              <span className="text-xs font-medium text-nvr-text-primary">{cam.name}</span>
            </div>
            <VideoPlayer
              src={urls[i]}
              onTimeUpdate={i === 0 ? handleTimeUpdate : undefined}
              onVideoRef={handleVideoRef(i)}
            />
          </div>
        ))}
      </div>

      {/* Shared timeline */}
      <div>
        <div className="text-xs text-nvr-text-muted mb-1">Shared Timeline (click to seek all cameras)</div>
        <Timeline
          ranges={mergedRanges}
          date={date}
          onSeek={handleSeek}
          playbackTime={currentTime}
          compact
        />
      </div>
    </div>
  )
}
