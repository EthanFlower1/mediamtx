import { useState, useEffect } from 'react'
import CloudSettings from '../components/CloudSettings'

type TabId =
  | 'system'
  | 'cloud'
  | 'audit'
  | 'storage'
  | 'ai'
  | 'notifications'
  | 'appearance'
  | 'config'
  | 'performance'

const TABS: { id: TabId; label: string; ready: boolean }[] = [
  { id: 'system', label: 'System', ready: true },
  { id: 'cloud', label: 'Remote Access', ready: true },
  { id: 'audit', label: 'Audit Log', ready: true },
  { id: 'storage', label: 'Storage', ready: false },
  { id: 'ai', label: 'AI Analytics', ready: false },
  { id: 'notifications', label: 'Notifications', ready: false },
  { id: 'appearance', label: 'Appearance', ready: false },
  { id: 'config', label: 'Configuration', ready: false },
  { id: 'performance', label: 'Performance', ready: false },
]

interface SystemInfo {
  status: string
  mode: string
  version: string
  serverName: string
}

interface AuditEntry {
  id?: number
  timestamp: string
  actor: string
  action: string
  resource: string
  detail?: string
}

function SystemTab() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    Promise.all([
      fetch('/api/nvr/system/health').then(r => r.ok ? r.json() : null).catch(() => null),
      fetch('/api/v1/discover').then(r => r.ok ? r.json() : null).catch(() => null),
    ]).then(([health, discover]) => {
      if (cancelled) return
      if (!health && !discover) {
        setError('Unable to reach server')
        return
      }
      setInfo({
        status: health?.status ?? 'unknown',
        mode: health?.mode ?? 'unknown',
        version: discover?.version ?? discover?.server_version ?? 'unknown',
        serverName: discover?.server_name ?? discover?.name ?? 'Raikada NVR',
      })
    })
    return () => { cancelled = true }
  }, [])

  if (error) {
    return (
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6">
        <p className="text-nvr-text-secondary text-sm">{error}</p>
      </div>
    )
  }

  if (!info) {
    return (
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 flex items-center gap-2">
        <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
        <span className="text-nvr-text-secondary text-sm">Loading...</span>
      </div>
    )
  }

  const rows = [
    { label: 'Server', value: info.serverName },
    { label: 'Status', value: info.status },
    { label: 'Mode', value: info.mode },
    { label: 'Version', value: info.version },
  ]

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6">
      <h2 className="text-lg font-semibold text-nvr-text-primary mb-4">System Information</h2>
      <dl className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {rows.map(r => (
          <div key={r.label}>
            <dt className="text-xs text-nvr-text-tertiary uppercase tracking-wide mb-1">{r.label}</dt>
            <dd className="text-sm text-nvr-text-primary font-medium">{r.value}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

function AuditTab() {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    fetch('/api/v1/admin/audit')
      .then(r => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return r.json()
      })
      .then(data => {
        const list = Array.isArray(data) ? data : data?.entries ?? data?.items ?? []
        setEntries(list)
      })
      .catch(() => setError('Failed to load audit log'))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6 flex items-center gap-2">
        <span className="inline-block w-4 h-4 border-2 border-nvr-accent/30 border-t-nvr-accent rounded-full animate-spin" />
        <span className="text-nvr-text-secondary text-sm">Loading audit log...</span>
      </div>
    )
  }

  if (error) {
    return (
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6">
        <p className="text-nvr-text-secondary text-sm">{error}</p>
      </div>
    )
  }

  if (entries.length === 0) {
    return (
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6">
        <p className="text-nvr-text-tertiary text-sm">No audit entries yet.</p>
      </div>
    )
  }

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl overflow-hidden">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-nvr-border text-nvr-text-tertiary text-xs uppercase tracking-wide">
              <th className="text-left px-4 py-3 font-medium">Time</th>
              <th className="text-left px-4 py-3 font-medium">Actor</th>
              <th className="text-left px-4 py-3 font-medium">Action</th>
              <th className="text-left px-4 py-3 font-medium">Resource</th>
              <th className="text-left px-4 py-3 font-medium">Detail</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={e.id ?? i} className="border-b border-nvr-border last:border-b-0 hover:bg-nvr-bg-primary/50 transition-colors">
                <td className="px-4 py-2.5 text-nvr-text-secondary whitespace-nowrap">
                  {e.timestamp ? new Date(e.timestamp).toLocaleString() : '-'}
                </td>
                <td className="px-4 py-2.5 text-nvr-text-primary">{e.actor || '-'}</td>
                <td className="px-4 py-2.5 text-nvr-text-primary">{e.action || '-'}</td>
                <td className="px-4 py-2.5 text-nvr-text-secondary">{e.resource || '-'}</td>
                <td className="px-4 py-2.5 text-nvr-text-tertiary truncate max-w-[200px]">{e.detail || '-'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function ComingSoonTab({ label }: { label: string }) {
  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-12 flex flex-col items-center justify-center text-center">
      <div className="w-12 h-12 rounded-full bg-nvr-bg-primary border border-nvr-border flex items-center justify-center mb-4">
        <svg className="w-6 h-6 text-nvr-text-tertiary" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M12 6v6h4.5m4.5 0a9 9 0 1 1-18 0 9 9 0 0 1 18 0Z" />
        </svg>
      </div>
      <h3 className="text-nvr-text-primary font-medium mb-1">{label}</h3>
      <p className="text-nvr-text-tertiary text-sm">Coming soon</p>
    </div>
  )
}

export default function Settings() {
  const [activeTab, setActiveTab] = useState<TabId>('system')

  const renderContent = () => {
    switch (activeTab) {
      case 'system':
        return <SystemTab />
      case 'cloud':
        return <CloudSettings />
      case 'audit':
        return <AuditTab />
      default: {
        const tab = TABS.find(t => t.id === activeTab)
        return <ComingSoonTab label={tab?.label ?? activeTab} />
      }
    }
  }

  return (
    <div className="p-4 md:p-6 max-w-5xl mx-auto">
      <h1 className="text-2xl font-bold text-nvr-text-primary mb-6">Settings</h1>

      {/* Tab bar */}
      <div className="flex gap-1 overflow-x-auto mb-6 border-b border-nvr-border pb-px">
        {TABS.map(tab => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`
              px-3 py-2 text-sm font-medium whitespace-nowrap rounded-t-lg transition-colors
              ${activeTab === tab.id
                ? 'text-nvr-text-primary border-b-2 border-nvr-accent bg-nvr-bg-secondary'
                : 'text-nvr-text-tertiary hover:text-nvr-text-secondary'
              }
            `}
          >
            {tab.label}
            {!tab.ready && (
              <span className="ml-1.5 text-[10px] text-nvr-text-tertiary opacity-60">TBD</span>
            )}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {renderContent()}
    </div>
  )
}
