/* Sub-Reseller Hierarchy page for the Integrator Portal (KAI-314).
   Renders a 3-level tree (NSC -> Regional -> City) with full CRUD
   and permission-narrowing enforcement. */

import { useState, useCallback } from 'react'
import { useResellerTree } from '../../hooks/useResellerTree'
import type { ResellerNode, ResellerPayload, ResellerTier } from './types'
import { childTierOf, TIER_META } from './types'
import ResellerTreeNode from './ResellerTreeNode'
import ResellerFormModal from './ResellerFormModal'
import MoveModal from './MoveModal'
import ConfirmDialog from '../../components/ConfirmDialog'

type ModalMode =
  | { kind: 'closed' }
  | { kind: 'create'; parent: ResellerNode | null; tier: ResellerTier }
  | { kind: 'edit'; node: ResellerNode; parent: ResellerNode | null }

interface MoveState {
  open: boolean
  node: ResellerNode | null
}

/** Walk the tree to find the parent of a node by parentId. */
function findNode(tree: ResellerNode[], id: string): ResellerNode | null {
  for (const n of tree) {
    if (n.id === id) return n
    if (n.children?.length) {
      const found = findNode(n.children, id)
      if (found) return found
    }
  }
  return null
}

/** Count total nodes in the tree. */
function countNodes(nodes: ResellerNode[]): number {
  let count = 0
  for (const n of nodes) {
    count += 1
    if (n.children?.length) count += countNodes(n.children)
  }
  return count
}

/** Count nodes at a specific tier. */
function countByTier(nodes: ResellerNode[], tier: ResellerTier): number {
  let count = 0
  for (const n of nodes) {
    if (n.tier === tier) count += 1
    if (n.children?.length) count += countByTier(n.children, tier)
  }
  return count
}

export default function SubResellerHierarchy() {
  const { tree, loading, error, create, update, remove, move } = useResellerTree()
  const [modal, setModal] = useState<ModalMode>({ kind: 'closed' })
  const [moveState, setMoveState] = useState<MoveState>({ open: false, node: null })
  const [deleteTarget, setDeleteTarget] = useState<ResellerNode | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const closeModal = useCallback(() => setModal({ kind: 'closed' }), [])

  // Handlers for tree node actions.
  const handleAddChild = useCallback((parent: ResellerNode) => {
    const tier = childTierOf(parent.tier)
    if (!tier) return
    setModal({ kind: 'create', parent, tier })
  }, [])

  const handleEdit = useCallback((node: ResellerNode) => {
    const parent = node.parentId ? findNode(tree, node.parentId) : null
    setModal({ kind: 'edit', node, parent })
  }, [tree])

  const handleMove = useCallback((node: ResellerNode) => {
    setMoveState({ open: true, node })
  }, [])

  const handleDelete = useCallback((node: ResellerNode) => {
    setDeleteTarget(node)
  }, [])

  // Save handler dispatches to create or update.
  const handleSave = useCallback(async (payload: ResellerPayload) => {
    setActionError(null)
    if (modal.kind === 'create') {
      await create(payload)
    } else if (modal.kind === 'edit') {
      await update(modal.node.id, payload)
    }
  }, [modal, create, update])

  const handleMoveConfirm = useCallback(async (nodeId: string, newParentId: string) => {
    setActionError(null)
    await move({ nodeId, newParentId })
  }, [move])

  const handleDeleteConfirm = useCallback(async () => {
    if (!deleteTarget) return
    setActionError(null)
    try {
      await remove(deleteTarget.id)
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Delete failed')
    }
    setDeleteTarget(null)
  }, [deleteTarget, remove])

  // Stats for the header.
  const totalNodes = countNodes(tree)
  const nscCount = countByTier(tree, 'nsc')
  const regionalCount = countByTier(tree, 'regional')
  const cityCount = countByTier(tree, 'city')

  return (
    <div>
      {/* Page header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold text-nvr-text-primary">Sub-Reseller Hierarchy</h1>
          <p className="text-sm text-nvr-text-muted mt-1">
            Manage your integrator network: NSC, Regional, and City-level resellers.
          </p>
        </div>
        <button
          onClick={() => setModal({ kind: 'create', parent: null, tier: 'nsc' })}
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2.5 rounded-lg transition-colors text-sm flex items-center gap-2 min-h-[44px] self-start"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          Add NSC
        </button>
      </div>

      {/* Stats bar */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
        {[
          { label: 'Total', value: totalNodes, color: '#94a3b8' },
          { label: 'NSC', value: nscCount, color: TIER_META.nsc.color },
          { label: 'Regional', value: regionalCount, color: TIER_META.regional.color },
          { label: 'City', value: cityCount, color: TIER_META.city.color },
        ].map(stat => (
          <div
            key={stat.label}
            className="bg-nvr-bg-secondary border border-nvr-border rounded-lg px-4 py-3"
          >
            <p className="text-xs font-medium text-nvr-text-muted uppercase tracking-wide">{stat.label}</p>
            <p className="text-xl font-bold mt-0.5" style={{ color: stat.color }}>{stat.value}</p>
          </div>
        ))}
      </div>

      {/* Error banner */}
      {(error || actionError) && (
        <div className="bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg px-4 py-3 text-sm text-nvr-danger mb-4">
          {error || actionError}
        </div>
      )}

      {/* Loading state */}
      {loading && (
        <div className="flex items-center justify-center py-16">
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

      {/* Empty state */}
      {!loading && tree.length === 0 && !error && (
        <div className="text-center py-16 bg-nvr-bg-secondary border border-nvr-border rounded-xl">
          <svg className="w-12 h-12 mx-auto text-nvr-text-muted mb-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 21h16.5M4.5 3h15M5.25 3v18m13.5-18v18M9 6.75h1.5m-1.5 3h1.5m-1.5 3h1.5m3-6H15m-1.5 3H15m-1.5 3H15M9 21v-3.375c0-.621.504-1.125 1.125-1.125h3.75c.621 0 1.125.504 1.125 1.125V21" />
          </svg>
          <h3 className="text-lg font-semibold text-nvr-text-primary mb-1">No resellers yet</h3>
          <p className="text-sm text-nvr-text-muted mb-4">
            Create your first NSC-level reseller to start building the hierarchy.
          </p>
          <button
            onClick={() => setModal({ kind: 'create', parent: null, tier: 'nsc' })}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm inline-flex items-center gap-2"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
            </svg>
            Add NSC
          </button>
        </div>
      )}

      {/* Tree */}
      {!loading && tree.length > 0 && (
        <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
          {tree.map(node => (
            <ResellerTreeNode
              key={node.id}
              node={node}
              depth={0}
              onEdit={handleEdit}
              onDelete={handleDelete}
              onAddChild={handleAddChild}
              onMove={handleMove}
            />
          ))}
        </div>
      )}

      {/* Create / Edit modal */}
      <ResellerFormModal
        open={modal.kind === 'create' || modal.kind === 'edit'}
        node={modal.kind === 'edit' ? modal.node : undefined}
        parent={modal.kind === 'create' ? modal.parent : modal.kind === 'edit' ? modal.parent : null}
        createTier={modal.kind === 'create' ? modal.tier : undefined}
        onSave={handleSave}
        onClose={closeModal}
      />

      {/* Move modal */}
      <MoveModal
        open={moveState.open}
        node={moveState.node}
        tree={tree}
        onMove={handleMoveConfirm}
        onClose={() => setMoveState({ open: false, node: null })}
      />

      {/* Delete confirmation */}
      <ConfirmDialog
        open={!!deleteTarget}
        title="Delete Reseller"
        message={
          deleteTarget
            ? `Delete "${deleteTarget.name}" and all its sub-resellers? This action cannot be undone.`
            : ''
        }
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={handleDeleteConfirm}
        onCancel={() => setDeleteTarget(null)}
      />
    </div>
  )
}
