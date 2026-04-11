import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import {
  type ColumnDef,
  type SortingState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import {
  type AuditLogEntry,
  type ImpersonationSessionDetail,
  getAllImpersonationAuditLog,
  impersonationAuditAllKey,
  impersonationSessionsKey,
  listAllSessions,
} from '@/api/impersonation';
import { cn } from '@/lib/utils';

// KAI-467: Session audit table.
//
// Two sections:
//   1. Active + recent impersonation sessions
//   2. Full audit log for impersonation actions
//
// Both are TanStack Table instances with sorting. The sessions table
// shows session lifecycle (start, end, status). The audit log shows
// every individual action taken during impersonation sessions.
//
// Accessibility:
//   - Full <table> semantics with scope="col" headers
//   - Sortable headers are buttons with aria-sort
//   - Result badges use color + text (never color alone)

const INTEGRATOR_TENANT_ID = 'integrator-001';

function ResultBadge({ result }: { readonly result: 'allow' | 'deny' }): JSX.Element {
  const { t } = useTranslation();
  return (
    <span
      data-testid={`result-badge-${result}`}
      className={cn(
        'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium',
        result === 'allow'
          ? 'border-emerald-300 bg-emerald-100 text-emerald-800'
          : 'border-red-300 bg-red-100 text-red-800',
      )}
    >
      <span aria-hidden="true">{result === 'allow' ? '\u2713' : '\u2717'}</span>
      <span>{t(`impersonation.audit.result.${result}`)}</span>
    </span>
  );
}

function StatusBadge({
  status,
}: {
  readonly status: ImpersonationSessionDetail['status'];
}): JSX.Element {
  const { t } = useTranslation();
  const styles: Record<string, string> = {
    active: 'border-blue-300 bg-blue-100 text-blue-800',
    expired: 'border-slate-300 bg-slate-100 text-slate-700',
    terminated: 'border-amber-300 bg-amber-100 text-amber-800',
  };
  return (
    <span
      data-testid={`session-status-${status}`}
      className={cn(
        'inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium',
        styles[status] ?? styles.expired,
      )}
    >
      {t(`impersonation.session.status.${status}`)}
    </span>
  );
}

function formatTimestamp(iso: string, locale: string): string {
  try {
    return new Intl.DateTimeFormat(locale, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).format(new Date(iso));
  } catch {
    return iso;
  }
}

function formatAction(action: string): string {
  // Transform "impersonation.session.start" -> "Session Start"
  const parts = action.split('.');
  const last = parts[parts.length - 1] ?? action;
  const secondLast = parts.length >= 2 ? parts[parts.length - 2] : '';
  return `${secondLast.charAt(0).toUpperCase() + secondLast.slice(1)} ${last.charAt(0).toUpperCase() + last.slice(1)}`;
}

export function SessionAuditTable(): JSX.Element {
  const { t, i18n } = useTranslation();
  const [sessionSorting, setSessionSorting] = useState<SortingState>([]);
  const [auditSorting, setAuditSorting] = useState<SortingState>([]);

  const sessionsQuery = useQuery({
    queryKey: impersonationSessionsKey(INTEGRATOR_TENANT_ID),
    queryFn: () => listAllSessions(INTEGRATOR_TENANT_ID),
  });

  const auditQuery = useQuery({
    queryKey: impersonationAuditAllKey(),
    queryFn: () => getAllImpersonationAuditLog(),
  });

  const sessionColumns = useMemo<ColumnDef<ImpersonationSessionDetail>[]>(
    () => [
      {
        id: 'status',
        header: () => t('impersonation.sessions.columns.status'),
        accessorKey: 'status',
        cell: (info) => <StatusBadge status={info.getValue<ImpersonationSessionDetail['status']>()} />,
      },
      {
        id: 'customer',
        header: () => t('impersonation.sessions.columns.customer'),
        accessorKey: 'impersonatedTenantName',
        cell: (info) => (
          <span className="font-medium text-slate-900">{info.getValue<string>()}</span>
        ),
      },
      {
        id: 'user',
        header: () => t('impersonation.sessions.columns.user'),
        accessorKey: 'impersonatingUserName',
      },
      {
        id: 'mode',
        header: () => t('impersonation.sessions.columns.mode'),
        accessorKey: 'mode',
        cell: (info) => t(`impersonation.session.mode.${info.getValue<string>()}`),
      },
      {
        id: 'reason',
        header: () => t('impersonation.sessions.columns.reason'),
        accessorKey: 'reason',
        cell: (info) => (
          <span className="max-w-xs truncate" title={info.getValue<string>()}>
            {info.getValue<string>()}
          </span>
        ),
      },
      {
        id: 'started',
        header: () => t('impersonation.sessions.columns.started'),
        accessorKey: 'createdAtIso',
        cell: (info) => formatTimestamp(info.getValue<string>(), i18n.language),
      },
      {
        id: 'expires',
        header: () => t('impersonation.sessions.columns.expires'),
        accessorKey: 'expiresAtIso',
        cell: (info) => formatTimestamp(info.getValue<string>(), i18n.language),
      },
    ],
    [t, i18n.language],
  );

  const auditColumns = useMemo<ColumnDef<AuditLogEntry>[]>(
    () => [
      {
        id: 'timestamp',
        header: () => t('impersonation.audit.columns.timestamp'),
        accessorKey: 'timestampIso',
        cell: (info) => (
          <span className="font-mono text-xs">
            {formatTimestamp(info.getValue<string>(), i18n.language)}
          </span>
        ),
      },
      {
        id: 'actor',
        header: () => t('impersonation.audit.columns.actor'),
        accessorKey: 'actorUserName',
        cell: (info) => (
          <span className="font-medium text-slate-900">{info.getValue<string>()}</span>
        ),
      },
      {
        id: 'action',
        header: () => t('impersonation.audit.columns.action'),
        accessorKey: 'action',
        cell: (info) => (
          <span className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-xs">
            {formatAction(info.getValue<string>())}
          </span>
        ),
      },
      {
        id: 'resource',
        header: () => t('impersonation.audit.columns.resource'),
        accessorFn: (row) => `${row.resourceType}/${row.resourceId}`,
        cell: (info) => (
          <span className="font-mono text-xs text-slate-600">{info.getValue<string>()}</span>
        ),
      },
      {
        id: 'result',
        header: () => t('impersonation.audit.columns.result'),
        accessorKey: 'result',
        cell: (info) => <ResultBadge result={info.getValue<'allow' | 'deny'>()} />,
      },
      {
        id: 'error',
        header: () => t('impersonation.audit.columns.error'),
        accessorKey: 'errorCode',
        cell: (info) => {
          const code = info.getValue<string | null>();
          return code ? (
            <span className="rounded bg-red-50 px-1.5 py-0.5 text-xs text-red-700">{code}</span>
          ) : (
            <span className="text-xs text-slate-400">-</span>
          );
        },
      },
    ],
    [t, i18n.language],
  );

  const sessionsTable = useReactTable({
    data: (sessionsQuery.data ?? []) as ImpersonationSessionDetail[],
    columns: sessionColumns,
    state: { sorting: sessionSorting },
    onSortingChange: setSessionSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  const auditTable = useReactTable({
    data: (auditQuery.data ?? []) as AuditLogEntry[],
    columns: auditColumns,
    state: { sorting: auditSorting },
    onSortingChange: setAuditSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  function renderTable<T>(
    table: ReturnType<typeof useReactTable<T>>,
    testId: string,
    emptyKey: string,
    colCount: number,
  ): JSX.Element {
    return (
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm" data-testid={testId}>
          <thead className="bg-slate-50">
            {table.getHeaderGroups().map((hg) => (
              <tr key={hg.id}>
                {hg.headers.map((header) => {
                  const canSort = header.column.getCanSort();
                  const sortDir = header.column.getIsSorted();
                  const ariaSort =
                    sortDir === 'asc'
                      ? 'ascending'
                      : sortDir === 'desc'
                        ? 'descending'
                        : 'none';
                  return (
                    <th
                      key={header.id}
                      scope="col"
                      aria-sort={canSort ? ariaSort : undefined}
                      className="border-b border-slate-200 px-3 py-2 text-left text-xs font-semibold text-slate-700"
                    >
                      {canSort ? (
                        <button
                          type="button"
                          onClick={header.column.getToggleSortingHandler()}
                          className="flex items-center gap-1 focus:outline-none focus:ring-2 focus:ring-blue-500"
                        >
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          <span aria-hidden="true">
                            {sortDir === 'asc' ? '\u25B2' : sortDir === 'desc' ? '\u25BC' : '\u2195'}
                          </span>
                        </button>
                      ) : (
                        flexRender(header.column.columnDef.header, header.getContext())
                      )}
                    </th>
                  );
                })}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.length === 0 ? (
              <tr>
                <td
                  colSpan={colCount}
                  data-testid={`${testId}-empty`}
                  className="px-3 py-6 text-center text-sm text-slate-500"
                >
                  {t(emptyKey)}
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => (
                <tr
                  key={row.id}
                  className="border-b border-slate-100 hover:bg-slate-50"
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-3 py-2 align-middle">
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    );
  }

  return (
    <div data-testid="session-audit-table" className="space-y-6">
      <section
        aria-label={t('impersonation.sessions.regionLabel')}
        className="rounded-lg border border-slate-200 bg-white shadow-sm"
      >
        <header className="border-b border-slate-200 px-4 py-3">
          <h2 className="text-base font-semibold text-slate-900">
            {t('impersonation.sessions.heading')}
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            {t('impersonation.sessions.description')}
          </p>
        </header>
        {sessionsQuery.isLoading ? (
          <p role="status" className="px-4 py-6 text-sm text-slate-500">
            {t('impersonation.sessions.loading')}
          </p>
        ) : (
          renderTable(
            sessionsTable,
            'sessions-table',
            'impersonation.sessions.empty',
            sessionColumns.length,
          )
        )}
      </section>

      <section
        aria-label={t('impersonation.audit.regionLabel')}
        className="rounded-lg border border-slate-200 bg-white shadow-sm"
      >
        <header className="border-b border-slate-200 px-4 py-3">
          <h2 className="text-base font-semibold text-slate-900">
            {t('impersonation.audit.heading')}
          </h2>
          <p className="mt-0.5 text-xs text-slate-500">
            {t('impersonation.audit.description')}
          </p>
        </header>
        {auditQuery.isLoading ? (
          <p role="status" className="px-4 py-6 text-sm text-slate-500">
            {t('impersonation.audit.loading')}
          </p>
        ) : (
          renderTable(
            auditTable,
            'audit-log-table',
            'impersonation.audit.empty',
            auditColumns.length,
          )
        )}
      </section>
    </div>
  );
}
