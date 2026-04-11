import { useState, useEffect, useMemo } from 'react'
import { Navigate } from 'react-router-dom'
import { useAuth } from '../auth/context'
import {
  useIntegrations,
  CATEGORY_LABELS,
  type IntegrationCategory,
  type IntegrationDefinition,
  type IntegrationConfig,
} from '../hooks/useIntegrations'
import IntegrationWizard from '../components/IntegrationWizard'

/* ------------------------------------------------------------------ */
/*  Status indicator                                                   */
/* ------------------------------------------------------------------ */

function StatusDot({ status }: { status?: string }) {
  if (status === 'connected') {
    return <span className="w-2 h-2 rounded-full bg-green-400" title="Connected" />
  }
  if (status === 'error') {
    return <span className="w-2 h-2 rounded-full bg-nvr-danger" title="Error" />
  }
  return null
}

/* ------------------------------------------------------------------ */
/*  Integration card                                                   */
/* ------------------------------------------------------------------ */

interface IntegrationCardProps {
  definition: IntegrationDefinition
  config?: IntegrationConfig
  onConfigure: () => void
  onToggle?: (enabled: boolean) => void
}

function IntegrationCard({ definition, config, onConfigure, onToggle }: IntegrationCardProps) {
  const isConfigured = !!config
  const isEnabled = config?.enabled ?? false

  return (
    <div
      className={`bg-nvr-bg-secondary border rounded-xl p-4 transition-colors hover:border-nvr-accent/30 ${
        isConfigured
          ? config?.status === 'error'
            ? 'border-nvr-danger/30'
            : 'border-nvr-border'
          : 'border-nvr-border border-dashed'
      }`}
      data-testid={`integration-card-${definition.id}`}
    >
      <div className="flex items-start gap-3">
        {/* Icon */}
        <div className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
          isConfigured ? 'bg-nvr-accent/10' : 'bg-nvr-bg-tertiary'
        }`}>
          <svg
            className={`w-5 h-5 ${isConfigured ? 'text-nvr-accent' : 'text-nvr-text-muted'}`}
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d={definition.icon} />
          </svg>
        </div>

        {/* Info */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-0.5">
            <h3 className="text-sm font-medium text-nvr-text-primary truncate">{definition.name}</h3>
            <StatusDot status={config?.status} />
            {isConfigured && (
              <span className={`px-2 py-0.5 rounded text-[10px] font-medium ${
                isEnabled
                  ? 'bg-green-500/10 text-green-400'
                  : 'bg-nvr-bg-tertiary text-nvr-text-muted'
              }`}>
                {isEnabled ? 'Active' : 'Disabled'}
              </span>
            )}
          </div>
          <p className="text-xs text-nvr-text-muted line-clamp-2">{definition.description}</p>
        </div>
      </div>

      {/* Error message */}
      {config?.error_message && (
        <div className="mt-2 px-3 py-1.5 rounded bg-nvr-danger/5 border border-nvr-danger/10">
          <p className="text-xs text-nvr-danger truncate" title={config.error_message}>
            {config.error_message}
          </p>
        </div>
      )}

      {/* Actions */}
      <div className="flex items-center gap-2 mt-3 pt-3 border-t border-nvr-border/50">
        {isConfigured && onToggle && (
          <button
            type="button"
            role="switch"
            aria-checked={isEnabled}
            onClick={() => onToggle(!isEnabled)}
            className={`relative inline-flex h-5 w-9 items-center rounded-full transition-colors shrink-0 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
              isEnabled ? 'bg-nvr-accent' : 'bg-nvr-bg-tertiary'
            }`}
            data-testid={`toggle-${definition.id}`}
          >
            <span
              className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white transition-transform ${
                isEnabled ? 'translate-x-[18px]' : 'translate-x-0.5'
              }`}
            />
          </button>
        )}
        <button
          onClick={onConfigure}
          className={`flex-1 font-medium py-1.5 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
            isConfigured
              ? 'bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary border border-nvr-border'
              : 'bg-nvr-accent hover:bg-nvr-accent-hover text-white'
          }`}
          data-testid={`configure-${definition.id}`}
        >
          {isConfigured ? 'Edit' : 'Configure'}
        </button>
      </div>
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Category filter tabs                                               */
/* ------------------------------------------------------------------ */

function CategoryTabs({
  categories,
  selected,
  onSelect,
  configuredCount,
}: {
  categories: IntegrationCategory[]
  selected: IntegrationCategory | 'all'
  onSelect: (cat: IntegrationCategory | 'all') => void
  configuredCount: Record<string, number>
}) {
  return (
    <div className="flex flex-wrap gap-2 mb-6">
      <button
        onClick={() => onSelect('all')}
        className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
          selected === 'all'
            ? 'bg-nvr-accent/10 text-nvr-accent border border-nvr-accent/20'
            : 'bg-nvr-bg-secondary text-nvr-text-secondary border border-nvr-border hover:bg-nvr-bg-tertiary'
        }`}
      >
        All
      </button>
      {categories.map((cat) => (
        <button
          key={cat}
          onClick={() => onSelect(cat)}
          className={`px-3 py-1.5 rounded-lg text-sm font-medium transition-colors inline-flex items-center gap-1.5 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
            selected === cat
              ? 'bg-nvr-accent/10 text-nvr-accent border border-nvr-accent/20'
              : 'bg-nvr-bg-secondary text-nvr-text-secondary border border-nvr-border hover:bg-nvr-bg-tertiary'
          }`}
        >
          {CATEGORY_LABELS[cat]}
          {(configuredCount[cat] ?? 0) > 0 && (
            <span className="bg-nvr-accent/20 text-nvr-accent text-[10px] font-bold px-1.5 py-0.5 rounded-full">
              {configuredCount[cat]}
            </span>
          )}
        </button>
      ))}
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Main page                                                          */
/* ------------------------------------------------------------------ */

export default function Integrations() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const {
    definitions,
    configs,
    loading,
    error: loadError,
    saveConfig,
    deleteConfig,
    testConnection,
    toggleEnabled,
  } = useIntegrations()

  const [selectedCategory, setSelectedCategory] = useState<IntegrationCategory | 'all'>('all')
  const [wizardDef, setWizardDef] = useState<IntegrationDefinition | null>(null)
  const [wizardConfig, setWizardConfig] = useState<IntegrationConfig | undefined>(undefined)

  // Page title
  useEffect(() => {
    document.title = 'Integrations — MediaMTX NVR'
    return () => { document.title = 'MediaMTX NVR' }
  }, [])

  // Derive unique categories from definitions
  const categories = useMemo(() => {
    const cats = new Set(definitions.map((d) => d.category))
    return Array.from(cats) as IntegrationCategory[]
  }, [definitions])

  // Count of configured integrations per category
  const configuredCount = useMemo(() => {
    const counts: Record<string, number> = {}
    configs.forEach((c) => {
      const def = definitions.find((d) => d.id === c.integration_id)
      if (def) {
        counts[def.category] = (counts[def.category] ?? 0) + 1
      }
    })
    return counts
  }, [configs, definitions])

  // Filter by selected category
  const filteredDefinitions = useMemo(() => {
    if (selectedCategory === 'all') return definitions
    return definitions.filter((d) => d.category === selectedCategory)
  }, [definitions, selectedCategory])

  // Group by category
  const grouped = useMemo(() => {
    const groups: Record<string, IntegrationDefinition[]> = {}
    filteredDefinitions.forEach((d) => {
      if (!groups[d.category]) groups[d.category] = []
      groups[d.category].push(d)
    })
    return groups
  }, [filteredDefinitions])

  // Find config for a definition
  const getConfig = (defId: string) => configs.find((c) => c.integration_id === defId)

  const openWizard = (def: IntegrationDefinition) => {
    setWizardDef(def)
    setWizardConfig(getConfig(def.id))
  }

  const handleToggle = async (config: IntegrationConfig, enabled: boolean) => {
    await toggleEnabled(config.id, enabled)
  }

  const totalConfigured = configs.length
  const totalActive = configs.filter((c) => c.enabled).length

  if (!isAdmin) {
    return <Navigate to="/cameras" replace />
  }

  return (
    <div>
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-6">
        <div>
          <h1 className="text-xl md:text-2xl font-bold text-nvr-text-primary">Integrations</h1>
          <p className="text-sm text-nvr-text-muted mt-0.5">
            {totalConfigured > 0
              ? `${totalConfigured} configured, ${totalActive} active`
              : 'Connect third-party services to extend your NVR'}
          </p>
        </div>
      </div>

      {/* Loading */}
      {loading && (
        <div className="flex items-center justify-center py-16">
          <span className="inline-block w-6 h-6 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
        </div>
      )}

      {/* Error */}
      {loadError && (
        <div className="bg-nvr-danger/10 border border-nvr-danger/20 rounded-xl p-4 text-sm text-nvr-danger mb-6">
          {loadError}
        </div>
      )}

      {/* Content */}
      {!loading && (
        <>
          {/* Category tabs */}
          <CategoryTabs
            categories={categories}
            selected={selectedCategory}
            onSelect={setSelectedCategory}
            configuredCount={configuredCount}
          />

          {/* Integration groups */}
          {Object.entries(grouped).map(([category, defs]) => (
            <div key={category} className="mb-8">
              <h2 className="text-sm font-semibold text-nvr-text-secondary uppercase tracking-wider mb-3">
                {CATEGORY_LABELS[category as IntegrationCategory]}
              </h2>
              <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
                {defs.map((def) => {
                  const config = getConfig(def.id)
                  return (
                    <IntegrationCard
                      key={def.id}
                      definition={def}
                      config={config}
                      onConfigure={() => openWizard(def)}
                      onToggle={
                        config
                          ? (enabled) => handleToggle(config, enabled)
                          : undefined
                      }
                    />
                  )
                })}
              </div>
            </div>
          ))}

          {/* Empty state when filtering */}
          {filteredDefinitions.length === 0 && (
            <div className="text-center py-12">
              <p className="text-nvr-text-muted">No integrations in this category.</p>
            </div>
          )}
        </>
      )}

      {/* Wizard modal */}
      {wizardDef && (
        <IntegrationWizard
          definition={wizardDef}
          existingConfig={wizardConfig}
          onSave={(enabled, config) => saveConfig(wizardDef.id, enabled, config)}
          onTest={(config) => testConnection(wizardDef.id, config)}
          onDelete={
            wizardConfig
              ? () => deleteConfig(wizardConfig.id)
              : undefined
          }
          onClose={() => {
            setWizardDef(null)
            setWizardConfig(undefined)
          }}
        />
      )}
    </div>
  )
}
