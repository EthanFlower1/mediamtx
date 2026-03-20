import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

interface SystemInfo {
  version: string
  platform: string
  uptime: string
}

interface CameraStorageInfo {
  camera_id: string
  camera_name: string
  total_bytes: number
  segment_count: number
}

interface StorageInfo {
  total_bytes: number
  used_bytes: number
  free_bytes: number
  recordings_bytes: number
  per_camera: CameraStorageInfo[]
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

function formatUptime(uptime: string): string {
  // Go outputs durations like "2h30m15.123s" - simplify for display
  const match = uptime.match(/(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?/)
  if (!match) return uptime
  const hours = match[1] ? parseInt(match[1]) : 0
  const minutes = match[2] ? parseInt(match[2]) : 0
  const seconds = match[3] ? Math.floor(parseFloat(match[3])) : 0

  const parts: string[] = []
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0) parts.push(`${minutes}m`)
  if (seconds > 0 || parts.length === 0) parts.push(`${seconds}s`)
  return parts.join(' ')
}

export default function Settings() {
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [storage, setStorage] = useState<StorageInfo | null>(null)
  const [storageLoading, setStorageLoading] = useState(true)

  useEffect(() => {
    apiFetch('/system/info').then(async res => {
      if (res.ok) setSystemInfo(await res.json())
    })
  }, [])

  const fetchStorage = useCallback(() => {
    apiFetch('/system/storage').then(async res => {
      if (res.ok) {
        setStorage(await res.json())
      }
      setStorageLoading(false)
    }).catch(() => setStorageLoading(false))
  }, [])

  useEffect(() => {
    fetchStorage()
    const interval = setInterval(fetchStorage, 30000)
    return () => clearInterval(interval)
  }, [fetchStorage])

  const usedPercent = storage && storage.total_bytes > 0
    ? Math.round((storage.used_bytes / storage.total_bytes) * 100)
    : 0

  const recordingsPercent = storage && storage.total_bytes > 0
    ? Math.round((storage.recordings_bytes / storage.total_bytes) * 100)
    : 0

  const otherUsedPercent = usedPercent - recordingsPercent

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-bold text-nvr-text-primary">Settings</h1>

      {/* System Information */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">System Information</h2>
        {systemInfo ? (
          <div>
            <div className="flex justify-between py-3 border-b border-nvr-border/50">
              <span className="text-sm text-nvr-text-secondary">Version</span>
              <span className="text-sm text-nvr-text-primary font-mono">{systemInfo.version}</span>
            </div>
            <div className="flex justify-between py-3 border-b border-nvr-border/50">
              <span className="text-sm text-nvr-text-secondary">Platform</span>
              <span className="text-sm text-nvr-text-primary font-mono">{systemInfo.platform}</span>
            </div>
            <div className="flex justify-between py-3">
              <span className="text-sm text-nvr-text-secondary">Uptime</span>
              <span className="text-sm text-nvr-text-primary">{formatUptime(systemInfo.uptime)}</span>
            </div>
          </div>
        ) : (
          <p className="text-nvr-text-muted text-sm">Loading...</p>
        )}
      </div>

      {/* Storage Overview */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-nvr-text-primary">Storage Overview</h2>
          <button
            onClick={fetchStorage}
            className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary transition-colors"
          >
            Refresh
          </button>
        </div>

        {storageLoading ? (
          <p className="text-nvr-text-muted text-sm">Loading storage info...</p>
        ) : storage ? (
          <div className="space-y-5">
            {/* Disk usage bar */}
            <div>
              <div className="flex justify-between text-sm mb-2">
                <span className="text-nvr-text-secondary">Disk Usage</span>
                <span className="text-nvr-text-primary">
                  {formatBytes(storage.used_bytes)} / {formatBytes(storage.total_bytes)} ({usedPercent}%)
                </span>
              </div>
              <div className="w-full h-4 bg-nvr-bg-primary rounded-full overflow-hidden flex">
                {/* Recordings portion (blue) */}
                {recordingsPercent > 0 && (
                  <div
                    className="h-full bg-nvr-accent transition-all duration-500"
                    style={{ width: `${recordingsPercent}%` }}
                    title={`Recordings: ${formatBytes(storage.recordings_bytes)} (${recordingsPercent}%)`}
                  />
                )}
                {/* Other used space (gray) */}
                {otherUsedPercent > 0 && (
                  <div
                    className="h-full bg-nvr-text-muted transition-all duration-500"
                    style={{ width: `${otherUsedPercent}%` }}
                    title={`Other: ${formatBytes(storage.used_bytes - storage.recordings_bytes)} (${otherUsedPercent}%)`}
                  />
                )}
                {/* Free space is the remainder (dark bg shows through) */}
              </div>
              <div className="flex gap-4 mt-2 text-xs text-nvr-text-muted">
                <span className="flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm bg-nvr-accent" />
                  Recordings ({formatBytes(storage.recordings_bytes)})
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm bg-nvr-text-muted" />
                  Other ({formatBytes(storage.used_bytes - storage.recordings_bytes)})
                </span>
                <span className="flex items-center gap-1.5">
                  <span className="inline-block w-2.5 h-2.5 rounded-sm bg-nvr-bg-primary border border-nvr-border" />
                  Free ({formatBytes(storage.free_bytes)})
                </span>
              </div>
            </div>

            {/* Per-camera storage breakdown */}
            {storage.per_camera.length > 0 && (
              <div>
                <h3 className="text-sm font-medium text-nvr-text-secondary mb-3">Per-Camera Breakdown</h3>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="text-nvr-text-muted border-b border-nvr-border/50">
                        <th className="text-left py-2 font-medium">Camera</th>
                        <th className="text-right py-2 font-medium">Segments</th>
                        <th className="text-right py-2 font-medium">Size</th>
                        <th className="text-right py-2 font-medium">% of Recordings</th>
                      </tr>
                    </thead>
                    <tbody>
                      {storage.per_camera.map(cam => (
                        <tr key={cam.camera_id} className="border-b border-nvr-border/30">
                          <td className="py-2 text-nvr-text-primary">{cam.camera_name || cam.camera_id}</td>
                          <td className="py-2 text-right text-nvr-text-secondary">{cam.segment_count}</td>
                          <td className="py-2 text-right text-nvr-text-primary font-mono">{formatBytes(cam.total_bytes)}</td>
                          <td className="py-2 text-right text-nvr-text-secondary">
                            {storage.recordings_bytes > 0
                              ? Math.round((cam.total_bytes / storage.recordings_bytes) * 100)
                              : 0}%
                          </td>
                        </tr>
                      ))}
                    </tbody>
                    <tfoot>
                      <tr className="border-t border-nvr-border">
                        <td className="py-2 font-medium text-nvr-text-primary">Total</td>
                        <td className="py-2 text-right font-medium text-nvr-text-primary">
                          {storage.per_camera.reduce((sum, c) => sum + c.segment_count, 0)}
                        </td>
                        <td className="py-2 text-right font-medium text-nvr-text-primary font-mono">
                          {formatBytes(storage.recordings_bytes)}
                        </td>
                        <td className="py-2 text-right font-medium text-nvr-text-primary">100%</td>
                      </tr>
                    </tfoot>
                  </table>
                </div>
              </div>
            )}

            {storage.per_camera.length === 0 && (
              <p className="text-nvr-text-muted text-sm">No recordings found.</p>
            )}
          </div>
        ) : (
          <p className="text-nvr-text-muted text-sm">Unable to load storage information.</p>
        )}
      </div>

      {/* Recording Defaults */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">Recording Defaults</h2>
        <p className="text-xs text-nvr-text-muted mb-4">
          These values are configured in mediamtx.yml under pathDefaults. Per-camera retention can be set on each camera.
        </p>
        <div>
          <div className="flex justify-between py-3 border-b border-nvr-border/50">
            <span className="text-sm text-nvr-text-secondary">Global Retention Period</span>
            <span className="text-sm text-nvr-text-primary font-mono">1d (recordDeleteAfter)</span>
          </div>
          <div className="flex justify-between py-3 border-b border-nvr-border/50">
            <span className="text-sm text-nvr-text-secondary">Recording Format</span>
            <span className="text-sm text-nvr-text-primary font-mono">fmp4</span>
          </div>
          <div className="flex justify-between py-3">
            <span className="text-sm text-nvr-text-secondary">Segment Duration</span>
            <span className="text-sm text-nvr-text-primary font-mono">1h</span>
          </div>
        </div>
      </div>
    </div>
  )
}
