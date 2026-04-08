import { useMemo, useRef, useState, useEffect, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
  createColumnHelper,
  type SortingState,
  type RowSelectionState,
} from '@tanstack/react-table';

import type { Camera, CameraStatus } from '@/api/cameras';

// KAI-321: Virtualized camera list.
//
// Uses TanStack Table for column definitions/sorting + a lightweight
// windowing implementation (no extra deps) so 100+ cameras render
// without layout cost. Row selection is controlled by the parent so
// bulk actions can act on the current selection.

const ROW_HEIGHT = 56;
const OVERSCAN = 6;

export interface CameraListProps {
  cameras: Camera[];
  height?: number;
  selection: RowSelectionState;
  onSelectionChange: (next: RowSelectionState) => void;
  onEdit: (camera: Camera) => void;
  onMove: (camera: Camera) => void;
  onDelete: (camera: Camera) => void;
  expandedId: string | null;
  onToggleExpand: (cameraId: string) => void;
}

const columnHelper = createColumnHelper<Camera>();

export function CameraList(props: CameraListProps): JSX.Element {
  const { t } = useTranslation();
  const {
    cameras,
    height = 520,
    selection,
    onSelectionChange,
    onEdit,
    onMove,
    onDelete,
    expandedId,
    onToggleExpand,
  } = props;

  const [sorting, setSorting] = useState<SortingState>([]);

  const columns = useMemo(
    () => [
      columnHelper.accessor('status', {
        id: 'status',
        header: () => t('cameras.columns.status'),
        enableSorting: true,
      }),
      columnHelper.accessor('name', {
        id: 'name',
        header: () => t('cameras.columns.name'),
        enableSorting: true,
      }),
      columnHelper.accessor('model', {
        id: 'model',
        header: () => t('cameras.columns.model'),
        enableSorting: true,
      }),
      columnHelper.accessor('ipAddress', {
        id: 'ipAddress',
        header: () => t('cameras.columns.ipAddress'),
        enableSorting: true,
      }),
      columnHelper.accessor('recorderName', {
        id: 'recorderName',
        header: () => t('cameras.columns.recorder'),
        enableSorting: true,
      }),
      columnHelper.accessor('retentionTier', {
        id: 'retentionTier',
        header: () => t('cameras.columns.retention'),
        enableSorting: true,
      }),
      columnHelper.accessor('profileName', {
        id: 'profileName',
        header: () => t('cameras.columns.profile'),
        enableSorting: true,
      }),
      columnHelper.accessor('lastSeenAt', {
        id: 'lastSeenAt',
        header: () => t('cameras.columns.lastSeen'),
        enableSorting: true,
      }),
    ],
    [t],
  );

  const table = useReactTable({
    data: cameras,
    columns,
    state: { sorting, rowSelection: selection },
    enableRowSelection: true,
    onSortingChange: setSorting,
    onRowSelectionChange: (updater) => {
      const next = typeof updater === 'function' ? updater(selection) : updater;
      onSelectionChange(next);
    },
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

  const allSelected =
    rows.length > 0 && rows.every((r) => selection[r.id]);
  const someSelected = rows.some((r) => selection[r.id]);

  const handleSelectAll = useCallback(() => {
    if (allSelected) {
      onSelectionChange({});
    } else {
      const next: RowSelectionState = {};
      for (const r of rows) next[r.id] = true;
      onSelectionChange(next);
    }
  }, [allSelected, rows, onSelectionChange]);

  return (
    <section
      aria-label={t('cameras.list.sectionLabel')}
      className="cameras-list"
      data-testid="cameras-list"
    >
      <table className="cameras-list__table">
        <thead>
          <tr>
            <th scope="col" style={{ width: 40 }}>
              <input
                type="checkbox"
                aria-label={t('cameras.list.selectAllAriaLabel')}
                checked={allSelected}
                ref={(el) => {
                  if (el) el.indeterminate = !allSelected && someSelected;
                }}
                onChange={handleSelectAll}
                data-testid="cameras-select-all"
              />
            </th>
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
                  data-testid={`cameras-header-${header.id}`}
                >
                  {canSort ? (
                    <button
                      type="button"
                      onClick={header.column.getToggleSortingHandler()}
                      aria-label={t('cameras.list.sortByAriaLabel', {
                        column: String(header.column.columnDef.header),
                      })}
                    >
                      {flexRender(header.column.columnDef.header, header.getContext())}
                      <span aria-hidden="true">
                        {sortDir === 'asc' ? ' ↑' : sortDir === 'desc' ? ' ↓' : ''}
                      </span>
                    </button>
                  ) : (
                    flexRender(header.column.columnDef.header, header.getContext())
                  )}
                </th>
              );
            })}
            <th scope="col">{t('cameras.columns.actions')}</th>
          </tr>
        </thead>
      </table>
      <div
        ref={scrollerRef}
        style={{ height, overflowY: 'auto', position: 'relative' }}
        data-testid="cameras-scroller"
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
              const camera = row.original;
              const isSelected = !!selection[row.id];
              const isExpanded = expandedId === camera.id;
              return (
                <li
                  key={row.id}
                  data-testid={`camera-row-${camera.id}`}
                  data-selected={isSelected ? 'true' : undefined}
                >
                  <div
                    aria-label={t('cameras.row.ariaLabel', {
                      name: camera.name,
                      status: t(`cameras.status.${camera.status}`),
                      model: camera.model,
                    })}
                    style={{
                      height: ROW_HEIGHT,
                      display: 'flex',
                      alignItems: 'center',
                      gap: 12,
                      padding: '0 12px',
                      borderBottom: '1px solid rgba(0,0,0,0.08)',
                    }}
                    data-camera-status={camera.status}
                  >
                    <input
                      type="checkbox"
                      checked={isSelected}
                      onChange={() => row.toggleSelected()}
                      aria-label={t('cameras.row.selectAriaLabel', { name: camera.name })}
                      data-testid={`camera-select-${camera.id}`}
                    />
                    <StatusIndicator status={camera.status} />
                    <button
                      type="button"
                      className="cameras-list__name"
                      onClick={() => onToggleExpand(camera.id)}
                      data-testid={`camera-expand-${camera.id}`}
                      aria-controls={`camera-detail-${camera.id}`}
                      aria-expanded={isExpanded}
                    >
                      {camera.name}
                    </button>
                    <span>{camera.model}</span>
                    <span>{camera.ipAddress}</span>
                    <span>{camera.recorderName}</span>
                    <span>{t(`cameras.retention.${camera.retentionTier}`)}</span>
                    <span>{camera.profileName}</span>
                    <time dateTime={camera.lastSeenAt}>
                      {new Date(camera.lastSeenAt).toLocaleString()}
                    </time>
                    <div className="cameras-list__actions">
                      <button
                        type="button"
                        onClick={() => onEdit(camera)}
                        data-testid={`camera-edit-${camera.id}`}
                        aria-label={t('cameras.actions.editAriaLabel', { name: camera.name })}
                      >
                        {t('cameras.actions.edit')}
                      </button>
                      <button
                        type="button"
                        onClick={() => onMove(camera)}
                        data-testid={`camera-move-${camera.id}`}
                        aria-label={t('cameras.actions.moveAriaLabel', { name: camera.name })}
                      >
                        {t('cameras.actions.move')}
                      </button>
                      <button
                        type="button"
                        onClick={() => onDelete(camera)}
                        data-testid={`camera-delete-${camera.id}`}
                        aria-label={t('cameras.actions.deleteAriaLabel', { name: camera.name })}
                      >
                        {t('cameras.actions.delete')}
                      </button>
                    </div>
                  </div>
                  {isExpanded && (
                    <div
                      id={`camera-detail-${camera.id}`}
                      className="cameras-list__detail"
                      data-testid={`camera-detail-${camera.id}`}
                    >
                      <p>{t('cameras.detail.previewPlaceholder')}</p>
                      <dl>
                        <dt>{t('cameras.detail.rtspUrl')}</dt>
                        <dd>{camera.rtspUrl}</dd>
                        <dt>{t('cameras.detail.vendor')}</dt>
                        <dd>{camera.vendor}</dd>
                      </dl>
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        </div>
      </div>
    </section>
  );
}

// Status indicator uses both color AND glyph so color is not the only
// signal (seam #6 / WCAG 1.4.1 Use of Color).
function StatusIndicator({ status }: { status: CameraStatus }): JSX.Element {
  const { t } = useTranslation();
  const glyph = status === 'online' ? '●' : status === 'warning' ? '▲' : '■';
  return (
    <span
      role="img"
      aria-label={t(`cameras.status.${status}`)}
      data-status={status}
      data-testid="camera-status-indicator"
      className="cameras-list__status"
    >
      <span aria-hidden="true">{glyph}</span>
    </span>
  );
}
