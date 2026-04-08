import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { FleetAlert, AlertSeverity } from '@/api/fleet';
import { cn } from '@/lib/utils';

// KAI-308: Cross-customer alert stream.
//
// This is a lightweight windowed virtualizer — it renders only the
// rows that fit in the viewport plus a small overscan. No third-party
// virtualization dependency is added; the algorithm is a fixed-height
// list (every row is ROW_HEIGHT px), which keeps the bundle small
// and the component trivially testable.
//
// Accessibility:
//  - The list uses role="log" with aria-live="polite" so new alerts
//    announce to screen readers without stealing focus.
//  - Each row is focusable and carries a full aria-label including
//    severity, customer and timestamp.
//  - Severity is encoded with icon + text, never color alone.

const ROW_HEIGHT = 64; // px
const OVERSCAN = 4;

const SEVERITY_GLYPH: Record<AlertSeverity, string> = {
  info: 'i',
  warning: '!',
  critical: '✕',
};

const SEVERITY_CLASS: Record<AlertSeverity, string> = {
  info: 'border-l-blue-500 bg-blue-50',
  warning: 'border-l-amber-500 bg-amber-50',
  critical: 'border-l-red-500 bg-red-50',
};

export interface AlertStreamProps {
  readonly alerts: readonly FleetAlert[];
  readonly height?: number;
}

export function AlertStream({ alerts, height = 480 }: AlertStreamProps): JSX.Element {
  const { t, i18n } = useTranslation();
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
    const end = Math.min(alerts.length, start + visible + OVERSCAN * 2);
    return { startIndex: start, endIndex: end };
  }, [alerts.length, height, scrollTop]);

  const visibleAlerts = alerts.slice(startIndex, endIndex);
  const totalHeight = alerts.length * ROW_HEIGHT;
  const offsetY = startIndex * ROW_HEIGHT;

  const dateFmt = useMemo(
    () =>
      new Intl.DateTimeFormat(i18n.language, {
        hour: '2-digit',
        minute: '2-digit',
      }),
    [i18n.language],
  );

  return (
    <section
      aria-label={t('fleet.alerts.sectionLabel')}
      className="rounded-lg border border-slate-200 bg-white shadow-sm"
    >
      <header className="flex items-center justify-between border-b border-slate-200 px-4 py-2">
        <h2 className="text-sm font-semibold text-slate-900">{t('fleet.alerts.title')}</h2>
        <span className="text-xs text-slate-500">
          {t('fleet.alerts.count', { count: alerts.length })}
        </span>
      </header>

      {alerts.length === 0 ? (
        <p className="p-4 text-sm text-slate-500">{t('fleet.alerts.empty')}</p>
      ) : (
        <div
          ref={scrollRef}
          role="log"
          aria-live="polite"
          aria-relevant="additions"
          data-testid="alert-stream-scroll"
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
              {visibleAlerts.map((alert) => {
                const severityLabel = t(`fleet.alerts.severity.${alert.severity}`);
                const message = t(alert.messageKey, { customer: alert.customerName });
                const ts = dateFmt.format(new Date(alert.createdAtIso));
                return (
                  <li
                    key={alert.id}
                    style={{ height: ROW_HEIGHT }}
                    className={cn(
                      'flex items-center gap-3 border-b border-l-4 border-slate-100 px-4',
                      SEVERITY_CLASS[alert.severity],
                    )}
                  >
                    <span
                      aria-hidden="true"
                      className="inline-flex h-6 w-6 items-center justify-center rounded-full bg-white text-sm font-bold"
                    >
                      {SEVERITY_GLYPH[alert.severity]}
                    </span>
                    <div
                      tabIndex={0}
                      role="article"
                      aria-label={`${severityLabel}: ${message} — ${alert.customerName} — ${ts}`}
                      data-testid={`alert-row-${alert.id}`}
                      className="flex-1 focus:outline-none focus:ring-2 focus:ring-blue-500"
                    >
                      <p className="text-sm font-medium text-slate-900">{message}</p>
                      <p className="text-xs text-slate-600">
                        <span className="sr-only">{severityLabel} — </span>
                        {alert.customerName} • {ts}
                      </p>
                    </div>
                  </li>
                );
              })}
            </ul>
          </div>
        </div>
      )}
    </section>
  );
}
