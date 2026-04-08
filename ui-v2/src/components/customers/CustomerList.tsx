import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  type ColumnDef,
  type SortingState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  useReactTable,
} from '@tanstack/react-table';
import type {
  CustomerRelationship,
  CustomerStatus,
  CustomerSummary,
} from '@/api/customers';
import { cn } from '@/lib/utils';

// KAI-309: Customer list for the Integrator Portal.
//
// TanStack Table keeps the data shape plain and sortable. We render
// a plain tbody (not the virtualizer) for the mock's 30 rows because
// the v1 mock dataset is tiny — the virtualization plumbing can land
// when the real dataset does. The row count is capped by the list
// query, so the DOM cost is bounded.
//
// Accessibility:
//  - Full <table> semantics; headers are buttons when sortable with
//    aria-sort on the <th>.
//  - Row activation is a <tr> with role="link", keyboard-enabled
//    (Enter / Space), aria-label summarizing the row.
//  - Status is communicated by color + icon + text — never color alone.

export interface CustomerListProps {
  readonly customers: readonly CustomerSummary[];
  readonly onRowActivate?: (customer: CustomerSummary) => void;
  readonly onCreateClick?: () => void;
  readonly search: string;
  readonly onSearchChange: (v: string) => void;
}

const STATUS_ICON: Record<CustomerStatus, string> = {
  active: '✓',
  pending: '…',
  suspended: '!',
  churned: '✕',
};

const STATUS_CLASSES: Record<CustomerStatus, string> = {
  active: 'bg-emerald-100 text-emerald-800 border-emerald-300',
  pending: 'bg-amber-100 text-amber-800 border-amber-300',
  suspended: 'bg-orange-100 text-orange-900 border-orange-300',
  churned: 'bg-slate-200 text-slate-700 border-slate-400',
};

function formatCents(cents: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

function formatRelativeIso(iso: string, locale: string): string {
  try {
    const d = new Date(iso);
    return new Intl.DateTimeFormat(locale, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    }).format(d);
  } catch {
    return iso;
  }
}

export function CustomerList({
  customers,
  onRowActivate,
  onCreateClick,
  search,
  onSearchChange,
}: CustomerListProps): JSX.Element {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [sorting, setSorting] = useState<SortingState>([]);

  const columns = useMemo<ColumnDef<CustomerSummary>[]>(
    () => [
      {
        id: 'status',
        header: () => t('customers.list.columns.status'),
        accessorKey: 'status',
        cell: (info) => {
          const s = info.getValue<CustomerStatus>();
          return (
            <span
              data-testid={`status-badge-${s}`}
              className={cn(
                'inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium',
                STATUS_CLASSES[s],
              )}
            >
              <span aria-hidden="true">{STATUS_ICON[s]}</span>
              <span>{t(`customers.status.${s}`)}</span>
            </span>
          );
        },
      },
      {
        id: 'name',
        header: () => t('customers.list.columns.name'),
        accessorKey: 'name',
        cell: (info) => (
          <span className="font-medium text-slate-900">{info.getValue<string>()}</span>
        ),
      },
      {
        id: 'tier',
        header: () => t('customers.list.columns.tier'),
        accessorKey: 'tier',
        cell: (info) => t(`fleet.tier.${info.getValue<CustomerSummary['tier']>()}`),
      },
      {
        id: 'cameras',
        header: () => t('customers.list.columns.camerasManaged'),
        accessorKey: 'camerasManaged',
        cell: (info) => info.getValue<number>().toLocaleString(i18n.language),
      },
      {
        id: 'mrr',
        header: () => t('customers.list.columns.mrr'),
        accessorKey: 'monthlyRecurringRevenueCents',
        cell: (info) => formatCents(info.getValue<number>(), i18n.language),
      },
      {
        id: 'lastActivity',
        header: () => t('customers.list.columns.lastActivity'),
        accessorKey: 'lastActivityIso',
        cell: (info) => formatRelativeIso(info.getValue<string>(), i18n.language),
      },
      {
        id: 'relationship',
        header: () => t('customers.list.columns.relationship'),
        accessorKey: 'relationship',
        cell: (info) =>
          t(`customers.relationship.${info.getValue<CustomerRelationship>()}`),
      },
      {
        id: 'actions',
        header: () => t('customers.list.columns.actions'),
        enableSorting: false,
        cell: (info) => (
          <button
            type="button"
            data-testid={`row-open-${info.row.original.id}`}
            onClick={(e) => {
              e.stopPropagation();
              const cust = info.row.original;
              if (onRowActivate) onRowActivate(cust);
              else navigate(`/command/customers/${cust.id}`);
            }}
            className="rounded border border-slate-300 px-2 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {t('customers.list.actions.open')}
          </button>
        ),
      },
    ],
    [t, i18n.language, onRowActivate, navigate],
  );

  const table = useReactTable({
    data: customers as CustomerSummary[],
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  function activate(cust: CustomerSummary) {
    if (onRowActivate) onRowActivate(cust);
    else navigate(`/command/customers/${cust.id}`);
  }

  return (
    <section
      data-testid="customer-list"
      role="region"
      aria-label={t('customers.list.regionLabel')}
      className="rounded-lg border border-slate-200 bg-white shadow-sm"
    >
      <header className="flex flex-wrap items-center gap-3 border-b border-slate-200 p-3">
        <label className="flex flex-col text-xs font-medium text-slate-700">
          <span>{t('customers.list.search.label')}</span>
          <input
            type="search"
            data-testid="customer-search"
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder={t('customers.list.search.placeholder')}
            aria-label={t('customers.list.search.label')}
            className="mt-1 rounded border border-slate-300 px-2 py-1 text-sm"
          />
        </label>
        <div className="ml-auto">
          <button
            type="button"
            data-testid="customer-create-open"
            onClick={onCreateClick}
            className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {t('customers.list.createButton')}
          </button>
        </div>
      </header>

      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm" data-testid="customer-table">
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
                      className="border-b border-slate-200 px-3 py-2 text-left font-semibold text-slate-700"
                    >
                      {canSort ? (
                        <button
                          type="button"
                          data-testid={`sort-${header.column.id}`}
                          onClick={header.column.getToggleSortingHandler()}
                          className="flex items-center gap-1 focus:outline-none focus:ring-2 focus:ring-blue-500"
                        >
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          <span aria-hidden="true">
                            {sortDir === 'asc' ? '▲' : sortDir === 'desc' ? '▼' : '↕'}
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
                  colSpan={columns.length}
                  data-testid="customer-list-empty"
                  className="px-3 py-6 text-center text-sm text-slate-500"
                >
                  {t('customers.list.empty')}
                </td>
              </tr>
            ) : (
              table.getRowModel().rows.map((row) => {
                const cust = row.original;
                return (
                  <tr
                    key={row.id}
                    role="link"
                    tabIndex={0}
                    data-testid={`customer-row-${cust.id}`}
                    aria-label={t('customers.list.rowAriaLabel', {
                      name: cust.name,
                      status: t(`customers.status.${cust.status}`),
                    })}
                    onClick={() => activate(cust)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        activate(cust);
                      }
                    }}
                    className="cursor-pointer border-b border-slate-100 hover:bg-slate-50 focus:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-blue-500"
                  >
                    {row.getVisibleCells().map((cell) => (
                      <td key={cell.id} className="px-3 py-2 align-middle">
                        {flexRender(cell.column.columnDef.cell, cell.getContext())}
                      </td>
                    ))}
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}
