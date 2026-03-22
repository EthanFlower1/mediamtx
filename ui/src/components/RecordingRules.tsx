import { useState, FormEvent } from 'react'
import { useRecordingRules, RecordingRule, CreateRulePayload } from '../hooks/useRecordingRules'
import SchedulePreview from './SchedulePreview'
import ConfirmDialog from './ConfirmDialog'

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const DAY_LETTERS = ['S', 'M', 'T', 'W', 'T', 'F', 'S']

interface RecordingRulesProps {
  cameraId: string
}

/** Parse the days JSON string into an array of day indices. */
function parseDays(daysStr: string): number[] {
  try {
    const arr = JSON.parse(daysStr)
    if (Array.isArray(arr)) return arr
  } catch { /* ignore */ }
  return []
}

/** Format days array into a readable string. */
function formatDays(daysStr: string): string {
  const days = parseDays(daysStr)
  if (days.length === 0) return 'None'
  if (days.length === 7) return 'Every Day'
  const weekdays = [1, 2, 3, 4, 5]
  const weekends = [0, 6]
  if (days.length === 5 && weekdays.every(d => days.includes(d))) return 'Weekdays'
  if (days.length === 2 && weekends.every(d => days.includes(d))) return 'Weekends'
  return days.map(d => DAY_NAMES[d]).join(', ')
}

// --- Status Bar ---

function StatusBar({ status }: { status: ReturnType<typeof useRecordingRules>['status'] }) {
  if (!status) return null

  return (
    <div className="flex flex-wrap items-center gap-4 p-3 bg-nvr-bg-tertiary border border-nvr-border rounded-lg mb-4 text-xs">
      <div className="flex items-center gap-1.5">
        <span className="text-nvr-text-muted">Mode:</span>
        <span className={`inline-block px-2 py-0.5 rounded-full font-semibold ${
          status.effective_mode === 'always'
            ? 'bg-nvr-accent/15 text-nvr-accent'
            : status.effective_mode === 'events'
              ? 'bg-nvr-warning/15 text-nvr-warning'
              : 'bg-nvr-text-muted/15 text-nvr-text-muted'
        }`}>
          {status.effective_mode === 'always' ? 'Always' : status.effective_mode === 'events' ? 'Events' : 'Off'}
        </span>
      </div>

      <div className="flex items-center gap-1.5">
        <span className={`w-2 h-2 rounded-full ${status.recording ? 'bg-nvr-success' : 'bg-nvr-text-muted'}`} />
        <span className={status.recording ? 'text-nvr-success' : 'text-nvr-text-muted'}>
          {status.recording ? 'Recording' : 'Not Recording'}
        </span>
      </div>

      {status.effective_mode === 'events' && (
        <div className="flex items-center gap-1.5">
          <span className="text-nvr-text-muted">Motion:</span>
          <span className={status.motion_state === 'active' ? 'text-nvr-warning font-medium' : 'text-nvr-text-muted'}>
            {status.motion_state}
          </span>
        </div>
      )}

      {status.active_rules.length > 0 && (
        <span className="text-nvr-text-muted">
          {status.active_rules.length} active rule{status.active_rules.length !== 1 ? 's' : ''}
        </span>
      )}
    </div>
  )
}

// --- Rule Form ---

interface RuleFormProps {
  initial?: RecordingRule
  onSave: (payload: CreateRulePayload) => void
  onCancel: () => void
}

function RuleForm({ initial, onSave, onCancel }: RuleFormProps) {
  const [name, setName] = useState(initial?.name ?? '')
  const [mode, setMode] = useState<'always' | 'events'>(initial?.mode ?? 'always')
  const [days, setDays] = useState<number[]>(initial ? parseDays(initial.days) : [0, 1, 2, 3, 4, 5, 6])
  const [startTime, setStartTime] = useState(initial?.start_time ?? '00:00')
  const [endTime, setEndTime] = useState(initial?.end_time ?? '23:59')
  const [postEventSeconds, setPostEventSeconds] = useState(initial?.post_event_seconds ?? 30)
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)

  const toggleDay = (d: number) => {
    setDays(prev => prev.includes(d) ? prev.filter(x => x !== d) : [...prev, d].sort())
  }

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault()
    onSave({ name, mode, days, start_time: startTime, end_time: endTime, post_event_seconds: postEventSeconds, enabled })
  }

  return (
    <form onSubmit={handleSubmit} className="bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-4 mb-3">
      <h4 className="text-sm font-semibold text-nvr-text-primary mb-4">
        {initial ? 'Edit Rule' : 'Add Rule'}
      </h4>

      {/* Name */}
      <div className="mb-4">
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1">Name</label>
        <input
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="e.g. Business Hours"
          required
          className="w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
        />
      </div>

      {/* Mode: two large toggle cards */}
      <div className="mb-4">
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1.5">Mode</label>
        <div className="grid grid-cols-2 gap-2">
          <button
            type="button"
            onClick={() => setMode('always')}
            className={`p-3 rounded-lg border text-center transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
              mode === 'always'
                ? 'bg-nvr-accent/15 border-nvr-accent text-nvr-accent'
                : 'bg-nvr-bg-input border-nvr-border text-nvr-text-muted hover:border-nvr-text-muted'
            }`}
          >
            <div className="text-sm font-semibold">Always Record</div>
            <div className="text-[10px] mt-0.5 opacity-70">Continuous recording</div>
          </button>
          <button
            type="button"
            onClick={() => setMode('events')}
            className={`p-3 rounded-lg border text-center transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
              mode === 'events'
                ? 'bg-nvr-warning/15 border-nvr-warning text-nvr-warning'
                : 'bg-nvr-bg-input border-nvr-border text-nvr-text-muted hover:border-nvr-text-muted'
            }`}
          >
            <div className="text-sm font-semibold">Events Only</div>
            <div className="text-[10px] mt-0.5 opacity-70">Record on motion</div>
          </button>
        </div>
        <p className="text-xs text-nvr-text-muted mt-1">Always: Record continuously during this time. Events: Record only when motion is detected.</p>
      </div>

      {/* Days: circle buttons */}
      <div className="mb-4">
        <div className="flex items-center justify-between mb-1.5">
          <label className="block text-xs font-medium text-nvr-text-secondary">Days</label>
          <div className="flex gap-1.5">
            <button
              type="button"
              onClick={() => setDays([1, 2, 3, 4, 5])}
              className="text-[10px] text-nvr-accent hover:text-nvr-accent-hover bg-nvr-accent/10 hover:bg-nvr-accent/20 border border-nvr-accent/30 rounded px-2 py-0.5 transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Weekdays
            </button>
            <button
              type="button"
              onClick={() => setDays([0, 6])}
              className="text-[10px] text-nvr-accent hover:text-nvr-accent-hover bg-nvr-accent/10 hover:bg-nvr-accent/20 border border-nvr-accent/30 rounded px-2 py-0.5 transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Weekends
            </button>
            <button
              type="button"
              onClick={() => setDays([0, 1, 2, 3, 4, 5, 6])}
              className="text-[10px] text-nvr-accent hover:text-nvr-accent-hover bg-nvr-accent/10 hover:bg-nvr-accent/20 border border-nvr-accent/30 rounded px-2 py-0.5 transition-colors font-medium focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Every Day
            </button>
          </div>
        </div>
        <div className="flex gap-1.5">
          {DAY_LETTERS.map((letter, i) => (
            <button
              key={i}
              type="button"
              onClick={() => toggleDay(i)}
              className={`w-9 h-9 rounded-full text-xs font-medium transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                days.includes(i)
                  ? 'bg-nvr-accent text-white'
                  : 'bg-nvr-bg-input border border-nvr-border text-nvr-text-muted hover:border-nvr-text-muted'
              }`}
            >
              {letter}
            </button>
          ))}
        </div>
      </div>

      {/* Time range: inline with "to" */}
      <div className="mb-4">
        <label className="block text-xs font-medium text-nvr-text-secondary mb-1">Time Range</label>
        <div className="flex items-center gap-2">
          <input
            type="time"
            value={startTime}
            onChange={e => setStartTime(e.target.value)}
            required
            className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />
          <span className="text-xs text-nvr-text-muted">to</span>
          <input
            type="time"
            value={endTime}
            onChange={e => setEndTime(e.target.value)}
            required
            className="flex-1 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
          />
        </div>
      </div>

      {/* Post-event buffer (events mode only) */}
      {mode === 'events' && (
        <div className="mb-4">
          <label className="block text-xs font-medium text-nvr-text-secondary mb-1">Post-event buffer</label>
          <div className="flex items-center gap-2">
            <input
              type="number"
              value={postEventSeconds}
              onChange={e => setPostEventSeconds(Number(e.target.value))}
              min={0}
              max={600}
              className="w-20 bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-sm text-nvr-text-primary focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors"
            />
            <span className="text-xs text-nvr-text-muted">seconds after motion stops</span>
          </div>
          <p className="text-xs text-nvr-text-muted mt-1">How many seconds to keep recording after motion stops. Prevents cutting off events too early.</p>
        </div>
      )}

      {/* Enabled toggle */}
      <div className="mb-4">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={enabled}
            onChange={e => setEnabled(e.target.checked)}
            className="w-4 h-4 rounded border-nvr-border text-nvr-accent focus:ring-nvr-accent bg-nvr-bg-input"
          />
          <span className="text-xs text-nvr-text-secondary">Enabled</span>
        </label>
      </div>

      {/* Actions */}
      <div className="flex gap-2">
        <button
          type="submit"
          className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Save
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="bg-nvr-bg-input hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}

// --- Rule Card ---

interface RuleCardProps {
  rule: RecordingRule
  isEditing: boolean
  onEdit: () => void
  onDelete: () => void
  onToggle: (enabled: boolean) => void
  onSave: (payload: CreateRulePayload) => void
  onCancelEdit: () => void
}

function RuleCard({ rule, isEditing, onEdit, onDelete, onToggle, onSave, onCancelEdit }: RuleCardProps) {
  if (isEditing) {
    return <RuleForm initial={rule} onSave={onSave} onCancel={onCancelEdit} />
  }

  return (
    <div className={`bg-nvr-bg-tertiary border border-nvr-border rounded-lg p-3 mb-2 flex flex-wrap items-center gap-3 transition-opacity ${
      rule.enabled ? 'opacity-100' : 'opacity-50'
    }`}>
      <div className="flex-1 min-w-[200px]">
        <div className="flex items-center gap-2 mb-1">
          <span className="text-sm font-medium text-nvr-text-primary">{rule.name}</span>
          <span className={`inline-block px-2 py-0.5 rounded-full text-[10px] font-semibold ${
            rule.mode === 'always'
              ? 'bg-nvr-accent/15 text-nvr-accent'
              : 'bg-nvr-warning/15 text-nvr-warning'
          }`}>
            {rule.mode === 'always' ? 'Always' : 'Events'}
          </span>
        </div>
        <div className="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-nvr-text-muted">
          <span>{formatDays(rule.days)}</span>
          <span>{rule.start_time} - {rule.end_time}</span>
          {rule.mode === 'events' && (
            <span>Buffer: {rule.post_event_seconds}s</span>
          )}
        </div>
      </div>

      <div className="flex items-center gap-2">
        {/* Enabled toggle */}
        <button
          onClick={() => onToggle(!rule.enabled)}
          className={`relative w-9 h-5 rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
            rule.enabled ? 'bg-nvr-accent' : 'bg-nvr-border'
          }`}
          aria-label={rule.enabled ? 'Disable rule' : 'Enable rule'}
        >
          <span className={`absolute top-0.5 w-4 h-4 rounded-full bg-white transition-transform ${
            rule.enabled ? 'left-[18px]' : 'left-0.5'
          }`} />
        </button>

        <button
          onClick={onEdit}
          className="p-1.5 rounded-md text-nvr-text-muted hover:text-nvr-text-primary hover:bg-nvr-bg-input transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          aria-label="Edit rule"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path d="M13.586 3.586a2 2 0 112.828 2.828l-.793.793-2.828-2.828.793-.793zM11.379 5.793L3 14.172V17h2.828l8.38-8.379-2.83-2.828z" />
          </svg>
        </button>

        <button
          onClick={onDelete}
          className="p-1.5 rounded-md text-nvr-text-muted hover:text-nvr-danger hover:bg-nvr-danger/10 transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          aria-label="Delete rule"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 20 20" fill="currentColor">
            <path fillRule="evenodd" d="M9 2a1 1 0 00-.894.553L7.382 4H4a1 1 0 000 2v10a2 2 0 002 2h8a2 2 0 002-2V6a1 1 0 100-2h-3.382l-.724-1.447A1 1 0 0011 2H9zM7 8a1 1 0 012 0v6a1 1 0 11-2 0V8zm5-1a1 1 0 00-1 1v6a1 1 0 102 0V8a1 1 0 00-1-1z" clipRule="evenodd" />
          </svg>
        </button>
      </div>
    </div>
  )
}

// --- Main Component ---

export default function RecordingRules({ cameraId }: RecordingRulesProps) {
  const { rules, status, loading, createRule, updateRule, deleteRule } = useRecordingRules(cameraId)
  const [showForm, setShowForm] = useState(false)
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null)
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null)

  const handleSave = async (payload: CreateRulePayload) => {
    if (editingRuleId) {
      await updateRule(editingRuleId, payload)
    } else {
      await createRule(payload)
    }
    setShowForm(false)
    setEditingRuleId(null)
  }

  const handleEdit = (rule: RecordingRule) => {
    setEditingRuleId(rule.id)
    setShowForm(false)
  }

  const handleCancelEdit = () => {
    setEditingRuleId(null)
  }

  const handleDelete = async (ruleId: string) => {
    await deleteRule(ruleId)
    setConfirmDeleteId(null)
  }

  const handleToggle = async (rule: RecordingRule, enabled: boolean) => {
    const days = parseDays(rule.days)
    await updateRule(rule.id, {
      name: rule.name,
      mode: rule.mode,
      days,
      start_time: rule.start_time,
      end_time: rule.end_time,
      post_event_seconds: rule.post_event_seconds,
      enabled,
    })
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="w-6 h-6 border-2 border-nvr-accent border-t-transparent rounded-full animate-spin" />
      </div>
    )
  }

  return (
    <div>
      <StatusBar status={status} />

      {/* Header with Add Rule button */}
      <div className="flex items-center justify-between mb-3">
        <h4 className="text-sm font-semibold text-nvr-text-primary">Recording Schedule</h4>
        {!showForm && (
          <button
            onClick={() => { setEditingRuleId(null); setShowForm(true) }}
            className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-xs focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          >
            Add Rule
          </button>
        )}
      </div>

      {/* Rules list */}
      {rules.length === 0 && !showForm && (
        <div className="text-nvr-text-muted text-center py-6 text-xs">
          No recording rules configured. Add a rule to start scheduled recording.
        </div>
      )}

      {rules.map(rule => (
        <RuleCard
          key={rule.id}
          rule={rule}
          isEditing={editingRuleId === rule.id}
          onEdit={() => handleEdit(rule)}
          onDelete={() => setConfirmDeleteId(rule.id)}
          onToggle={enabled => handleToggle(rule, enabled)}
          onSave={handleSave}
          onCancelEdit={handleCancelEdit}
        />
      ))}

      {/* New rule form */}
      {showForm && (
        <RuleForm
          onSave={handleSave}
          onCancel={() => setShowForm(false)}
        />
      )}

      {/* Schedule preview */}
      {rules.length > 0 && (
        <div className="mt-6">
          <h4 className="text-sm font-semibold text-nvr-text-primary mb-2">Schedule Preview</h4>
          <SchedulePreview rules={rules} />
        </div>
      )}

      <ConfirmDialog
        open={confirmDeleteId !== null}
        title="Delete Recording Rule"
        message="Are you sure you want to delete this recording rule? This action cannot be undone."
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={() => confirmDeleteId && handleDelete(confirmDeleteId)}
        onCancel={() => setConfirmDeleteId(null)}
      />
    </div>
  )
}
