import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useCameras } from '../hooks/useCameras'
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts'
import Timeline, { MotionEvent } from '../components/Timeline'
import VideoPlayer from '../components/VideoPlayer'
import { apiFetch } from '../api/client'

interface Segment {
  start: string
}

interface RecordingList {
  name: string
  segments: Segment[]
}

// Format a Date as RFC3339 with local timezone offset (not UTC).
// MediaMTX playback server matches against local-time file paths,
// so we must send local time, not UTC.
function toLocalRFC3339(d: Date): string {
  const pad = (n: number) => n.toString().padStart(2, '0')
  const offset = -d.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const absOffset = Math.abs(offset)
  const offH = pad(Math.floor(absOffset / 60))
  const offM = pad(absOffset % 60)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}${sign}${offH}:${offM}`
}

function shiftDate(dateStr: string, days: number): string {
  const d = new Date(dateStr + 'T00:00:00')
  d.setDate(d.getDate() + days)
  return d.toISOString().split('T')[0]
}

function formatDuration(ms: number): string {
  const totalSecs = Math.round(ms / 1000)
  const h = Math.floor(totalSecs / 3600)
  const m = Math.floor((totalSecs % 3600) / 60)
  const s = totalSecs % 60
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function getRelativeDateLabel(dateStr: string): string {
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const target = new Date(dateStr + 'T00:00:00')
  target.setHours(0, 0, 0, 0)

  const diffMs = today.getTime() - target.getTime()
  const diffDays = Math.round(diffMs / (24 * 60 * 60 * 1000))

  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Yesterday'
  if (diffDays > 1 && diffDays <= 7) {
    return target.toLocaleDateString(undefined, { weekday: 'long' })
  }
  return target.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

interface AllCameraRanges {
  cameraId: string
  cameraName: string
  mediamtxPath: string
  ranges: { start: string; end: string }[]
  loading: boolean
}

export default function Recordings() {
  const { cameras, loading: camerasLoading } = useCameras()
  const [selectedCamera, setSelectedCamera] = useState<string | null>(() => {
    return localStorage.getItem('nvr-recordings-camera')
  })
  const [cameraFilter, setCameraFilter] = useState('')

  const handleCameraChange = (id: string | null) => {
    setSelectedCamera(id)
    if (id) localStorage.setItem('nvr-recordings-camera', id)
    else localStorage.removeItem('nvr-recordings-camera')
  }

  const filteredCameras = cameraFilter
    ? cameras.filter(c => c.name.toLowerCase().includes(cameraFilter.toLowerCase()))
    : cameras

  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const [timelineRanges, setTimelineRanges] = useState<{ start: string; end: string }[]>([])
  const [playbackTime, setPlaybackTime] = useState<Date | null>(null)
  const [playbackUrl, setPlaybackUrl] = useState<string | null>(null)
  const [loadingRecordings, setLoadingRecordings] = useState(false)
  const [hasRecordings, setHasRecordings] = useState(false)

  // Motion events
  const [motionEvents, setMotionEvents] = useState<MotionEvent[]>([])
  const [motionPanelOpen, setMotionPanelOpen] = useState(false)
  const [highlightedEventIdx, setHighlightedEventIdx] = useState<number | null>(null)
  const eventRefs = useRef<(HTMLDivElement | null)[]>([])

  // "All Cameras" mode
  const isAllCameras = selectedCamera === '__all__'
  const [allCameraRanges, setAllCameraRanges] = useState<AllCameraRanges[]>([])
  const [allCamerasPlaybackCamera, setAllCamerasPlaybackCamera] = useState<string | null>(null)

  // Clip creation state
  const [clipMode, setClipMode] = useState(false)
  const [clipStart, setClipStart] = useState<Date | null>(null)
  const [clipEnd, setClipEnd] = useState<Date | null>(null)
  const [clipDownloading, setClipDownloading] = useState(false)

  // Track video start time for timeline sync
  const videoStartTimeRef = useRef<Date | null>(null)

  // Page title
  useEffect(() => {
    document.title = 'Recordings — MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  const selectedCameraObj = cameras.find(c => c.id === selectedCamera)
  const mediamtxPath = selectedCameraObj?.mediamtx_path || ''

  // Helper to fetch recordings for a single camera path and date
  const fetchCameraRecordings = useCallback((path: string, dateStr: string): Promise<{ start: string; end: string }[]> => {
    const dayStart = new Date(dateStr + 'T00:00:00')
    const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000)

    return fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${path}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: RecordingList | null) => {
        if (!data || !data.segments) return []

        const filtered = data.segments.filter(s => {
          const t = new Date(s.start)
          return t >= dayStart && t < dayEnd
        })

        // Build timeline ranges from segments. Each segment only has a start time.
        // We estimate each segment's end as the start of the next segment, but cap
        // the duration to detect real gaps in recording coverage.
        // Max segment duration = 65 minutes (slightly over the 1h default to account for variation)
        const MAX_SEGMENT_MS = 65 * 60 * 1000

        const ranges: { start: string; end: string }[] = []
        for (let i = 0; i < filtered.length; i++) {
          const segStartMs = new Date(filtered[i].start).getTime()
          let segEndMs: number

          if (i + 1 < filtered.length) {
            const nextStartMs = new Date(filtered[i + 1].start).getTime()
            const gap = nextStartMs - segStartMs
            // If the gap is larger than max segment duration, this segment
            // ended before the next one started — cap its duration
            segEndMs = gap > MAX_SEGMENT_MS
              ? segStartMs + MAX_SEGMENT_MS
              : nextStartMs
          } else {
            // Last segment: estimate 5 minutes (or until now if today)
            segEndMs = segStartMs + 5 * 60 * 1000
          }

          const segStart = filtered[i].start
          const segEnd = new Date(segEndMs).toISOString()

          // Merge with previous range if they're close (< 30 seconds gap)
          if (ranges.length > 0) {
            const prevEndMs = new Date(ranges[ranges.length - 1].end).getTime()
            if (segStartMs - prevEndMs < 30000) {
              ranges[ranges.length - 1].end = segEnd
              continue
            }
          }
          ranges.push({ start: segStart, end: segEnd })
        }
        return ranges
      })
      .catch(() => [])
  }, [])

  // Fetch recordings from MediaMTX when single camera or date changes
  useEffect(() => {
    if (isAllCameras || !mediamtxPath || !date) {
      if (!isAllCameras) {
        setTimelineRanges([])
        setHasRecordings(false)
      }
      return
    }

    setLoadingRecordings(true)

    fetchCameraRecordings(mediamtxPath, date)
      .then(ranges => {
        setTimelineRanges(ranges)
        setHasRecordings(ranges.length > 0)
      })
      .finally(() => setLoadingRecordings(false))
  }, [mediamtxPath, date, isAllCameras, fetchCameraRecordings])

  // Fetch motion events when single camera and date are selected
  useEffect(() => {
    if (isAllCameras || !selectedCamera || !date) {
      setMotionEvents([])
      return
    }

    apiFetch(`/cameras/${selectedCamera}/motion-events?date=${date}`)
      .then(res => res.ok ? res.json() : [])
      .then((data: MotionEvent[]) => setMotionEvents(data))
      .catch(() => setMotionEvents([]))
  }, [selectedCamera, date, isAllCameras])

  // Fetch recordings for ALL cameras when "All Cameras" selected
  useEffect(() => {
    if (!isAllCameras || !date || cameras.length === 0) {
      if (isAllCameras) setAllCameraRanges([])
      return
    }

    // Initialize all cameras as loading
    const initial: AllCameraRanges[] = cameras
      .filter(c => c.mediamtx_path)
      .map(c => ({
        cameraId: c.id,
        cameraName: c.name,
        mediamtxPath: c.mediamtx_path,
        ranges: [],
        loading: true,
      }))
    setAllCameraRanges(initial)

    // Fetch each camera's recordings in parallel
    initial.forEach(cam => {
      fetchCameraRecordings(cam.mediamtxPath, date).then(ranges => {
        setAllCameraRanges(prev =>
          prev.map(c =>
            c.cameraId === cam.cameraId
              ? { ...c, ranges, loading: false }
              : c
          )
        )
      })
    })
  }, [isAllCameras, date, cameras, fetchCameraRecordings])

  const handleSeek = useCallback((time: Date) => {
    if (!mediamtxPath) return
    setPlaybackTime(time)
    videoStartTimeRef.current = time
    const startISO = toLocalRFC3339(time)
    // Use duration=86400 (full day) so playback continues across segment boundaries
    // MediaMTX will naturally end the stream when no more footage exists
    const url = `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(mediamtxPath)}&start=${encodeURIComponent(startISO)}&duration=86400`
    setPlaybackUrl(url)
  }, [mediamtxPath])

  // Handle seek from the "All Cameras" view
  const handleAllCamerasSeek = useCallback((cameraId: string, time: Date) => {
    const cam = cameras.find(c => c.id === cameraId)
    if (!cam?.mediamtx_path) return
    setAllCamerasPlaybackCamera(cameraId)
    setPlaybackTime(time)
    videoStartTimeRef.current = time
    const startISO = toLocalRFC3339(time)
    const url = `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(cam.mediamtx_path)}&start=${encodeURIComponent(startISO)}&duration=86400`
    setPlaybackUrl(url)
  }, [cameras])

  // Update timeline playback marker as video plays
  const handleVideoTimeUpdate = useCallback((videoSeconds: number) => {
    if (!videoStartTimeRef.current) return
    const currentWallTime = new Date(videoStartTimeRef.current.getTime() + videoSeconds * 1000)
    setPlaybackTime(currentWallTime)
  }, [])

  // Handle clicking a motion event on the timeline — scroll list to it and highlight
  const handleTimelineEventClick = useCallback((index: number) => {
    setMotionPanelOpen(true)
    setHighlightedEventIdx(index)
    // Scroll the event row into view after panel opens
    setTimeout(() => {
      eventRefs.current[index]?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }, 100)
  }, [])

  // Seek to 5 seconds before a motion event start
  const handleMotionEventSeek = useCallback((event: MotionEvent) => {
    const evStart = new Date(event.started_at)
    const seekTime = new Date(evStart.getTime() - 5000) // 5 seconds before
    handleSeek(seekTime)
  }, [handleSeek])

  // Determine which motion event is currently "playing" based on playbackTime
  const activeEventIdx = useMemo(() => {
    if (!playbackTime || motionEvents.length === 0) return null
    const pt = playbackTime.getTime()
    for (let i = 0; i < motionEvents.length; i++) {
      const ev = motionEvents[i]
      const start = new Date(ev.started_at).getTime() - 5000 // include 5s pre-roll
      const end = ev.ended_at ? new Date(ev.ended_at).getTime() : Date.now()
      if (pt >= start && pt <= end) return i
    }
    return null
  }, [playbackTime, motionEvents])

  const handleClipDownload = async () => {
    if (!mediamtxPath || !clipStart || !clipEnd) return
    setClipDownloading(true)

    const startISO = toLocalRFC3339(clipStart)
    const durationSecs = (clipEnd.getTime() - clipStart.getTime()) / 1000
    const url = `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(mediamtxPath)}&start=${encodeURIComponent(startISO)}&duration=${durationSecs}`

    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error('Download failed')
      const blob = await res.blob()
      const blobUrl = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = blobUrl
      link.download = `clip_${mediamtxPath.replace(/\//g, '_')}_${startISO.replace(/[:.]/g, '-')}.mp4`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(blobUrl)
    } catch {
      // fallback: open in new tab
      window.open(url, '_blank')
    } finally {
      setClipDownloading(false)
    }
  }

  const exitClipMode = () => {
    setClipMode(false)
    setClipStart(null)
    setClipEnd(null)
  }

  const resetPlayback = () => {
    setPlaybackTime(null)
    setPlaybackUrl(null)
    videoStartTimeRef.current = null
    setAllCamerasPlaybackCamera(null)
  }

  // Keyboard shortcuts: left/right arrows to navigate dates
  const recordingsShortcuts = useMemo(() => [
    {
      key: 'ArrowLeft',
      handler: () => { setDate(prev => shiftDate(prev, -1)); resetPlayback(); exitClipMode() },
      description: 'Previous day',
    },
    {
      key: 'ArrowRight',
      handler: () => { setDate(prev => shiftDate(prev, 1)); resetPlayback(); exitClipMode() },
      description: 'Next day',
    },
  ], [])
  useKeyboardShortcuts(recordingsShortcuts)

  const isToday = date === new Date().toISOString().split('T')[0]
  const relativeDateLabel = getRelativeDateLabel(date)

  if (camerasLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <span className="inline-block w-5 h-5 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-3" />
        <span className="text-nvr-text-secondary">Loading cameras...</span>
      </div>
    )
  }

  const clipDurationMs = clipStart && clipEnd ? clipEnd.getTime() - clipStart.getTime() : 0

  return (
    <div>
      {/* Top bar: camera selector, date nav, clip button */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-3 mb-6">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <div className="flex flex-col gap-1">
            {cameras.length > 5 && (
              <input
                type="text"
                placeholder="Filter cameras..."
                value={cameraFilter}
                onChange={e => setCameraFilter(e.target.value)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-1.5 text-xs text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors w-full sm:w-48"
              />
            )}
            <select
              value={selectedCamera || ''}
              onChange={e => {
                handleCameraChange(e.target.value || null)
                resetPlayback()
                exitClipMode()
              }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors min-h-[44px] w-full sm:w-48"
            >
              <option value="">Select Camera</option>
              <option value="__all__">All Cameras</option>
              {filteredCameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
            </select>
          </div>

          <div className="flex items-center gap-1 shrink-0">
            <button
              onClick={() => { setDate(shiftDate(date, -1)); resetPlayback(); exitClipMode() }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg p-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              title="Previous day"
              aria-label="Previous day"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="15 18 9 12 15 6" /></svg>
            </button>
            <input
              type="date"
              value={date}
              onChange={e => { setDate(e.target.value); resetPlayback(); exitClipMode() }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors min-h-[44px]"
            />
            <button
              onClick={() => { setDate(shiftDate(date, 1)); resetPlayback(); exitClipMode() }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg p-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              title="Next day"
              aria-label="Next day"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
            </button>
            {/* Relative date label */}
            <span className="text-sm text-nvr-text-secondary font-medium ml-1 hidden sm:inline">
              {relativeDateLabel}
            </span>
            {/* Motion events count badge */}
            {!isAllCameras && motionEvents.length > 0 && (
              <span className="hidden sm:inline-flex items-center gap-1 ml-2 bg-nvr-accent/15 text-nvr-accent text-xs font-medium px-2 py-0.5 rounded-full">
                {'\u{1F3C3}'} {motionEvents.length}
              </span>
            )}
            {!isToday && (
              <button
                onClick={() => { setDate(new Date().toISOString().split('T')[0]); resetPlayback(); exitClipMode() }}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                Today
              </button>
            )}
          </div>
        </div>

        {/* Create Clip button */}
        {selectedCamera && !isAllCameras && hasRecordings && (
          <div className="shrink-0">
            {clipMode ? (
              <button
                onClick={exitClipMode}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px] inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
                Cancel Clip
              </button>
            ) : (
              <button
                onClick={() => setClipMode(true)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px] inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><circle cx="6" cy="6" r="3" /><circle cx="6" cy="18" r="3" /><line x1="20" y1="4" x2="8.12" y2="15.88" /><line x1="14.47" y1="14.48" x2="20" y2="20" /><line x1="8.12" y1="8.12" x2="12" y2="12" /></svg>
                Create Clip
              </button>
            )}
          </div>
        )}
      </div>

      {/* Main content */}
      {!selectedCamera && cameras.length > 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <div className="flex items-center gap-3 mb-5">
            {[1, 2, 3, 4].map(i => (
              <div key={i} className={`w-10 h-10 rounded-lg flex items-center justify-center ${i === 1 ? 'bg-nvr-accent/15' : 'bg-nvr-bg-tertiary'}`}>
                <svg xmlns="http://www.w3.org/2000/svg" className={`w-5 h-5 ${i === 1 ? 'text-nvr-accent' : 'text-nvr-text-muted/50'}`} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><path d="M15.75 10.5l4.72-4.72a.75.75 0 011.28.53v11.38a.75.75 0 01-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 002.25-2.25v-9a2.25 2.25 0 00-2.25-2.25h-9A2.25 2.25 0 002.25 7.5v9a2.25 2.25 0 002.25 2.25z" /></svg>
              </div>
            ))}
          </div>
          <p className="text-nvr-text-secondary text-lg mb-1">Select a camera above to view its recordings</p>
          <p className="text-nvr-text-muted text-sm">Choose from the dropdown to browse recorded footage</p>
        </div>
      )}

      {cameras.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-12 h-12 text-nvr-text-muted/50 mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><path d="M23 19a2 2 0 01-2 2H3a2 2 0 01-2-2V8a2 2 0 012-2h4l2-3h6l2 3h4a2 2 0 012 2z" /><circle cx="12" cy="13" r="4" /></svg>
          <p className="text-nvr-text-secondary text-lg mb-1">No cameras configured</p>
          <p className="text-nvr-text-muted text-sm">Add cameras first to start recording</p>
        </div>
      )}

      {/* "All Cameras" multi-camera timeline view */}
      {isAllCameras && (
        <>
          <div className="mb-6">
            {/* Hour labels header */}
            <div className="flex items-center mb-1">
              <div className="w-36 shrink-0" />
              <div className="relative flex-1 h-5">
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
            </div>

            {/* Camera rows */}
            {allCameraRanges.length === 0 && (
              <div className="flex items-center gap-2 py-4">
                <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                <span className="text-nvr-text-muted text-sm">Loading all camera recordings...</span>
              </div>
            )}

            {allCameraRanges.map(cam => (
              <div key={cam.cameraId} className="flex items-center mb-1 group/row">
                <div className="w-36 shrink-0 pr-3 text-right">
                  <span className="text-xs text-nvr-text-secondary truncate block" title={cam.cameraName}>
                    {cam.cameraName}
                  </span>
                </div>
                <div className="flex-1 relative">
                  {cam.loading ? (
                    <div className="h-8 bg-nvr-bg-input rounded border border-nvr-border flex items-center justify-center">
                      <span className="inline-block w-3 h-3 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                    </div>
                  ) : (
                    <Timeline
                      ranges={cam.ranges}
                      date={date}
                      onSeek={(time) => handleAllCamerasSeek(cam.cameraId, time)}
                      playbackTime={allCamerasPlaybackCamera === cam.cameraId ? playbackTime : null}
                      compact
                    />
                  )}
                </div>
              </div>
            ))}

            {allCameraRanges.length > 0 && allCameraRanges.every(c => !c.loading) && allCameraRanges.every(c => c.ranges.length === 0) && (
              <div className="text-center py-8">
                <p className="text-nvr-text-secondary text-sm">No recordings found for any camera on this date.</p>
              </div>
            )}
          </div>

          {/* Video player for "All Cameras" mode */}
          {playbackTime && playbackUrl && allCamerasPlaybackCamera && (
            <div className="mb-6">
              <div className="flex items-center gap-2 mb-2">
                <span className="text-sm text-nvr-text-secondary">
                  Playing {cameras.find(c => c.id === allCamerasPlaybackCamera)?.name || 'camera'} from {playbackTime.toLocaleTimeString()}
                </span>
                <button
                  onClick={resetPlayback}
                  className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
                >
                  Close
                </button>
              </div>
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
                <VideoPlayer
                  src={playbackUrl}
                  onTimeUpdate={handleVideoTimeUpdate}
                />
              </div>
            </div>
          )}
        </>
      )}

      {selectedCamera && !isAllCameras && (
        <>
          {/* Timeline — full width, shows blue bars where footage exists */}
          <div className="mb-6">
            <Timeline
              ranges={timelineRanges}
              date={date}
              onSeek={clipMode ? undefined : handleSeek}
              playbackTime={playbackTime}
              events={motionEvents}
              onEventClick={handleTimelineEventClick}
              clipMode={clipMode}
              clipStart={clipStart}
              clipEnd={clipEnd}
              onClipStartChange={setClipStart}
              onClipEndChange={setClipEnd}
            />
            {loadingRecordings && (
              <div className="flex items-center gap-2 mt-2">
                <span className="inline-block w-3 h-3 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                <span className="text-nvr-text-muted text-sm">Loading recordings...</span>
              </div>
            )}
          </div>

          {/* Motion Events panel — collapsible list */}
          {!isAllCameras && selectedCamera && !loadingRecordings && (
            <div className="mb-6">
              {/* Panel header — click to toggle */}
              <button
                onClick={() => setMotionPanelOpen(prev => !prev)}
                className="flex items-center gap-2 w-full text-left py-2 group focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  className={`w-3.5 h-3.5 text-nvr-text-muted transition-transform ${motionPanelOpen ? 'rotate-0' : '-rotate-90'}`}
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={2}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <polyline points="6 9 12 15 18 9" />
                </svg>
                <span className="text-sm font-medium text-nvr-text-primary">
                  Motion Events
                </span>
                {motionEvents.length > 0 && (
                  <span className="bg-nvr-accent/15 text-nvr-accent text-xs font-medium px-2 py-0.5 rounded-full">
                    {motionEvents.length} event{motionEvents.length !== 1 ? 's' : ''}
                  </span>
                )}
              </button>

              {/* Expanded content */}
              {motionPanelOpen && (
                <div className="mt-2 space-y-2 max-h-80 overflow-y-auto">
                  {motionEvents.length === 0 ? (
                    <p className="text-sm text-nvr-text-muted py-4 text-center">
                      No motion events detected on this date
                    </p>
                  ) : (
                    motionEvents.map((ev, i) => {
                      const evStart = new Date(ev.started_at)
                      const evEnd = ev.ended_at ? new Date(ev.ended_at) : null
                      const startLabel = evStart.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' })
                      const endLabel = evEnd
                        ? evEnd.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', second: '2-digit' })
                        : 'ongoing'
                      const durationMs = evEnd
                        ? evEnd.getTime() - evStart.getTime()
                        : null
                      const durationLabel = durationMs !== null ? formatDuration(durationMs) : '\u2014'
                      const isActive = activeEventIdx === i
                      const isHighlighted = highlightedEventIdx === i

                      return (
                        <div
                          key={i}
                          ref={el => { eventRefs.current[i] = el }}
                          onClick={() => {
                            handleMotionEventSeek(ev)
                            setHighlightedEventIdx(i)
                          }}
                          className={`flex items-center gap-3 bg-nvr-bg-secondary border rounded-lg px-4 py-3 cursor-pointer hover:bg-nvr-bg-tertiary transition-colors ${
                            isActive
                              ? 'border-nvr-accent bg-nvr-accent/5'
                              : isHighlighted
                                ? 'border-nvr-accent/50'
                                : 'border-nvr-border'
                          }`}
                        >
                          {/* Emoji */}
                          <span className="text-base shrink-0" role="img" aria-label="Motion event">
                            {'\u{1F3C3}'}
                          </span>

                          {/* Time range */}
                          <div className="flex-1 min-w-0">
                            <span className="text-sm text-nvr-text-primary font-mono">
                              {startLabel}
                            </span>
                            <span className="text-sm text-nvr-text-muted mx-1.5">&mdash;</span>
                            <span className="text-sm text-nvr-text-primary font-mono">
                              {endLabel}
                            </span>
                            <span className="text-xs text-nvr-text-muted ml-3">
                              ({durationLabel})
                            </span>
                          </div>

                          {/* Play button */}
                          <button
                            onClick={(e) => {
                              e.stopPropagation()
                              handleMotionEventSeek(ev)
                              setHighlightedEventIdx(i)
                            }}
                            className="shrink-0 flex items-center gap-1 text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded px-2 py-1"
                            title="Play from this event"
                          >
                            Play
                            <svg xmlns="http://www.w3.org/2000/svg" className="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
                          </button>
                        </div>
                      )
                    })
                  )}
                </div>
              )}
            </div>
          )}

          {/* Clip creator panel — shown when clip mode is active and both points are set */}
          {clipMode && clipStart && clipEnd && (
            <div className="mb-6 bg-nvr-bg-secondary border border-emerald-500/30 rounded-xl p-4">
              <div className="flex flex-col sm:flex-row sm:items-center gap-4">
                <div className="flex items-center gap-4 flex-1">
                  <div>
                    <label className="block text-xs text-nvr-text-muted mb-0.5">Start</label>
                    <span className="text-sm text-nvr-text-primary font-mono">
                      {clipStart.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    </span>
                  </div>
                  <div className="text-nvr-text-muted">
                    <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="5" y1="12" x2="19" y2="12" /><polyline points="12 5 19 12 12 19" /></svg>
                  </div>
                  <div>
                    <label className="block text-xs text-nvr-text-muted mb-0.5">End</label>
                    <span className="text-sm text-nvr-text-primary font-mono">
                      {clipEnd.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    </span>
                  </div>
                  <div className="border-l border-nvr-border pl-4">
                    <label className="block text-xs text-nvr-text-muted mb-0.5">Duration</label>
                    <span className="text-sm text-nvr-text-primary font-medium">
                      {formatDuration(clipDurationMs)}
                    </span>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => { handleSeek(clipStart!) }}
                    className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm inline-flex items-center gap-1.5 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
                    Preview
                  </button>
                  <button
                    onClick={handleClipDownload}
                    disabled={clipDownloading}
                    className="bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 disabled:cursor-not-allowed text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm inline-flex items-center gap-2 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    {clipDownloading ? (
                      <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    ) : (
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
                    )}
                    {clipDownloading ? 'Downloading...' : 'Download Clip'}
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* Empty state for no recordings */}
          {!loadingRecordings && !hasRecordings && (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <div className="relative mb-4">
                <svg xmlns="http://www.w3.org/2000/svg" className="w-12 h-12 text-nvr-text-muted/40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" /></svg>
                <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5 text-nvr-danger absolute -bottom-1 -right-1" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.5} strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
              </div>
              <p className="text-nvr-text-secondary text-lg mb-1">
                No recordings found for {new Date(date + 'T00:00:00').toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric' })}
              </p>
              <p className="text-nvr-text-muted text-sm mb-4 max-w-sm">
                Check that recording is enabled in this camera's recording schedule.
              </p>
              <a
                href="/cameras"
                className="inline-flex items-center gap-1.5 text-sm text-nvr-accent hover:text-nvr-accent-hover transition-colors font-medium"
              >
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg>
                Go to Camera Settings
              </a>
            </div>
          )}

          {/* Video player — full width, shows when user clicks timeline */}
          {playbackTime && playbackUrl && (
            <div className="mb-6">
              <div className="flex items-center gap-2 mb-2">
                <span className="text-sm text-nvr-text-secondary">
                  Playing from {playbackTime.toLocaleTimeString()}
                </span>
                <button
                  onClick={resetPlayback}
                  className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
                >
                  Close
                </button>
              </div>
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
                <VideoPlayer
                  src={playbackUrl}
                  onTimeUpdate={handleVideoTimeUpdate}
                />
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
