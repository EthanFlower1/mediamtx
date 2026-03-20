import { useRef, useState, useEffect, useCallback } from 'react'

interface VideoPlayerProps {
  src?: string
  stream?: MediaStream
  live?: boolean
  onTimeUpdate?: (time: number) => void
  onRetry?: () => void
  onVideoRef?: (el: HTMLVideoElement | null) => void
}

const SPEEDS = [0.5, 1, 2, 4, 8]

function formatTime(seconds: number): string {
  if (!isFinite(seconds) || seconds < 0) return '0:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  return `${m}:${s.toString().padStart(2, '0')}`
}

export default function VideoPlayer({ src, stream, live, onTimeUpdate, onRetry, onVideoRef }: VideoPlayerProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const videoRef = useRef<HTMLVideoElement>(null)
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [playing, setPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(1)
  const [muted, setMuted] = useState(true)
  const [speed, setSpeed] = useState(1)
  const [showSpeedMenu, setShowSpeedMenu] = useState(false)
  const [isFullscreen, setIsFullscreen] = useState(false)
  const [controlsVisible, setControlsVisible] = useState(true)
  const [videoError, setVideoError] = useState(false)

  // Expose video element to parent via callback.
  useEffect(() => {
    onVideoRef?.(videoRef.current)
    return () => onVideoRef?.(null)
  }, [onVideoRef])

  // Attach stream source when provided.
  useEffect(() => {
    const video = videoRef.current
    if (!video || !stream) return
    video.srcObject = stream
    return () => {
      video.srcObject = null
    }
  }, [stream])

  // Attach URL source when provided.
  useEffect(() => {
    const video = videoRef.current
    if (!video || !src) return
    video.src = src
    video.play().catch(() => {})
  }, [src])

  // Listen to video events.
  useEffect(() => {
    const video = videoRef.current
    if (!video) return

    const onPlay = () => { setPlaying(true); setVideoError(false) }
    const onPause = () => setPlaying(false)
    const onTimeUpdateEvt = () => {
      setCurrentTime(video.currentTime)
      onTimeUpdate?.(video.currentTime)
    }
    const onDurationChange = () => setDuration(video.duration)
    const onVolumeChange = () => {
      setVolume(video.volume)
      setMuted(video.muted)
    }
    const onError = () => setVideoError(true)

    video.addEventListener('play', onPlay)
    video.addEventListener('pause', onPause)
    video.addEventListener('timeupdate', onTimeUpdateEvt)
    video.addEventListener('durationchange', onDurationChange)
    video.addEventListener('volumechange', onVolumeChange)
    video.addEventListener('error', onError)

    return () => {
      video.removeEventListener('play', onPlay)
      video.removeEventListener('pause', onPause)
      video.removeEventListener('timeupdate', onTimeUpdateEvt)
      video.removeEventListener('durationchange', onDurationChange)
      video.removeEventListener('volumechange', onVolumeChange)
      video.removeEventListener('error', onError)
    }
  }, [onTimeUpdate])

  // Fullscreen change detection.
  useEffect(() => {
    const onChange = () => {
      setIsFullscreen(!!document.fullscreenElement)
    }
    document.addEventListener('fullscreenchange', onChange)
    return () => document.removeEventListener('fullscreenchange', onChange)
  }, [])

  // Auto-hide controls after 3 seconds of no mouse movement.
  const resetHideTimer = useCallback(() => {
    setControlsVisible(true)
    if (hideTimerRef.current) clearTimeout(hideTimerRef.current)
    hideTimerRef.current = setTimeout(() => {
      if (videoRef.current && !videoRef.current.paused) {
        setControlsVisible(false)
        setShowSpeedMenu(false)
      }
    }, 3000)
  }, [])

  useEffect(() => {
    return () => {
      if (hideTimerRef.current) clearTimeout(hideTimerRef.current)
    }
  }, [])

  // Handlers.
  const togglePlay = () => {
    const video = videoRef.current
    if (!video) return
    if (video.paused) {
      video.play().catch(() => {})
    } else {
      video.pause()
    }
  }

  const handleSeek = (e: React.MouseEvent<HTMLDivElement>) => {
    if (live) return
    const video = videoRef.current
    if (!video || !isFinite(duration) || duration <= 0) return
    const rect = e.currentTarget.getBoundingClientRect()
    const ratio = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    video.currentTime = ratio * duration
  }

  const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const video = videoRef.current
    if (!video) return
    const val = parseFloat(e.target.value)
    video.volume = val
    video.muted = val === 0
  }

  const toggleMute = () => {
    const video = videoRef.current
    if (!video) return
    video.muted = !video.muted
  }

  const changeSpeed = (s: number) => {
    const video = videoRef.current
    if (!video) return
    video.playbackRate = s
    setSpeed(s)
    setShowSpeedMenu(false)
  }

  const toggleFullscreen = () => {
    if (!containerRef.current) return
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {})
    } else {
      containerRef.current.requestFullscreen().catch(() => {})
    }
  }

  const progress = duration > 0 && isFinite(duration) ? (currentTime / duration) * 100 : 0

  return (
    <div
      ref={containerRef}
      className="relative bg-black w-full aspect-video overflow-hidden group select-none"
      onMouseMove={resetHideTimer}
      onMouseEnter={resetHideTimer}
      onMouseLeave={() => {
        if (!videoRef.current?.paused) {
          setControlsVisible(false)
          setShowSpeedMenu(false)
        }
      }}
    >
      {/* Video element */}
      <video
        ref={videoRef}
        autoPlay
        muted={muted}
        playsInline
        className="w-full h-full object-contain"
        onClick={togglePlay}
      />

      {/* Error overlay */}
      {videoError && (
        <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/80 z-10">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 text-nvr-danger mb-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
            <circle cx="12" cy="12" r="10" />
            <line x1="12" y1="8" x2="12" y2="12" />
            <line x1="12" y1="16" x2="12.01" y2="16" />
          </svg>
          <p className="text-white text-sm mb-3">Failed to load video. Check camera connection.</p>
          <button
            onClick={(e) => {
              e.stopPropagation()
              setVideoError(false)
              if (onRetry) {
                onRetry()
              } else {
                const video = videoRef.current
                if (video && src) {
                  video.src = src
                  video.play().catch(() => {})
                }
              }
            }}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Retry
          </button>
        </div>
      )}

      {/* Timestamp overlay - top left */}
      <div
        className={`absolute top-3 left-3 bg-black/60 backdrop-blur-sm rounded-md px-2.5 py-1 text-xs font-mono text-white/90 transition-opacity duration-300 ${
          controlsVisible ? 'opacity-100' : 'opacity-0'
        }`}
      >
        {live ? (
          <span className="flex items-center gap-1.5">
            <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />
            LIVE
          </span>
        ) : (
          <span>{formatTime(currentTime)} / {formatTime(duration)}</span>
        )}
      </div>

      {/* Controls bar */}
      <div
        className={`absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/90 via-black/60 to-transparent transition-opacity duration-300 ${
          controlsVisible ? 'opacity-100' : 'opacity-0 pointer-events-none'
        }`}
      >
        {/* Progress bar (recordings only) */}
        {!live && (
          <div
            role="slider"
            aria-label="Video progress"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(progress)}
            className="w-full h-1.5 bg-white/20 cursor-pointer group/progress hover:h-2.5 transition-all"
            onClick={handleSeek}
          >
            <div
              className="h-full bg-nvr-accent rounded-r-full transition-[width] duration-100"
              style={{ width: `${progress}%` }}
            />
          </div>
        )}

        {/* Controls row */}
        <div className="flex items-center gap-2 px-3 py-2">
          {/* Play / Pause */}
          <button
            onClick={togglePlay}
            className="text-white hover:text-nvr-accent transition-colors p-1 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
            aria-label={playing ? 'Pause' : 'Play'}
          >
            {playing ? (
              <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M6 4h4v16H6V4zm8 0h4v16h-4V4z" />
              </svg>
            ) : (
              <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M8 5v14l11-7z" />
              </svg>
            )}
          </button>

          {/* Time display */}
          {!live && (
            <span className="text-xs text-white/80 font-mono min-w-[80px]">
              {formatTime(currentTime)} / {formatTime(duration)}
            </span>
          )}

          {/* Live indicator in controls bar */}
          {live && (
            <span className="text-xs text-white/80 font-medium flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-red-500" />
              LIVE
            </span>
          )}

          {/* Spacer */}
          <div className="flex-1" />

          {/* Speed selector (recordings only) */}
          {!live && (
            <div className="relative">
              <button
                onClick={() => setShowSpeedMenu(!showSpeedMenu)}
                className="text-xs text-white/80 hover:text-white bg-white/10 hover:bg-white/20 rounded px-2 py-1 transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                aria-label={`Playback speed ${speed}x`}
              >
                {speed}x
              </button>
              {showSpeedMenu && (
                <div className="absolute bottom-full mb-1 right-0 bg-nvr-bg-secondary border border-nvr-border rounded-lg shadow-xl overflow-hidden min-w-[60px]">
                  {SPEEDS.map(s => (
                    <button
                      key={s}
                      onClick={() => changeSpeed(s)}
                      className={`block w-full text-left px-3 py-1.5 text-xs transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                        s === speed
                          ? 'text-nvr-accent bg-nvr-accent/10'
                          : 'text-nvr-text-primary hover:bg-nvr-bg-tertiary'
                      }`}
                      aria-label={`Set speed to ${s}x`}
                    >
                      {s}x
                    </button>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Volume */}
          <div className="flex items-center gap-1.5 group/vol">
            <button
              onClick={toggleMute}
              className="text-white/80 hover:text-white transition-colors p-1 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
              aria-label={muted ? 'Unmute' : 'Mute'}
            >
              {muted || volume === 0 ? (
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M16.5 12c0-1.77-1.02-3.29-2.5-4.03v2.21l2.45 2.45c.03-.2.05-.41.05-.63zm2.5 0c0 .94-.2 1.82-.54 2.64l1.51 1.51A8.796 8.796 0 0021 12c0-4.28-2.99-7.86-7-8.77v2.06c2.89.86 5 3.54 5 6.71zM4.27 3L3 4.27 7.73 9H3v6h4l5 5v-6.73l4.25 4.25c-.67.52-1.42.93-2.25 1.18v2.06a8.99 8.99 0 003.69-1.81L19.73 21 21 19.73l-9-9L4.27 3zM12 4L9.91 6.09 12 8.18V4z" />
                </svg>
              ) : volume < 0.5 ? (
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M18.5 12c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM5 9v6h4l5 5V4L9 9H5z" />
                </svg>
              ) : (
                <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M3 9v6h4l5 5V4L7 9H3zm13.5 3c0-1.77-1.02-3.29-2.5-4.03v8.05c1.48-.73 2.5-2.25 2.5-4.02zM14 3.23v2.06c2.89.86 5 3.54 5 6.71s-2.11 5.85-5 6.71v2.06c4.01-.91 7-4.49 7-8.77s-2.99-7.86-7-8.77z" />
                </svg>
              )}
            </button>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={muted ? 0 : volume}
              onChange={handleVolumeChange}
              aria-label="Volume"
              className="w-0 group-hover/vol:w-20 transition-all duration-200 accent-nvr-accent h-1 cursor-pointer opacity-0 group-hover/vol:opacity-100"
            />
          </div>

          {/* Fullscreen */}
          <button
            onClick={toggleFullscreen}
            className="text-white/80 hover:text-white transition-colors p-1 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
            aria-label={isFullscreen ? 'Exit fullscreen' : 'Enter fullscreen'}
          >
            {isFullscreen ? (
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                <path d="M5 16h3v3h2v-5H5v2zm3-8H5v2h5V5H8v3zm6 11h2v-3h3v-2h-5v5zm2-11V5h-2v5h5V8h-3z" />
              </svg>
            ) : (
              <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="currentColor">
                <path d="M7 14H5v5h5v-2H7v-3zm-2-4h2V7h3V5H5v5zm12 7h-3v2h5v-5h-2v3zM14 5v2h3v3h2V5h-5z" />
              </svg>
            )}
          </button>
        </div>
      </div>
    </div>
  )
}
