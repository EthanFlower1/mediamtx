import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'
import VideoPlayer from './VideoPlayer'

interface EdgeRecording {
  recording_token: string
  source_name: string
  earliest_time: string
  latest_time: string
}

interface EdgeRecordingSummary {
  total_recordings: number
  earliest_time: string
  latest_time: string
}

interface EdgeRecordingsResponse {
  summary: EdgeRecordingSummary
  recordings: EdgeRecording[]
}

interface CameraStorageBrowserProps {
  cameraId: string
}

function formatDateTime(iso: string): string {
  if (!iso) return '--'
  const d = new Date(iso)
  if (isNaN(d.getTime())) return iso
  return d.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}

export default function CameraStorageBrowser({ cameraId }: CameraStorageBrowserProps) {
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [summary, setSummary] = useState<EdgeRecordingSummary | null>(null)
  const [recordings, setRecordings] = useState<EdgeRecording[]>([])
  const [playbackUri, setPlaybackUri] = useState<string | null>(null)
  const [playbackToken, setPlaybackToken] = useState<string | null>(null)
  const [loadingPlayback, setLoadingPlayback] = useState<string | null>(null)

  const fetchEdgeRecordings = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await apiFetch(`/cameras/${cameraId}/edge-recordings`)
      if (!res.ok) {
        const data = await res.json().catch(() => ({ error: 'Unknown error' }))
        setError(data.error || `Failed to fetch (HTTP ${res.status})`)
        return
      }
      const data: EdgeRecordingsResponse = await res.json()
      setSummary(data.summary)
      setRecordings(data.recordings || [])
    } catch (err) {
      setError('Failed to connect to camera edge storage')
    } finally {
      setLoading(false)
    }
  }, [cameraId])

  useEffect(() => {
    fetchEdgeRecordings()
  }, [fetchEdgeRecordings])

  const handlePlay = async (token: string) => {
    setLoadingPlayback(token)
    try {
      const res = await apiFetch(`/cameras/${cameraId}/edge-recordings/playback?recording_token=${encodeURIComponent(token)}`)
      if (!res.ok) {
        setError('Failed to get playback URI')
        return
      }
      const data = await res.json()
      setPlaybackUri(data.replay_uri)
      setPlaybackToken(token)
    } catch {
      setError('Failed to get playback URI')
    } finally {
      setLoadingPlayback(null)
    }
  }

  const closePlayback = () => {
    setPlaybackUri(null)
    setPlaybackToken(null)
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <span className="inline-block w-5 h-5 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin mr-3" />
        <span className="text-nvr-text-secondary">Scanning camera storage...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center">
        <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 text-nvr-danger/60 mb-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round">
          <circle cx="12" cy="12" r="10" /><line x1="15" y1="9" x2="9" y2="15" /><line x1="9" y1="9" x2="15" y2="15" />
        </svg>
        <p className="text-nvr-text-secondary text-sm mb-2">{error}</p>
        <button
          onClick={fetchEdgeRecordings}
          className="text-sm text-nvr-accent hover:text-nvr-accent-hover transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded px-3 py-1"
        >
          Retry
        </button>
      </div>
    )
  }

  return (
    <div>
      {/* Summary */}
      {summary && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4 mb-4">
          <h3 className="text-sm font-medium text-nvr-text-primary mb-3">Camera Storage Summary</h3>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label className="block text-xs text-nvr-text-muted mb-0.5">Total Recordings</label>
              <span className="text-lg font-semibold text-nvr-text-primary">{summary.total_recordings}</span>
            </div>
            <div>
              <label className="block text-xs text-nvr-text-muted mb-0.5">Earliest</label>
              <span className="text-sm text-nvr-text-primary">{formatDateTime(summary.earliest_time)}</span>
            </div>
            <div>
              <label className="block text-xs text-nvr-text-muted mb-0.5">Latest</label>
              <span className="text-sm text-nvr-text-primary">{formatDateTime(summary.latest_time)}</span>
            </div>
          </div>
        </div>
      )}

      {/* Playback player */}
      {playbackUri && playbackToken && (
        <div className="mb-4">
          <div className="flex items-center gap-2 mb-2">
            <span className="text-sm text-nvr-text-secondary">
              Playing recording: <span className="font-mono text-nvr-text-primary">{playbackToken}</span>
            </span>
            <button
              onClick={closePlayback}
              className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
            >
              Close
            </button>
          </div>
          <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
            <VideoPlayer src={playbackUri} />
          </div>
          <p className="text-xs text-nvr-text-muted mt-2">
            RTSP URI: <code className="bg-nvr-bg-tertiary px-1.5 py-0.5 rounded text-nvr-text-secondary select-all">{playbackUri}</code>
          </p>
        </div>
      )}

      {/* Recording list */}
      {recordings.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <svg xmlns="http://www.w3.org/2000/svg" className="w-10 h-10 text-nvr-text-muted/40 mb-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round">
            <rect x="2" y="3" width="20" height="14" rx="2" ry="2" /><line x1="8" y1="21" x2="16" y2="21" /><line x1="12" y1="17" x2="12" y2="21" />
          </svg>
          <p className="text-nvr-text-secondary text-sm">No recordings found on camera storage</p>
        </div>
      ) : (
        <div className="space-y-2">
          <h3 className="text-sm font-medium text-nvr-text-primary mb-2">
            Recordings ({recordings.length})
          </h3>
          {recordings.map((rec) => {
            const isPlaying = playbackToken === rec.recording_token
            const isLoading = loadingPlayback === rec.recording_token

            return (
              <div
                key={rec.recording_token}
                className={`flex items-center gap-3 bg-nvr-bg-secondary border rounded-lg px-4 py-3 transition-colors ${
                  isPlaying ? 'border-nvr-accent bg-nvr-accent/5' : 'border-nvr-border hover:bg-nvr-bg-tertiary'
                }`}
              >
                {/* SD card icon */}
                <svg xmlns="http://www.w3.org/2000/svg" className="w-5 h-5 text-nvr-text-muted shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} strokeLinecap="round" strokeLinejoin="round">
                  <rect x="2" y="5" width="20" height="14" rx="2" /><line x1="8" y1="5" x2="8" y2="9" /><line x1="12" y1="5" x2="12" y2="9" /><line x1="16" y1="5" x2="16" y2="9" />
                </svg>

                {/* Info */}
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm text-nvr-text-primary font-mono truncate">
                      {rec.recording_token}
                    </span>
                    {rec.source_name && (
                      <span className="text-xs text-nvr-text-muted bg-nvr-bg-tertiary px-1.5 py-0.5 rounded">
                        {rec.source_name}
                      </span>
                    )}
                  </div>
                  <div className="text-xs text-nvr-text-muted mt-0.5">
                    {formatDateTime(rec.earliest_time)} &mdash; {formatDateTime(rec.latest_time)}
                  </div>
                </div>

                {/* Play button */}
                <button
                  onClick={() => handlePlay(rec.recording_token)}
                  disabled={isLoading}
                  className="shrink-0 flex items-center gap-1 text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded px-2 py-1 disabled:opacity-50"
                  title="Play this recording"
                >
                  {isLoading ? (
                    <span className="inline-block w-3 h-3 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
                  ) : (
                    <>
                      Play
                      <svg xmlns="http://www.w3.org/2000/svg" className="w-3 h-3" viewBox="0 0 24 24" fill="currentColor"><path d="M8 5v14l11-7z" /></svg>
                    </>
                  )}
                </button>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
