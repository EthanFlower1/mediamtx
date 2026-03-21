import { useState, useMemo } from 'react'

interface RecordingCalendarProps {
  /** Set of date strings (YYYY-MM-DD) that have recordings */
  recordingDates: Set<string>
  /** Set of date strings (YYYY-MM-DD) that have motion events */
  motionDates: Set<string>
  /** Currently selected date (YYYY-MM-DD) */
  selectedDate: string
  /** Called when user clicks a day */
  onSelectDate: (dateString: string) => void
}

const DAY_LABELS = ['S', 'M', 'T', 'W', 'T', 'F', 'S']

function pad(n: number): string {
  return n.toString().padStart(2, '0')
}

function toDateString(year: number, month: number, day: number): string {
  return `${year}-${pad(month + 1)}-${pad(day)}`
}

export default function RecordingCalendar({
  recordingDates,
  motionDates,
  selectedDate,
  onSelectDate,
}: RecordingCalendarProps) {
  const selectedParts = selectedDate.split('-').map(Number)
  const [viewYear, setViewYear] = useState(selectedParts[0])
  const [viewMonth, setViewMonth] = useState(selectedParts[1] - 1) // 0-indexed

  const todayStr = useMemo(() => {
    const now = new Date()
    return toDateString(now.getFullYear(), now.getMonth(), now.getDate())
  }, [])

  // Build calendar grid
  const calendarDays = useMemo(() => {
    const firstDay = new Date(viewYear, viewMonth, 1)
    const startDow = firstDay.getDay() // 0=Sunday
    const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate()

    const days: { day: number; dateStr: string; isCurrentMonth: boolean }[] = []

    // Previous month fill
    if (startDow > 0) {
      const prevMonthDays = new Date(viewYear, viewMonth, 0).getDate()
      const prevMonth = viewMonth === 0 ? 11 : viewMonth - 1
      const prevYear = viewMonth === 0 ? viewYear - 1 : viewYear
      for (let i = startDow - 1; i >= 0; i--) {
        const d = prevMonthDays - i
        days.push({
          day: d,
          dateStr: toDateString(prevYear, prevMonth, d),
          isCurrentMonth: false,
        })
      }
    }

    // Current month
    for (let d = 1; d <= daysInMonth; d++) {
      days.push({
        day: d,
        dateStr: toDateString(viewYear, viewMonth, d),
        isCurrentMonth: true,
      })
    }

    // Next month fill to complete last row
    const remaining = 7 - (days.length % 7)
    if (remaining < 7) {
      const nextMonth = viewMonth === 11 ? 0 : viewMonth + 1
      const nextYear = viewMonth === 11 ? viewYear + 1 : viewYear
      for (let d = 1; d <= remaining; d++) {
        days.push({
          day: d,
          dateStr: toDateString(nextYear, nextMonth, d),
          isCurrentMonth: false,
        })
      }
    }

    return days
  }, [viewYear, viewMonth])

  const monthLabel = new Date(viewYear, viewMonth).toLocaleDateString(undefined, {
    month: 'long',
    year: 'numeric',
  })

  const goToPrevMonth = () => {
    if (viewMonth === 0) {
      setViewMonth(11)
      setViewYear(viewYear - 1)
    } else {
      setViewMonth(viewMonth - 1)
    }
  }

  const goToNextMonth = () => {
    if (viewMonth === 11) {
      setViewMonth(0)
      setViewYear(viewYear + 1)
    } else {
      setViewMonth(viewMonth + 1)
    }
  }

  return (
    <div className="w-64 bg-nvr-bg-secondary border border-nvr-border rounded-lg shadow-xl p-3 select-none">
      {/* Month navigation */}
      <div className="flex items-center justify-between mb-2">
        <button
          onClick={goToPrevMonth}
          className="p-1 text-nvr-text-secondary hover:text-nvr-text-primary transition-colors rounded focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          aria-label="Previous month"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="15 18 9 12 15 6" /></svg>
        </button>
        <span className="text-sm font-medium text-nvr-text-primary">{monthLabel}</span>
        <button
          onClick={goToNextMonth}
          className="p-1 text-nvr-text-secondary hover:text-nvr-text-primary transition-colors rounded focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
          aria-label="Next month"
        >
          <svg xmlns="http://www.w3.org/2000/svg" className="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round"><polyline points="9 18 15 12 9 6" /></svg>
        </button>
      </div>

      {/* Day-of-week headers */}
      <div className="grid grid-cols-7 mb-1">
        {DAY_LABELS.map((label, i) => (
          <div key={i} className="text-center text-[10px] text-nvr-text-muted font-medium py-1">
            {label}
          </div>
        ))}
      </div>

      {/* Day cells */}
      <div className="grid grid-cols-7">
        {calendarDays.map((cell, i) => {
          const isSelected = cell.dateStr === selectedDate
          const isToday = cell.dateStr === todayStr
          const hasRecording = recordingDates.has(cell.dateStr)
          const hasMotion = motionDates.has(cell.dateStr)

          return (
            <button
              key={i}
              onClick={() => onSelectDate(cell.dateStr)}
              className={`relative flex flex-col items-center justify-center h-8 text-xs rounded transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                isSelected
                  ? 'bg-nvr-accent text-white font-semibold'
                  : isToday
                    ? 'border border-nvr-accent/50 text-nvr-text-primary hover:bg-nvr-bg-tertiary'
                    : cell.isCurrentMonth
                      ? 'text-nvr-text-primary hover:bg-nvr-bg-tertiary'
                      : 'text-nvr-text-muted/40 hover:bg-nvr-bg-tertiary/50'
              }`}
            >
              <span className="leading-none">{cell.day}</span>
              {/* Dots for recordings and motion */}
              {(hasRecording || hasMotion) && (
                <div className="flex items-center gap-0.5 mt-0.5">
                  {hasRecording && (
                    <span className={`block w-1 h-1 rounded-full ${isSelected ? 'bg-white/80' : 'bg-blue-400'}`} />
                  )}
                  {hasMotion && (
                    <span className={`block w-1 h-1 rounded-full ${isSelected ? 'bg-amber-200' : 'bg-amber-400'}`} />
                  )}
                </div>
              )}
            </button>
          )
        })}
      </div>

      {/* Legend */}
      <div className="flex items-center gap-3 mt-2 pt-2 border-t border-nvr-border">
        <div className="flex items-center gap-1">
          <span className="block w-1.5 h-1.5 rounded-full bg-blue-400" />
          <span className="text-[10px] text-nvr-text-muted">Recordings</span>
        </div>
        <div className="flex items-center gap-1">
          <span className="block w-1.5 h-1.5 rounded-full bg-amber-400" />
          <span className="text-[10px] text-nvr-text-muted">Motion</span>
        </div>
      </div>
    </div>
  )
}
