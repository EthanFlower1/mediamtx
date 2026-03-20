import { useState, useEffect, useRef, useCallback } from 'react'
import { useCameras } from '../hooks/useCameras'
import Timeline from '../components/Timeline'
import VideoPlayer from '../components/VideoPlayer'

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

export default function Recordings() {
  const { cameras, loading: camerasLoading } = useCameras()
  const [selectedCamera, setSelectedCamera] = useState<string | null>(() => {
    return localStorage.getItem('nvr-recordings-camera')
  })

  const handleCameraChange = (id: string | null) => {
    setSelectedCamera(id)
    if (id) localStorage.setItem('nvr-recordings-camera', id)
    else localStorage.removeItem('nvr-recordings-camera')
  }
  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const [timelineRanges, setTimelineRanges] = useState<{ start: string; end: string }[]>([])
  const [playbackTime, setPlaybackTime] = useState<Date | null>(null)
  const [playbackUrl, setPlaybackUrl] = useState<string | null>(null)
  const [loadingRecordings, setLoadingRecordings] = useState(false)
  const [hasRecordings, setHasRecordings] = useState(false)

  // Clip creation state
  const [clipMode, setClipMode] = useState(false)
  const [clipStart, setClipStart] = useState<Date | null>(null)
  const [clipEnd, setClipEnd] = useState<Date | null>(null)
  const [clipDownloading, setClipDownloading] = useState(false)

  // Track video start time for timeline sync
  const videoStartTimeRef = useRef<Date | null>(null)

  const selectedCameraObj = cameras.find(c => c.id === selectedCamera)
  const mediamtxPath = selectedCameraObj?.mediamtx_path || ''

  // Fetch recordings from MediaMTX when camera or date changes
  useEffect(() => {
    if (!mediamtxPath || !date) {
      setTimelineRanges([])
      setHasRecordings(false)
      return
    }

    setLoadingRecordings(true)

    fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${mediamtxPath}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: RecordingList | null) => {
        if (!data || !data.segments) {
          setTimelineRanges([])
          setHasRecordings(false)
          return
        }

        const dayStart = new Date(date + 'T00:00:00')
        const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000)

        const filtered = data.segments.filter(s => {
          const t = new Date(s.start)
          return t >= dayStart && t < dayEnd
        })

        // Build continuous ranges from segments — merge adjacent segments
        // so the user sees uninterrupted footage blocks
        const ranges: { start: string; end: string }[] = []
        for (let i = 0; i < filtered.length; i++) {
          const segStart = filtered[i].start
          const segEnd = i + 1 < filtered.length
            ? filtered[i + 1].start
            : new Date(new Date(segStart).getTime() + 5 * 60 * 1000).toISOString()

          // Merge with previous range if gap is small (< 10 seconds)
          if (ranges.length > 0) {
            const prevEnd = new Date(ranges[ranges.length - 1].end).getTime()
            const curStart = new Date(segStart).getTime()
            if (curStart - prevEnd < 10000) {
              ranges[ranges.length - 1].end = segEnd
              continue
            }
          }
          ranges.push({ start: segStart, end: segEnd })
        }

        setTimelineRanges(ranges)
        setHasRecordings(filtered.length > 0)
      })
      .catch(() => {
        setTimelineRanges([])
        setHasRecordings(false)
      })
      .finally(() => setLoadingRecordings(false))
  }, [mediamtxPath, date])

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

  // Update timeline playback marker as video plays
  const handleVideoTimeUpdate = useCallback((videoSeconds: number) => {
    if (!videoStartTimeRef.current) return
    const currentWallTime = new Date(videoStartTimeRef.current.getTime() + videoSeconds * 1000)
    setPlaybackTime(currentWallTime)
  }, [])

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
  }

  const isToday = date === new Date().toISOString().split('T')[0]

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
            {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>

          <div className="flex items-center gap-1 shrink-0">
            <button
              onClick={() => { setDate(shiftDate(date, -1)); resetPlayback(); exitClipMode() }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg p-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center"
              title="Previous day"
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
              className="bg-nvr-bg-input border border-nvr-border rounded-lg p-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center"
              title="Next day"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
            </button>
            {!isToday && (
              <button
                onClick={() => { setDate(new Date().toISOString().split('T')[0]); resetPlayback(); exitClipMode() }}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px]"
              >
                Today
              </button>
            )}
          </div>
        </div>

        {/* Create Clip button */}
        {selectedCamera && hasRecordings && (
          <div className="shrink-0">
            {clipMode ? (
              <button
                onClick={exitClipMode}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px] inline-flex items-center gap-2"
              >
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" /></svg>
                Cancel Clip
              </button>
            ) : (
              <button
                onClick={() => setClipMode(true)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px] inline-flex items-center gap-2"
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
          <svg xmlns="http://www.w3.org/2000/svg" className="w-12 h-12 text-nvr-text-muted/50 mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><path d="M23 19a2 2 0 01-2 2H3a2 2 0 01-2-2V8a2 2 0 012-2h4l2-3h6l2 3h4a2 2 0 012 2z" /><circle cx="12" cy="13" r="4" /></svg>
          <p className="text-nvr-text-secondary text-lg mb-1">Select a camera to view recordings</p>
          <p className="text-nvr-text-muted text-sm">Choose from the dropdown above</p>
        </div>
      )}

      {cameras.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-12 h-12 text-nvr-text-muted/50 mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><path d="M23 19a2 2 0 01-2 2H3a2 2 0 01-2-2V8a2 2 0 012-2h4l2-3h6l2 3h4a2 2 0 012 2z" /><circle cx="12" cy="13" r="4" /></svg>
          <p className="text-nvr-text-secondary text-lg mb-1">No cameras configured</p>
          <p className="text-nvr-text-muted text-sm">Add cameras first to start recording</p>
        </div>
      )}

      {selectedCamera && (
        <>
          {/* Timeline — full width, shows blue bars where footage exists */}
          <div className="mb-6">
            <Timeline
              ranges={timelineRanges}
              date={date}
              onSeek={clipMode ? undefined : handleSeek}
              playbackTime={playbackTime}
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
                    className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm inline-flex items-center gap-1.5"
                  >
                    <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
                    Preview
                  </button>
                  <button
                    onClick={handleClipDownload}
                    disabled={clipDownloading}
                    className="bg-emerald-600 hover:bg-emerald-700 disabled:opacity-50 text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm inline-flex items-center gap-2"
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
              <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 text-nvr-text-muted/40 mb-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" /></svg>
              <p className="text-nvr-text-secondary mb-1">No recordings on this date</p>
              <p className="text-nvr-text-muted text-sm">Try a different date or check that recording is enabled</p>
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
                  className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors"
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
