/* Recursive tree node component for sub-reseller hierarchy (KAI-314). */

import { useState } from 'react'
import type { ResellerNode } from './types'
import { TIER_META, childTierOf } from './types'

interface TreeNodeProps {
  node: ResellerNode
  depth: number
  onEdit: (node: ResellerNode) => void
  onDelete: (node: ResellerNode) => void
  onAddChild: (parent: ResellerNode) => void
  onMove: (node: ResellerNode) => void
}

export default function ResellerTreeNode({
  node,
  depth,
  onEdit,
  onDelete,
  onAddChild,
  onMove,
}: TreeNodeProps) {
  const [expanded, setExpanded] = useState(true)
  const meta = TIER_META[node.tier]
  const canAddChild = childTierOf(node.tier) !== null
  const hasChildren = node.children && node.children.length > 0

  return (
    <div className="select-none">
      {/* Node row */}
      <div
        className="group flex items-center gap-2 py-2 px-3 rounded-lg hover:bg-nvr-bg-tertiary/50 transition-colors"
        style={{ marginLeft: `${depth * 24}px` }}
      >
        {/* Expand / collapse toggle */}
        <button
          onClick={() => setExpanded(!expanded)}
          className={`w-5 h-5 flex items-center justify-center rounded transition-colors text-nvr-text-muted hover:text-nvr-text-primary ${
            !hasChildren ? 'invisible' : ''
          }`}
          aria-label={expanded ? 'Collapse' : 'Expand'}
        >
          <svg
            className={`w-3.5 h-3.5 transition-transform ${expanded ? 'rotate-90' : ''}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
          </svg>
        </button>

        {/* Tier badge */}
        <span
          className="text-[10px] font-bold uppercase tracking-wider px-1.5 py-0.5 rounded"
          style={{ backgroundColor: meta.color + '22', color: meta.color }}
        >
          {meta.label}
        </span>

        {/* Name */}
        <span className="text-sm font-medium text-nvr-text-primary flex-1 truncate">
          {node.name}
        </span>

        {/* Permission summary */}
        <span className="text-xs text-nvr-text-muted hidden sm:inline-flex items-center gap-1">
          <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
          </svg>
          {node.permissions.maxCameras}
          <span className="mx-1 text-nvr-border">|</span>
          <svg className="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
          </svg>
          {node.permissions.maxCustomers}
        </span>

        {/* Action buttons (visible on hover) */}
        <div className="opacity-0 group-hover:opacity-100 flex items-center gap-1 transition-opacity">
          {canAddChild && (
            <button
              onClick={() => onAddChild(node)}
              className="w-7 h-7 flex items-center justify-center rounded text-nvr-text-muted hover:text-nvr-accent hover:bg-nvr-accent/10 transition-colors"
              title={`Add ${childTierOf(node.tier)} child`}
            >
              <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
              </svg>
            </button>
          )}
          <button
            onClick={() => onEdit(node)}
            className="w-7 h-7 flex items-center justify-center rounded text-nvr-text-muted hover:text-nvr-accent hover:bg-nvr-accent/10 transition-colors"
            title="Edit"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
          </button>
          <button
            onClick={() => onMove(node)}
            className="w-7 h-7 flex items-center justify-center rounded text-nvr-text-muted hover:text-nvr-accent hover:bg-nvr-accent/10 transition-colors"
            title="Move to different parent"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
            </svg>
          </button>
          <button
            onClick={() => onDelete(node)}
            className="w-7 h-7 flex items-center justify-center rounded text-nvr-text-muted hover:text-nvr-danger hover:bg-nvr-danger/10 transition-colors"
            title="Delete"
          >
            <svg className="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </button>
        </div>
      </div>

      {/* Children */}
      {expanded && hasChildren && (
        <div>
          {node.children.map(child => (
            <ResellerTreeNode
              key={child.id}
              node={child}
              depth={depth + 1}
              onEdit={onEdit}
              onDelete={onDelete}
              onAddChild={onAddChild}
              onMove={onMove}
            />
          ))}
        </div>
      )}
    </div>
  )
}
