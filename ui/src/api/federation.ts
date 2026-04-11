import { apiFetch } from './client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

export interface Federation {
  id: string
  name: string
  created_at: string
  peer_count: number
}

export type PeerStatus = 'connected' | 'disconnected' | 'pending' | 'error'

export interface FederationPeer {
  id: string
  federation_id: string
  name: string
  endpoint: string
  status: PeerStatus
  last_sync: string | null
  grants: string[]
  joined_at: string
}

export interface InviteToken {
  token: string
  expires_at: string
}

export interface JoinProgress {
  step: 'connecting' | 'handshake' | 'syncing' | 'done' | 'error'
  message: string
}

/* ------------------------------------------------------------------ */
/*  API calls                                                          */
/* ------------------------------------------------------------------ */

/** Fetch the current federation (if any) and its peers. */
export async function getFederation(): Promise<{
  federation: Federation | null
  peers: FederationPeer[]
}> {
  const res = await apiFetch('/federation')
  if (res.status === 404) {
    return { federation: null, peers: [] }
  }
  if (!res.ok) throw new Error(`Failed to fetch federation: ${res.status}`)
  return res.json()
}

/** Create a new federation with the given name. */
export async function createFederation(name: string): Promise<Federation> {
  const res = await apiFetch('/federation', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Failed to create federation: ${res.status}`)
  }
  return res.json()
}

/** Generate an invite token for the current federation. */
export async function generateInvite(): Promise<InviteToken> {
  const res = await apiFetch('/federation/invite', {
    method: 'POST',
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Failed to generate invite: ${res.status}`)
  }
  return res.json()
}

/** Join a federation using an invite token. */
export async function joinFederation(
  token: string,
  onProgress?: (progress: JoinProgress) => void,
): Promise<void> {
  // Notify the caller of initial connection step.
  onProgress?.({ step: 'connecting', message: 'Connecting to peer...' })

  const res = await apiFetch('/federation/join', {
    method: 'POST',
    body: JSON.stringify({ token }),
  })

  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    onProgress?.({ step: 'error', message: body.error || 'Join failed' })
    throw new Error(body.error || `Failed to join federation: ${res.status}`)
  }

  onProgress?.({ step: 'handshake', message: 'Performing handshake...' })

  // Small delay to let the caller see the progress UI.
  await new Promise((r) => setTimeout(r, 400))

  onProgress?.({ step: 'syncing', message: 'Synchronising directory...' })
  await new Promise((r) => setTimeout(r, 400))

  onProgress?.({ step: 'done', message: 'Joined successfully' })
}

/** Remove a peer from the federation. */
export async function removePeer(peerId: string): Promise<void> {
  const res = await apiFetch(`/federation/peers/${peerId}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Failed to remove peer: ${res.status}`)
  }
}

/** Delete (disband) the current federation entirely. */
export async function deleteFederation(): Promise<void> {
  const res = await apiFetch('/federation', {
    method: 'DELETE',
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Failed to delete federation: ${res.status}`)
  }
}
