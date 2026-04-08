import { useMemo, useRef, useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { AiEvent } from '@/api/dashboard';

// KAI-320: Virtualized event stream.
//
// A lightweight windowing implementation (no extra deps) so the
// dashboard can render thousands of events without layout cost.
// Exported for reuse by the standalone Events page in a later ticket.

const ROW_HEIGHT = 56;
const OVERSCAN = 4;

export interface EventStreamProps {
  events: AiEvent[];
  height?: number;
  emptyLabel?: string;
}

export function EventStream({ events, height = 320, emptyLabel }: EventStreamProps): JSX.Element {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);

  const onScroll = useCallback(() => {
    if (containerRef.current) {
      setScrollTop(containerRef.current.scrollTop);
    }
  }, []);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    el.addEventListener('scroll', onScroll, { passive: true });
    return () => el.removeEventListener('scroll', onScroll);
  }, [onScroll]);

  const { startIndex, endIndex, offsetY, totalHeight } = useMemo(() => {
    const total = events.length * ROW_HEIGHT;
    const firstVisible = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
    const visibleCount = Math.ceil(height / ROW_HEIGHT) + OVERSCAN * 2;
    const lastVisible = Math.min(events.length, firstVisible + visibleCount);
    return {
      startIndex: firstVisible,
      endIndex: lastVisible,
      offsetY: firstVisible * ROW_HEIGHT,
      totalHeight: total,
    };
  }, [events.length, scrollTop, height]);

  const visible = events.slice(startIndex, endIndex);

  if (events.length === 0) {
    return (
      <section aria-label={t('dashboard.events.sectionLabel')}>
        <h2>{t('dashboard.events.heading')}</h2>
        <p>{emptyLabel ?? t('dashboard.events.empty')}</p>
      </section>
    );
  }

  return (
    <section
      aria-label={t('dashboard.events.sectionLabel')}
      className="admin-dashboard__event-stream"
    >
      <h2 id="event-stream-heading">{t('dashboard.events.heading')}</h2>
      <div
        ref={containerRef}
        role="log"
        aria-labelledby="event-stream-heading"
        aria-live="polite"
        aria-relevant="additions"
        data-testid="event-stream-scroller"
        tabIndex={0}
        style={{ height, overflowY: 'auto', position: 'relative' }}
      >
        <div style={{ height: totalHeight, position: 'relative' }}>
          <ul
            role="list"
            style={{
              position: 'absolute',
              top: offsetY,
              left: 0,
              right: 0,
              margin: 0,
              padding: 0,
              listStyle: 'none',
            }}
          >
            {visible.map((evt) => (
              <li
                key={evt.id}
                data-testid="event-row"
                data-severity={evt.severity}
                style={{
                  height: ROW_HEIGHT,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '0 12px',
                  borderBottom: '1px solid rgba(0,0,0,0.08)',
                }}
              >
                <span className="admin-dashboard__event-camera">{evt.cameraName}</span>
                <span className="admin-dashboard__event-type">
                  {t(`dashboard.events.type.${evt.eventType}`, {
                    defaultValue: evt.eventType,
                  })}
                </span>
                <time
                  dateTime={evt.timestamp}
                  className="admin-dashboard__event-time"
                >
                  {new Date(evt.timestamp).toLocaleTimeString()}
                </time>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </section>
  );
}
