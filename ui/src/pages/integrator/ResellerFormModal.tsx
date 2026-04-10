/* Create / Edit modal for sub-reseller nodes (KAI-314). */

import { useState, useEffect, FormEvent } from 'react'
import type { ResellerNode, ResellerPayload, ResellerTier, PermissionScope } from './types'
import { TIER_META, childTierOf, isPermissionNarrowed } from './types'

interface ResellerFormModalProps {
  open: boolean
  /** The node being edited, or undefined for create mode. */
  node?: ResellerNode
  /** The parent node (required for create, available for edit). */
  parent?: ResellerNode | null
  /** The tier to create (only used in create mode). */
  createTier?: ResellerTier
  onSave: (payload: ResellerPayload) => Promise<void>
  onClose: () => void
}

const DEFAULT_FEATURES = ['live_view', 'playback', 'export', 'analytics', 'ptz_control']
const DEFAULT_REGIONS = ['north', 'south', 'east', 'west', 'central']

export default function ResellerFormModal({
  open,
  node,
  parent,
  createTier,
  onSave,
  onClose,
}: ResellerFormModalProps) {
  const isEdit = !!node
  const tier = isEdit ? node.tier : (createTier ?? 'nsc')
  const meta = TIER_META[tier]
  const parentPerms = parent?.permissions ?? null

  const [name, setName] = useState('')
  const [maxCameras, setMaxCameras] = useState(100)
  const [maxCustomers, setMaxCustomers] = useState(50)
  const [features, setFeatures] = useState<string[]>([...DEFAULT_FEATURES])
  const [regions, setRegions] = useState<string[]>([...DEFAULT_REGIONS])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Populate form when editing or when parent changes.
  useEffect(() => {
    if (!open) return
    if (isEdit && node) {
      setName(node.name)
      setMaxCameras(node.permissions.maxCameras)
      setMaxCustomers(node.permissions.maxCustomers)
      setFeatures([...node.permissions.features])
      setRegions([...node.permissions.regions])
    } else {
      setName('')
      // Default to parent limits (user must narrow).
      if (parentPerms) {
        setMaxCameras(parentPerms.maxCameras)
        setMaxCustomers(parentPerms.maxCustomers)
        setFeatures([...parentPerms.features])
        setRegions([...parentPerms.regions])
      } else {
        setMaxCameras(100)
        setMaxCustomers(50)
        setFeatures([...DEFAULT_FEATURES])
        setRegions([...DEFAULT_REGIONS])
      }
    }
    setError(null)
  }, [open, isEdit, node, parentPerms])

  // Escape key closes modal.
  useEffect(() => {
    if (!open) return
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onClose])

  function toggleItem(list: string[], item: string, setter: (v: string[]) => void) {
    if (list.includes(item)) {
      setter(list.filter(x => x !== item))
    } else {
      setter([...list, item])
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)

    if (!name.trim()) {
      setError('Name is required.')
      return
    }

    const permissions: PermissionScope = {
      maxCameras,
      maxCustomers,
      features,
      regions,
    }

    // Validate permission narrowing against parent.
    if (parentPerms && !isPermissionNarrowed(parentPerms, permissions)) {
      setError('Permissions cannot exceed the parent scope. Reduce cameras, customers, features, or regions.')
      return
    }

    const payload: ResellerPayload = {
      name: name.trim(),
      parentId: parent?.id ?? node?.parentId ?? null,
      tier,
      permissions,
    }

    setSaving(true)
    try {
      await onSave(payload)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Operation failed')
    } finally {
      setSaving(false)
    }
  }

  if (!open) return null

  // Available features/regions are limited by parent.
  const availableFeatures = parentPerms ? parentPerms.features : DEFAULT_FEATURES
  const availableRegions = parentPerms ? parentPerms.regions : DEFAULT_REGIONS

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm animate-fade-in" />
      <div
        className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto animate-scale-in"
        onClick={e => e.stopPropagation()}
      >
        <form onSubmit={handleSubmit}>
          {/* Header */}
          <div className="flex items-center justify-between px-5 py-4 border-b border-nvr-border">
            <div className="flex items-center gap-2">
              <span
                className="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded"
                style={{ backgroundColor: meta.color + '22', color: meta.color }}
              >
                {meta.label}
              </span>
              <h2 className="text-lg font-semibold text-nvr-text-primary">
                {isEdit ? 'Edit Reseller' : 'New Reseller'}
              </h2>
            </div>
            <button
              type="button"
              onClick={onClose}
              className="w-8 h-8 flex items-center justify-center rounded text-nvr-text-muted hover:text-nvr-text-primary transition-colors"
            >
              <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          </div>

          {/* Body */}
          <div className="px-5 py-4 space-y-4">
            {error && (
              <div className="bg-nvr-danger/10 border border-nvr-danger/30 rounded-lg px-3 py-2 text-sm text-nvr-danger">
                {error}
              </div>
            )}

            {/* Name */}
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-1">Name</label>
              <input
                type="text"
                value={name}
                onChange={e => setName(e.target.value)}
                className="w-full bg-nvr-bg-tertiary border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
                placeholder="e.g. Northeast Region"
                autoFocus
              />
            </div>

            {/* Max cameras */}
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-1">
                Max Cameras
                {parentPerms && <span className="text-nvr-text-muted font-normal"> (parent: {parentPerms.maxCameras})</span>}
              </label>
              <input
                type="number"
                min={0}
                max={parentPerms?.maxCameras ?? 10000}
                value={maxCameras}
                onChange={e => setMaxCameras(Number(e.target.value))}
                className="w-full bg-nvr-bg-tertiary border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
              />
            </div>

            {/* Max customers */}
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-1">
                Max Customers
                {parentPerms && <span className="text-nvr-text-muted font-normal"> (parent: {parentPerms.maxCustomers})</span>}
              </label>
              <input
                type="number"
                min={0}
                max={parentPerms?.maxCustomers ?? 10000}
                value={maxCustomers}
                onChange={e => setMaxCustomers(Number(e.target.value))}
                className="w-full bg-nvr-bg-tertiary border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:outline-none focus:ring-2 focus:ring-nvr-accent/50"
              />
            </div>

            {/* Features */}
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Features</label>
              <div className="flex flex-wrap gap-2">
                {availableFeatures.map(f => (
                  <button
                    key={f}
                    type="button"
                    onClick={() => toggleItem(features, f, setFeatures)}
                    className={`text-xs px-2.5 py-1 rounded-full border transition-colors ${
                      features.includes(f)
                        ? 'bg-nvr-accent/20 border-nvr-accent/40 text-nvr-accent'
                        : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-muted hover:text-nvr-text-secondary'
                    }`}
                  >
                    {f.replace(/_/g, ' ')}
                  </button>
                ))}
              </div>
            </div>

            {/* Regions */}
            <div>
              <label className="block text-sm font-medium text-nvr-text-secondary mb-2">Regions</label>
              <div className="flex flex-wrap gap-2">
                {availableRegions.map(r => (
                  <button
                    key={r}
                    type="button"
                    onClick={() => toggleItem(regions, r, setRegions)}
                    className={`text-xs px-2.5 py-1 rounded-full border transition-colors capitalize ${
                      regions.includes(r)
                        ? 'bg-nvr-accent/20 border-nvr-accent/40 text-nvr-accent'
                        : 'bg-nvr-bg-tertiary border-nvr-border text-nvr-text-muted hover:text-nvr-text-secondary'
                    }`}
                  >
                    {r}
                  </button>
                ))}
              </div>
            </div>
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
            <button
              type="submit"
              disabled={saving}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm min-h-[44px] disabled:opacity-50"
            >
              {saving ? 'Saving...' : isEdit ? 'Update' : 'Create'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
