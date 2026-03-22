import { useState, useEffect } from 'react'
import { apiFetch } from '../api/client'

interface AnalyticsModule {
  name: string
  type: string
  parameters: Record<string, string>
}

interface AnalyticsRule {
  name: string
  type: string
  parameters: Record<string, string>
}

interface Props {
  cameraId: string
}

export default function AnalyticsConfig({ cameraId }: Props) {
  const [modules, setModules] = useState<AnalyticsModule[]>([])
  const [rules, setRules] = useState<AnalyticsRule[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [deletingRule, setDeletingRule] = useState<string | null>(null)

  const fetchData = () => {
    setLoading(true)
    setError(null)

    Promise.all([
      apiFetch(`/cameras/${cameraId}/analytics/modules`).then(res => {
        if (!res.ok) throw new Error('Failed to load analytics modules')
        return res.json()
      }),
      apiFetch(`/cameras/${cameraId}/analytics/rules`).then(res => {
        if (!res.ok) throw new Error('Failed to load analytics rules')
        return res.json()
      }),
    ])
      .then(([modulesData, rulesData]) => {
        setModules(modulesData || [])
        setRules(rulesData || [])
      })
      .catch(err => {
        setError(err.message || 'Analytics not available')
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchData()
  }, [cameraId])

  const handleDeleteRule = async (ruleName: string) => {
    setDeletingRule(ruleName)
    try {
      const res = await apiFetch(`/cameras/${cameraId}/analytics/rules/${encodeURIComponent(ruleName)}`, {
        method: 'DELETE',
      })
      if (res.ok) {
        setRules(prev => prev.filter(r => r.name !== ruleName))
      }
    } catch {
      // silently fail
    } finally {
      setDeletingRule(null)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center gap-2 py-4">
        <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
        <span className="text-sm text-nvr-text-muted">Loading analytics configuration...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="py-4 text-center">
        <p className="text-sm text-nvr-text-muted">Analytics not available</p>
        <p className="text-xs text-nvr-text-muted mt-1">{error}</p>
      </div>
    )
  }

  return (
    <div>
      {/* Analytics Modules */}
      <div className="mb-4">
        <h5 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide mb-2">
          Modules ({modules.length})
        </h5>
        {modules.length === 0 ? (
          <p className="text-sm text-nvr-text-muted">No analytics modules reported by camera.</p>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            {modules.map(mod => (
              <div
                key={mod.name}
                className="bg-nvr-bg-primary border border-nvr-border/50 rounded-lg p-3"
              >
                <div className="text-sm font-medium text-nvr-text-primary truncate" title={mod.name}>
                  {mod.name}
                </div>
                <div className="text-xs text-nvr-text-muted truncate mt-0.5" title={mod.type}>
                  {mod.type.split('/').pop() || mod.type}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Analytics Rules */}
      <div>
        <div className="flex items-center justify-between mb-2">
          <h5 className="text-xs font-semibold text-nvr-text-secondary uppercase tracking-wide">
            Rules ({rules.length})
          </h5>
        </div>
        {rules.length === 0 ? (
          <p className="text-sm text-nvr-text-muted">No analytics rules configured on this camera.</p>
        ) : (
          <div className="space-y-2">
            {rules.map(rule => (
              <div
                key={rule.name}
                className="flex items-center gap-3 bg-nvr-bg-primary border border-nvr-border/50 rounded-lg p-3"
              >
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-nvr-text-primary truncate" title={rule.name}>
                    {rule.name}
                  </div>
                  <div className="text-xs text-nvr-text-muted truncate mt-0.5" title={rule.type}>
                    {rule.type.split('/').pop() || rule.type}
                  </div>
                </div>
                <button
                  onClick={() => handleDeleteRule(rule.name)}
                  disabled={deletingRule === rule.name}
                  className="shrink-0 p-1.5 rounded text-nvr-text-muted hover:text-nvr-danger hover:bg-nvr-danger/10 transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  title="Delete rule"
                  aria-label={`Delete rule ${rule.name}`}
                >
                  {deletingRule === rule.name ? (
                    <span className="inline-block w-3.5 h-3.5 border-2 border-nvr-text-muted/30 border-t-nvr-text-muted rounded-full animate-spin" />
                  ) : (
                    <svg xmlns="http://www.w3.org/2000/svg" className="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
                      <path fillRule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clipRule="evenodd" />
                    </svg>
                  )}
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
