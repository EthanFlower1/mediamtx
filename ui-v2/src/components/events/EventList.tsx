import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { AiEvent } from '@/api/events';
import { EventCard } from './EventCard';

// KAI-324: Virtualized AI event list.
//
// Reuses the same lightweight windowing approach as the KAI-308
// AlertStream — fixed-height rows so the visible slice is an O(1)
// computation and no third-party virtualization dep is needed.
//
// Accessibility:
//  - The outer region is role="log" with aria-live="polite" so new
//    events announce without stealing focus.
//  - Each row is a focusable article carrying a full aria-label
//    including severity, event type, camera, and timestamp (via
//    EventCard).
//  - Severity is encoded as icon + text + border style, never color
//    alone (honored by EventCard).

const ROW_HEIGHT = 96; // px — thumbnail + two text lines + padding
const OVERSCAN = 4;

export interface EventListProps {
  readonly events: readonly AiEvent[];
  readonly height?: number;
  readonly onJumpToPlayback: (event: AiEvent) => void;
}

export function EventList({
  events,
  height = 640,
  onJumpToPlayback,
}: EventListProps): JSX.Element {
  const { t } = useTranslation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = (): void => setScrollTop(el.scrollTop);
    el.addEventListener('scroll', onScroll);
    return () => el.removeEventListener('scroll', onScroll);
  }, []);

  const { startIndex, endIndex } = useMemo(() => {
    const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
    const visible = Math.ceil(height / ROW_HEIGHT);
    const end = Math.min(events.length, start + visible + OVERSCAN * 2);
    return { startIndex: start, endIndex: end };
  }, [events.length, height, scrollTop]);

  if (events.length === 0) {
    return (
      <section
        aria-label={t('events.list.sectionLabel')}
        className="rounded-lg border border-slate-200 bg-white p-6 text-sm text-slate-500 shadow-sm"
        data-testid="events-list-empty"
      >
        {t('events.list.empty')}
      </section>
    );
  }

  const visibleEvents = events.slice(startIndex, endIndex);
  const totalHeight = events.length * ROW_HEIGHT;
  const offsetY = startIndex * ROW_HEIGHT;

  return (
    <section
      aria-label={t('events.list.sectionLabel')}
      className="rounded-lg border border-slate-200 bg-white shadow-sm"
    >
      <header className="flex items-center justify-between border-b border-slate-200 px-4 py-2">
        <h2 className="text-sm font-semibold text-slate-900">
          {t('events.list.heading')}
        </h2>
        <span className="text-xs text-slate-500" data-testid="events-list-count">
          {t('events.list.count', { count: events.length })}
        </span>
      </header>

      <div
        ref={scrollRef}
        role="log"
        aria-live="polite"
        aria-relevant="additions"
        data-testid="events-list-scroll"
        className="overflow-y-auto"
        style={{ height }}
      >
        <div style={{ height: totalHeight, position: 'relative' }}>
          <ul
            style={{
              transform: `translateY(${offsetY}px)`,
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              margin: 0,
              padding: 0,
              listStyle: 'none',
            }}
          >
            {visibleEvents.map((event) => (
              <li key={event.id} style={{ height: ROW_HEIGHT }}>
                <EventCard event={event} onJumpToPlayback={onJumpToPlayback} />
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
