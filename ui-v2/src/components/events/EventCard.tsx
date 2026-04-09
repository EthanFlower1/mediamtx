import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { AiEvent, EventSeverity } from '@/api/events';
import { cn } from '@/lib/utils';

// KAI-324: Single row in the virtualized event list.
//
// Severity is expressed with three independent signals so it never
// relies on color alone (a11y seam #6):
//   - a glyph (i / · / ! / ‼ / ✕)
//   - a plain-text severity label inside the aria-label
//   - a left border style
//
// The row is a focusable article; keyboard users can tab through
// the list and press Enter to jump to playback.

const SEVERITY_GLYPH: Record<EventSeverity, string> = {
  info: 'i',
  low: '·',
  medium: '!',
  high: '‼',
  critical: '✕',
};

const SEVERITY_CLASS: Record<EventSeverity, string> = {
  info: 'border-l-blue-500 bg-blue-50',
  low: 'border-l-sky-500 bg-sky-50',
  medium: 'border-l-amber-500 bg-amber-50',
  high: 'border-l-orange-500 bg-orange-50',
  critical: 'border-l-red-500 bg-red-50',
};

export interface EventCardProps {
  readonly event: AiEvent;
  readonly onJumpToPlayback: (event: AiEvent) => void;
}

export function EventCard({ event, onJumpToPlayback }: EventCardProps): JSX.Element {
  const { t, i18n } = useTranslation();

  const dateFmt = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.language, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      }),
    [i18n.language],
  );

  const ts = dateFmt.format(new Date(event.timestamp));
  const severityLabel = t(`events.severity.${event.severity}`);
  const typeLabel = t(`events.type.${event.type}`);
  const ariaLabel = t('events.row.ariaLabel', {
    severity: severityLabel,
    type: typeLabel,
    camera: event.cameraName,
    time: ts,
  });

  return (
    <article
      tabIndex={0}
      role="article"
      aria-label={ariaLabel}
      data-testid={`event-row-${event.id}`}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onJumpToPlayback(event);
        }
      }}
      className={cn(
        'flex h-full items-center gap-3 border-b border-l-4 border-slate-100 px-4 focus:outline-none focus:ring-2 focus:ring-blue-500',
        SEVERITY_CLASS[event.severity],
      )}
    >
      <span
        aria-hidden="true"
        className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-white text-sm font-bold"
      >
        {SEVERITY_GLYPH[event.severity]}
      </span>

      <div
        aria-hidden="true"
        className="h-16 w-24 shrink-0 overflow-hidden rounded bg-slate-200"
        data-testid={`event-thumbnail-${event.id}`}
      >
        {/* Real thumbnail src is set via the Multi-Recorder timeline
            API (KAI-262). In the scaffold we render a neutral
            placeholder so the layout is deterministic in tests. */}
        <span className="sr-only">{t('events.thumbnail.placeholder')}</span>
      </div>

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-slate-900">
          <span className="sr-only">{severityLabel} — </span>
          {typeLabel}
        </p>
        <p className="truncate text-xs text-slate-600">
          {event.cameraName} • {ts}
        </p>
      </div>

      <button
        type="button"
        onClick={() => onJumpToPlayback(event)}
        data-testid={`event-jump-${event.id}`}
        className="shrink-0 rounded border border-slate-300 bg-white px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        {t('events.row.jumpToPlayback')}
      </button>
    </article>
  );
}
