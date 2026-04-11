/* Sub-reseller hierarchy types for the integrator portal (KAI-314). */

/** The three tiers in the NSC -> Regional -> City hierarchy. */
export type ResellerTier = 'nsc' | 'regional' | 'city'

/** Permission scope that narrows as you descend the tree. */
export interface PermissionScope {
  /** Maximum number of cameras this reseller may manage. */
  maxCameras: number
  /** Maximum number of end-customer accounts. */
  maxCustomers: number
  /** Feature flags inherited (and narrowable) from parent. */
  features: string[]
  /** Geographic regions this reseller can operate in. */
  regions: string[]
}

/** A single node in the sub-reseller tree. */
export interface ResellerNode {
  id: string
  parentId: string | null
  name: string
  tier: ResellerTier
  permissions: PermissionScope
  children: ResellerNode[]
  createdAt: string
  updatedAt: string
}

/** Payload for creating / updating a reseller. */
export interface ResellerPayload {
  name: string
  parentId: string | null
  tier: ResellerTier
  permissions: PermissionScope
}

/** Payload for moving a node to a new parent. */
export interface MovePayload {
  nodeId: string
  newParentId: string
}

/** Tier metadata for rendering. */
export const TIER_META: Record<ResellerTier, { label: string; color: string; depth: number }> = {
  nsc:      { label: 'NSC',      color: '#6366f1', depth: 0 },
  regional: { label: 'Regional', color: '#22d3ee', depth: 1 },
  city:     { label: 'City',     color: '#a78bfa', depth: 2 },
}

/** Returns the child tier for a given tier, or null if leaf. */
export function childTierOf(tier: ResellerTier): ResellerTier | null {
  if (tier === 'nsc') return 'regional'
  if (tier === 'regional') return 'city'
  return null
}

/** Validates that `child` permissions do not exceed `parent` permissions. */
export function isPermissionNarrowed(parent: PermissionScope, child: PermissionScope): boolean {
  if (child.maxCameras > parent.maxCameras) return false
  if (child.maxCustomers > parent.maxCustomers) return false
  // Every child feature must exist in parent features.
  if (!child.features.every(f => parent.features.includes(f))) return false
  // Every child region must exist in parent regions.
  if (!child.regions.every(r => parent.regions.includes(r))) return false
  return true
}
