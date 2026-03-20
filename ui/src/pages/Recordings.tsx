import { useState, useEffect, useRef } from 'react'
import { useCameras } from '../hooks/useCameras'
import Timeline from '../components/Timeline'

interface Segment {
  start: string
}

interface RecordingList {
  name: string
  segments: Segment[]
}

export default function Recordings() {
  const { cameras, loading: camerasLoading } = useCameras()
  const [selectedCamera, setSelectedCamera] = useState<string | null>(null)
  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const [segments, setSegments] = useState<Segment[]>([])
  const [timelineRanges, setTimelineRanges] = useState<{ start: string; end: string }[]>([])
  const [playbackTime, setPlaybackTime] = useState<Date | null>(null)
  const [loadingSegments, setLoadingSegments] = useState(false)
  const videoRef = useRef<HTMLVideoElement>(null)

  const selectedCameraObj = cameras.find(c => c.id === selectedCamera)
  const mediamtxPath = selectedCameraObj?.mediamtx_path || ''

  // Fetch recordings from MediaMTX's recordings API when camera or date changes.
  useEffect(() => {
    if (!mediamtxPath || !date) {
      setSegments([])
      setTimelineRanges([])
      return
    }

    setLoadingSegments(true)

    fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${mediamtxPath}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: RecordingList | null) => {
        if (!data || !data.segments) {
          setSegments([])
          setTimelineRanges([])
          return
        }

        // Filter segments to the selected date.
        const dayStart = new Date(date + 'T00:00:00')
        const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000)

        const filtered = data.segments.filter(s => {
          const t = new Date(s.start)
          return t >= dayStart && t < dayEnd
        })

        setSegments(filtered)

        // Build timeline ranges from consecutive segments.
        // Each segment is roughly the configured duration (1h default, with parts).
        const ranges: { start: string; end: string }[] = []
        for (let i = 0; i < filtered.length; i++) {
          const start = filtered[i].start
          const end = i + 1 < filtered.length
            ? filtered[i + 1].start
            : new Date(new Date(start).getTime() + 5 * 60 * 1000).toISOString() // estimate 5min for last segment
          ranges.push({ start, end })
        }
        setTimelineRanges(ranges)
      })
      .catch(() => {
        setSegments([])
        setTimelineRanges([])
      })
      .finally(() => setLoadingSegments(false))
  }, [mediamtxPath, date])

  // Play recording at the selected time via MediaMTX playback server.
  const handleSeek = (time: Date) => {
    if (!mediamtxPath) return
    setPlaybackTime(time)

    const startISO = time.toISOString()
    // Duration in seconds — play 5 minutes from the seek point.
    const playbackUrl = `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(mediamtxPath)}&start=${encodeURIComponent(startISO)}&duration=300`

    if (videoRef.current) {
      videoRef.current.src = playbackUrl
      videoRef.current.play().catch(() => {})
    }
  }

  if (camerasLoading) return <div className="text-nvr-text-secondary">Loading...</div>

  return (
    <div>
      <h1 className="text-2xl font-bold text-nvr-text-primary mb-6">Recordings</h1>

      <div className="flex items-center gap-3 mb-6">
        <select
          value={selectedCamera || ''}
          onChange={e => { setSelectedCamera(e.target.value || null); setPlaybackTime(null) }}
          className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        >
          <option value="">Select Camera</option>
          {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <input
          type="date"
          value={date}
          onChange={e => { setDate(e.target.value); setPlaybackTime(null) }}
          className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        />
        {selectedCamera && (
          <span className="text-sm text-nvr-text-muted">
            {segments.length} segment{segments.length !== 1 ? 's' : ''} found
          </span>
        )}
      </div>

      {selectedCamera && (
        <>
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5 mb-6">
            <Timeline ranges={timelineRanges} date={date} onSeek={handleSeek} />
            {loadingSegments && <p className="text-nvr-text-muted text-sm mt-2">Loading recordings...</p>}
            {!loadingSegments && segments.length === 0 && (
              <p className="text-nvr-text-muted text-sm mt-2">No recordings found for this date.</p>
            )}
          </div>

          {playbackTime && (
            <div className="mb-6">
              <div className="text-sm text-nvr-text-secondary mb-2">
                Playing from: {playbackTime.toLocaleTimeString()}
              </div>
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
                <video
                  ref={videoRef}
                  controls
                  autoPlay
                  className="w-full max-h-[70vh]"
                />
              </div>
            </div>
          )}

          {segments.length > 0 && (
            <div>
              <h3 className="text-lg font-semibold text-nvr-text-primary mb-3">Segments</h3>
              <div className="flex flex-col gap-1.5">
                {segments.map((s, i) => {
                  const t = new Date(s.start)
                  return (
                    <div
                      key={i}
                      onClick={() => handleSeek(t)}
                      className="flex items-center justify-between px-4 py-3 rounded-lg cursor-pointer border border-nvr-border bg-nvr-bg-secondary hover:bg-nvr-bg-tertiary transition-colors"
                    >
                      <span className="text-sm text-nvr-text-primary">{t.toLocaleTimeString()}</span>
                      <button
                        onClick={(e) => { e.stopPropagation(); handleSeek(t) }}
                        className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-sm"
                      >
                        Play
                      </button>
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </>
      )}

      {!selectedCamera && cameras.length > 0 && (
        <p className="text-center py-12 text-nvr-text-muted">Select a camera to view its recordings.</p>
      )}
      {cameras.length === 0 && (
        <p className="text-center py-12 text-nvr-text-muted">No cameras configured. Add cameras first.</p>
      )}
    </div>
  )
}
