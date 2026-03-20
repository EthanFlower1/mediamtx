import { useState, useEffect } from 'react'
import { useCameras } from '../hooks/useCameras'
import { apiFetch } from '../api/client'
import Timeline from '../components/Timeline'
import VideoPlayer from '../components/VideoPlayer'

interface Segment {
  start: string
}

interface RecordingList {
  name: string
  segments: Segment[]
}

interface ExportSegment {
  id: number
  start_time: string
  end_time: string
  file_size: number
  url: string
}

interface ExportResponse {
  segments: ExportSegment[]
  count: number
}

function shiftDate(dateStr: string, days: number): string {
  const d = new Date(dateStr + 'T00:00:00')
  d.setDate(d.getDate() + days)
  return d.toISOString().split('T')[0]
}

export default function Recordings() {
  const { cameras, loading: camerasLoading } = useCameras()
  const [selectedCamera, setSelectedCamera] = useState<string | null>(null)
  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const [segments, setSegments] = useState<Segment[]>([])
  const [timelineRanges, setTimelineRanges] = useState<{ start: string; end: string }[]>([])
  const [playbackTime, setPlaybackTime] = useState<Date | null>(null)
  const [playbackUrl, setPlaybackUrl] = useState<string | null>(null)
  const [loadingSegments, setLoadingSegments] = useState(false)

  // Export state.
  const [exportStartDate, setExportStartDate] = useState(new Date().toISOString().split('T')[0])
  const [exportEndDate, setExportEndDate] = useState(new Date().toISOString().split('T')[0])
  const [exportCamera, setExportCamera] = useState<string | null>(null)
  const [exportSegments, setExportSegments] = useState<ExportSegment[]>([])
  const [exportLoading, setExportLoading] = useState(false)
  const [exportDone, setExportDone] = useState(false)

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
    // Duration in seconds - play 5 minutes from the seek point.
    const url = `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(mediamtxPath)}&start=${encodeURIComponent(startISO)}&duration=300`
    setPlaybackUrl(url)
  }

  // Download a recording segment by ID.
  const handleDownload = (segmentId: number) => {
    // Open the download URL in a new tab. The browser will handle the
    // Content-Disposition: attachment header from the server.
    const link = document.createElement('a')
    link.href = `/api/nvr/recordings/${segmentId}/download`
    link.download = ''
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  }

  // Run export query.
  const handleExport = async () => {
    if (!exportCamera) return
    setExportLoading(true)
    setExportDone(false)
    setExportSegments([])

    const start = exportStartDate + 'T00:00:00Z'
    const end = exportEndDate + 'T23:59:59Z'

    try {
      const res = await apiFetch('/recordings/export', {
        method: 'POST',
        body: JSON.stringify({ camera_id: exportCamera, start, end }),
      })
      if (res.ok) {
        const data: ExportResponse = await res.json()
        setExportSegments(data.segments || [])
        setExportDone(true)
      }
    } catch {
      // ignore
    } finally {
      setExportLoading(false)
    }
  }

  if (camerasLoading) return <div className="text-nvr-text-secondary">Loading...</div>

  return (
    <div>
      <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary mb-4 md:mb-6">Recordings</h1>

      <div className="flex flex-col sm:flex-row sm:items-center gap-2 md:gap-3 mb-4 md:mb-6 flex-wrap">
        <select
          value={selectedCamera || ''}
          onChange={e => { setSelectedCamera(e.target.value || null); setPlaybackTime(null); setPlaybackUrl(null) }}
          className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors w-full sm:w-auto min-h-[44px]"
        >
          <option value="">Select Camera</option>
          {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>

        {/* Date navigation with prev/next arrows */}
        <div className="flex items-center gap-1">
          <button
            onClick={() => { setDate(shiftDate(date, -1)); setPlaybackTime(null); setPlaybackUrl(null) }}
            className="bg-nvr-bg-input border border-nvr-border rounded-lg px-2.5 py-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center"
            title="Previous day"
          >
            &larr;
          </button>
          <input
            type="date"
            value={date}
            onChange={e => { setDate(e.target.value); setPlaybackTime(null); setPlaybackUrl(null) }}
            className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors min-h-[44px] flex-1 sm:flex-none"
          />
          <button
            onClick={() => { setDate(shiftDate(date, 1)); setPlaybackTime(null); setPlaybackUrl(null) }}
            className="bg-nvr-bg-input border border-nvr-border rounded-lg px-2.5 py-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center"
            title="Next day"
          >
            &rarr;
          </button>
          <button
            onClick={() => { setDate(new Date().toISOString().split('T')[0]); setPlaybackTime(null); setPlaybackUrl(null) }}
            className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px]"
            title="Jump to today"
          >
            Today
          </button>
        </div>

        {selectedCamera && (
          <span className="text-sm text-nvr-text-muted">
            {segments.length} segment{segments.length !== 1 ? 's' : ''} found
          </span>
        )}
      </div>

      {selectedCamera && (
        <>
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-3 md:p-5 mb-4 md:mb-6">
            <Timeline ranges={timelineRanges} date={date} onSeek={handleSeek} />
            {loadingSegments && <p className="text-nvr-text-muted text-sm mt-2">Loading recordings...</p>}
            {!loadingSegments && segments.length === 0 && (
              <p className="text-nvr-text-muted text-sm mt-2">No recordings found for this date.</p>
            )}
          </div>

          {playbackTime && playbackUrl && (
            <div className="mb-4 md:mb-6">
              <div className="text-sm text-nvr-text-secondary mb-2">
                Playing from: {playbackTime.toLocaleTimeString()}
              </div>
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden w-full">
                <VideoPlayer src={playbackUrl} />
              </div>
            </div>
          )}

          {segments.length > 0 && (
            <div>
              <h3 className="text-base md:text-lg font-semibold text-nvr-text-primary mb-3">Segments</h3>
              <div className="flex flex-col gap-1.5">
                {segments.map((s, i) => {
                  const t = new Date(s.start)
                  return (
                    <div
                      key={i}
                      onClick={() => handleSeek(t)}
                      className="flex items-center justify-between px-3 md:px-4 py-3 rounded-lg cursor-pointer border border-nvr-border bg-nvr-bg-secondary hover:bg-nvr-bg-tertiary transition-colors"
                    >
                      <span className="text-sm text-nvr-text-primary">{t.toLocaleTimeString()}</span>
                      <div className="flex items-center gap-2">
                        <button
                          onClick={(e) => { e.stopPropagation(); handleSeek(t) }}
                          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-sm min-h-[44px]"
                        >
                          Play
                        </button>
                      </div>
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

      {/* Export Section */}
      {cameras.length > 0 && (
        <div className="mt-6 md:mt-8 bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 md:p-5">
          <h3 className="text-base md:text-lg font-semibold text-nvr-text-primary mb-4">Export Recordings</h3>
          <p className="text-sm text-nvr-text-muted mb-4">
            Search across multiple days and download recording segments.
          </p>
          <div className="flex flex-col sm:flex-row sm:items-end gap-2 md:gap-3 flex-wrap">
            <div className="w-full sm:w-auto">
              <label className="block text-xs text-nvr-text-muted mb-1">Camera</label>
              <select
                value={exportCamera || ''}
                onChange={e => setExportCamera(e.target.value || null)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors w-full sm:w-auto min-h-[44px]"
              >
                <option value="">Select Camera</option>
                {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
              </select>
            </div>
            <div className="w-full sm:w-auto">
              <label className="block text-xs text-nvr-text-muted mb-1">Start Date</label>
              <input
                type="date"
                value={exportStartDate}
                onChange={e => setExportStartDate(e.target.value)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors w-full sm:w-auto min-h-[44px]"
              />
            </div>
            <div className="w-full sm:w-auto">
              <label className="block text-xs text-nvr-text-muted mb-1">End Date</label>
              <input
                type="date"
                value={exportEndDate}
                onChange={e => setExportEndDate(e.target.value)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors w-full sm:w-auto min-h-[44px]"
              />
            </div>
            <button
              onClick={handleExport}
              disabled={!exportCamera || exportLoading}
              className="bg-nvr-accent hover:bg-nvr-accent-hover disabled:opacity-50 text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] w-full sm:w-auto"
            >
              {exportLoading ? 'Searching...' : 'Export'}
            </button>
          </div>

          {exportDone && (
            <div className="mt-4">
              {exportSegments.length === 0 ? (
                <p className="text-sm text-nvr-text-muted">No recordings found for the selected range.</p>
              ) : (
                <>
                  <p className="text-sm text-nvr-text-secondary mb-2">
                    {exportSegments.length} segment{exportSegments.length !== 1 ? 's' : ''} found
                  </p>
                  <div className="flex flex-col gap-1.5 max-h-64 overflow-y-auto">
                    {exportSegments.map(seg => (
                      <div
                        key={seg.id}
                        className="flex flex-col sm:flex-row sm:items-center justify-between px-3 md:px-4 py-2.5 rounded-lg border border-nvr-border bg-nvr-bg-primary gap-2"
                      >
                        <div className="flex flex-col min-w-0">
                          <span className="text-xs md:text-sm text-nvr-text-primary truncate">
                            {new Date(seg.start_time).toLocaleString()} &mdash; {new Date(seg.end_time).toLocaleTimeString()}
                          </span>
                          {seg.file_size > 0 && (
                            <span className="text-xs text-nvr-text-muted">
                              {(seg.file_size / (1024 * 1024)).toFixed(1)} MB
                            </span>
                          )}
                        </div>
                        <button
                          onClick={() => handleDownload(seg.id)}
                          className="bg-nvr-bg-input border border-nvr-border hover:bg-nvr-bg-tertiary text-nvr-text-primary font-medium px-3 py-1.5 rounded-lg transition-colors text-sm min-h-[44px] shrink-0 self-end sm:self-center"
                        >
                          Download
                        </button>
                      </div>
                    ))}
                  </div>
                </>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
