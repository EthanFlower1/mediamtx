import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

export interface Branding {
  product_name: string
  accent_color: string
  logo_url: string
}

const DEFAULT_BRANDING: Branding = {
  product_name: 'MediaMTX NVR',
  accent_color: '#3B82F6',
  logo_url: '',
}

// Global branding state shared across hook instances.
let globalBranding: Branding = { ...DEFAULT_BRANDING }
let listeners: Array<(b: Branding) => void> = []
let fetched = false

function notifyListeners(b: Branding) {
  globalBranding = b
  listeners.forEach(fn => fn(b))
}

export function useBranding(isAuthenticated: boolean) {
  const [branding, setBranding] = useState<Branding>(globalBranding)

  useEffect(() => {
    listeners.push(setBranding)
    // Sync with current global state.
    setBranding(globalBranding)
    return () => {
      listeners = listeners.filter(fn => fn !== setBranding)
    }
  }, [])

  const fetchBranding = useCallback(async () => {
    try {
      const res = await apiFetch('/system/branding')
      if (res.ok) {
        const data: Branding = await res.json()
        notifyListeners(data)
      }
    } catch {
      // Ignore errors, keep defaults.
    }
  }, [])

  useEffect(() => {
    if (isAuthenticated && !fetched) {
      fetched = true
      fetchBranding()
    }
  }, [isAuthenticated, fetchBranding])

  const refetch = useCallback(() => {
    fetchBranding()
  }, [fetchBranding])

  return { branding, refetch }
}
