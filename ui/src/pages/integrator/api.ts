/* API layer for sub-reseller hierarchy (KAI-314). */

import { apiFetch } from '../../api/client'
import type { ResellerNode, ResellerPayload, MovePayload } from './types'

const BASE = '/integrator/resellers'

/** Fetch the full reseller tree for the current integrator. */
export async function fetchResellerTree(): Promise<ResellerNode[]> {
  const res = await apiFetch(BASE)
  if (!res.ok) throw new Error(`Failed to fetch reseller tree: ${res.status}`)
  return res.json()
}

/** Create a new reseller node. */
export async function createReseller(payload: ResellerPayload): Promise<ResellerNode> {
  const res = await apiFetch(BASE, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(err.error || `Create failed: ${res.status}`)
  }
  return res.json()
}

/** Update an existing reseller node. */
export async function updateReseller(id: string, payload: Partial<ResellerPayload>): Promise<ResellerNode> {
  const res = await apiFetch(`${BASE}/${id}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(err.error || `Update failed: ${res.status}`)
  }
  return res.json()
}

/** Delete a reseller node (and its descendants). */
export async function deleteReseller(id: string): Promise<void> {
  const res = await apiFetch(`${BASE}/${id}`, { method: 'DELETE' })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(err.error || `Delete failed: ${res.status}`)
  }
}

/** Move a node to a different parent. Server enforces permission narrowing. */
export async function moveReseller(payload: MovePayload): Promise<ResellerNode> {
  const res = await apiFetch(`${BASE}/move`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(err.error || `Move failed: ${res.status}`)
  }
  return res.json()
}
