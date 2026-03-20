import { useState, useEffect, useRef } from 'react'
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

function formatShortTime(d: Date): string {
  return d.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
}

function estimateDuration(segments: Segment[], index: number): string {
  if (index + 1 < segments.length) {
    const start = new Date(segments[index].start).getTime()
    const end = new Date(segments[index + 1].start).getTime()
    const mins = Math.round((end - start) / 60000)
    if (mins < 60) return `${mins}m`
    const h = Math.floor(mins / 60)
    const m = mins % 60
    return m > 0 ? `${h}h ${m}m` : `${h}h`
  }
  return '~5m'
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
  const [selectedSegmentIndex, setSelectedSegmentIndex] = useState<number | null>(null)

  // Export popover state
  const [showExport, setShowExport] = useState(false)
  const [exportStartDate, setExportStartDate] = useState(new Date().toISOString().split('T')[0])
  const [exportEndDate, setExportEndDate] = useState(new Date().toISOString().split('T')[0])
  const [exportCamera, setExportCamera] = useState<string | null>(null)
  const [exportSegments, setExportSegments] = useState<ExportSegment[]>([])
  const [exportLoading, setExportLoading] = useState(false)
  const [exportDone, setExportDone] = useState(false)
  const [downloadingId, setDownloadingId] = useState<number | null>(null)
  const exportRef = useRef<HTMLDivElement>(null)

  const selectedCameraObj = cameras.find(c => c.id === selectedCamera)
  const mediamtxPath = selectedCameraObj?.mediamtx_path || ''

  // Close export popover on outside click
  useEffect(() => {
    if (!showExport) return
    const handler = (e: MouseEvent) => {
      if (exportRef.current && !exportRef.current.contains(e.target as Node)) {
        setShowExport(false)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [showExport])

  // Fetch recordings from MediaMTX when camera or date changes
  useEffect(() => {
    if (!mediamtxPath || !date) {
      setSegments([])
      setTimelineRanges([])
      return
    }

    setLoadingSegments(true)
    setSelectedSegmentIndex(null)

    fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${mediamtxPath}`)
      .then(res => res.ok ? res.json() : null)
      .then((data: RecordingList | null) => {
        if (!data || !data.segments) {
          setSegments([])
          setTimelineRanges([])
          return
        }

        const dayStart = new Date(date + 'T00:00:00')
        const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000)

        const filtered = data.segments.filter(s => {
          const t = new Date(s.start)
          return t >= dayStart && t < dayEnd
        })

        setSegments(filtered)

        const ranges: { start: string; end: string }[] = []
        for (let i = 0; i < filtered.length; i++) {
          const start = filtered[i].start
          const end = i + 1 < filtered.length
            ? filtered[i + 1].start
            : new Date(new Date(start).getTime() + 5 * 60 * 1000).toISOString()
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

  const handleSeek = (time: Date) => {
    if (!mediamtxPath) return
    setPlaybackTime(time)
    const startISO = time.toISOString()
    const url = `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(mediamtxPath)}&start=${encodeURIComponent(startISO)}&duration=300`
    setPlaybackUrl(url)
  }

  const handleSegmentClick = (index: number) => {
    setSelectedSegmentIndex(index)
    const t = new Date(segments[index].start)
    handleSeek(t)
  }

  const handleDownload = async (segmentId: number) => {
    setDownloadingId(segmentId)
    try {
      const link = document.createElement('a')
      link.href = `/api/nvr/recordings/${segmentId}/download`
      link.download = ''
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      await new Promise(resolve => setTimeout(resolve, 1000))
    } finally {
      setDownloadingId(null)
    }
  }

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

  const isToday = date === new Date().toISOString().split('T')[0]

  if (camerasLoading) {
    return (
      <div className="flex items-center justify-center py-20">
        <span className="inline-block w-5 h-5 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-3" />
        <span className="text-nvr-text-secondary">Loading cameras...</span>
      </div>
    )
  }

  return (
    <div>
      {/* Top bar: camera selector, date nav, export button */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-3 mb-6">
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <select
            value={selectedCamera || ''}
            onChange={e => {
              setSelectedCamera(e.target.value || null)
              setPlaybackTime(null)
              setPlaybackUrl(null)
              setSelectedSegmentIndex(null)
            }}
            className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors min-h-[44px] w-full sm:w-48"
          >
            <option value="">Select Camera</option>
            {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>

          <div className="flex items-center gap-1 shrink-0">
            <button
              onClick={() => { setDate(shiftDate(date, -1)); setPlaybackTime(null); setPlaybackUrl(null); setSelectedSegmentIndex(null) }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg p-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center"
              title="Previous day"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="15 18 9 12 15 6" /></svg>
            </button>
            <input
              type="date"
              value={date}
              onChange={e => { setDate(e.target.value); setPlaybackTime(null); setPlaybackUrl(null); setSelectedSegmentIndex(null) }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors min-h-[44px]"
            />
            <button
              onClick={() => { setDate(shiftDate(date, 1)); setPlaybackTime(null); setPlaybackUrl(null); setSelectedSegmentIndex(null) }}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg p-2 text-nvr-text-primary hover:bg-nvr-bg-tertiary transition-colors min-h-[44px] min-w-[44px] flex items-center justify-center"
              title="Next day"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
            </button>
            {!isToday && (
              <button
                onClick={() => { setDate(new Date().toISOString().split('T')[0]); setPlaybackTime(null); setPlaybackUrl(null); setSelectedSegmentIndex(null) }}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px]"
              >
                Today
              </button>
            )}
          </div>
        </div>

        {/* Export dropdown */}
        {cameras.length > 0 && (
          <div className="relative shrink-0" ref={exportRef}>
            <button
              onClick={() => setShowExport(!showExport)}
              className="bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-secondary hover:bg-nvr-bg-tertiary transition-colors text-sm min-h-[44px] inline-flex items-center gap-2"
            >
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
              Export
            </button>

            {showExport && (
              <div className="absolute right-0 top-full mt-2 w-80 bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl z-50 p-4">
                <h3 className="text-sm font-semibold text-nvr-text-primary mb-3">Export Recordings</h3>
                <p className="text-xs text-nvr-text-muted mb-3">Search across multiple days and download segments.</p>

                <div className="space-y-2 mb-3">
                  <select
                    value={exportCamera || ''}
                    onChange={e => setExportCamera(e.target.value || null)}
                    className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary text-sm focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                  >
                    <option value="">Select Camera</option>
                    {cameras.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
                  </select>
                  <div className="grid grid-cols-2 gap-2">
                    <div>
                      <label className="block text-xs text-nvr-text-muted mb-1">From</label>
                      <input
                        type="date"
                        value={exportStartDate}
                        onChange={e => setExportStartDate(e.target.value)}
                        className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-2 py-1.5 text-nvr-text-primary text-sm focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                      />
                    </div>
                    <div>
                      <label className="block text-xs text-nvr-text-muted mb-1">To</label>
                      <input
                        type="date"
                        value={exportEndDate}
                        onChange={e => setExportEndDate(e.target.value)}
                        className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-2 py-1.5 text-nvr-text-primary text-sm focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
                      />
                    </div>
                  </div>
                  <button
                    onClick={handleExport}
                    disabled={!exportCamera || exportLoading}
                    className="w-full bg-nvr-accent hover:bg-nvr-accent-hover disabled:opacity-50 text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm inline-flex items-center justify-center gap-2"
                  >
                    {exportLoading && <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />}
                    {exportLoading ? 'Searching...' : 'Search'}
                  </button>
                </div>

                {exportDone && (
                  <div className="border-t border-nvr-border pt-3">
                    {exportSegments.length === 0 ? (
                      <p className="text-xs text-nvr-text-muted text-center py-2">No recordings found.</p>
                    ) : (
                      <div className="max-h-48 overflow-y-auto space-y-1">
                        <p className="text-xs text-nvr-text-secondary mb-1">{exportSegments.length} segment{exportSegments.length !== 1 ? 's' : ''}</p>
                        {exportSegments.map(seg => (
                          <div
                            key={seg.id}
                            className="flex items-center justify-between px-2 py-1.5 rounded-lg bg-nvr-bg-primary text-xs group"
                          >
                            <div className="min-w-0">
                              <span className="text-nvr-text-primary truncate block">
                                {new Date(seg.start_time).toLocaleString([], { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })}
                              </span>
                              {seg.file_size > 0 && (
                                <span className="text-nvr-text-muted">{(seg.file_size / (1024 * 1024)).toFixed(1)} MB</span>
                              )}
                            </div>
                            <button
                              onClick={() => handleDownload(seg.id)}
                              disabled={downloadingId === seg.id}
                              className="text-nvr-text-muted hover:text-nvr-accent transition-colors p-1 disabled:opacity-50"
                              title="Download"
                            >
                              {downloadingId === seg.id ? (
                                <span className="inline-block w-4 h-4 border-2 border-nvr-text-muted/30 border-t-nvr-text-primary rounded-full animate-spin" />
                              ) : (
                                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" /><polyline points="7 10 12 15 17 10" /><line x1="12" y1="15" x2="12" y2="3" /></svg>
                              )}
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
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
          {/* Timeline - always visible, full width */}
          <div className="mb-6">
            <Timeline
              ranges={timelineRanges}
              date={date}
              onSeek={(time) => {
                setSelectedSegmentIndex(null)
                handleSeek(time)
              }}
              playbackTime={playbackTime}
            />
            {loadingSegments && (
              <div className="flex items-center gap-2 mt-2">
                <span className="inline-block w-3 h-3 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                <span className="text-nvr-text-muted text-sm">Loading recordings...</span>
              </div>
            )}
          </div>

          {/* Empty state for no recordings */}
          {!loadingSegments && segments.length === 0 && (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 text-nvr-text-muted/40 mb-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round"><rect x="3" y="4" width="18" height="18" rx="2" ry="2" /><line x1="16" y1="2" x2="16" y2="6" /><line x1="8" y1="2" x2="8" y2="6" /><line x1="3" y1="10" x2="21" y2="10" /></svg>
              <p className="text-nvr-text-secondary mb-1">No recordings on this date</p>
              <p className="text-nvr-text-muted text-sm">Try a different date or check that recording is enabled</p>
            </div>
          )}

          {/* Video player - shows when user clicks timeline or segment */}
          {playbackTime && playbackUrl && (
            <div className="mb-6">
              <div className="flex items-center gap-2 mb-2">
                <span className="text-sm text-nvr-text-secondary">
                  Playing from {playbackTime.toLocaleTimeString()}
                </span>
                <button
                  onClick={() => { setPlaybackTime(null); setPlaybackUrl(null); setSelectedSegmentIndex(null) }}
                  className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors"
                >
                  Close
                </button>
              </div>
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
                <VideoPlayer src={playbackUrl} />
              </div>
            </div>
          )}

          {/* Segment chips grid */}
          {segments.length > 0 && (
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-medium text-nvr-text-secondary">
                  {segments.length} segment{segments.length !== 1 ? 's' : ''}
                </h3>
              </div>
              <div className="flex flex-wrap gap-2">
                {segments.map((s, i) => {
                  const t = new Date(s.start)
                  const isSelected = selectedSegmentIndex === i
                  return (
                    <button
                      key={i}
                      onClick={() => handleSegmentClick(i)}
                      className={`group relative flex flex-col items-center px-3 py-2 rounded-lg border transition-colors text-center min-w-[80px] ${
                        isSelected
                          ? 'bg-nvr-accent/15 border-nvr-accent text-nvr-accent'
                          : 'bg-nvr-bg-secondary border-nvr-border text-nvr-text-primary hover:bg-nvr-bg-tertiary hover:border-nvr-border'
                      }`}
                    >
                      <span className="text-sm font-medium">{formatShortTime(t)}</span>
                      <span className={`text-xs mt-0.5 ${isSelected ? 'text-nvr-accent/70' : 'text-nvr-text-muted'}`}>
                        {estimateDuration(segments, i)}
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
