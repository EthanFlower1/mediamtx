import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { useCameras } from '../hooks/useCameras'
import { apiFetch } from '../api/client'
import { MotionEvent, eventEmoji } from '../components/Timeline'
import { pushToast } from '../components/Toast'

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function toLocalRFC3339(d: Date): string {
  const pad = (n: number) => n.toString().padStart(2, '0')
  const offset = -d.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const absOffset = Math.abs(offset)
  const offH = pad(Math.floor(absOffset / 60))
  const offM = pad(absOffset % 60)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}${sign}${offH}:${offM}`
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

function formatDurationShort(ms: number): string {
  const totalSecs = Math.round(ms / 1000)
  const m = Math.floor(totalSecs / 60)
  const s = totalSecs % 60
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function thumbnailUrl(path: string | undefined): string | null {
  if (!path) return null
  const filename = path.split('/').pop()
  return filename ? `/thumbnails/${filename}` : null
}

/** Badge color for event type. */
function eventBadgeColor(ev: MotionEvent): string {
  if (ev.event_type === 'tampering') return 'bg-red-500/20 text-red-400 border-red-500/30'
  switch (ev.object_class) {
    case 'person':  return 'bg-blue-500/20 text-blue-400 border-blue-500/30'
    case 'vehicle': return 'bg-amber-500/20 text-amber-400 border-amber-500/30'
    case 'animal':  return 'bg-green-500/20 text-green-400 border-green-500/30'
    default:        return 'bg-purple-500/20 text-purple-400 border-purple-500/30'
  }
}

/** Position (0-1) of a time within the day. */
function dayFraction(d: Date): number {
  return (d.getHours() * 3600 + d.getMinutes() * 60 + d.getSeconds()) / 86400
}

interface SavedClip {
  id: string
  camera_id: string
  name: string
  start_time: string
  end_time: string
  tags: string
  notes: string
  created_at: string
}

interface SearchResult extends MotionEvent {
  camera_id: string
  camera_name: string
}

type SortMode = 'newest' | 'oldest' | 'longest'

const EVENT_TYPES = [
  { value: '', label: 'All Events' },
  { value: 'motion', label: 'Motion' },
  { value: 'person', label: 'Person' },
  { value: 'vehicle', label: 'Vehicle' },
  { value: 'animal', label: 'Animal' },
  { value: 'tampering', label: 'Tampering' },
]

const PAGE_SIZE = 20

/* ------------------------------------------------------------------ */
/*  Timeline Mini-Preview                                              */
/* ------------------------------------------------------------------ */
function TimelineMini({ startTime, endTime }: { startTime: Date; endTime: Date | null }) {
  const startFrac = dayFraction(startTime)
  const endFrac = endTime ? dayFraction(endTime) : Math.min(startFrac + 0.003, 1)
  const width = Math.max(endFrac - startFrac, 0.003) // min visible width

  return (
    <div className="w-full h-1.5 bg-nvr-bg-input rounded-full overflow-hidden relative" title={`${startTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })} in the day`}>
      {/* Hour markers */}
      {[6, 12, 18].map(h => (
        <div key={h} className="absolute top-0 bottom-0 w-px bg-nvr-border/40" style={{ left: `${(h / 24) * 100}%` }} />
      ))}
      {/* Event bar */}
      <div
        className="absolute top-0 bottom-0 bg-nvr-accent rounded-full min-w-[3px]"
        style={{
          left: `${startFrac * 100}%`,
          width: `${width * 100}%`,
        }}
      />
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Search Result Card                                                 */
/* ------------------------------------------------------------------ */
interface ResultCardProps {
  result: SearchResult
  onPlay: () => void
  onPlayInPlayback: () => void
  onSave: () => void
  onDownload: () => void
}

function ResultCard({ result, onPlay, onPlayInPlayback, onSave, onDownload }: ResultCardProps) {
  const [hoverThumb, setHoverThumb] = useState(false)
  const startTime = new Date(result.started_at)
  const endTime = result.ended_at ? new Date(result.ended_at) : null
  const duration = endTime ? endTime.getTime() - startTime.getTime() : 0
  const { emoji, label } = eventEmoji(result)
  const thumbSrc = thumbnailUrl(result.thumbnail_path)
  const badgeColor = eventBadgeColor(result)

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden hover:border-nvr-accent/40 transition-all group relative">
      {/* Card body: horizontal layout */}
      <div className="flex">
        {/* Left: Thumbnail */}
        <div
          className="relative w-40 min-w-[160px] shrink-0"
          onMouseEnter={() => setHoverThumb(true)}
          onMouseLeave={() => setHoverThumb(false)}
        >
          {thumbSrc ? (
            <img
              src={thumbSrc}
              alt="Event thumbnail"
              className="w-full h-full object-cover aspect-video"
              onError={(e) => {
                const el = e.target as HTMLImageElement
                el.style.display = 'none'
                const sib = el.nextElementSibling as HTMLElement | null
                if (sib) sib.style.display = 'flex'
              }}
            />
          ) : null}
          {/* Fallback placeholder (shown if no thumbnail or img fails) */}
          <div
            className="absolute inset-0 bg-nvr-bg-input flex items-center justify-center aspect-video"
            style={thumbSrc ? { display: 'none' } : {}}
          >
            <span className="text-4xl select-none">{emoji}</span>
          </div>

          {/* Play overlay on hover */}
          <button
            onClick={onPlay}
            className="absolute inset-0 bg-black/40 flex items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer"
          >
            <div className="w-10 h-10 rounded-full bg-white/20 backdrop-blur flex items-center justify-center">
              <svg className="w-5 h-5 text-white ml-0.5" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
            </div>
          </button>

          {/* Hover preview tooltip */}
          {hoverThumb && thumbSrc && (
            <div className="absolute z-50 bottom-full left-1/2 -translate-x-1/2 mb-2 pointer-events-none">
              <div className="bg-nvr-bg-secondary border border-nvr-border rounded-lg shadow-2xl overflow-hidden">
                <img src={thumbSrc} alt="Preview" className="w-72 h-auto" />
              </div>
            </div>
          )}
        </div>

        {/* Right: Event details */}
        <div className="flex-1 min-w-0 p-3 flex flex-col justify-between">
          {/* Top row: camera name + badge */}
          <div>
            <p className="text-sm font-semibold text-nvr-text-primary truncate">{result.camera_name}</p>
            <div className="flex items-center gap-2 mt-1 flex-wrap">
              <span className={`inline-flex items-center gap-1 text-[11px] font-medium px-2 py-0.5 rounded-full border ${badgeColor}`}>
                <span>{emoji}</span>
                <span>{label}</span>
              </span>
              {result.confidence != null && result.confidence > 0 && (
                <span className="text-[11px] font-medium text-nvr-text-muted bg-nvr-bg-input px-1.5 py-0.5 rounded">
                  {Math.round(result.confidence * 100)}%
                </span>
              )}
            </div>
          </div>

          {/* Middle: time + duration */}
          <div className="flex items-center gap-2 text-xs text-nvr-text-muted mt-2">
            <span>
              {startTime.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })},{' '}
              {startTime.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit', hour12: true })}
            </span>
            {duration > 0 && (
              <span className="inline-flex items-center bg-nvr-bg-input text-nvr-text-secondary text-[11px] font-medium px-1.5 py-0.5 rounded">
                {formatDurationShort(duration)}
              </span>
            )}
          </div>

          {/* Timeline mini-bar */}
          <div className="mt-2">
            <TimelineMini startTime={startTime} endTime={endTime} />
          </div>
        </div>
      </div>

      {/* Bottom: action buttons */}
      <div className="flex items-center border-t border-nvr-border/50">
        <button
          onClick={onPlay}
          className="flex-1 flex items-center justify-center gap-1.5 text-xs font-medium text-nvr-text-secondary hover:text-white hover:bg-nvr-accent py-2 transition-colors"
          title="Play"
        >
          <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24">
            <path d="M8 5v14l11-7z" />
          </svg>
          Play
        </button>
        <div className="w-px h-5 bg-nvr-border/50" />
        <button
          onClick={onPlayInPlayback}
          className="flex-1 flex items-center justify-center gap-1.5 text-xs font-medium text-nvr-text-secondary hover:text-nvr-accent hover:bg-nvr-bg-tertiary py-2 transition-colors"
          title="Open in multi-camera Playback page"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
          </svg>
          Playback
        </button>
        <div className="w-px h-5 bg-nvr-border/50" />
        <button
          onClick={onDownload}
          className="flex-1 flex items-center justify-center gap-1.5 text-xs font-medium text-nvr-text-secondary hover:text-nvr-text-primary hover:bg-nvr-bg-tertiary py-2 transition-colors"
          title="Download clip"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
          Download
        </button>
        <div className="w-px h-5 bg-nvr-border/50" />
        <button
          onClick={onSave}
          className="flex-1 flex items-center justify-center gap-1.5 text-xs font-medium text-nvr-text-secondary hover:text-nvr-text-primary hover:bg-nvr-bg-tertiary py-2 transition-colors"
          title="Save clip bookmark"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
          </svg>
          Save
        </button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Saved Clip Row                                                     */
/* ------------------------------------------------------------------ */
interface SavedClipRowProps {
  clip: SavedClip
  cameraName: string
  onPlay: () => void
  onPlayInPlayback: () => void
  onDownload: () => void
  onDelete: () => void
}

function SavedClipRow({ clip, cameraName, onPlay, onPlayInPlayback, onDownload, onDelete }: SavedClipRowProps) {
  const startTime = new Date(clip.start_time)
  const endTime = new Date(clip.end_time)
  const duration = endTime.getTime() - startTime.getTime()
  const tags = clip.tags ? clip.tags.split(',').map(t => t.trim()).filter(Boolean) : []

  return (
    <div className="flex items-center gap-3 py-3 px-4 border-b border-nvr-border last:border-b-0 hover:bg-nvr-bg-tertiary/30 transition-colors group">
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-nvr-text-primary truncate">{clip.name}</p>
        <div className="flex items-center gap-2 text-xs text-nvr-text-muted mt-0.5">
          <span>{cameraName}</span>
          <span className="text-nvr-border">|</span>
          <span>{startTime.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</span>
          <span>{startTime.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
          <span className="text-nvr-border">|</span>
          <span>{formatDuration(duration)}</span>
        </div>
        {tags.length > 0 && (
          <div className="flex items-center gap-1 mt-1">
            {tags.map(tag => (
              <span key={tag} className="text-[10px] bg-nvr-accent/10 text-nvr-accent rounded px-1.5 py-0.5">{tag}</span>
            ))}
          </div>
        )}
      </div>

      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={onPlay}
          className="min-w-[36px] min-h-[36px] flex items-center justify-center rounded-lg bg-nvr-accent hover:bg-nvr-accent-hover text-white transition-colors"
          title="Play"
        >
          <svg className="w-3.5 h-3.5" fill="currentColor" viewBox="0 0 24 24">
            <path d="M8 5v14l11-7z" />
          </svg>
        </button>
        <button
          onClick={onPlayInPlayback}
          className="min-w-[36px] min-h-[36px] flex items-center justify-center rounded-lg bg-nvr-bg-tertiary hover:bg-nvr-bg-input text-nvr-text-secondary hover:text-nvr-accent transition-colors"
          title="Open in Playback"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
          </svg>
        </button>
        <button
          onClick={onDownload}
          className="min-w-[36px] min-h-[36px] flex items-center justify-center rounded-lg bg-nvr-bg-tertiary hover:bg-nvr-bg-input text-nvr-text-secondary hover:text-nvr-text-primary transition-colors"
          title="Download"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
          </svg>
        </button>
        <button
          onClick={onDelete}
          className="min-w-[36px] min-h-[36px] flex items-center justify-center rounded-lg bg-nvr-bg-tertiary hover:bg-nvr-danger/20 text-nvr-text-secondary hover:text-nvr-danger transition-colors"
          title="Delete"
        >
          <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
        </button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Save Clip Modal                                                    */
/* ------------------------------------------------------------------ */
interface SaveModalProps {
  cameraId: string
  startTime: Date
  endTime: Date | null
  onClose: () => void
  onSaved: () => void
}

function SaveClipModal({ cameraId, startTime, endTime, onClose, onSaved }: SaveModalProps) {
  const [name, setName] = useState('')
  const [tags, setTags] = useState('')
  const [notes, setNotes] = useState('')
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    if (!name.trim()) return
    setSaving(true)
    try {
      const clipEndTime = endTime ?? new Date(startTime.getTime() + 30000)
      const res = await apiFetch('/saved-clips', {
        method: 'POST',
        body: JSON.stringify({
          camera_id: cameraId,
          name: name.trim(),
          start_time: toLocalRFC3339(startTime),
          end_time: toLocalRFC3339(clipEndTime),
          tags: tags.trim(),
          notes: notes.trim(),
        }),
      })
      if (res.ok) {
        onSaved()
        onClose()
      } else {
        const data = await res.json().catch(() => ({}))
        pushToast({
          id: crypto.randomUUID(),
          type: 'error',
          title: 'Save Failed',
          message: (data as any).error || 'Server error saving clip',
          timestamp: new Date(),
        })
      }
    } catch {
      pushToast({
        id: crypto.randomUUID(),
        type: 'error',
        title: 'Save Failed',
        message: 'Failed to save clip. Please try again.',
        timestamp: new Date(),
      })
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
      <div className="relative z-10 bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl w-full max-w-md mx-4 p-6" onClick={e => e.stopPropagation()}>
        <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">Save Clip Bookmark</h3>

        <div className="space-y-3">
          <div>
            <label className="text-xs text-nvr-text-muted block mb-1">Clip Name *</label>
            <input
              type="text"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. Front door motion"
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
              autoFocus
            />
          </div>

          <div>
            <label className="text-xs text-nvr-text-muted block mb-1">Tags (comma separated)</label>
            <input
              type="text"
              value={tags}
              onChange={e => setTags(e.target.value)}
              placeholder="e.g. important, review"
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
            />
          </div>

          <div>
            <label className="text-xs text-nvr-text-muted block mb-1">Notes</label>
            <textarea
              value={notes}
              onChange={e => setNotes(e.target.value)}
              rows={2}
              placeholder="Optional notes..."
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50 resize-none"
            />
          </div>

          <div className="text-xs text-nvr-text-muted">
            <span>{startTime.toLocaleString()}</span>
            {endTime && <span> to {endTime.toLocaleString()}</span>}
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-5">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-nvr-text-secondary hover:text-nvr-text-primary bg-nvr-bg-tertiary hover:bg-nvr-bg-input rounded-lg transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !name.trim()}
            className="px-4 py-2 text-sm font-medium text-white bg-nvr-accent hover:bg-nvr-accent-hover rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {saving ? 'Saving...' : 'Save Clip'}
          </button>
        </div>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  ClipSearch Page                                                    */
/* ------------------------------------------------------------------ */
export default function ClipSearch() {
  const navigate = useNavigate()
  const { cameras, loading: camerasLoading } = useCameras()

  // Search filters
  const [selectedCameraIds, setSelectedCameraIds] = useState<string[]>([])
  const [startDate, setStartDate] = useState(() => {
    const d = new Date()
    d.setDate(d.getDate() - 7)
    return d.toISOString().split('T')[0]
  })
  const [endDate, setEndDate] = useState(new Date().toISOString().split('T')[0])
  const [eventType, setEventType] = useState('')
  const [cameraDropdownOpen, setCameraDropdownOpen] = useState(false)
  const [sortMode, setSortMode] = useState<SortMode>('newest')

  // Results
  const [results, setResults] = useState<SearchResult[]>([])
  const [searching, setSearching] = useState(false)
  const [searched, setSearched] = useState(false)
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)

  // Saved clips
  const [savedClips, setSavedClips] = useState<SavedClip[]>([])
  const [savedClipsOpen, setSavedClipsOpen] = useState(true)

  // Save modal state
  const [saveModal, setSaveModal] = useState<{ cameraId: string; startTime: Date; endTime: Date | null } | null>(null)

  // AI semantic search state
  const [semanticQuery, setSemanticQuery] = useState('')
  const [semanticResults, setSemanticResults] = useState<SearchResult[]>([])
  const [semanticSearching, setSemanticSearching] = useState(false)
  const [semanticSearched, setSemanticSearched] = useState(false)

  // Inline video player state
  const [playingClip, setPlayingClip] = useState<{
    url: string
    title: string
    cameraId: string
    startTime: Date
    durationSecs: number
  } | null>(null)
  const clipVideoRef = useRef<HTMLVideoElement | null>(null)

  // Page title
  useEffect(() => {
    document.title = 'Clips \u2014 MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  // Fetch saved clips
  const fetchSavedClips = useCallback(() => {
    apiFetch('/saved-clips')
      .then(res => res.ok ? res.json() : [])
      .then((data: SavedClip[]) => setSavedClips(data))
      .catch(() => setSavedClips([]))
  }, [])

  useEffect(() => {
    fetchSavedClips()
  }, [fetchSavedClips])

  // Camera name lookup
  const cameraNameMap = new Map(cameras.map(c => [c.id, c.name]))
  const cameraPathMap = new Map(cameras.map(c => [c.id, c.mediamtx_path]))

  // Toggle camera selection
  const toggleCamera = (id: string) => {
    setSelectedCameraIds(prev =>
      prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]
    )
  }

  // Sort results
  const sortResults = useCallback((items: SearchResult[], mode: SortMode): SearchResult[] => {
    const sorted = [...items]
    switch (mode) {
      case 'newest':
        sorted.sort((a, b) => new Date(b.started_at).getTime() - new Date(a.started_at).getTime())
        break
      case 'oldest':
        sorted.sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())
        break
      case 'longest': {
        const dur = (r: SearchResult) => {
          if (!r.ended_at) return 0
          return new Date(r.ended_at).getTime() - new Date(r.started_at).getTime()
        }
        sorted.sort((a, b) => dur(b) - dur(a))
        break
      }
    }
    return sorted
  }, [])

  // AI semantic search function
  const handleSemanticSearch = async () => {
    if (!semanticQuery.trim()) return
    setSemanticSearching(true)
    setSemanticSearched(true)
    try {
      const res = await apiFetch(`/search?q=${encodeURIComponent(semanticQuery.trim())}&limit=20`)
      if (res.ok) {
        const data = await res.json()
        // Backend search returns detection-level fields (frame_time, class, detection_id).
        // Map them to MotionEvent shape (started_at, object_class) that ResultCard expects.
        const items: SearchResult[] = (data.results || data || []).map((r: Record<string, unknown>) => ({
          ...r,
          started_at: r.frame_time as string,
          ended_at: null,
          event_type: 'ai_detection',
          object_class: r.class as string,
          camera_id: r.camera_id as string,
          camera_name: (r.camera_name as string) || cameraNameMap.get(r.camera_id as string) || 'Unknown',
          thumbnail_path: r.thumbnail_path as string,
          confidence: r.confidence as number,
        }))
        setSemanticResults(items)
      } else {
        setSemanticResults([])
      }
    } catch {
      setSemanticResults([])
    } finally {
      setSemanticSearching(false)
    }
  }

  // Search function
  const handleSearch = async () => {
    setSearching(true)
    setSearched(true)
    setVisibleCount(PAGE_SIZE)

    const camsToSearch = selectedCameraIds.length > 0
      ? cameras.filter(c => selectedCameraIds.includes(c.id))
      : cameras

    // Generate list of dates between startDate and endDate
    const dates: string[] = []
    const current = new Date(startDate + 'T00:00:00')
    const end = new Date(endDate + 'T00:00:00')
    while (current <= end) {
      dates.push(current.toISOString().split('T')[0])
      current.setDate(current.getDate() + 1)
    }

    try {
      const allResults: SearchResult[] = []

      // For each camera and each day, fetch motion events
      const promises: Promise<void>[] = []
      for (const cam of camsToSearch) {
        for (const d of dates) {
          let url = `/cameras/${cam.id}/motion-events?date=${d}`
          if (eventType === 'tampering') {
            url += '&event_type=tampering'
          } else if (eventType) {
            url += `&object_class=${encodeURIComponent(eventType)}`
          }

          promises.push(
            apiFetch(url)
              .then(res => res.ok ? res.json() : [])
              .then((events: MotionEvent[]) => {
                events.forEach(ev => {
                  allResults.push({
                    ...ev,
                    camera_id: cam.id,
                    camera_name: cam.name,
                  })
                })
              })
              .catch(() => {})
          )
        }
      }

      await Promise.all(promises)

      setResults(sortResults(allResults, sortMode))
    } catch {
      setResults([])
    } finally {
      setSearching(false)
    }
  }

  // Re-sort when sortMode changes
  useEffect(() => {
    if (results.length > 0) {
      setResults(prev => sortResults(prev, sortMode))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sortMode, sortResults])

  // Play event inline in the clip player
  const handlePlayEvent = (result: SearchResult) => {
    const path = cameraPathMap.get(result.camera_id)
    if (!path) return
    const eventStart = new Date(result.started_at)
    const eventEnd = result.ended_at ? new Date(result.ended_at) : new Date(eventStart.getTime() + 30000)
    const preRoll = 5000
    const postRoll = 5000
    const startTime = new Date(eventStart.getTime() - preRoll)
    const durationSecs = (eventEnd.getTime() - eventStart.getTime() + preRoll + postRoll) / 1000
    const startISO = toLocalRFC3339(startTime)
    const url = `${window.location.protocol}//${window.location.hostname}:9996/get?path=${encodeURIComponent(path)}&start=${encodeURIComponent(startISO)}&duration=${durationSecs}`
    setPlayingClip({
      url,
      title: `${result.camera_name} — ${eventStart.toLocaleTimeString()}`,
      cameraId: result.camera_id,
      startTime,
      durationSecs,
    })
  }

  // Download a clip segment
  const handleDownloadEvent = async (result: SearchResult) => {
    const path = cameraPathMap.get(result.camera_id)
    if (!path) return
    const startTime = new Date(result.started_at)
    const endTime = result.ended_at ? new Date(result.ended_at) : new Date(startTime.getTime() + 30000)
    const durationSecs = (endTime.getTime() - startTime.getTime()) / 1000
    const startISO = toLocalRFC3339(startTime)
    const url = `${window.location.protocol}//${window.location.hostname}:9996/get?path=${encodeURIComponent(path)}&start=${encodeURIComponent(startISO)}&duration=${durationSecs}`

    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error('Download failed')
      const blob = await res.blob()
      const blobUrl = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = blobUrl
      link.download = `clip_${result.camera_name.replace(/[^a-zA-Z0-9_-]/g, '_')}_${startTime.toISOString().replace(/[:.]/g, '-')}.mp4`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(blobUrl)
    } catch {
      window.open(url, '_blank')
    }
  }

  // Play a saved clip inline
  const handlePlaySavedClip = (clip: SavedClip) => {
    const path = cameraPathMap.get(clip.camera_id)
    if (!path) return
    const startTime = new Date(clip.start_time)
    const endTime = new Date(clip.end_time)
    const durationSecs = Math.max((endTime.getTime() - startTime.getTime()) / 1000, 10)
    const startISO = toLocalRFC3339(startTime)
    const url = `${window.location.protocol}//${window.location.hostname}:9996/get?path=${encodeURIComponent(path)}&start=${encodeURIComponent(startISO)}&duration=${durationSecs}`
    setPlayingClip({
      url,
      title: `${cameraNameMap.get(clip.camera_id) ?? 'Unknown'} — ${startTime.toLocaleTimeString()}`,
      cameraId: clip.camera_id,
      startTime,
      durationSecs,
    })
  }

  // Download a saved clip
  const handleDownloadSavedClip = async (clip: SavedClip) => {
    const path = cameraPathMap.get(clip.camera_id)
    if (!path) return
    const startTime = new Date(clip.start_time)
    const endTime = new Date(clip.end_time)
    const durationSecs = (endTime.getTime() - startTime.getTime()) / 1000
    const startISO = toLocalRFC3339(startTime)
    const url = `${window.location.protocol}//${window.location.hostname}:9996/get?path=${encodeURIComponent(path)}&start=${encodeURIComponent(startISO)}&duration=${durationSecs}`

    try {
      const res = await fetch(url)
      if (!res.ok) throw new Error('Download failed')
      const blob = await res.blob()
      const blobUrl = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = blobUrl
      link.download = `${clip.name.replace(/[^a-zA-Z0-9_-]/g, '_')}.mp4`
      document.body.appendChild(link)
      link.click()
      document.body.removeChild(link)
      URL.revokeObjectURL(blobUrl)
    } catch {
      window.open(url, '_blank')
    }
  }

  // Delete a saved clip
  const handleDeleteSavedClip = async (clipId: string) => {
    try {
      const res = await apiFetch(`/saved-clips/${clipId}`, { method: 'DELETE' })
      if (res.ok) {
        fetchSavedClips()
      } else {
        const data = await res.json().catch(() => ({}))
        pushToast({
          id: crypto.randomUUID(),
          type: 'error',
          title: 'Delete Failed',
          message: (data as any).error || 'Server error deleting clip',
          timestamp: new Date(),
        })
      }
    } catch {
      pushToast({
        id: crypto.randomUUID(),
        type: 'error',
        title: 'Delete Failed',
        message: 'Failed to delete clip. Please try again.',
        timestamp: new Date(),
      })
    }
  }

  const visibleResults = results.slice(0, visibleCount)
  const hasMore = visibleCount < results.length

  if (camerasLoading) {
    return (
      <div className="flex items-center justify-center h-96">
        <svg className="w-8 h-8 text-nvr-accent animate-spin" fill="none" viewBox="0 0 24 24">
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
        </svg>
      </div>
    )
  }

  return (
    <div>
      <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary mb-6">Clip Search</h1>

      {/* AI Semantic Search */}
      <div className="mb-6 bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
        <div className="flex items-center gap-3">
          <div className="flex-1 relative">
            <input
              type="text"
              value={semanticQuery}
              onChange={e => setSemanticQuery(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') handleSemanticSearch() }}
              placeholder="Search: 'person in red shirt', 'white car', 'dog at door'..."
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg pl-10 pr-4 py-2.5 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none"
            />
            <svg className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-nvr-text-muted" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <circle cx="11" cy="11" r="8" />
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35" />
            </svg>
          </div>
          <button
            onClick={handleSemanticSearch}
            disabled={semanticSearching || !semanticQuery.trim()}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2.5 rounded-lg transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {semanticSearching ? 'Searching...' : 'Search'}
          </button>
        </div>
        <p className="text-xs text-nvr-text-muted mt-2">
          Search by object class (person, car, dog) or natural language if AI models are installed
        </p>
      </div>

      {/* AI Search Results */}
      {semanticSearched && (
        <div className="mb-6">
          <div className="flex items-center gap-2 mb-3">
            <div className="w-1.5 h-1.5 rounded-full bg-purple-400" />
            <h2 className="text-sm font-semibold text-purple-400">AI Search Results</h2>
            {semanticResults.length > 0 && (
              <span className="text-xs text-nvr-text-muted bg-nvr-bg-input px-2 py-0.5 rounded-full">
                {semanticResults.length} match{semanticResults.length !== 1 ? 'es' : ''}
              </span>
            )}
          </div>
          {semanticSearching ? (
            <div className="flex items-center justify-center py-8">
              <svg className="w-6 h-6 text-nvr-accent animate-spin" fill="none" viewBox="0 0 24 24">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
              </svg>
            </div>
          ) : semanticResults.length === 0 ? (
            <div className="text-center py-8 bg-nvr-bg-secondary border border-nvr-border rounded-xl">
              <p className="text-sm text-nvr-text-muted">No matches found. Try different search terms.</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
              {semanticResults.map((result, idx) => (
                <div key={`semantic-${idx}`} className="relative">
                  <ResultCard
                    result={result}
                    onPlay={() => handlePlayEvent(result)}
                    onPlayInPlayback={() => {
                      localStorage.setItem('nvr-playback-camera', result.camera_id)
                      localStorage.setItem('nvr-playback-time', new Date(result.started_at).toISOString())
                      navigate('/playback')
                    }}
                    onSave={() => setSaveModal({
                      cameraId: result.camera_id,
                      startTime: new Date(result.started_at),
                      endTime: result.ended_at ? new Date(result.ended_at) : null,
                    })}
                    onDownload={() => handleDownloadEvent(result)}
                  />
                  {/* Similarity badge */}
                  {(result as SearchResult & { similarity?: number }).similarity != null && (
                    <div className="absolute top-2 right-2 bg-purple-500/90 text-white text-[10px] font-bold px-2 py-0.5 rounded-full">
                      {Math.round(((result as SearchResult & { similarity?: number }).similarity ?? 0) * 100)}% match
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* ---- Inline Video Player ---- */}
      {playingClip && (
        <div className="mb-6 bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
          <div className="flex items-center justify-between px-4 py-2 border-b border-nvr-border">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium text-nvr-text-primary">{playingClip.title}</span>
              <span className="text-xs text-nvr-text-muted bg-nvr-bg-input px-2 py-0.5 rounded">
                {formatDuration(playingClip.durationSecs * 1000)}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={() => {
                  localStorage.setItem('nvr-playback-camera', playingClip.cameraId)
                  localStorage.setItem('nvr-playback-time', playingClip.startTime.toISOString())
                  navigate('/playback')
                }}
                className="text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors flex items-center gap-1"
              >
                Open in Playback →
              </button>
              <button onClick={() => setPlayingClip(null)} className="text-nvr-text-muted hover:text-nvr-text-primary transition-colors">
                ✕
              </button>
            </div>
          </div>
          <video
            ref={clipVideoRef}
            src={playingClip.url}
            autoPlay
            controls
            className="w-full max-h-[50vh] bg-black"
          />
        </div>
      )}

      {/* ---- Search bar ---- */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 mb-6">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
          {/* Camera filter (multi-select dropdown) */}
          <div className="relative">
            <label className="text-xs text-nvr-text-muted block mb-1">Cameras</label>
            <button
              onClick={() => setCameraDropdownOpen(!cameraDropdownOpen)}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary text-left flex items-center justify-between focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
            >
              <span className="truncate">
                {selectedCameraIds.length === 0
                  ? 'All cameras'
                  : `${selectedCameraIds.length} selected`}
              </span>
              <svg className={`w-4 h-4 text-nvr-text-muted transition-transform ${cameraDropdownOpen ? 'rotate-180' : ''}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
              </svg>
            </button>

            {cameraDropdownOpen && (
              <div className="absolute z-20 top-full mt-1 left-0 right-0 bg-nvr-bg-secondary border border-nvr-border rounded-lg shadow-xl max-h-48 overflow-y-auto">
                <button
                  onClick={() => setSelectedCameraIds([])}
                  className={`w-full text-left px-3 py-2 text-xs transition-colors ${
                    selectedCameraIds.length === 0
                      ? 'text-nvr-accent bg-nvr-accent/10'
                      : 'text-nvr-text-secondary hover:bg-nvr-bg-tertiary'
                  }`}
                >
                  All cameras
                </button>
                {cameras.map(cam => (
                  <button
                    key={cam.id}
                    onClick={() => toggleCamera(cam.id)}
                    className={`w-full text-left px-3 py-2 text-xs transition-colors flex items-center gap-2 ${
                      selectedCameraIds.includes(cam.id)
                        ? 'text-nvr-accent bg-nvr-accent/10'
                        : 'text-nvr-text-secondary hover:bg-nvr-bg-tertiary'
                    }`}
                  >
                    <span className={`w-3.5 h-3.5 rounded border flex items-center justify-center flex-shrink-0 ${
                      selectedCameraIds.includes(cam.id)
                        ? 'bg-nvr-accent border-nvr-accent'
                        : 'border-nvr-border'
                    }`}>
                      {selectedCameraIds.includes(cam.id) && (
                        <svg className="w-2.5 h-2.5 text-white" fill="currentColor" viewBox="0 0 20 20">
                          <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
                        </svg>
                      )}
                    </span>
                    <span className="truncate">{cam.name}</span>
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Start date */}
          <div>
            <label className="text-xs text-nvr-text-muted block mb-1">Start Date</label>
            <input
              type="date"
              value={startDate}
              onChange={e => setStartDate(e.target.value)}
              max={endDate}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
            />
          </div>

          {/* End date */}
          <div>
            <label className="text-xs text-nvr-text-muted block mb-1">End Date</label>
            <input
              type="date"
              value={endDate}
              onChange={e => setEndDate(e.target.value)}
              min={startDate}
              max={new Date().toISOString().split('T')[0]}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
            />
          </div>

          {/* Event type */}
          <div>
            <label className="text-xs text-nvr-text-muted block mb-1">Event Type</label>
            <select
              value={eventType}
              onChange={e => setEventType(e.target.value)}
              className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
            >
              {EVENT_TYPES.map(et => (
                <option key={et.value} value={et.value}>{et.label}</option>
              ))}
            </select>
          </div>
        </div>

        <div className="flex items-center justify-between mt-3">
          <div className="flex items-center gap-3">
            <div className="text-xs text-nvr-text-muted">
              {searched && !searching && (
                <span>{results.length} result{results.length !== 1 ? 's' : ''} found</span>
              )}
            </div>
            {searched && results.length > 1 && (
              <select
                value={sortMode}
                onChange={e => setSortMode(e.target.value as SortMode)}
                className="bg-nvr-bg-input border border-nvr-border rounded-lg px-2 py-1 text-xs text-nvr-text-secondary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
              >
                <option value="newest">Newest first</option>
                <option value="oldest">Oldest first</option>
                <option value="longest">Longest duration</option>
              </select>
            )}
          </div>
          <button
            onClick={handleSearch}
            disabled={searching}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-5 py-2 rounded-lg transition-colors text-sm disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {searching ? (
              <>
                <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                </svg>
                Searching...
              </>
            ) : (
              <>
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                Search
              </>
            )}
          </button>
        </div>
      </div>

      {/* ---- Search Results ---- */}
      {searched && !searching && results.length === 0 && (
        <div className="text-center py-16">
          <div className="w-16 h-16 rounded-2xl bg-nvr-bg-tertiary flex items-center justify-center mx-auto mb-4">
            <svg className="w-8 h-8 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </div>
          <p className="text-nvr-text-secondary text-sm">No events found for your search criteria.</p>
          <p className="text-nvr-text-muted text-xs mt-1">Try adjusting your filters or date range.</p>
        </div>
      )}

      {visibleResults.length > 0 && (
        <div className="mb-8">
          <h2 className="text-sm font-semibold text-nvr-text-secondary uppercase tracking-wider mb-3">
            Search Results
          </h2>
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {visibleResults.map((result, i) => (
              <ResultCard
                key={`${result.camera_id}-${result.started_at}-${i}`}
                result={result}
                onPlay={() => handlePlayEvent(result)}
                onPlayInPlayback={() => {
                  localStorage.setItem('nvr-playback-camera', result.camera_id)
                  localStorage.setItem('nvr-playback-time', new Date(new Date(result.started_at).getTime() - 5000).toISOString())
                  navigate('/playback')
                }}
                onSave={() => setSaveModal({
                  cameraId: result.camera_id,
                  startTime: new Date(result.started_at),
                  endTime: result.ended_at ? new Date(result.ended_at) : null,
                })}
                onDownload={() => handleDownloadEvent(result)}
              />
            ))}
          </div>

          {hasMore && (
            <div className="text-center mt-4">
              <button
                onClick={() => setVisibleCount(prev => prev + PAGE_SIZE)}
                className="bg-nvr-bg-tertiary hover:bg-nvr-bg-input text-nvr-text-secondary hover:text-nvr-text-primary font-medium px-6 py-2.5 rounded-lg transition-colors text-sm"
              >
                Load More ({results.length - visibleCount} remaining)
              </button>
            </div>
          )}
        </div>
      )}

      {/* ---- Saved Clips ---- */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
        <button
          onClick={() => setSavedClipsOpen(!savedClipsOpen)}
          className="w-full flex items-center justify-between px-4 py-3 hover:bg-nvr-bg-tertiary/30 transition-colors"
        >
          <div className="flex items-center gap-2">
            <svg className="w-4 h-4 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z" />
            </svg>
            <span className="text-sm font-semibold text-nvr-text-primary">Saved Clips</span>
            {savedClips.length > 0 && (
              <span className="text-xs bg-nvr-bg-tertiary text-nvr-text-secondary rounded-full px-2 py-0.5">
                {savedClips.length}
              </span>
            )}
          </div>
          <svg className={`w-4 h-4 text-nvr-text-muted transition-transform ${savedClipsOpen ? 'rotate-0' : '-rotate-90'}`} fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
          </svg>
        </button>

        {savedClipsOpen && (
          <div className="border-t border-nvr-border">
            {savedClips.length === 0 ? (
              <div className="px-4 py-8 text-center">
                <p className="text-sm text-nvr-text-muted">No saved clips yet.</p>
                <p className="text-xs text-nvr-text-muted mt-1">Search for events and save them as clips for quick access.</p>
              </div>
            ) : (
              savedClips.map(clip => (
                <SavedClipRow
                  key={clip.id}
                  clip={clip}
                  cameraName={cameraNameMap.get(clip.camera_id) ?? 'Unknown'}
                  onPlay={() => handlePlaySavedClip(clip)}
                  onPlayInPlayback={() => {
                    localStorage.setItem('nvr-playback-camera', clip.camera_id)
                    localStorage.setItem('nvr-playback-time', clip.start_time)
                    navigate('/playback')
                  }}
                  onDownload={() => handleDownloadSavedClip(clip)}
                  onDelete={() => handleDeleteSavedClip(clip.id)}
                />
              ))
            )}
          </div>
        )}
      </div>

      {/* Save clip modal */}
      {saveModal && (
        <SaveClipModal
          cameraId={saveModal.cameraId}
          startTime={saveModal.startTime}
          endTime={saveModal.endTime}
          onClose={() => setSaveModal(null)}
          onSaved={fetchSavedClips}
        />
      )}
    </div>
  )
}
