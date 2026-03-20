import { RecordingRule } from '../hooks/useRecordingRules'

const DAY_LABELS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const SLOTS_PER_DAY = 48 // 30-minute slots

interface SchedulePreviewProps {
  rules: RecordingRule[]
}

/** Parse the days JSON string into an array of day indices (0=Sun..6=Sat). */
function parseDays(daysStr: string): number[] {
  try {
    const arr = JSON.parse(daysStr)
    if (Array.isArray(arr)) return arr
  } catch { /* ignore */ }
  return []
}

/** Convert "HH:MM" to minutes since midnight. */
function timeToMinutes(t: string): number {
  const [h, m] = t.split(':').map(Number)
  return h * 60 + (m || 0)
}

/**
 * Check if a given slot (day + 30-minute block) is covered by a rule.
 * Returns 'always' | 'events' | null.
 */
function slotMode(
  rules: RecordingRule[],
  day: number,
  slotIndex: number,
): 'always' | 'events' | null {
  const slotStart = slotIndex * 30
  const slotEnd = slotStart + 30

  let result: 'always' | 'events' | null = null

  for (const rule of rules) {
    if (!rule.enabled) continue

    const days = parseDays(rule.days)
    if (!days.includes(day)) continue

    const ruleStart = timeToMinutes(rule.start_time)
    const ruleEnd = timeToMinutes(rule.end_time)

    let covered = false
    if (ruleStart <= ruleEnd) {
      // Same-day range, e.g. 08:00-18:00
      covered = slotStart < ruleEnd && slotEnd > ruleStart
    } else {
      // Overnight range, e.g. 22:00-06:00
      // The slot is covered if it falls in [ruleStart..24:00) or [00:00..ruleEnd)
      covered = slotEnd > ruleStart || slotStart < ruleEnd
    }

    if (covered) {
      // "always" takes priority over "events"
      if (rule.mode === 'always') return 'always'
      result = 'events'
    }
  }

  return result
}

const containerStyle: React.CSSProperties = {
  display: 'grid',
  gridTemplateColumns: '48px repeat(7, 1fr)',
  gap: 1,
  fontSize: 11,
  lineHeight: '14px',
  userSelect: 'none',
}

const headerStyle: React.CSSProperties = {
  textAlign: 'center',
  fontWeight: 600,
  padding: '4px 0',
  color: '#9ca3af',
}

const timeLabelStyle: React.CSSProperties = {
  textAlign: 'right',
  paddingRight: 6,
  color: '#6b7280',
  fontSize: 10,
}

export default function SchedulePreview({ rules }: SchedulePreviewProps) {
  const rows: JSX.Element[] = []

  // Header row
  rows.push(
    <div key="corner" style={timeLabelStyle} />,
    ...DAY_LABELS.map(d => (
      <div key={`h-${d}`} style={headerStyle}>{d}</div>
    )),
  )

  for (let slot = 0; slot < SLOTS_PER_DAY; slot++) {
    const hour = Math.floor(slot / 2)
    const isHalf = slot % 2 === 1
    // Show label every 2 hours on the hour mark
    const showLabel = slot % 4 === 0
    const timeStr = showLabel
      ? `${hour.toString().padStart(2, '0')}:${isHalf ? '30' : '00'}`
      : ''

    rows.push(
      <div key={`t-${slot}`} style={{ ...timeLabelStyle, alignSelf: 'center' }}>
        {timeStr}
      </div>,
    )

    for (let day = 0; day < 7; day++) {
      const mode = slotMode(rules, day, slot)
      let bg = 'transparent'
      if (mode === 'always') bg = 'rgba(59, 130, 246, 0.3)' // blue
      else if (mode === 'events') bg = 'rgba(245, 158, 11, 0.3)' // amber

      rows.push(
        <div
          key={`c-${day}-${slot}`}
          style={{
            background: bg,
            borderRadius: 2,
            minHeight: 10,
            border: '1px solid rgba(255,255,255,0.04)',
          }}
        />,
      )
    }
  }

  return (
    <div>
      <div style={{ display: 'flex', gap: 16, marginBottom: 8, fontSize: 12, color: '#9ca3af' }}>
        <span>
          <span style={{
            display: 'inline-block',
            width: 12,
            height: 12,
            background: 'rgba(59, 130, 246, 0.3)',
            borderRadius: 2,
            marginRight: 4,
            verticalAlign: 'middle',
          }} />
          Always
        </span>
        <span>
          <span style={{
            display: 'inline-block',
            width: 12,
            height: 12,
            background: 'rgba(245, 158, 11, 0.3)',
            borderRadius: 2,
            marginRight: 4,
            verticalAlign: 'middle',
          }} />
          Events
        </span>
      </div>
      <div style={containerStyle}>
        {rows}
      </div>
    </div>
  )
}
