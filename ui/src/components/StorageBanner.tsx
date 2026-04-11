import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

interface StorageStatus {
  total_bytes: number
  used_bytes: number
  warning: boolean
  critical: boolean
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

export default function StorageBanner() {
  const [storage, setStorage] = useState<StorageStatus | null>(null)

  const fetchStorage = useCallback(() => {
    apiFetch('/system/storage').then(async res => {
      if (res.ok) setStorage(await res.json())
    }).catch(() => {})
  }, [])

  useEffect(() => {
    fetchStorage()
    const interval = setInterval(fetchStorage, 30000)
    return () => clearInterval(interval)
  }, [fetchStorage])

  if (!storage || (!storage.warning && !storage.critical)) return null

  const usedPercent = storage.total_bytes > 0
    ? Math.round((storage.used_bytes / storage.total_bytes) * 100)
    : 0

  if (storage.critical) {
    return (
      <div role="alert" className="bg-red-900/80 border-b border-red-700 px-4 py-2 text-sm text-red-200 flex items-center gap-2">
        <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <span className="flex-1">Disk space critically low! Recordings may stop. ({usedPercent}% used, {formatBytes(storage.total_bytes - storage.used_bytes)} free)</span>
        <a href="/storage" className="bg-white/20 hover:bg-white/30 text-white text-xs font-medium px-3 py-1 rounded-lg transition-colors shrink-0">
          Manage Storage &rarr;
        </a>
      </div>
    )
  }

  return (
    <div role="alert" className="bg-amber-900/60 border-b border-amber-700 px-4 py-2 text-sm text-amber-200 flex items-center gap-2">
      <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} aria-hidden="true">
        <path d="M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
        <line x1="12" y1="9" x2="12" y2="13" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
      <span className="flex-1">Disk space running low ({usedPercent}% used, {formatBytes(storage.total_bytes - storage.used_bytes)} free)</span>
      <a href="/settings" className="bg-white/20 hover:bg-white/30 text-white text-xs font-medium px-3 py-1 rounded-lg transition-colors shrink-0">
        Manage Storage &rarr;
      </a>
    </div>
  )
}
