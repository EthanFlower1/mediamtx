import { useState, useEffect, useRef, FormEvent, useCallback } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth/context'
import { useFederation } from '../hooks/useFederation'
import { InviteToken, JoinProgress, PeerStatus } from '../api/federation'
import ConfirmDialog from '../components/ConfirmDialog'

/* ------------------------------------------------------------------ */
/*  Helpers                                                            */
/* ------------------------------------------------------------------ */

function formatTimeAgo(iso: string | null): string {
  if (!iso) return 'Never'
  const diff = Date.now() - new Date(iso).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 60) return 'Just now'
  const mins = Math.floor(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  const days = Math.floor(hrs / 24)
  return `${days}d ago`
}

function statusColor(status: PeerStatus): string {
  switch (status) {
    case 'connected':
      return 'bg-nvr-success'
    case 'pending':
      return 'bg-nvr-warning'
    case 'disconnected':
      return 'bg-nvr-text-muted'
    case 'error':
      return 'bg-nvr-danger'
    default:
      return 'bg-nvr-text-muted'
  }
}

function statusLabel(status: PeerStatus): string {
  return status.charAt(0).toUpperCase() + status.slice(1)
}

/* ------------------------------------------------------------------ */
/*  Modal wrapper (reused from UserManagement pattern)                 */
/* ------------------------------------------------------------------ */

function Modal({
  open,
  onClose,
  children,
}: {
  open: boolean
  onClose: () => void
  children: React.ReactNode
}) {
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
      <div
        className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        {children}
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Create Federation wizard                                           */
/* ------------------------------------------------------------------ */

function CreateFederationForm({
  onCreate,
  onCancel,
}: {
  onCreate: (name: string) => Promise<void>
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) {
      setError('Federation name is required')
      return
    }
    setSaving(true)
    setError('')
    try {
      await onCreate(trimmed)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create federation')
      setSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="p-5">
      <h3 className="text-lg font-semibold text-nvr-text-primary mb-4">Create Federation</h3>
      <label className="block text-sm text-nvr-text-secondary mb-1.5">Federation Name</label>
      <input
        ref={inputRef}
        type="text"
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="e.g. Acme Security Network"
        className="w-full bg-nvr-bg-tertiary border border-nvr-border rounded-lg px-3 py-2.5 text-sm text-nvr-text-primary placeholder:text-nvr-text-muted focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none mb-4"
        maxLength={128}
        disabled={saving}
      />
      {error && <p className="text-sm text-nvr-danger mb-3">{error}</p>}
      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          disabled={saving}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving || !name.trim()}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          {saving ? 'Creating...' : 'Create'}
        </button>
      </div>
    </form>
  )
}

/* ------------------------------------------------------------------ */
/*  Invite Peer dialog                                                 */
/* ------------------------------------------------------------------ */

function InviteDialog({
  onGenerate,
  onClose,
}: {
  onGenerate: () => Promise<InviteToken>
  onClose: () => void
}) {
  const [token, setToken] = useState<InviteToken | null>(null)
  const [generating, setGenerating] = useState(false)
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  const [countdown, setCountdown] = useState(0)

  const generate = useCallback(async () => {
    setGenerating(true)
    setError('')
    try {
      const result = await onGenerate()
      setToken(result)
      // Calculate countdown in seconds.
      const expires = new Date(result.expires_at).getTime()
      setCountdown(Math.max(0, Math.floor((expires - Date.now()) / 1000)))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate invite')
    } finally {
      setGenerating(false)
    }
  }, [onGenerate])

  // Auto-generate on mount.
  useEffect(() => {
    generate()
  }, [generate])

  // Countdown timer.
  useEffect(() => {
    if (countdown <= 0) return
    const id = setInterval(() => {
      setCountdown((prev) => {
        if (prev <= 1) {
          clearInterval(id)
          return 0
        }
        return prev - 1
      })
    }, 1000)
    return () => clearInterval(id)
  }, [countdown])

  const copyToken = async () => {
    if (!token) return
    try {
      await navigator.clipboard.writeText(token.token)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Fallback: select the text.
    }
  }

  const formatCountdown = (secs: number): string => {
    const m = Math.floor(secs / 60)
    const s = secs % 60
    return `${m}:${s.toString().padStart(2, '0')}`
  }

  return (
    <div className="p-5">
      <h3 className="text-lg font-semibold text-nvr-text-primary mb-2">Invite Peer Directory</h3>
      <p className="text-sm text-nvr-text-secondary mb-4">
        Share this token with the other directory admin. They will use it to join your federation.
      </p>

      {generating && (
        <div className="flex items-center justify-center py-6">
          <svg
            className="w-6 h-6 text-nvr-accent animate-spin"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
        </div>
      )}

      {error && <p className="text-sm text-nvr-danger mb-3">{error}</p>}

      {token && !generating && (
        <>
          <div className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-3 mb-3">
            <code className="text-sm text-nvr-accent break-all select-all font-mono leading-relaxed">
              {token.token}
            </code>
          </div>
          <div className="flex items-center justify-between mb-4">
            <button
              onClick={copyToken}
              className="flex items-center gap-1.5 text-sm text-nvr-accent hover:text-nvr-accent-hover transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
              </svg>
              {copied ? 'Copied!' : 'Copy token'}
            </button>
            <span className={`text-sm font-mono ${countdown <= 60 ? 'text-nvr-danger' : 'text-nvr-text-muted'}`}>
              {countdown > 0 ? `Expires in ${formatCountdown(countdown)}` : 'Expired'}
            </span>
          </div>
        </>
      )}

      <div className="flex justify-end">
        <button
          onClick={onClose}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Close
        </button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Join Federation dialog                                             */
/* ------------------------------------------------------------------ */

function JoinDialog({
  onJoin,
  onClose,
}: {
  onJoin: (token: string, onProgress?: (p: JoinProgress) => void) => Promise<void>
  onClose: () => void
}) {
  const [token, setToken] = useState('')
  const [progress, setProgress] = useState<JoinProgress | null>(null)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const inputRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const trimmed = token.trim()
    if (!trimmed) {
      setError('Paste the federation invite token')
      return
    }
    setBusy(true)
    setError('')
    setProgress(null)
    try {
      await onJoin(trimmed, setProgress)
      // Success: close after a brief delay so user sees "done".
      setTimeout(onClose, 600)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to join')
      setBusy(false)
    }
  }

  const stepIcon = (step: JoinProgress['step']) => {
    switch (step) {
      case 'done':
        return (
          <svg className="w-5 h-5 text-nvr-success" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
          </svg>
        )
      case 'error':
        return (
          <svg className="w-5 h-5 text-nvr-danger" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
          </svg>
        )
      default:
        return (
          <svg
            className="w-5 h-5 text-nvr-accent animate-spin"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path
              className="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
            />
          </svg>
        )
    }
  }

  return (
    <form onSubmit={handleSubmit} className="p-5">
      <h3 className="text-lg font-semibold text-nvr-text-primary mb-2">Join Federation</h3>
      <p className="text-sm text-nvr-text-secondary mb-4">
        Paste the invite token you received from the federation admin.
      </p>

      <textarea
        ref={inputRef}
        value={token}
        onChange={(e) => setToken(e.target.value)}
        placeholder="FED-..."
        rows={3}
        disabled={busy}
        className="w-full bg-nvr-bg-tertiary border border-nvr-border rounded-lg px-3 py-2.5 text-sm text-nvr-text-primary placeholder:text-nvr-text-muted focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none font-mono mb-4 resize-none"
      />

      {progress && (
        <div className="flex items-center gap-2 mb-4 p-3 bg-nvr-bg-tertiary rounded-lg border border-nvr-border">
          {stepIcon(progress.step)}
          <span className="text-sm text-nvr-text-primary">{progress.message}</span>
        </div>
      )}

      {error && !progress && <p className="text-sm text-nvr-danger mb-3">{error}</p>}

      <div className="flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          disabled={busy && progress?.step !== 'error'}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={busy || !token.trim()}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          {busy ? 'Joining...' : 'Join'}
        </button>
      </div>
    </form>
  )
}

/* ------------------------------------------------------------------ */
/*  Peer list table                                                    */
/* ------------------------------------------------------------------ */

function PeerTable({
  peers,
  onRemove,
}: {
  peers: { id: string; name: string; endpoint: string; status: PeerStatus; last_sync: string | null; grants: string[] }[]
  onRemove: (id: string, name: string) => void
}) {
  if (peers.length === 0) {
    return (
      <div className="text-center py-8 text-nvr-text-muted text-sm">
        No peers yet. Invite another directory or join an existing federation.
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-nvr-border text-nvr-text-muted text-left">
            <th className="pb-2 pr-4 font-medium">Peer</th>
            <th className="pb-2 pr-4 font-medium">Status</th>
            <th className="pb-2 pr-4 font-medium hidden sm:table-cell">Last Sync</th>
            <th className="pb-2 pr-4 font-medium hidden md:table-cell">Grants</th>
            <th className="pb-2 font-medium text-right">Actions</th>
          </tr>
        </thead>
        <tbody>
          {peers.map((peer) => (
            <tr key={peer.id} className="border-b border-nvr-border/50 hover:bg-nvr-bg-tertiary/30 transition-colors">
              <td className="py-3 pr-4">
                <div className="flex items-center gap-2">
                  <div className="w-8 h-8 rounded-full bg-nvr-accent/20 text-nvr-accent text-xs font-bold flex items-center justify-center flex-shrink-0">
                    {peer.name.slice(0, 2).toUpperCase()}
                  </div>
                  <div>
                    <p className="text-nvr-text-primary font-medium">{peer.name}</p>
                    <p className="text-xs text-nvr-text-muted truncate max-w-[200px]">{peer.endpoint}</p>
                  </div>
                </div>
              </td>
              <td className="py-3 pr-4">
                <span className="inline-flex items-center gap-1.5">
                  <span className={`w-2 h-2 rounded-full ${statusColor(peer.status)}`} />
                  <span className="text-nvr-text-secondary">{statusLabel(peer.status)}</span>
                </span>
              </td>
              <td className="py-3 pr-4 text-nvr-text-secondary hidden sm:table-cell">
                {formatTimeAgo(peer.last_sync)}
              </td>
              <td className="py-3 pr-4 hidden md:table-cell">
                {peer.grants.length > 0 ? (
                  <div className="flex flex-wrap gap-1">
                    {peer.grants.slice(0, 3).map((g) => (
                      <span
                        key={g}
                        className="inline-block bg-nvr-bg-tertiary text-nvr-text-secondary text-xs px-2 py-0.5 rounded-full border border-nvr-border"
                      >
                        {g}
                      </span>
                    ))}
                    {peer.grants.length > 3 && (
                      <span className="text-xs text-nvr-text-muted">+{peer.grants.length - 3}</span>
                    )}
                  </div>
                ) : (
                  <span className="text-nvr-text-muted">None</span>
                )}
              </td>
              <td className="py-3 text-right">
                <button
                  onClick={() => onRemove(peer.id, peer.name)}
                  className="text-nvr-danger hover:text-nvr-danger-hover text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none px-2 py-1 rounded"
                  title={`Remove ${peer.name}`}
                >
                  Remove
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Empty state — no federation yet                                    */
/* ------------------------------------------------------------------ */

function EmptyState({
  onCreateClick,
  onJoinClick,
}: {
  onCreateClick: () => void
  onJoinClick: () => void
}) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      {/* Federation icon */}
      <div className="w-16 h-16 rounded-2xl bg-nvr-accent/10 flex items-center justify-center mb-6">
        <svg className="w-8 h-8 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.293-1.243a4.5 4.5 0 00-1.242-7.244l-4.5-4.5a4.5 4.5 0 00-6.364 6.364L4.34 8.364"
          />
        </svg>
      </div>
      <h2 className="text-xl font-semibold text-nvr-text-primary mb-2">No Federation</h2>
      <p className="text-sm text-nvr-text-secondary max-w-md mb-8">
        Create a federation to link multiple directory instances together, or join an existing one
        using an invite token.
      </p>
      <div className="flex flex-col sm:flex-row gap-3">
        <button
          onClick={onCreateClick}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-5 py-2.5 rounded-lg transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          + Create Federation
        </button>
        <button
          onClick={onJoinClick}
          className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-primary font-medium px-5 py-2.5 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Join Federation
        </button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main page                                                          */
/* ------------------------------------------------------------------ */

export default function Federation() {
  const { user } = useAuth()

  // Only admins can manage federations.
  if (user?.role !== 'admin') {
    return <Navigate to="/cameras" replace />
  }

  return <FederationContent />
}

function FederationContent() {
  const {
    federation,
    peers,
    loading,
    error,
    create,
    invite,
    join,
    remove,
    disband,
    refresh,
    clearError,
  } = useFederation()

  const [modal, setModal] = useState<'create' | 'invite' | 'join' | null>(null)
  const [removePeerTarget, setRemovePeerTarget] = useState<{ id: string; name: string } | null>(null)
  const [disbandConfirm, setDisbandConfirm] = useState(false)
  const [actionError, setActionError] = useState('')

  const handleCreate = async (name: string) => {
    await create(name)
    setModal(null)
  }

  const handleRemove = async () => {
    if (!removePeerTarget) return
    try {
      await remove(removePeerTarget.id)
      setRemovePeerTarget(null)
      setActionError('')
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to remove peer')
    }
  }

  const handleDisband = async () => {
    try {
      await disband()
      setDisbandConfirm(false)
      setActionError('')
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Failed to disband federation')
    }
  }

  /* Loading state */
  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <svg
          className="w-8 h-8 text-nvr-accent animate-spin"
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
          />
        </svg>
      </div>
    )
  }

  /* Error + no federation */
  if (!federation) {
    return (
      <div>
        {/* Page header */}
        <div className="flex items-center justify-between mb-6">
          <h1 className="text-2xl font-bold text-nvr-text-primary">Federation</h1>
        </div>

        {error && (
          <div className="bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg px-4 py-3 mb-6 flex items-start gap-2">
            <svg className="w-5 h-5 text-nvr-danger flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
            <div className="flex-1">
              <p className="text-sm text-nvr-danger">{error}</p>
              <button onClick={() => { clearError(); refresh() }} className="text-xs text-nvr-accent hover:underline mt-1">
                Retry
              </button>
            </div>
          </div>
        )}

        <EmptyState
          onCreateClick={() => setModal('create')}
          onJoinClick={() => setModal('join')}
        />

        {/* Modals */}
        <Modal open={modal === 'create'} onClose={() => setModal(null)}>
          <CreateFederationForm onCreate={handleCreate} onCancel={() => setModal(null)} />
        </Modal>
        <Modal open={modal === 'join'} onClose={() => setModal(null)}>
          <JoinDialog onJoin={join} onClose={() => setModal(null)} />
        </Modal>
      </div>
    )
  }

  /* Federation exists — show header + peer list */
  return (
    <div>
      {/* Page header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold text-nvr-text-primary">Federation</h1>
          <p className="text-sm text-nvr-text-secondary mt-1">
            <span className="font-medium text-nvr-text-primary">{federation.name}</span>
            {' '}&middot; {federation.peer_count} peer{federation.peer_count !== 1 ? 's' : ''}
          </p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <button
            onClick={() => setModal('invite')}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            + Invite Peer
          </button>
          <button
            onClick={() => setModal('join')}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-primary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px] focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Join Federation
          </button>
          <button
            onClick={() => setDisbandConfirm(true)}
            className="text-nvr-danger hover:text-nvr-danger-hover text-sm font-medium px-3 py-2 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            title="Disband federation"
          >
            Disband
          </button>
        </div>
      </div>

      {/* Error banner */}
      {(error || actionError) && (
        <div className="bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg px-4 py-3 mb-6 flex items-start gap-2">
          <svg className="w-5 h-5 text-nvr-danger flex-shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <p className="text-sm text-nvr-danger">{actionError || error}</p>
        </div>
      )}

      {/* Peer list */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-5">
        <h2 className="text-base font-semibold text-nvr-text-primary mb-4">Peers</h2>
        <PeerTable
          peers={peers}
          onRemove={(id, name) => setRemovePeerTarget({ id, name })}
        />
      </div>

      {/* Modals */}
      <Modal open={modal === 'invite'} onClose={() => setModal(null)}>
        <InviteDialog onGenerate={invite} onClose={() => setModal(null)} />
      </Modal>
      <Modal open={modal === 'join'} onClose={() => setModal(null)}>
        <JoinDialog onJoin={join} onClose={() => setModal(null)} />
      </Modal>

      {/* Confirm remove peer */}
      <ConfirmDialog
        open={!!removePeerTarget}
        title="Remove Peer"
        message={`Are you sure you want to remove "${removePeerTarget?.name}" from the federation? This will revoke all shared access.`}
        confirmLabel="Remove"
        confirmVariant="danger"
        onConfirm={handleRemove}
        onCancel={() => setRemovePeerTarget(null)}
      />

      {/* Confirm disband */}
      <ConfirmDialog
        open={disbandConfirm}
        title="Disband Federation"
        message="This will permanently remove the federation and disconnect all peers. This action cannot be undone."
        confirmLabel="Disband"
        confirmVariant="danger"
        onConfirm={handleDisband}
        onCancel={() => setDisbandConfirm(false)}
      />
    </div>
  )
}
