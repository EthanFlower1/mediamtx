import { useState, useRef, useEffect } from 'react'
import type { Notification } from '../hooks/useNotifications'

interface NotificationBellProps {
  notifications: Notification[]
  unreadCount: number
  onMarkAllRead: () => void
}

function typeIcon(type: Notification['type']): string {
  switch (type) {
    case 'motion':
      return '\uD83D\uDEB6'
    case 'camera_offline':
      return '\uD83D\uDD34'
    case 'camera_online':
      return '\uD83D\uDFE2'
    case 'recording_started':
      return '\u23FA'
    case 'recording_stopped':
      return '\u23F9'
    default:
      return '\uD83D\uDD14'
  }
}

function relativeTime(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000)
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

export default function NotificationBell({ notifications, unreadCount, onMarkAllRead }: NotificationBellProps) {
  const [open, setOpen] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)

  // Close dropdown on outside click.
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    if (open) {
      document.addEventListener('mousedown', handleClickOutside)
    }
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [open])

  return (
    <div className="relative" ref={dropdownRef}>
      <button
        onClick={() => setOpen(!open)}
        className="relative p-1.5 text-nvr-text-secondary hover:text-nvr-text-primary transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none rounded"
        aria-label={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ''}`}
        aria-expanded={open}
        aria-haspopup="true"
      >
        <svg
          className="w-5 h-5"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          strokeWidth={2}
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75v-.7V9A6 6 0 006 9v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0"
          />
        </svg>
        {unreadCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 bg-red-500 text-white text-[10px] font-bold rounded-full min-w-[18px] h-[18px] flex items-center justify-center px-1">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        )}
      </button>

      {open && (
        <div className="absolute right-0 mt-2 w-80 bg-nvr-bg-secondary border border-nvr-border rounded-lg shadow-xl overflow-hidden z-50">
          <div className="flex items-center justify-between px-4 py-2.5 border-b border-nvr-border">
            <span className="text-sm font-semibold text-nvr-text-primary">Notifications</span>
            {unreadCount > 0 && (
              <button
                onClick={() => {
                  onMarkAllRead()
                }}
                className="text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors"
              >
                Mark all read
              </button>
            )}
          </div>

          <div className="max-h-80 overflow-y-auto">
            {notifications.length === 0 ? (
              <div className="px-4 py-8 text-center text-nvr-text-muted text-sm">
                No notifications
              </div>
            ) : (
              notifications.map(notif => (
                <div
                  key={notif.id}
                  className={`px-4 py-2.5 border-b border-nvr-border last:border-b-0 hover:bg-nvr-bg-tertiary transition-colors ${
                    !notif.read ? 'bg-nvr-bg-tertiary/50' : ''
                  }`}
                >
                  <div className="flex items-start gap-2">
                    <span className="text-base mt-0.5 flex-shrink-0" role="img" aria-label={notif.type}>
                      {typeIcon(notif.type)}
                    </span>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-1.5">
                        <span className="text-xs font-medium text-nvr-text-primary truncate">
                          {notif.camera}
                        </span>
                        {!notif.read && (
                          <span className="w-1.5 h-1.5 rounded-full bg-nvr-accent flex-shrink-0" />
                        )}
                      </div>
                      <p className="text-xs text-nvr-text-secondary mt-0.5 truncate">
                        {notif.message}
                      </p>
                      <p className="text-[10px] text-nvr-text-muted mt-0.5">
                        {relativeTime(notif.time)}
                      </p>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  )
}
