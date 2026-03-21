import { useState } from 'react'
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

/** Format slot index to time string. */
function slotToTime(slot: number): string {
  const minutes = slot * 30
  const h = Math.floor(minutes / 60)
  const m = minutes % 60
  return `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}`
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
    const ruleStart = timeToMinutes(rule.start_time)
    const ruleEnd = timeToMinutes(rule.end_time)

    let covered = false
    if (ruleStart <= ruleEnd) {
      if (days.includes(day)) {
        covered = slotStart < ruleEnd && slotEnd > ruleStart
      }
    } else {
      const prevDay = (day - 1 + 7) % 7
      const eveningCovered = days.includes(day) && slotStart < 1440 && slotEnd > ruleStart
      const morningCovered = days.includes(prevDay) && slotStart < ruleEnd
      covered = eveningCovered || morningCovered
    }

    if (covered) {
      if (rule.mode === 'always') return 'always'
      result = 'events'
    }
  }

  return result
}

export default function SchedulePreview({ rules }: SchedulePreviewProps) {
  const [hoveredSlot, setHoveredSlot] = useState<{ day: number; slot: number } | null>(null)

  return (
    <div>
      {/* Legend */}
      <div className="flex gap-4 mb-2 text-xs text-nvr-text-secondary">
        <span className="flex items-center gap-1.5">
          <span className="inline-block w-3 h-3 rounded-sm bg-nvr-accent/30" />
          Always
        </span>
        <span className="flex items-center gap-1.5">
          <span className="inline-block w-3 h-3 rounded-sm bg-nvr-warning/30" />
          Events
        </span>
      </div>

      {/* Grid */}
      <div className="grid gap-px text-[10px] leading-[14px] select-none grid-cols-[48px_repeat(7,1fr)]">
        {/* Header row */}
        <div />
        {DAY_LABELS.map(d => (
          <div key={`h-${d}`} className="text-center font-semibold py-1 text-nvr-text-secondary">
            {d}
          </div>
        ))}

        {/* Slot rows */}
        {Array.from({ length: SLOTS_PER_DAY }, (_, slot) => {
          const showLabel = slot % 4 === 0
          const hour = Math.floor(slot / 2)
          const isHalf = slot % 2 === 1
          const timeStr = showLabel
            ? `${hour.toString().padStart(2, '0')}:${isHalf ? '30' : '00'}`
            : ''

          return [
            <div key={`t-${slot}`} className="text-right pr-1.5 text-nvr-text-muted self-center text-[10px]">
              {timeStr}
            </div>,
            ...Array.from({ length: 7 }, (_, day) => {
              const mode = slotMode(rules, day, slot)
              const isHovered = hoveredSlot?.day === day && hoveredSlot?.slot === slot

              return (
                <div
                  key={`c-${day}-${slot}`}
                  className={`min-h-[12px] rounded-sm border border-white/[0.04] transition-colors ${
                    mode === 'always'
                      ? 'bg-nvr-accent/30'
                      : mode === 'events'
                        ? 'bg-nvr-warning/30'
                        : 'bg-transparent'
                  } ${isHovered ? 'ring-1 ring-nvr-text-secondary' : ''}`}
                  onMouseEnter={() => setHoveredSlot({ day, slot })}
                  onMouseLeave={() => setHoveredSlot(null)}
                  title={`${DAY_LABELS[day]} ${slotToTime(slot)} - ${slotToTime(slot + 1)}${mode ? ` (${mode})` : ''}`}
                />
              )
            }),
          ]
        })}
      </div>
    </div>
  )
}
