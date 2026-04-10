import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { buildEventsCsv, exportEventsPdf, type AiEvent } from '@/api/events';
import { useSessionStore } from '@/stores/session';

// KAI-324: CSV + PDF export.
//
// CSV is generated client-side from the currently-rendered event
// slice — no server round trip needed. PDF hits a mock backend
// endpoint because the real PDF renderer needs the original video
// frames (backend-only). Both buttons show an ARIA-live status
// region so screen reader users get export confirmation.

export interface EventExportProps {
  readonly events: readonly AiEvent[];
}

export function EventExport({ events }: EventExportProps): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const [status, setStatus] = useState<string>('');

  const downloadCsv = (): void => {
    const csv = buildEventsCsv(events);
    if (typeof URL !== 'undefined' && typeof URL.createObjectURL === 'function') {
      // Real browser: trigger a blob download.
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'events.csv';
      a.click();
      URL.revokeObjectURL(url);
    }
    // jsdom path: just announce success; the CSV string is verified
    // directly in unit tests via buildEventsCsv().
    setStatus(t('events.export.csvReady', { count: events.length }));
  };

  const downloadPdf = async (): Promise<void> => {
    setStatus(t('events.export.pdfRendering'));
    const result = await exportEventsPdf({
      tenantId,
      eventIds: events.map((e) => e.id),
    });
    setStatus(t('events.export.pdfReady', { pages: result.pageCount }));
  };

  return (
    <section
      aria-label={t('events.export.sectionLabel')}
      data-testid="events-export"
      className="flex items-center gap-2"
    >
      <button
        type="button"
        onClick={downloadCsv}
        disabled={events.length === 0}
        data-testid="events-export-csv"
        className="rounded border border-slate-300 bg-white px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {t('events.export.csvButton')}
      </button>
      <button
        type="button"
        onClick={() => {
          void downloadPdf();
        }}
        disabled={events.length === 0}
        data-testid="events-export-pdf"
        className="rounded border border-slate-300 bg-white px-3 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {t('events.export.pdfButton')}
      </button>
      <p
        role="status"
        aria-live="polite"
        data-testid="events-export-status"
        className="text-xs text-slate-600"
      >
        {status}
      </p>
    </section>
  );
}
