import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useCameras, Camera } from '../hooks/useCameras'
import { apiFetch } from '../api/client'
import { MotionEvent, eventEmoji } from '../components/Timeline'

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

function formatTimeHHMMSS(d: Date): string {
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

interface Segment { start: string }
interface RecordingList { name: string; segments: Segment[] }

interface CameraRanges {
  cameraId: string
  ranges: { start: string; end: string }[]
}

/* ------------------------------------------------------------------ */
/*  Grid cols helper                                                   */
/* ------------------------------------------------------------------ */
function gridClass(count: number): string {
  if (count <= 1) return 'grid-cols-1'
  if (count <= 2) return 'grid-cols-2'
  if (count <= 4) return 'grid-cols-2'
  if (count <= 6) return 'grid-cols-3'
  return 'grid-cols-3'
}

/* ------------------------------------------------------------------ */
/*  Camera Tile                                                        */
/* ------------------------------------------------------------------ */
interface CameraTileProps {
  camera: Camera
  playbackTime: Date | null
  playing: boolean
  speed: number
  onVideoRef: (cameraId: string, el: HTMLVideoElement | null) => void
  onRemove: () => void
}

function CameraTile({ camera, playbackTime, playing, speed, onVideoRef, onRemove }: CameraTileProps) {
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const [error, setError] = useState(false)
  // Track the last seek time as a string to avoid re-rendering on Date object identity changes.
  const lastSeekRef = useRef<string>('')

  const src = useMemo(() => {
    if (!camera.mediamtx_path || !playbackTime) return null
    const startISO = toLocalRFC3339(playbackTime)
    return `http://${window.location.hostname}:9996/get?path=${encodeURIComponent(camera.mediamtx_path)}&start=${encodeURIComponent(startISO)}&duration=86400`
  }, [camera.mediamtx_path, playbackTime])

  // Only reload video when src actually changes (new seek, not play/pause).
  useEffect(() => {
    const video = videoRef.current
    if (!video || !src) return
    // Avoid reloading if src hasn't actually changed.
    if (src === lastSeekRef.current) return
    lastSeekRef.current = src
    setError(false)
    video.src = src
    video.load()
    if (playing) {
      video.play().catch(() => {})
    }
  }, [src]) // eslint-disable-line react-hooks/exhaustive-deps

  // Play/pause sync — does NOT reload the video.
  useEffect(() => {
    const video = videoRef.current
    if (!video || !video.src) return
    if (playing) {
      video.play().catch(() => {})
    } else {
      video.pause()
    }
  }, [playing])

  // Speed sync — does NOT reload the video.
  useEffect(() => {
    const video = videoRef.current
    if (!video) return
    video.playbackRate = speed
  }, [speed])

  const handleRef = useCallback((el: HTMLVideoElement | null) => {
    videoRef.current = el
    onVideoRef(camera.id, el)
  }, [camera.id, onVideoRef])

  return (
    <div className="relative bg-black rounded-lg overflow-hidden group aspect-video">
      {/* Camera label */}
      <div className="absolute top-2 left-2 z-10 bg-black/60 backdrop-blur-sm rounded px-2 py-0.5 text-xs text-white font-medium">
        {camera.name}
      </div>

      {/* Remove button */}
      <button
        onClick={onRemove}
        className="absolute top-2 right-2 z-10 w-6 h-6 flex items-center justify-center bg-black/60 backdrop-blur-sm rounded hover:bg-nvr-danger transition-colors text-white/80 hover:text-white opacity-0 group-hover:opacity-100"
        aria-label={`Remove ${camera.name}`}
      >
        <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>

      {error && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/80 z-[5]">
          <p className="text-xs text-nvr-text-muted">No recording at this time</p>
        </div>
      )}

      {!src ? (
        <div className="w-full h-full flex items-center justify-center bg-nvr-bg-secondary">
          <p className="text-xs text-nvr-text-muted">Select a time to play</p>
        </div>
      ) : (
        <video
          ref={handleRef}
          autoPlay={playing}
          muted
          playsInline
          className="w-full h-full object-contain"
          onError={() => setError(true)}
          onPlay={() => setError(false)}
        />
      )}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Vertical Timeline                                                  */
/* ------------------------------------------------------------------ */
interface VerticalTimelineProps {
  date: string
  cameraRanges: CameraRanges[]
  events: MotionEvent[]
  playbackTime: Date | null
  onSeek: (time: Date) => void
  cameras: Camera[]
}

const COLORS = ['bg-blue-500/60', 'bg-emerald-500/60', 'bg-purple-500/60', 'bg-amber-500/60', 'bg-rose-500/60', 'bg-cyan-500/60']

function VerticalTimeline({ date, cameraRanges, events, playbackTime, onSeek, cameras }: VerticalTimelineProps) {
  const barRef = useRef<HTMLDivElement>(null)
  const TOTAL_HEIGHT = 960 // pixels for 24 hours
  const dayStart = new Date(date + 'T00:00:00')
  const dayMs = 24 * 60 * 60 * 1000

  const timeToPx = useCallback((time: Date) => {
    const ms = time.getTime() - dayStart.getTime()
    return (ms / dayMs) * TOTAL_HEIGHT
  }, [dayStart, dayMs])

  const pxToTime = useCallback((px: number) => {
    const ms = (px / TOTAL_HEIGHT) * dayMs
    return new Date(dayStart.getTime() + ms)
  }, [dayStart, dayMs])

  const handleClick = (e: React.MouseEvent<HTMLDivElement>) => {
    const rect = e.currentTarget.getBoundingClientRect()
    const y = e.clientY - rect.top + e.currentTarget.scrollTop
    const time = pxToTime(y)
    onSeek(time)
  }

  const playbackY = playbackTime ? timeToPx(playbackTime) : null

  // Auto-scroll to playback position
  useEffect(() => {
    if (playbackY !== null && barRef.current) {
      const container = barRef.current
      const containerH = container.clientHeight
      const scrollTop = container.scrollTop
      if (playbackY < scrollTop || playbackY > scrollTop + containerH) {
        container.scrollTo({ top: playbackY - containerH / 2, behavior: 'smooth' })
      }
    }
  }, [playbackY])

  return (
    <div ref={barRef} className="relative overflow-y-auto flex-1 cursor-crosshair" onClick={handleClick}>
      <div className="relative" style={{ height: `${TOTAL_HEIGHT}px` }}>
        {/* Hour grid lines */}
        {Array.from({ length: 25 }, (_, h) => {
          const y = (h / 24) * TOTAL_HEIGHT
          return (
            <div key={h} className="absolute left-0 right-0" style={{ top: `${y}px` }}>
              <div className="border-t border-nvr-border/40 w-full" />
              {h < 24 && (
                <span className="absolute -top-2.5 left-1 text-[10px] text-nvr-text-muted select-none">
                  {h.toString().padStart(2, '0')}:00
                </span>
              )}
            </div>
          )
        })}

        {/* Recording coverage bars per camera */}
        {cameraRanges.map((cr, ci) => {
          const cam = cameras.find(c => c.id === cr.cameraId)
          const colorClass = COLORS[ci % COLORS.length]
          const laneWidth = Math.max(12, 60 / Math.max(cameraRanges.length, 1))
          const left = 48 + ci * (laneWidth + 2)

          return cr.ranges.map((r, ri) => {
            const startMs = new Date(r.start).getTime() - dayStart.getTime()
            const endMs = new Date(r.end).getTime() - dayStart.getTime()
            const top = (startMs / dayMs) * TOTAL_HEIGHT
            const height = ((endMs - startMs) / dayMs) * TOTAL_HEIGHT

            return (
              <div
                key={`${ci}-${ri}`}
                className={`absolute ${colorClass} rounded-sm hover:opacity-80 transition-opacity`}
                style={{
                  top: `${top}px`,
                  height: `${Math.max(height, 2)}px`,
                  left: `${left}px`,
                  width: `${laneWidth}px`,
                }}
                title={cam?.name ?? cr.cameraId}
              />
            )
          })
        })}

        {/* Motion event markers */}
        {events.map((ev, i) => {
          const evStart = new Date(ev.started_at)
          const y = timeToPx(evStart)
          const { emoji, label } = eventEmoji(ev)
          const timeLabel = evStart.toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })

          return (
            <div
              key={i}
              className="absolute right-2 z-[8] cursor-pointer select-none group/marker"
              style={{ top: `${y}px`, transform: 'translateY(-50%)' }}
              title={`${label} at ${timeLabel}`}
              onClick={(e) => {
                e.stopPropagation()
                onSeek(new Date(evStart.getTime() - 5000))
              }}
            >
              <span className="text-xs leading-none drop-shadow-sm" role="img" aria-label={label}>
                {emoji}
              </span>
            </div>
          )
        })}

        {/* Playback position marker */}
        {playbackY !== null && playbackY >= 0 && playbackY <= TOTAL_HEIGHT && (
          <div
            className="absolute left-0 right-0 h-0.5 bg-white z-10 pointer-events-none"
            style={{ top: `${playbackY}px` }}
          >
            <div className="absolute -top-1.5 left-9 w-3 h-3 bg-white rounded-full shadow" />
          </div>
        )}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  VCR Controls                                                       */
/* ------------------------------------------------------------------ */
interface VCRControlsProps {
  playing: boolean
  speed: number
  playbackTime: Date | null
  onTogglePlay: () => void
  onSpeedChange: (speed: number) => void
  onJumpBack: () => void
  onJumpForward: () => void
}

const SPEED_OPTIONS = [1, 2, 4, 8]

function VCRControls({ playing, speed, playbackTime, onTogglePlay, onSpeedChange, onJumpBack, onJumpForward }: VCRControlsProps) {
  return (
    <div className="border-t border-nvr-border bg-nvr-bg-secondary p-3">
      {/* Current time display */}
      <div className="text-center mb-3">
        <span className="text-lg font-mono text-nvr-text-primary">
          {playbackTime ? formatTimeHHMMSS(playbackTime) : '--:--:--'}
        </span>
      </div>

      {/* Transport controls */}
      <div className="flex items-center justify-center gap-2">
        {/* Jump back 30s */}
        <button
          onClick={onJumpBack}
          className="min-w-[44px] min-h-[44px] flex items-center justify-center rounded-lg bg-nvr-bg-tertiary hover:bg-nvr-bg-input text-nvr-text-secondary hover:text-nvr-text-primary transition-colors"
          aria-label="Jump back 30 seconds"
          title="Jump back 30s"
        >
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12.066 11.2a1 1 0 000 1.6l5.334 4A1 1 0 0019 16V8a1 1 0 00-1.6-.8l-5.333 4zM4.066 11.2a1 1 0 000 1.6l5.334 4A1 1 0 0011 16V8a1 1 0 00-1.6-.8l-5.334 4z" />
          </svg>
        </button>

        {/* Play / Pause */}
        <button
          onClick={onTogglePlay}
          className="min-w-[52px] min-h-[52px] flex items-center justify-center rounded-xl bg-nvr-accent hover:bg-nvr-accent-hover text-white transition-colors shadow-lg"
          aria-label={playing ? 'Pause' : 'Play'}
        >
          {playing ? (
            <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 24 24">
              <path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
            </svg>
          ) : (
            <svg className="w-6 h-6" fill="currentColor" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" />
            </svg>
          )}
        </button>

        {/* Jump forward 30s */}
        <button
          onClick={onJumpForward}
          className="min-w-[44px] min-h-[44px] flex items-center justify-center rounded-lg bg-nvr-bg-tertiary hover:bg-nvr-bg-input text-nvr-text-secondary hover:text-nvr-text-primary transition-colors"
          aria-label="Jump forward 30 seconds"
          title="Jump forward 30s"
        >
          <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M11.933 12.8a1 1 0 000-1.6L6.6 7.2A1 1 0 005 8v8a1 1 0 001.6.8l5.333-4zM19.933 12.8a1 1 0 000-1.6l-5.333-4A1 1 0 0013 8v8a1 1 0 001.6.8l5.333-4z" />
          </svg>
        </button>
      </div>

      {/* Speed selector */}
      <div className="flex items-center justify-center gap-1 mt-3">
        <span className="text-xs text-nvr-text-muted mr-1">Speed:</span>
        {SPEED_OPTIONS.map(s => (
          <button
            key={s}
            onClick={() => onSpeedChange(s)}
            className={`px-2.5 py-1 text-xs font-medium rounded transition-colors min-h-[32px] ${
              speed === s
                ? 'bg-nvr-accent text-white'
                : 'bg-nvr-bg-tertiary text-nvr-text-secondary hover:text-nvr-text-primary hover:bg-nvr-bg-input'
            }`}
          >
            {s}x
          </button>
        ))}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Camera Chip (draggable)                                            */
/* ------------------------------------------------------------------ */
interface CameraChipProps {
  camera: Camera
  isSelected: boolean
  onAdd: () => void
}

function CameraChip({ camera, isSelected, onAdd }: CameraChipProps) {
  const handleDragStart = (e: React.DragEvent) => {
    e.dataTransfer.setData('camera-id', camera.id)
    e.dataTransfer.effectAllowed = 'copy'
  }

  return (
    <button
      draggable
      onDragStart={handleDragStart}
      onClick={onAdd}
      disabled={isSelected}
      className={`flex items-center gap-2 px-3 py-2 rounded-lg text-xs font-medium transition-colors text-left w-full ${
        isSelected
          ? 'bg-nvr-accent/10 text-nvr-accent border border-nvr-accent/30 cursor-default'
          : 'bg-nvr-bg-tertiary text-nvr-text-secondary hover:text-nvr-text-primary hover:bg-nvr-bg-input border border-transparent cursor-grab active:cursor-grabbing'
      }`}
    >
      <span className={`w-2 h-2 rounded-full flex-shrink-0 ${camera.status === 'online' ? 'bg-nvr-success' : 'bg-nvr-danger'}`} />
      <span className="truncate flex-1">{camera.name}</span>
      {isSelected ? (
        <svg className="w-3.5 h-3.5 text-nvr-accent flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
          <path fillRule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clipRule="evenodd" />
        </svg>
      ) : (
        <svg className="w-3.5 h-3.5 text-nvr-text-muted flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
        </svg>
      )}
    </button>
  )
}

/* ------------------------------------------------------------------ */
/*  Playback Page                                                      */
/* ------------------------------------------------------------------ */
export default function Playback() {
  const { cameras, loading: camerasLoading } = useCameras()

  // Playback state
  const [selectedCameras, setSelectedCameras] = useState<Camera[]>([])
  const [playbackTime, setPlaybackTime] = useState<Date | null>(null)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(1)
  const [date, setDate] = useState(new Date().toISOString().split('T')[0])
  const [dropHighlight, setDropHighlight] = useState(false)

  // Video element refs (keyed by camera ID)
  const videoRefs = useRef<Map<string, HTMLVideoElement>>(new Map())

  // Track whether we've already consumed localStorage auto-load params
  const consumedLocalStorageRef = useRef(false)

  // Recording ranges per camera
  const [cameraRanges, setCameraRanges] = useState<CameraRanges[]>([])

  // Motion events across all selected cameras
  const [motionEvents, setMotionEvents] = useState<MotionEvent[]>([])

  // Track video start time for syncing playback marker
  const videoStartTimeRef = useRef<Date | null>(null)
  const timeUpdateIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Page title
  useEffect(() => {
    document.title = 'Playback \u2014 MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  // Auto-load camera and time from Clips page navigation
  useEffect(() => {
    if (consumedLocalStorageRef.current || cameras.length === 0) return

    const cameraId = localStorage.getItem('nvr-playback-camera')
    const timeStr = localStorage.getItem('nvr-playback-time')

    if (cameraId && timeStr) {
      consumedLocalStorageRef.current = true
      localStorage.removeItem('nvr-playback-camera')
      localStorage.removeItem('nvr-playback-time')

      const cam = cameras.find(c => c.id === cameraId)
      if (cam) {
        setSelectedCameras([cam])
        videoRefs.current.clear()
        const time = new Date(timeStr)
        setPlaybackTime(time)
        setPlaying(true)
        setDate(time.toISOString().split('T')[0])
      }
    }
  }, [cameras])

  // Fetch recording ranges for each selected camera when date or cameras change
  useEffect(() => {
    if (selectedCameras.length === 0) {
      setCameraRanges([])
      return
    }

    const MAX_SEGMENT_MS = 65 * 60 * 1000

    selectedCameras.forEach(cam => {
      if (!cam.mediamtx_path) return
      const dayStart = new Date(date + 'T00:00:00')
      const dayEnd = new Date(dayStart.getTime() + 24 * 60 * 60 * 1000)

      fetch(`http://${window.location.hostname}:9997/v3/recordings/get/${cam.mediamtx_path}`)
        .then(res => res.ok ? res.json() : null)
        .then((data: RecordingList | null) => {
          if (!data || !data.segments) {
            setCameraRanges(prev => {
              const exists = prev.find(cr => cr.cameraId === cam.id)
              if (exists) return prev.map(cr => cr.cameraId === cam.id ? { ...cr, ranges: [] } : cr)
              return [...prev, { cameraId: cam.id, ranges: [] }]
            })
            return
          }

          const filtered = data.segments.filter(s => {
            const t = new Date(s.start)
            return t >= dayStart && t < dayEnd
          })

          const ranges: { start: string; end: string }[] = []
          for (let i = 0; i < filtered.length; i++) {
            const segStartMs = new Date(filtered[i].start).getTime()
            let segEndMs: number
            if (i + 1 < filtered.length) {
              const nextStartMs = new Date(filtered[i + 1].start).getTime()
              segEndMs = (nextStartMs - segStartMs) > MAX_SEGMENT_MS ? segStartMs + MAX_SEGMENT_MS : nextStartMs
            } else {
              segEndMs = segStartMs + 5 * 60 * 1000
            }

            if (ranges.length > 0) {
              const prevEndMs = new Date(ranges[ranges.length - 1].end).getTime()
              if (segStartMs - prevEndMs < 30000) {
                ranges[ranges.length - 1].end = new Date(segEndMs).toISOString()
                continue
              }
            }
            ranges.push({ start: filtered[i].start, end: new Date(segEndMs).toISOString() })
          }

          setCameraRanges(prev => {
            const exists = prev.find(cr => cr.cameraId === cam.id)
            if (exists) return prev.map(cr => cr.cameraId === cam.id ? { ...cr, ranges } : cr)
            return [...prev, { cameraId: cam.id, ranges }]
          })
        })
        .catch(() => {})
    })
  }, [selectedCameras, date])

  // Clean up camera ranges when cameras are removed
  useEffect(() => {
    const ids = new Set(selectedCameras.map(c => c.id))
    setCameraRanges(prev => prev.filter(cr => ids.has(cr.cameraId)))
  }, [selectedCameras])

  // Fetch motion events for selected cameras on date
  useEffect(() => {
    if (selectedCameras.length === 0) {
      setMotionEvents([])
      return
    }

    const promises = selectedCameras.map(cam =>
      apiFetch(`/cameras/${cam.id}/motion-events?date=${date}`)
        .then(res => res.ok ? res.json() : [])
        .catch(() => [])
    )

    Promise.all(promises).then((results: MotionEvent[][]) => {
      const all = results.flat()
      all.sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime())
      setMotionEvents(all)
    })
  }, [selectedCameras, date])

  // Periodically update playback time while playing
  useEffect(() => {
    if (playing && videoStartTimeRef.current) {
      // Pick the first video element to track time
      const firstVideo = videoRefs.current.values().next().value
      if (firstVideo) {
        timeUpdateIntervalRef.current = setInterval(() => {
          if (videoStartTimeRef.current && firstVideo && !firstVideo.paused) {
            const wallTime = new Date(videoStartTimeRef.current.getTime() + firstVideo.currentTime * 1000)
            setPlaybackTime(wallTime)
          }
        }, 250)
      }
    }

    return () => {
      if (timeUpdateIntervalRef.current) {
        clearInterval(timeUpdateIntervalRef.current)
        timeUpdateIntervalRef.current = null
      }
    }
  }, [playing])

  // Callbacks
  const handleVideoRef = useCallback((cameraId: string, el: HTMLVideoElement | null) => {
    if (el) {
      videoRefs.current.set(cameraId, el)
    } else {
      videoRefs.current.delete(cameraId)
    }
  }, [])

  const handleSeek = useCallback((time: Date) => {
    setPlaybackTime(time)
    videoStartTimeRef.current = time
    // Video elements will re-render with new src via CameraTile
    setPlaying(true)
  }, [])

  const handleTogglePlay = useCallback(() => {
    setPlaying(prev => {
      const newPlaying = !prev
      videoRefs.current.forEach(video => {
        if (newPlaying) {
          video.play().catch(() => {})
        } else {
          video.pause()
        }
      })
      return newPlaying
    })
  }, [])

  const handleSpeedChange = useCallback((newSpeed: number) => {
    setSpeed(newSpeed)
    videoRefs.current.forEach(video => {
      video.playbackRate = newSpeed
    })
  }, [])

  const handleJumpBack = useCallback(() => {
    if (!playbackTime) return
    const newTime = new Date(playbackTime.getTime() - 30000)
    handleSeek(newTime)
  }, [playbackTime, handleSeek])

  const handleJumpForward = useCallback(() => {
    if (!playbackTime) return
    const newTime = new Date(playbackTime.getTime() + 30000)
    handleSeek(newTime)
  }, [playbackTime, handleSeek])

  const addCamera = useCallback((cam: Camera) => {
    setSelectedCameras(prev => {
      if (prev.find(c => c.id === cam.id)) return prev
      return [...prev, cam]
    })
  }, [])

  const removeCamera = useCallback((cameraId: string) => {
    videoRefs.current.delete(cameraId)
    setSelectedCameras(prev => prev.filter(c => c.id !== cameraId))
  }, [])

  // Drag and drop handlers for the grid area
  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'copy'
    setDropHighlight(true)
  }, [])

  const handleDragLeave = useCallback(() => {
    setDropHighlight(false)
  }, [])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    setDropHighlight(false)
    const cameraId = e.dataTransfer.getData('camera-id')
    if (cameraId) {
      const cam = cameras.find(c => c.id === cameraId)
      if (cam) addCamera(cam)
    }
  }, [cameras, addCamera])

  const selectedIds = useMemo(() => new Set(selectedCameras.map(c => c.id)), [selectedCameras])

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
    <div className="flex gap-0 -mx-4 sm:-mx-6 lg:-mx-8 -my-6 h-[calc(100vh-3.5rem)] overflow-hidden">
      {/* ---- Left Panel: Camera Grid ---- */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden">
        {/* Camera selector bar */}
        <div className="px-4 py-3 border-b border-nvr-border bg-nvr-bg-secondary/50">
          <div className="flex items-center gap-3 flex-wrap">
            <h1 className="text-lg font-bold text-nvr-text-primary">Playback</h1>
            <span className="text-xs text-nvr-text-muted">
              {selectedCameras.length} camera{selectedCameras.length !== 1 ? 's' : ''} selected
            </span>
            {selectedCameras.length > 0 && (
              <button
                onClick={() => { setSelectedCameras([]); videoRefs.current.clear() }}
                className="text-xs text-nvr-text-secondary hover:text-nvr-danger transition-colors"
              >
                Clear all
              </button>
            )}
          </div>

          {/* Camera chips */}
          <div className="flex flex-wrap gap-2 mt-2">
            {cameras.map(cam => (
              <CameraChip
                key={cam.id}
                camera={cam}
                isSelected={selectedIds.has(cam.id)}
                onAdd={() => addCamera(cam)}
              />
            ))}
          </div>
        </div>

        {/* Grid area */}
        <div
          className={`flex-1 p-4 overflow-auto ${dropHighlight ? 'bg-nvr-accent/5 ring-2 ring-nvr-accent/30 ring-inset' : 'bg-nvr-bg-primary'}`}
          onDragOver={handleDragOver}
          onDragLeave={handleDragLeave}
          onDrop={handleDrop}
        >
          {selectedCameras.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-center">
              <div className="w-16 h-16 rounded-2xl bg-nvr-accent/15 flex items-center justify-center mb-4">
                <svg className="w-8 h-8 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
              </div>
              <h2 className="text-lg font-semibold text-nvr-text-primary mb-1">No cameras selected</h2>
              <p className="text-sm text-nvr-text-secondary max-w-md">
                Click cameras above or drag them here to start synchronized playback.
              </p>
            </div>
          ) : (
            <div className={`grid ${gridClass(selectedCameras.length)} gap-2 h-full`}>
              {selectedCameras.map(cam => (
                <CameraTile
                  key={cam.id}
                  camera={cam}
                  playbackTime={playbackTime}
                  playing={playing}
                  speed={speed}
                  onVideoRef={handleVideoRef}
                  onRemove={() => removeCamera(cam.id)}
                />
              ))}
            </div>
          )}
        </div>
      </div>

      {/* ---- Right Panel: Timeline & Controls ---- */}
      <div className="w-80 lg:w-96 border-l border-nvr-border bg-nvr-bg-secondary flex flex-col flex-shrink-0 overflow-hidden">
        {/* Date picker */}
        <div className="px-4 py-3 border-b border-nvr-border">
          <label className="text-xs text-nvr-text-muted block mb-1">Date</label>
          <input
            type="date"
            value={date}
            onChange={e => setDate(e.target.value)}
            max={new Date().toISOString().split('T')[0]}
            className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
          />
        </div>

        {/* Camera color legend */}
        {selectedCameras.length > 0 && (
          <div className="px-4 py-2 border-b border-nvr-border">
            <div className="flex flex-wrap gap-2">
              {selectedCameras.map((cam, i) => (
                <span key={cam.id} className="flex items-center gap-1 text-[10px] text-nvr-text-muted">
                  <span className={`w-2 h-2 rounded-sm ${COLORS[i % COLORS.length].replace('/60', '')}`} />
                  {cam.name}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Vertical timeline */}
        <VerticalTimeline
          date={date}
          cameraRanges={cameraRanges}
          events={motionEvents}
          playbackTime={playbackTime}
          onSeek={handleSeek}
          cameras={selectedCameras}
        />

        {/* VCR Controls */}
        <VCRControls
          playing={playing}
          speed={speed}
          playbackTime={playbackTime}
          onTogglePlay={handleTogglePlay}
          onSpeedChange={handleSpeedChange}
          onJumpBack={handleJumpBack}
          onJumpForward={handleJumpForward}
        />
      </div>
    </div>
  )
}
