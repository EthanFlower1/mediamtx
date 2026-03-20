import { useState, FormEvent } from 'react'
import { useRecordingRules, RecordingRule, CreateRulePayload } from '../hooks/useRecordingRules'
import SchedulePreview from './SchedulePreview'

const DAY_NAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

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

// --- Styles ---

const cardStyle: React.CSSProperties = {
  background: '#1e1e2e',
  border: '1px solid #2a2a3e',
  borderRadius: 8,
  padding: 16,
  marginBottom: 12,
}

const badgeBase: React.CSSProperties = {
  display: 'inline-block',
  padding: '2px 8px',
  borderRadius: 9999,
  fontSize: 12,
  fontWeight: 600,
}

function modeBadge(mode: string): React.CSSProperties {
  if (mode === 'always') return { ...badgeBase, background: 'rgba(59,130,246,0.2)', color: '#60a5fa' }
  if (mode === 'events') return { ...badgeBase, background: 'rgba(245,158,11,0.2)', color: '#fbbf24' }
  return { ...badgeBase, background: 'rgba(107,114,128,0.2)', color: '#9ca3af' }
}

const btnStyle: React.CSSProperties = {
  padding: '6px 14px',
  borderRadius: 6,
  border: 'none',
  cursor: 'pointer',
  fontSize: 13,
  fontWeight: 500,
}

const btnPrimary: React.CSSProperties = {
  ...btnStyle,
  background: '#3b82f6',
  color: '#fff',
}

const btnSecondary: React.CSSProperties = {
  ...btnStyle,
  background: '#374151',
  color: '#d1d5db',
}

const btnDanger: React.CSSProperties = {
  ...btnStyle,
  background: '#7f1d1d',
  color: '#fca5a5',
}

const inputStyle: React.CSSProperties = {
  padding: '6px 10px',
  borderRadius: 6,
  border: '1px solid #374151',
  background: '#111827',
  color: '#e5e7eb',
  fontSize: 13,
  width: '100%',
  boxSizing: 'border-box',
}

const labelStyle: React.CSSProperties = {
  display: 'block',
  fontSize: 12,
  fontWeight: 500,
  color: '#9ca3af',
  marginBottom: 4,
}

// --- Status Bar ---

function StatusBar({ status }: { status: ReturnType<typeof useRecordingRules>['status'] }) {
  if (!status) return null

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: 16,
      padding: '10px 16px',
      background: '#1e1e2e',
      border: '1px solid #2a2a3e',
      borderRadius: 8,
      marginBottom: 16,
      fontSize: 13,
    }}>
      <div>
        <span style={{ color: '#6b7280', marginRight: 6 }}>Mode:</span>
        <span style={modeBadge(status.effective_mode)}>
          {status.effective_mode === 'always' ? 'Always' : status.effective_mode === 'events' ? 'Events' : 'Off'}
        </span>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
        <span style={{
          display: 'inline-block',
          width: 8,
          height: 8,
          borderRadius: '50%',
          background: status.recording ? '#22c55e' : '#4b5563',
        }} />
        <span style={{ color: status.recording ? '#22c55e' : '#6b7280' }}>
          {status.recording ? 'Recording' : 'Not Recording'}
        </span>
      </div>

      {status.effective_mode === 'events' && (
        <div>
          <span style={{ color: '#6b7280', marginRight: 6 }}>Motion:</span>
          <span style={{ color: status.motion_state === 'active' ? '#fbbf24' : '#6b7280' }}>
            {status.motion_state}
          </span>
        </div>
      )}

      {status.active_rules.length > 0 && (
        <div style={{ color: '#6b7280', fontSize: 11 }}>
          {status.active_rules.length} active rule{status.active_rules.length !== 1 ? 's' : ''}
        </div>
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

  const dayBtnStyle = (active: boolean): React.CSSProperties => ({
    padding: '4px 8px',
    borderRadius: 4,
    border: active ? '1px solid #3b82f6' : '1px solid #374151',
    background: active ? 'rgba(59,130,246,0.2)' : 'transparent',
    color: active ? '#60a5fa' : '#6b7280',
    cursor: 'pointer',
    fontSize: 12,
    fontWeight: 500,
  })

  const modeBtnStyle = (active: boolean): React.CSSProperties => ({
    padding: '6px 16px',
    borderRadius: 6,
    border: 'none',
    background: active ? 'rgba(59,130,246,0.2)' : '#1f2937',
    color: active ? '#60a5fa' : '#6b7280',
    cursor: 'pointer',
    fontSize: 13,
    fontWeight: 500,
  })

  return (
    <form onSubmit={handleSubmit} style={{ ...cardStyle, background: '#161625' }}>
      <h4 style={{ margin: '0 0 16px', fontSize: 15, fontWeight: 600, color: '#e5e7eb' }}>
        {initial ? 'Edit Rule' : 'Add Rule'}
      </h4>

      {/* Name */}
      <div style={{ marginBottom: 12 }}>
        <label style={labelStyle}>Name</label>
        <input
          style={inputStyle}
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="e.g. Business Hours"
          required
        />
      </div>

      {/* Mode */}
      <div style={{ marginBottom: 12 }}>
        <label style={labelStyle}>Mode</label>
        <div style={{ display: 'flex', gap: 8 }}>
          <button type="button" style={modeBtnStyle(mode === 'always')} onClick={() => setMode('always')}>
            Always
          </button>
          <button type="button" style={modeBtnStyle(mode === 'events')} onClick={() => setMode('events')}>
            Events
          </button>
        </div>
      </div>

      {/* Days */}
      <div style={{ marginBottom: 12 }}>
        <label style={labelStyle}>Days</label>
        <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginBottom: 6 }}>
          {DAY_NAMES.map((d, i) => (
            <button key={d} type="button" style={dayBtnStyle(days.includes(i))} onClick={() => toggleDay(i)}>
              {d}
            </button>
          ))}
        </div>
        <div style={{ display: 'flex', gap: 4 }}>
          <button type="button" style={{ ...btnStyle, fontSize: 11, padding: '2px 8px', background: '#1f2937', color: '#9ca3af', border: 'none' }}
            onClick={() => setDays([1, 2, 3, 4, 5])}>Weekdays</button>
          <button type="button" style={{ ...btnStyle, fontSize: 11, padding: '2px 8px', background: '#1f2937', color: '#9ca3af', border: 'none' }}
            onClick={() => setDays([0, 6])}>Weekends</button>
          <button type="button" style={{ ...btnStyle, fontSize: 11, padding: '2px 8px', background: '#1f2937', color: '#9ca3af', border: 'none' }}
            onClick={() => setDays([0, 1, 2, 3, 4, 5, 6])}>Every Day</button>
        </div>
      </div>

      {/* Time range */}
      <div style={{ display: 'flex', gap: 12, marginBottom: 12 }}>
        <div style={{ flex: 1 }}>
          <label style={labelStyle}>Start Time</label>
          <input type="time" style={inputStyle} value={startTime} onChange={e => setStartTime(e.target.value)} required />
        </div>
        <div style={{ flex: 1 }}>
          <label style={labelStyle}>End Time</label>
          <input type="time" style={inputStyle} value={endTime} onChange={e => setEndTime(e.target.value)} required />
        </div>
      </div>

      {/* Post-event buffer (events mode only) */}
      {mode === 'events' && (
        <div style={{ marginBottom: 12 }}>
          <label style={labelStyle}>Post-Event Buffer (seconds)</label>
          <input
            type="number"
            style={{ ...inputStyle, width: 120 }}
            value={postEventSeconds}
            onChange={e => setPostEventSeconds(Number(e.target.value))}
            min={0}
            max={600}
          />
        </div>
      )}

      {/* Enabled */}
      <div style={{ marginBottom: 16 }}>
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, color: '#d1d5db', cursor: 'pointer' }}>
          <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} />
          Enabled
        </label>
      </div>

      {/* Actions */}
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="submit" style={btnPrimary}>Save</button>
        <button type="button" style={btnSecondary} onClick={onCancel}>Cancel</button>
      </div>
    </form>
  )
}

// --- Rule Card ---

interface RuleCardProps {
  rule: RecordingRule
  onEdit: () => void
  onDelete: () => void
  onToggle: (enabled: boolean) => void
}

function RuleCard({ rule, onEdit, onDelete, onToggle }: RuleCardProps) {
  return (
    <div style={{
      ...cardStyle,
      opacity: rule.enabled ? 1 : 0.5,
      display: 'flex',
      justifyContent: 'space-between',
      alignItems: 'center',
      flexWrap: 'wrap',
      gap: 12,
    }}>
      <div style={{ flex: 1, minWidth: 200 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}>
          <strong style={{ fontSize: 14, color: '#e5e7eb' }}>{rule.name}</strong>
          <span style={modeBadge(rule.mode)}>
            {rule.mode === 'always' ? 'Always' : 'Events'}
          </span>
        </div>
        <div style={{ fontSize: 12, color: '#6b7280', display: 'flex', gap: 12, flexWrap: 'wrap' }}>
          <span>{formatDays(rule.days)}</span>
          <span>{rule.start_time} &ndash; {rule.end_time}</span>
          {rule.mode === 'events' && (
            <span>Buffer: {rule.post_event_seconds}s</span>
          )}
        </div>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12, color: '#9ca3af', cursor: 'pointer' }}>
          <input
            type="checkbox"
            checked={rule.enabled}
            onChange={e => onToggle(e.target.checked)}
          />
          Enabled
        </label>
        <button style={btnSecondary} onClick={onEdit}>Edit</button>
        <button style={btnDanger} onClick={onDelete}>Delete</button>
      </div>
    </div>
  )
}

// --- Main Component ---

export default function RecordingRules({ cameraId }: RecordingRulesProps) {
  const { rules, status, loading, createRule, updateRule, deleteRule } = useRecordingRules(cameraId)
  const [showForm, setShowForm] = useState(false)
  const [editingRule, setEditingRule] = useState<RecordingRule | null>(null)

  const handleSave = async (payload: CreateRulePayload) => {
    if (editingRule) {
      await updateRule(editingRule.id, payload)
    } else {
      await createRule(payload)
    }
    setShowForm(false)
    setEditingRule(null)
  }

  const handleEdit = (rule: RecordingRule) => {
    setEditingRule(rule)
    setShowForm(true)
  }

  const handleDelete = async (ruleId: string) => {
    if (!confirm('Delete this recording rule?')) return
    await deleteRule(ruleId)
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

  const handleCancel = () => {
    setShowForm(false)
    setEditingRule(null)
  }

  if (loading) return <div style={{ color: '#6b7280', padding: 16 }}>Loading rules...</div>

  return (
    <div>
      <StatusBar status={status} />

      {/* Rules list */}
      {rules.length === 0 && !showForm && (
        <div style={{ color: '#6b7280', padding: '16px 0', textAlign: 'center', fontSize: 13 }}>
          No recording rules configured. Add a rule to start scheduled recording.
        </div>
      )}

      {rules.map(rule => (
        <RuleCard
          key={rule.id}
          rule={rule}
          onEdit={() => handleEdit(rule)}
          onDelete={() => handleDelete(rule.id)}
          onToggle={enabled => handleToggle(rule, enabled)}
        />
      ))}

      {/* Add / Edit form */}
      {showForm ? (
        <RuleForm
          initial={editingRule ?? undefined}
          onSave={handleSave}
          onCancel={handleCancel}
        />
      ) : (
        <button
          style={{ ...btnPrimary, marginTop: 8 }}
          onClick={() => { setEditingRule(null); setShowForm(true) }}
        >
          + Add Rule
        </button>
      )}

      {/* Schedule preview */}
      {rules.length > 0 && (
        <div style={{ marginTop: 24 }}>
          <h4 style={{ fontSize: 14, fontWeight: 600, color: '#e5e7eb', marginBottom: 8 }}>Schedule Preview</h4>
          <SchedulePreview rules={rules} />
        </div>
      )}
    </div>
  )
}
