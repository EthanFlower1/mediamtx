/* Move-node modal for sub-reseller hierarchy (KAI-314).
   Lets the user pick a new parent from eligible nodes. */

import { useState, useEffect, useMemo } from 'react'
import type { ResellerNode, ResellerTier } from './types'
import { TIER_META } from './types'

interface MoveModalProps {
  open: boolean
  node: ResellerNode | null
  tree: ResellerNode[]
  onMove: (nodeId: string, newParentId: string) => Promise<void>
  onClose: () => void
}

/** Flatten tree into a list, excluding a subtree rooted at `excludeId`. */
function flattenExcluding(nodes: ResellerNode[], excludeId: string): ResellerNode[] {
  const result: ResellerNode[] = []
  for (const n of nodes) {
    if (n.id === excludeId) continue
    result.push(n)
    if (n.children?.length) {
      result.push(...flattenExcluding(n.children, excludeId))
    }
  }
  return result
}

/** Return the tier that should be the parent of `childTier`. */
function parentTierOf(tier: ResellerTier): ResellerTier | null {
  if (tier === 'regional') return 'nsc'
  if (tier === 'city') return 'regional'
  return null
}

export default function MoveModal({ open, node, tree, onMove, onClose }: MoveModalProps) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (open) {
      setSelectedId(null)
      setError(null)
    }
  }, [open])

  // Escape key.
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  // Eligible parents: same tier as current parent, excluding self and descendants.
  const eligibleParents = useMemo(() => {
    if (!node) return []
    const requiredTier = parentTierOf(node.tier)
    if (!requiredTier) return [] // NSC nodes cannot be moved.
    const flat = flattenExcluding(tree, node.id)
    return flat.filter(n => n.tier === requiredTier && n.id !== node.parentId)
  }, [node, tree])

  async function handleMove() {
    if (!node || !selectedId) return
    setSaving(true)
    setError(null)
    try {
      await onMove(node.id, selectedId)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Move failed')
    } finally {
      setSaving(false)
    }
  }

  if (!open || !node) return null

  const isNSC = node.tier === 'nsc'

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm animate-fade-in" />
      <div
        className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-md w-full mx-4 animate-scale-in"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="px-5 py-4 border-b border-nvr-border">
          <h2 className="text-lg font-semibold text-nvr-text-primary">Move "{node.name}"</h2>
          <p className="text-xs text-nvr-text-muted mt-1">
            Select a new parent. Permissions will be re-validated after move.
          </p>
        </div>

        {/* Body */}
        <div className="px-5 py-4">
          {error && (
            <div className="bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg px-3 py-2 text-sm text-nvr-danger mb-3">
              {error}
            </div>
          )}

          {isNSC ? (
            <p className="text-sm text-nvr-text-secondary">
              NSC-level nodes are top-level and cannot be moved.
            </p>
          ) : eligibleParents.length === 0 ? (
            <p className="text-sm text-nvr-text-secondary">
              No eligible parents found for this node.
            </p>
          ) : (
            <div className="space-y-1 max-h-60 overflow-y-auto">
              {eligibleParents.map(p => {
                const meta = TIER_META[p.tier]
                return (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => setSelectedId(p.id)}
                    className={`w-full text-left flex items-center gap-2 px-3 py-2 rounded-lg border transition-colors text-sm ${
                      selectedId === p.id
                        ? 'border-nvr-accent bg-nvr-accent/10 text-nvr-text-primary'
                        : 'border-transparent hover:bg-nvr-bg-tertiary text-nvr-text-secondary'
                    }`}
                  >
                    <span
                      className="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded"
                      style={{ backgroundColor: meta.color + '22', color: meta.color }}
                    >
                      {meta.label}
                    </span>
                    <span className="truncate">{p.name}</span>
                  </button>
                )
              })}
            </div>
          )}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 px-5 py-4 border-t border-nvr-border">
          <button
            type="button"
            onClick={onClose}
            className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm min-h-[44px]"
          >
            Cancel
          </button>
          {!isNSC && eligibleParents.length > 0 && (
            <button
              type="button"
              onClick={handleMove}
              disabled={!selectedId || saving}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] disabled:opacity-50"
            >
              {saving ? 'Moving...' : 'Move'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
