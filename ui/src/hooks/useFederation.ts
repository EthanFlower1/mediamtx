import { useState, useEffect, useCallback } from 'react'
import {
  Federation,
  FederationPeer,
  getFederation,
  createFederation,
  generateInvite,
  joinFederation,
  removePeer,
  deleteFederation,
  InviteToken,
  JoinProgress,
} from '../api/federation'

export interface UseFederationResult {
  federation: Federation | null
  peers: FederationPeer[]
  loading: boolean
  error: string
  /** Create a new federation. */
  create: (name: string) => Promise<void>
  /** Generate an invite token. */
  invite: () => Promise<InviteToken>
  /** Join using an invite token. Reports progress via the callback. */
  join: (token: string, onProgress?: (p: JoinProgress) => void) => Promise<void>
  /** Remove a peer by ID. */
  remove: (peerId: string) => Promise<void>
  /** Disband the entire federation. */
  disband: () => Promise<void>
  /** Refresh the data from the server. */
  refresh: () => Promise<void>
  /** Clear the current error. */
  clearError: () => void
}

export function useFederation(): UseFederationResult {
  const [federation, setFederation] = useState<Federation | null>(null)
  const [peers, setPeers] = useState<FederationPeer[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      setLoading(true)
      const data = await getFederation()
      setFederation(data.federation)
      setPeers(data.peers)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load federation data')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const create = useCallback(async (name: string) => {
    const fed = await createFederation(name)
    setFederation(fed)
    setPeers([])
    setError('')
  }, [])

  const invite = useCallback(async () => {
    return generateInvite()
  }, [])

  const join = useCallback(async (token: string, onProgress?: (p: JoinProgress) => void) => {
    await joinFederation(token, onProgress)
    await refresh()
  }, [refresh])

  const remove = useCallback(async (peerId: string) => {
    await removePeer(peerId)
    setPeers((prev) => prev.filter((p) => p.id !== peerId))
    setFederation((prev) =>
      prev ? { ...prev, peer_count: Math.max(0, prev.peer_count - 1) } : prev,
    )
  }, [])

  const disband = useCallback(async () => {
    await deleteFederation()
    setFederation(null)
    setPeers([])
  }, [])

  const clearError = useCallback(() => setError(''), [])

  return {
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
  }
}
