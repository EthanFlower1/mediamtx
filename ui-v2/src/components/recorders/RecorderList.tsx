import { useMemo, useRef, useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
  createColumnHelper,
  type SortingState,
} from '@tanstack/react-table';

import type { Recorder, RecorderHealth } from '@/api/recorders';

// KAI-322: Virtualized recorder list.
//
// Mirrors the CameraList pattern (KAI-321): TanStack Table for column
// definitions/sorting, lightweight scroll-position windowing so 100+
// recorders render without layout cost. Renders in: customer admin
// (/admin/recorders). NOT rendered in integrator portal or on-prem embed
// directly, but the on-prem embed does serve /admin/* routes, so this
// component must be accessible (WCAG 2.1 AA) and white-label ready
// (no hardcoded colors — all via CSS variables from brand config / KAI-310).

const ROW_HEIGHT = 60;
const OVERSCAN = 5;

export interface RecorderListProps {
  recorders: Recorder[];
  height?: number;
  onDetail: (recorder: Recorder) => void;
  onUnpair: (recorder: Recorder) => void;
}

const columnHelper = createColumnHelper<Recorder>();

function formatBytes(bytes: number): string {
  if (bytes >= 1e12) return `${(bytes / 1e12).toFixed(1)} TB`;
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  return `${(bytes / 1e6).toFixed(0)} MB`;
}

export function RecorderList(props: RecorderListProps): JSX.Element {
  const { t } = useTranslation();
  const { recorders, height = 520, onDetail, onUnpair } = props;

  const [sorting, setSorting] = useState<SortingState>([]);

  const columns = useMemo(
    () => [
      columnHelper.accessor('health', {
        id: 'health',
        header: () => t('recorders.columns.health'),
        enableSorting: true,
      }),
      columnHelper.accessor('name', {
        id: 'name',
        header: () => t('recorders.columns.name'),
        enableSorting: true,
      }),
      columnHelper.accessor((row) => row.hardware.cpuModel, {
        id: 'hardware',
        header: () => t('recorders.columns.hardware'),
        enableSorting: true,
      }),
      columnHelper.accessor('cameraCount', {
        id: 'cameraCount',
        header: () => t('recorders.columns.cameras'),
        enableSorting: true,
      }),
      columnHelper.accessor('lastCheckIn', {
        id: 'lastCheckIn',
        header: () => t('recorders.columns.lastCheckIn'),
        enableSorting: true,
      }),
      columnHelper.accessor((row) => row.storage.usedBytes / row.storage.totalBytes, {
        id: 'storage',
        header: () => t('recorders.columns.storage'),
        enableSorting: true,
      }),
    ],
    [t],
  );

  const table = useReactTable({
    data: recorders,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getRowId: (row) => row.id,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const rows = table.getRowModel().rows;

  // --- virtualization ------------------------------------------------
  const scrollerRef = useRef<HTMLDivElement | null>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const onScroll = useCallback(() => {
    if (scrollerRef.current) setScrollTop(scrollerRef.current.scrollTop);
  }, []);
  useEffect(() => {
    const el = scrollerRef.current;
    if (!el) return;
    el.addEventListener('scroll', onScroll, { passive: true });
    return () => el.removeEventListener('scroll', onScroll);
  }, [onScroll]);

  const { startIndex, endIndex, offsetY, totalHeight } = useMemo(() => {
    const total = rows.length * ROW_HEIGHT;
    const first = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
    const visibleCount = Math.ceil(height / ROW_HEIGHT) + OVERSCAN * 2;
    const last = Math.min(rows.length, first + visibleCount);
    return { startIndex: first, endIndex: last, offsetY: first * ROW_HEIGHT, totalHeight: total };
  }, [rows.length, scrollTop, height]);

  const visible = rows.slice(startIndex, endIndex);

  if (recorders.length === 0) {
    return (
      <section
        aria-label={t('recorders.list.sectionLabel')}
        className="recorders-list recorders-list--empty"
        data-testid="recorders-list"
      >
        <p data-testid="recorders-empty">{t('recorders.list.empty')}</p>
      </section>
    );
  }

  return (
    <section
      aria-label={t('recorders.list.sectionLabel')}
      className="recorders-list"
      data-testid="recorders-list"
    >
      <table className="recorders-list__table">
        <thead>
          <tr>
            {table.getHeaderGroups()[0]!.headers.map((header) => {
              const canSort = header.column.getCanSort();
              const sortDir = header.column.getIsSorted();
              const ariaSort =
                sortDir === 'asc' ? 'ascending' : sortDir === 'desc' ? 'descending' : 'none';
              return (
                <th
                  key={header.id}
                  scope="col"
                  aria-sort={ariaSort}
                  data-testid={`recorders-header-${header.id}`}
                >
                  {canSort ? (
                    <button
                      type="button"
                      onClick={header.column.getToggleSortingHandler()}
                      aria-label={t('recorders.list.sortByAriaLabel', {
                        column: String(header.column.columnDef.header),
                      })}
                    >
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      <span aria-hidden="true">
                        {sortDir === 'asc' ? ' \u2191' : sortDir === 'desc' ? ' \u2193' : ''}
                      </span>
                    </button>
                  ) : (
                    flexRender(header.column.columnDef.header, header.getContext())
                  )}
                </th>
              );
            })}
            <th scope="col">{t('recorders.columns.actions')}</th>
          </tr>
        </thead>
      </table>

      <div
        ref={scrollerRef}
        style={{ height, overflowY: 'auto', position: 'relative' }}
        data-testid="recorders-scroller"
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
            {visible.map((row) => {
              const recorder = row.original;
              return (
                <li
                  key={row.id}
                  data-testid={`recorder-row-${recorder.id}`}
                  style={{ height: ROW_HEIGHT }}
                >
                  <div
                    aria-label={t('recorders.row.ariaLabel', {
                      name: recorder.name,
                      health: t(`recorders.health.${recorder.health}`),
                    })}
                    style={{
                      height: ROW_HEIGHT,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 12,
                      padding: '0 12px',
                      borderBottom: '1px solid rgba(0,0,0,0.08)',
                    }}
                  >
                    <RecorderHealthIndicator health={recorder.health} />
                    <span style={{ minWidth: 160 }}>{recorder.name}</span>
                    <span style={{ minWidth: 200, fontSize: '0.85em', color: 'var(--color-text-secondary, #666)' }}>
                      {recorder.hardware.cpuModel}
                    </span>
                    <span style={{ minWidth: 80 }}>
                      {t('recorders.row.cameraCount', { count: recorder.cameraCount })}
                    </span>
                    <time dateTime={recorder.lastCheckIn} style={{ minWidth: 160, fontSize: '0.85em' }}>
                      {new Date(recorder.lastCheckIn).toLocaleString()}
                    </time>
                    <StorageBar usage={recorder.storage} />
                    <div className="recorders-list__actions" style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
                      <button
                        type="button"
                        onClick={() => onDetail(recorder)}
                        data-testid={`recorder-detail-${recorder.id}`}
                        aria-label={t('recorders.actions.detailAriaLabel', { name: recorder.name })}
                      >
                        {t('recorders.actions.detail')}
                      </button>
                      <button
                        type="button"
                        onClick={() => onUnpair(recorder)}
                        data-testid={`recorder-unpair-${recorder.id}`}
                        aria-label={t('recorders.actions.unpairAriaLabel', { name: recorder.name })}
                      >
                        {t('recorders.actions.unpair')}
                      </button>
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>
        </div>
      </div>
    </section>
  );
}

function RecorderHealthIndicator({ health }: { health: RecorderHealth }): JSX.Element {
  const { t } = useTranslation();
  const glyph = health === 'online' ? '●' : health === 'degraded' ? '▲' : '■';
  return (
    <span
      role="img"
      aria-label={t(`recorders.health.${health}`)}
      data-health={health}
      data-testid="recorder-health-indicator"
      className="recorders-list__health"
    >
      <span aria-hidden="true">{glyph}</span>
    </span>
  );
}

function StorageBar({ usage }: { usage: { usedBytes: number; totalBytes: number } }): JSX.Element {
  const { t } = useTranslation();
  const pct = Math.round((usage.usedBytes / usage.totalBytes) * 100);
  const used = formatBytes(usage.usedBytes);
  const total = formatBytes(usage.totalBytes);
  return (
    <span
      title={t('recorders.storage.label', { used, total })}
      aria-label={t('recorders.storage.label', { used, total })}
      style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 140 }}
      data-testid="recorder-storage"
    >
      <span
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={t('recorders.storage.label', { used, total })}
        style={{
          display: 'inline-block',
          width: 80,
          height: 8,
          background: 'var(--color-surface-secondary, #e5e7eb)',
          borderRadius: 4,
          overflow: 'hidden',
        }}
      >
        <span
          aria-hidden="true"
          style={{
            display: 'block',
            width: `${pct}%`,
            height: '100%',
            background: pct > 85 ? 'var(--color-danger, #ef4444)' : 'var(--color-primary, #3b82f6)',
          }}
        />
      </span>
      <span style={{ fontSize: '0.8em' }}>{pct}%</span>
    </span>
  );
}
