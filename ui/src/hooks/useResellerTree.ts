/* Hook for managing the sub-reseller hierarchy (KAI-314). */

import { useState, useEffect, useCallback } from 'react'
import type { ResellerNode, ResellerPayload, MovePayload } from '../pages/integrator/types'
import * as api from '../pages/integrator/api'

interface UseResellerTreeReturn {
  tree: ResellerNode[]
  loading: boolean
  error: string | null
  /** Refetch the tree from the server. */
  refresh: () => Promise<void>
  /** Create a new reseller. */
  create: (payload: ResellerPayload) => Promise<void>
  /** Update an existing reseller. */
  update: (id: string, payload: Partial<ResellerPayload>) => Promise<void>
  /** Delete a reseller and its descendants. */
  remove: (id: string) => Promise<void>
  /** Move a node to a new parent. */
  move: (payload: MovePayload) => Promise<void>
}

export function useResellerTree(): UseResellerTreeReturn {
  const [tree, setTree] = useState<ResellerNode[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await api.fetchResellerTree()
      setTree(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load reseller hierarchy')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  const create = useCallback(async (payload: ResellerPayload) => {
    await api.createReseller(payload)
    await refresh()
  }, [refresh])

  const update = useCallback(async (id: string, payload: Partial<ResellerPayload>) => {
    await api.updateReseller(id, payload)
    await refresh()
  }, [refresh])

  const remove = useCallback(async (id: string) => {
    await api.deleteReseller(id)
    await refresh()
  }, [refresh])

  const move = useCallback(async (payload: MovePayload) => {
    await api.moveReseller(payload)
    await refresh()
  }, [refresh])

  return { tree, loading, error, refresh, create, update, remove, move }
}
