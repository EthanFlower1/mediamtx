import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'
import { useAuth } from '../auth/context'
import { useCameras, Camera } from '../hooks/useCameras'
import ConfirmDialog from '../components/ConfirmDialog'

// --- Types ---

interface NotificationPref {
  id: number
  user_id: string
  camera_id: string
  event_type: string
  channel: string
  enabled: boolean
}

interface QuietHoursConfig {
  user_id: string
  enabled: boolean
  start_time: string
  end_time: string
  timezone: string
  days: string
}

interface EscalationStep {
  channel: string
  target: string
}

interface EscalationRule {
  id: number
  name: string
  event_type: string
  camera_id: string
  enabled: boolean
  delay_minutes: number
  repeat_count: number
  repeat_interval_minutes: number
  escalation_chain: string
  created_at?: string
}

const EVENT_TYPES = [
  { value: 'motion', label: 'Motion Detected' },
  { value: 'camera_offline', label: 'Camera Offline' },
  { value: 'camera_online', label: 'Camera Online' },
  { value: 'recording_started', label: 'Recording Started' },
  { value: 'recording_stopped', label: 'Recording Stopped' },
  { value: 'disk_warning', label: 'Disk Warning' },
  { value: 'disk_critical', label: 'Disk Critical' },
]

const CHANNELS = [
  { value: 'email', label: 'Email' },
  { value: 'sms', label: 'SMS' },
  { value: 'push', label: 'Push' },
  { value: 'slack', label: 'Slack' },
  { value: 'teams', label: 'Teams' },
]

const WEEKDAYS = [
  { value: 'mon', label: 'Mon' },
  { value: 'tue', label: 'Tue' },
  { value: 'wed', label: 'Wed' },
  { value: 'thu', label: 'Thu' },
  { value: 'fri', label: 'Fri' },
  { value: 'sat', label: 'Sat' },
  { value: 'sun', label: 'Sun' },
]

// --- Helper ---

function buildMatrixKey(cameraId: string, eventType: string, channel: string): string {
  return `${cameraId}::${eventType}::${channel}`
}

// --- Tab Component ---

function Tab({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`px-4 py-2.5 text-sm font-medium rounded-t-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
        active
          ? 'text-white bg-nvr-bg-secondary border-b-2 border-nvr-accent'
          : 'text-nvr-text-secondary hover:text-nvr-text-primary bg-nvr-bg-tertiary/30'
      }`}
    >
      {children}
    </button>
  )
}

// ============================================================
//  Preference Matrix
// ============================================================

function PreferenceMatrix({
  cameras,
  prefs,
  onToggle,
  saving,
  selectedCamera,
  onCameraChange,
}: {
  cameras: Camera[]
  prefs: Map<string, boolean>
  onToggle: (cameraId: string, eventType: string, channel: string) => void
  saving: boolean
  selectedCamera: string
  onCameraChange: (id: string) => void
}) {
  return (
    <div>
      {/* Camera selector */}
      <div className="mb-4">
        <label className="text-sm text-nvr-text-secondary mb-1 block">Camera scope</label>
        <select
          value={selectedCamera}
          onChange={(e) => onCameraChange(e.target.value)}
          className="bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm w-full max-w-xs focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
        >
          <option value="*">All cameras (global)</option>
          {cameras.map(cam => (
            <option key={cam.id} value={cam.id}>{cam.name}</option>
          ))}
        </select>
      </div>

      {/* Matrix table */}
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-nvr-border">
              <th className="text-left py-3 px-3 text-nvr-text-secondary font-medium">Event Type</th>
              {CHANNELS.map(ch => (
                <th key={ch.value} className="text-center py-3 px-3 text-nvr-text-secondary font-medium">
                  {ch.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {EVENT_TYPES.map(evt => (
              <tr key={evt.value} className="border-b border-nvr-border/50 hover:bg-nvr-bg-tertiary/30 transition-colors">
                <td className="py-3 px-3 text-nvr-text-primary">{evt.label}</td>
                {CHANNELS.map(ch => {
                  const key = buildMatrixKey(selectedCamera, evt.value, ch.value)
                  const enabled = prefs.get(key) ?? false
                  return (
                    <td key={ch.value} className="text-center py-3 px-3">
                      <button
                        onClick={() => onToggle(selectedCamera, evt.value, ch.value)}
                        disabled={saving}
                        className={`w-8 h-8 rounded-lg transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                          enabled
                            ? 'bg-nvr-accent/20 text-nvr-accent hover:bg-nvr-accent/30'
                            : 'bg-nvr-bg-tertiary text-nvr-text-muted hover:bg-nvr-bg-tertiary/80'
                        } ${saving ? 'opacity-50 cursor-not-allowed' : ''}`}
                        aria-label={`${evt.label} via ${ch.label}: ${enabled ? 'enabled' : 'disabled'}`}
                      >
                        {enabled ? (
                          <svg className="w-4 h-4 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                          </svg>
                        ) : (
                          <svg className="w-4 h-4 mx-auto" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                          </svg>
                        )}
                      </button>
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ============================================================
//  Quiet Hours Editor
// ============================================================

function QuietHoursEditor({
  config,
  onChange,
  onSave,
  saving,
}: {
  config: QuietHoursConfig
  onChange: (c: QuietHoursConfig) => void
  onSave: () => void
  saving: boolean
}) {
  const days: string[] = (() => {
    try { return JSON.parse(config.days) } catch { return [] }
  })()

  function toggleDay(day: string) {
    const next = days.includes(day) ? days.filter(d => d !== day) : [...days, day]
    onChange({ ...config, days: JSON.stringify(next) })
  }

  return (
    <div className="space-y-6">
      {/* Enable toggle */}
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-nvr-text-primary">Enable Quiet Hours</p>
          <p className="text-xs text-nvr-text-muted mt-0.5">
            Suppress non-critical notifications during the configured time window.
          </p>
        </div>
        <button
          onClick={() => onChange({ ...config, enabled: !config.enabled })}
          className={`relative w-11 h-6 rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
            config.enabled ? 'bg-nvr-accent' : 'bg-nvr-bg-tertiary'
          }`}
        >
          <span
            className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full shadow transition-transform ${
              config.enabled ? 'translate-x-5' : ''
            }`}
          />
        </button>
      </div>

      {config.enabled && (
        <>
          {/* Time range */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">Start time</label>
              <input
                type="time"
                value={config.start_time}
                onChange={(e) => onChange({ ...config, start_time: e.target.value })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              />
            </div>
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">End time</label>
              <input
                type="time"
                value={config.end_time}
                onChange={(e) => onChange({ ...config, end_time: e.target.value })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              />
            </div>
          </div>

          {/* Timezone */}
          <div>
            <label className="text-sm text-nvr-text-secondary mb-1 block">Timezone</label>
            <select
              value={config.timezone}
              onChange={(e) => onChange({ ...config, timezone: e.target.value })}
              className="bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm w-full max-w-xs focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
            >
              {['UTC', 'America/New_York', 'America/Chicago', 'America/Denver', 'America/Los_Angeles', 'Europe/London', 'Europe/Paris', 'Asia/Tokyo', 'Australia/Sydney'].map(tz => (
                <option key={tz} value={tz}>{tz}</option>
              ))}
            </select>
          </div>

          {/* Days */}
          <div>
            <label className="text-sm text-nvr-text-secondary mb-2 block">Active days</label>
            <div className="flex gap-2">
              {WEEKDAYS.map(d => (
                <button
                  key={d.value}
                  onClick={() => toggleDay(d.value)}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                    days.includes(d.value)
                      ? 'bg-nvr-accent/20 text-nvr-accent'
                      : 'bg-nvr-bg-tertiary text-nvr-text-muted hover:text-nvr-text-secondary'
                  }`}
                >
                  {d.label}
                </button>
              ))}
            </div>
          </div>
        </>
      )}

      <button
        onClick={onSave}
        disabled={saving}
        className="px-4 py-2 bg-nvr-accent hover:bg-nvr-accent-hover text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
      >
        {saving ? 'Saving...' : 'Save Quiet Hours'}
      </button>
    </div>
  )
}

// ============================================================
//  Escalation Rule Editor
// ============================================================

function EscalationRuleCard({
  rule,
  cameras,
  onEdit,
  onDelete,
}: {
  rule: EscalationRule
  cameras: Camera[]
  onEdit: () => void
  onDelete: () => void
}) {
  const cameraName = rule.camera_id === '*'
    ? 'All cameras'
    : cameras.find(c => c.id === rule.camera_id)?.name || rule.camera_id.slice(0, 8)
  const eventLabel = EVENT_TYPES.find(e => e.value === rule.event_type)?.label || rule.event_type

  let steps: EscalationStep[] = []
  try { steps = JSON.parse(rule.escalation_chain) } catch { /* empty */ }

  return (
    <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-4">
      <div className="flex items-start justify-between mb-3">
        <div>
          <div className="flex items-center gap-2">
            <h3 className="text-sm font-semibold text-nvr-text-primary">{rule.name}</h3>
            <span className={`inline-block w-2 h-2 rounded-full ${rule.enabled ? 'bg-green-500' : 'bg-nvr-text-muted'}`} />
          </div>
          <p className="text-xs text-nvr-text-muted mt-0.5">
            {eventLabel} -- {cameraName}
          </p>
        </div>
        <div className="flex gap-1.5">
          <button
            onClick={onEdit}
            className="p-1.5 text-nvr-text-secondary hover:text-nvr-text-primary transition-colors rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            aria-label="Edit rule"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
            </svg>
          </button>
          <button
            onClick={onDelete}
            className="p-1.5 text-nvr-text-secondary hover:text-nvr-danger transition-colors rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            aria-label="Delete rule"
          >
            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </button>
        </div>
      </div>

      <div className="grid grid-cols-3 gap-3 text-xs mb-3">
        <div>
          <span className="text-nvr-text-muted">Delay</span>
          <p className="text-nvr-text-primary font-medium">{rule.delay_minutes} min</p>
        </div>
        <div>
          <span className="text-nvr-text-muted">Repeats</span>
          <p className="text-nvr-text-primary font-medium">{rule.repeat_count}x every {rule.repeat_interval_minutes}min</p>
        </div>
        <div>
          <span className="text-nvr-text-muted">Steps</span>
          <p className="text-nvr-text-primary font-medium">{steps.length} step{steps.length !== 1 ? 's' : ''}</p>
        </div>
      </div>

      {steps.length > 0 && (
        <div className="border-t border-nvr-border/50 pt-2">
          <p className="text-xs text-nvr-text-muted mb-1.5">Escalation chain:</p>
          <div className="flex flex-wrap gap-1.5">
            {steps.map((step, i) => (
              <span
                key={i}
                className="inline-flex items-center gap-1 bg-nvr-bg-tertiary text-nvr-text-secondary text-xs px-2 py-1 rounded"
              >
                <span className="text-nvr-accent font-medium">{i + 1}.</span>
                {step.channel}: {step.target}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// ============================================================
//  Escalation Rule Modal
// ============================================================

function EscalationRuleModal({
  rule,
  cameras,
  onSave,
  onClose,
  saving,
}: {
  rule: Partial<EscalationRule> | null
  cameras: Camera[]
  onSave: (r: Partial<EscalationRule>) => void
  onClose: () => void
  saving: boolean
}) {
  const [form, setForm] = useState<Partial<EscalationRule>>(() => ({
    name: '',
    event_type: 'motion',
    camera_id: '*',
    enabled: true,
    delay_minutes: 5,
    repeat_count: 3,
    repeat_interval_minutes: 10,
    escalation_chain: '[]',
    ...rule,
  }))

  const [steps, setSteps] = useState<EscalationStep[]>(() => {
    try { return JSON.parse(form.escalation_chain || '[]') } catch { return [] }
  })

  useEffect(() => {
    const handler = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  function addStep() {
    setSteps([...steps, { channel: 'email', target: '' }])
  }

  function removeStep(i: number) {
    setSteps(steps.filter((_, idx) => idx !== i))
  }

  function updateStep(i: number, field: keyof EscalationStep, value: string) {
    const next = [...steps]
    next[i] = { ...next[i], [field]: value }
    setSteps(next)
  }

  function handleSave() {
    onSave({ ...form, escalation_chain: JSON.stringify(steps) })
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
      <div
        className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto"
        onClick={e => e.stopPropagation()}
      >
        <div className="px-6 py-4 border-b border-nvr-border">
          <h2 className="text-lg font-bold text-nvr-text-primary">
            {rule?.id ? 'Edit Escalation Rule' : 'New Escalation Rule'}
          </h2>
        </div>

        <div className="px-6 py-4 space-y-4">
          {/* Name */}
          <div>
            <label className="text-sm text-nvr-text-secondary mb-1 block">Name</label>
            <input
              type="text"
              value={form.name || ''}
              onChange={e => setForm({ ...form, name: e.target.value })}
              className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              placeholder="e.g., After-hours motion escalation"
            />
          </div>

          {/* Event type + Camera */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">Event type</label>
              <select
                value={form.event_type}
                onChange={e => setForm({ ...form, event_type: e.target.value })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              >
                {EVENT_TYPES.map(et => (
                  <option key={et.value} value={et.value}>{et.label}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">Camera</label>
              <select
                value={form.camera_id}
                onChange={e => setForm({ ...form, camera_id: e.target.value })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              >
                <option value="*">All cameras</option>
                {cameras.map(cam => (
                  <option key={cam.id} value={cam.id}>{cam.name}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Timing */}
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">Initial delay (min)</label>
              <input
                type="number"
                min={1}
                value={form.delay_minutes || 5}
                onChange={e => setForm({ ...form, delay_minutes: parseInt(e.target.value) || 5 })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              />
            </div>
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">Repeat count</label>
              <input
                type="number"
                min={0}
                value={form.repeat_count || 3}
                onChange={e => setForm({ ...form, repeat_count: parseInt(e.target.value) || 3 })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              />
            </div>
            <div>
              <label className="text-sm text-nvr-text-secondary mb-1 block">Repeat interval (min)</label>
              <input
                type="number"
                min={1}
                value={form.repeat_interval_minutes || 10}
                onChange={e => setForm({ ...form, repeat_interval_minutes: parseInt(e.target.value) || 10 })}
                className="w-full bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-3 py-2 text-sm focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
              />
            </div>
          </div>

          {/* Enabled */}
          <div className="flex items-center gap-3">
            <button
              onClick={() => setForm({ ...form, enabled: !form.enabled })}
              className={`relative w-11 h-6 rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                form.enabled ? 'bg-nvr-accent' : 'bg-nvr-bg-tertiary'
              }`}
            >
              <span
                className={`absolute top-0.5 left-0.5 w-5 h-5 bg-white rounded-full shadow transition-transform ${
                  form.enabled ? 'translate-x-5' : ''
                }`}
              />
            </button>
            <span className="text-sm text-nvr-text-primary">Enabled</span>
          </div>

          {/* Escalation chain */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm text-nvr-text-secondary">Escalation chain</label>
              <button
                onClick={addStep}
                className="text-xs text-nvr-accent hover:text-nvr-accent-hover transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                + Add step
              </button>
            </div>
            {steps.length === 0 && (
              <p className="text-xs text-nvr-text-muted py-2">No escalation steps configured. Add a step to define who gets notified.</p>
            )}
            <div className="space-y-2">
              {steps.map((step, i) => (
                <div key={i} className="flex items-center gap-2">
                  <span className="text-xs text-nvr-accent font-bold w-5 text-center">{i + 1}</span>
                  <select
                    value={step.channel}
                    onChange={e => updateStep(i, 'channel', e.target.value)}
                    className="bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-2 py-1.5 text-xs focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
                  >
                    {CHANNELS.map(ch => (
                      <option key={ch.value} value={ch.value}>{ch.label}</option>
                    ))}
                  </select>
                  <input
                    type="text"
                    value={step.target}
                    onChange={e => updateStep(i, 'target', e.target.value)}
                    placeholder="Recipient (email, phone, webhook URL...)"
                    className="flex-1 bg-nvr-bg-tertiary text-nvr-text-primary border border-nvr-border rounded-lg px-2 py-1.5 text-xs focus:ring-2 focus:ring-nvr-accent/50 focus:outline-none"
                  />
                  <button
                    onClick={() => removeStep(i)}
                    className="p-1 text-nvr-text-muted hover:text-nvr-danger transition-colors rounded focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                    aria-label="Remove step"
                  >
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                      <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="px-6 py-4 border-t border-nvr-border flex justify-end gap-3">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-nvr-text-secondary hover:text-nvr-text-primary transition-colors rounded-lg focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !form.name}
            className="px-4 py-2 bg-nvr-accent hover:bg-nvr-accent-hover text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            {saving ? 'Saving...' : rule?.id ? 'Update Rule' : 'Create Rule'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ============================================================
//  Main Notifications Page
// ============================================================

export default function Notifications() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const { cameras } = useCameras()

  const [activeTab, setActiveTab] = useState<'preferences' | 'quiet-hours' | 'escalation'>('preferences')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  // Preference matrix state
  const [prefs, setPrefs] = useState<Map<string, boolean>>(new Map())
  const [rawPrefs, setRawPrefs] = useState<NotificationPref[]>([])
  const [selectedCamera, setSelectedCamera] = useState('*')

  // Quiet hours state
  const [quietHours, setQuietHours] = useState<QuietHoursConfig>({
    user_id: '',
    enabled: false,
    start_time: '22:00',
    end_time: '07:00',
    timezone: 'UTC',
    days: '["mon","tue","wed","thu","fri","sat","sun"]',
  })

  // Escalation state
  const [escalationRules, setEscalationRules] = useState<EscalationRule[]>([])
  const [editingRule, setEditingRule] = useState<Partial<EscalationRule> | null>(null)
  const [showRuleModal, setShowRuleModal] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState<EscalationRule | null>(null)

  // --- Fetch ---

  const loadPrefs = useCallback(async () => {
    try {
      const res = await apiFetch('/notification-preferences')
      if (res.ok) {
        const data: NotificationPref[] = await res.json()
        setRawPrefs(data)
        const map = new Map<string, boolean>()
        for (const p of data) {
          map.set(buildMatrixKey(p.camera_id, p.event_type, p.channel), p.enabled)
        }
        setPrefs(map)
      }
    } catch {
      setError('Failed to load notification preferences')
    }
  }, [])

  const loadQuietHours = useCallback(async () => {
    try {
      const res = await apiFetch('/notification-preferences/quiet-hours')
      if (res.ok) {
        const data = await res.json()
        setQuietHours(data)
      }
    } catch {
      setError('Failed to load quiet hours')
    }
  }, [])

  const loadEscalation = useCallback(async () => {
    if (!isAdmin) return
    try {
      const res = await apiFetch('/escalation-rules')
      if (res.ok) {
        const data = await res.json()
        setEscalationRules(data)
      }
    } catch {
      setError('Failed to load escalation rules')
    }
  }, [isAdmin])

  useEffect(() => {
    loadPrefs()
    loadQuietHours()
    loadEscalation()
  }, [loadPrefs, loadQuietHours, loadEscalation])

  // Clear success message after 3 seconds
  useEffect(() => {
    if (success) {
      const t = setTimeout(() => setSuccess(null), 3000)
      return () => clearTimeout(t)
    }
  }, [success])

  // --- Preference toggle ---

  async function handleToggle(cameraId: string, eventType: string, channel: string) {
    const key = buildMatrixKey(cameraId, eventType, channel)
    const current = prefs.get(key) ?? false
    const next = !current

    // Optimistic update
    setPrefs(prev => {
      const m = new Map(prev)
      m.set(key, next)
      return m
    })

    setSaving(true)
    setError(null)
    try {
      const res = await apiFetch('/notification-preferences', {
        method: 'PUT',
        body: JSON.stringify({
          preferences: [{ camera_id: cameraId, event_type: eventType, channel, enabled: next }],
        }),
      })
      if (!res.ok) {
        throw new Error('Failed to update')
      }
      const data: NotificationPref[] = await res.json()
      setRawPrefs(data)
      const map = new Map<string, boolean>()
      for (const p of data) {
        map.set(buildMatrixKey(p.camera_id, p.event_type, p.channel), p.enabled)
      }
      setPrefs(map)
    } catch {
      // Revert optimistic update
      setPrefs(prev => {
        const m = new Map(prev)
        m.set(key, current)
        return m
      })
      setError('Failed to update preference')
    } finally {
      setSaving(false)
    }
  }

  // --- Quiet hours save ---

  async function handleSaveQuietHours() {
    setSaving(true)
    setError(null)
    try {
      const res = await apiFetch('/notification-preferences/quiet-hours', {
        method: 'PUT',
        body: JSON.stringify(quietHours),
      })
      if (!res.ok) throw new Error('Failed')
      const data = await res.json()
      setQuietHours(data)
      setSuccess('Quiet hours saved')
    } catch {
      setError('Failed to save quiet hours')
    } finally {
      setSaving(false)
    }
  }

  // --- Escalation CRUD ---

  async function handleSaveEscalationRule(rule: Partial<EscalationRule>) {
    setSaving(true)
    setError(null)
    try {
      const isEdit = !!rule.id
      const res = await apiFetch(isEdit ? `/escalation-rules/${rule.id}` : '/escalation-rules', {
        method: isEdit ? 'PUT' : 'POST',
        body: JSON.stringify(rule),
      })
      if (!res.ok) throw new Error('Failed')
      setShowRuleModal(false)
      setEditingRule(null)
      setSuccess(isEdit ? 'Escalation rule updated' : 'Escalation rule created')
      loadEscalation()
    } catch {
      setError('Failed to save escalation rule')
    } finally {
      setSaving(false)
    }
  }

  async function handleDeleteEscalationRule(rule: EscalationRule) {
    setSaving(true)
    setError(null)
    try {
      const res = await apiFetch(`/escalation-rules/${rule.id}`, { method: 'DELETE' })
      if (!res.ok) throw new Error('Failed')
      setDeleteConfirm(null)
      setSuccess('Escalation rule deleted')
      loadEscalation()
    } catch {
      setError('Failed to delete escalation rule')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white">Notifications</h1>
          <p className="text-sm text-nvr-text-muted mt-1">
            Configure how and when you receive notifications from your cameras.
          </p>
        </div>
      </div>

      {/* Status messages */}
      {error && (
        <div className="mb-4 px-4 py-3 bg-red-500/10 border border-red-500/30 rounded-lg text-sm text-red-400">
          {error}
          <button onClick={() => setError(null)} className="float-right text-red-400 hover:text-red-300">&times;</button>
        </div>
      )}
      {success && (
        <div className="mb-4 px-4 py-3 bg-green-500/10 border border-green-500/30 rounded-lg text-sm text-green-400">
          {success}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-6">
        <Tab active={activeTab === 'preferences'} onClick={() => setActiveTab('preferences')}>
          Channel Preferences
        </Tab>
        <Tab active={activeTab === 'quiet-hours'} onClick={() => setActiveTab('quiet-hours')}>
          Quiet Hours
        </Tab>
        {isAdmin && (
          <Tab active={activeTab === 'escalation'} onClick={() => setActiveTab('escalation')}>
            Escalation Rules
          </Tab>
        )}
      </div>

      {/* Tab content */}
      <div className="bg-nvr-bg-secondary border border-nvr-border rounded-xl p-6">
        {activeTab === 'preferences' && (
          <PreferenceMatrix
            cameras={cameras}
            prefs={prefs}
            onToggle={handleToggle}
            saving={saving}
            selectedCamera={selectedCamera}
            onCameraChange={setSelectedCamera}
          />
        )}

        {activeTab === 'quiet-hours' && (
          <QuietHoursEditor
            config={quietHours}
            onChange={setQuietHours}
            onSave={handleSaveQuietHours}
            saving={saving}
          />
        )}

        {activeTab === 'escalation' && isAdmin && (
          <div>
            <div className="flex items-center justify-between mb-4">
              <div>
                <h2 className="text-sm font-semibold text-nvr-text-primary">Escalation Rules</h2>
                <p className="text-xs text-nvr-text-muted mt-0.5">
                  Define chains of notifications that fire when events are not acknowledged.
                </p>
              </div>
              <button
                onClick={() => { setEditingRule(null); setShowRuleModal(true) }}
                className="px-3 py-2 bg-nvr-accent hover:bg-nvr-accent-hover text-white rounded-lg text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
              >
                + New Rule
              </button>
            </div>

            {escalationRules.length === 0 ? (
              <div className="text-center py-12 text-nvr-text-muted text-sm">
                No escalation rules configured yet. Create one to get started.
              </div>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2">
                {escalationRules.map(rule => (
                  <EscalationRuleCard
                    key={rule.id}
                    rule={rule}
                    cameras={cameras}
                    onEdit={() => { setEditingRule(rule); setShowRuleModal(true) }}
                    onDelete={() => setDeleteConfirm(rule)}
                  />
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      {/* Escalation rule modal */}
      {showRuleModal && (
        <EscalationRuleModal
          rule={editingRule}
          cameras={cameras}
          onSave={handleSaveEscalationRule}
          onClose={() => { setShowRuleModal(false); setEditingRule(null) }}
          saving={saving}
        />
      )}

      {/* Delete confirmation */}
      {deleteConfirm && (
        <ConfirmDialog
          title="Delete Escalation Rule"
          message={`Are you sure you want to delete "${deleteConfirm.name}"? This action cannot be undone.`}
          confirmLabel="Delete"
          confirmVariant="danger"
          onConfirm={() => handleDeleteEscalationRule(deleteConfirm)}
          onCancel={() => setDeleteConfirm(null)}
        />
      )}
    </div>
  )
}
