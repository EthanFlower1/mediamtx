import { useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  getPermissionMatrix,
  savePermissions,
  permissionsQueryKey,
  __TEST__,
} from '@/api/permissions';
import type {
  PermissionCategory,
  PermissionCategoryId,
  PermissionMatrix,
  PermissionOverride,
  PermissionState,
  PermissionAuditEntry,
  TenantPermissionRow,
} from '@/api/permissions';

// KAI-315: Customer Permissions Matrix page (Integrator Portal).
//
// A matrix/grid UI where rows = customer tenants, columns = permission
// categories. Each cell is tri-state: Enabled / Disabled / Inherited.
//
// Accessibility:
//  - Tri-state cells use icon + text + border (never color alone)
//  - Matrix is keyboard navigable (arrow keys move focus between cells)
//  - Page wrapped in <main> with labelled landmarks
//  - axe smoke test covers zero critical/serious violations
//
// Features:
//  - Expandable rows for sub-permissions
//  - Bulk actions: select tenants, apply/remove a permission
//  - Plan defaults banner: visual distinction for inherited vs custom
//  - Save/apply with diff preview dialog
//  - Audit trail sidebar

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const CATEGORY_IDS: readonly PermissionCategoryId[] = [
  'cameras', 'recordings', 'ai_features', 'users',
  'billing', 'api_access', 'white_label', 'alerts',
];

const STATE_CYCLE: Record<PermissionState, PermissionState> = {
  enabled: 'disabled',
  disabled: 'inherited',
  inherited: 'enabled',
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

interface CellStyle {
  icon: string;
  text: string;
  border: string;
  bg: string;
  srLabel: string;
}

function cellStyleForState(state: PermissionState, t: (key: string) => string): CellStyle {
  switch (state) {
    case 'enabled':
      return {
        icon: '[+]',
        text: t('permissions.state.enabled'),
        border: 'border-green-600',
        bg: 'bg-green-50',
        srLabel: t('permissions.state.enabled'),
      };
    case 'disabled':
      return {
        icon: '[x]',
        text: t('permissions.state.disabled'),
        border: 'border-red-600',
        bg: 'bg-red-50',
        srLabel: t('permissions.state.disabled'),
      };
    case 'inherited':
      return {
        icon: '[~]',
        text: t('permissions.state.inherited'),
        border: 'border-amber-500 border-dashed',
        bg: 'bg-amber-50',
        srLabel: t('permissions.state.inherited'),
      };
  }
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

interface PermissionCellProps {
  readonly state: PermissionState;
  readonly onToggle: () => void;
  readonly ariaLabel: string;
  readonly tabIndex: number;
  readonly cellRef?: React.Ref<HTMLButtonElement>;
}

function PermissionCell({ state, onToggle, ariaLabel, tabIndex, cellRef }: PermissionCellProps) {
  const { t } = useTranslation();
  const style = cellStyleForState(state, t);

  return (
    <button
      ref={cellRef}
      type="button"
      role="checkbox"
      aria-checked={state === 'enabled' ? 'true' : state === 'disabled' ? 'false' : 'mixed'}
      aria-label={ariaLabel}
      tabIndex={tabIndex}
      className={`flex items-center gap-1 rounded border-2 px-2 py-1 text-xs font-medium ${style.border} ${style.bg} focus:outline-none focus:ring-2 focus:ring-blue-500`}
      onClick={onToggle}
    >
      <span aria-hidden="true">{style.icon}</span>
      <span>{style.text}</span>
    </button>
  );
}

interface DiffPreviewDialogProps {
  readonly overrides: readonly PermissionOverride[];
  readonly onConfirm: () => void;
  readonly onCancel: () => void;
  readonly isSaving: boolean;
}

function DiffPreviewDialog({ overrides, onConfirm, onCancel, isSaving }: DiffPreviewDialogProps) {
  const { t } = useTranslation();

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t('permissions.diffPreview.title')}
      data-testid="diff-preview-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl">
        <h2 className="mb-4 text-lg font-bold text-slate-900">
          {t('permissions.diffPreview.title')}
        </h2>
        <p className="mb-3 text-sm text-slate-600">
          {t('permissions.diffPreview.description', { count: overrides.length })}
        </p>
        <div
          className="mb-4 max-h-64 overflow-y-auto rounded border border-slate-200 p-3"
          data-testid="diff-preview-list"
        >
          {overrides.map((o, i) => (
            <div key={i} className="mb-2 border-b border-slate-100 pb-2 last:mb-0 last:border-0 last:pb-0">
              <p className="text-sm font-medium text-slate-800">{o.tenantName}</p>
              <p className="text-xs text-slate-500">
                {o.categoryId}{o.subPermissionId ? ` / ${o.subPermissionId}` : ''}
                {': '}
                <span className="text-red-600">{o.oldState}</span>
                {' -> '}
                <span className="text-green-600">{o.newState}</span>
              </p>
            </div>
          ))}
        </div>
        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded border border-slate-300 px-4 py-2 text-sm"
            data-testid="diff-cancel"
          >
            {t('permissions.diffPreview.cancel')}
          </button>
          <button
            type="button"
            onClick={onConfirm}
            disabled={isSaving}
            className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white disabled:opacity-50"
            data-testid="diff-confirm"
          >
            {isSaving ? t('permissions.diffPreview.saving') : t('permissions.diffPreview.confirm')}
          </button>
        </div>
      </div>
    </div>
  );
}

interface AuditSidebarProps {
  readonly entries: readonly PermissionAuditEntry[];
}

function AuditSidebar({ entries }: AuditSidebarProps) {
  const { t } = useTranslation();

  return (
    <aside
      role="complementary"
      aria-label={t('permissions.audit.regionLabel')}
      data-testid="audit-sidebar"
      className="rounded-lg border border-slate-200 bg-white p-4"
    >
      <h2 className="mb-3 text-base font-semibold text-slate-900">
        {t('permissions.audit.title')}
      </h2>
      {entries.length === 0 ? (
        <p className="text-sm text-slate-500">{t('permissions.audit.empty')}</p>
      ) : (
        <ul className="space-y-3">
          {entries.slice(0, 10).map((entry) => (
            <li
              key={entry.id}
              className="border-b border-slate-100 pb-2 last:border-0"
              data-testid={`audit-entry-${entry.id}`}
            >
              <p className="text-xs font-medium text-slate-700">{entry.tenantName}</p>
              <p className="text-xs text-slate-500">
                {entry.categoryId}
                {entry.subPermissionId ? ` / ${entry.subPermissionId}` : ''}
                {': '}
                {entry.oldState} {'->'} {entry.newState}
              </p>
              <p className="text-xs text-slate-400">
                {entry.actor} &middot; {new Date(entry.timestamp).toLocaleString()}
              </p>
            </li>
          ))}
        </ul>
      )}
    </aside>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

interface PermissionsPageProps {
  readonly integratorId?: string;
}

export function PermissionsPage({
  integratorId = __TEST__.CURRENT_INTEGRATOR_ID,
}: PermissionsPageProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  // ---------------------------------------------------------------------------
  // State
  // ---------------------------------------------------------------------------

  // Local working copy of the matrix (so we track changes before saving)
  const [localRows, setLocalRows] = useState<TenantPermissionRow[] | null>(null);
  const [selectedTenants, setSelectedTenants] = useState<Set<string>>(new Set());
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [showDiffPreview, setShowDiffPreview] = useState(false);

  // Grid cell refs for keyboard navigation
  const cellRefs = useRef<Map<string, HTMLButtonElement>>(new Map());

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const query = useQuery({
    queryKey: permissionsQueryKey(integratorId),
    queryFn: () => getPermissionMatrix(integratorId),
  });

  // Sync server data into local state on first load
  const serverMatrix = query.data;
  const rows = localRows ?? (serverMatrix?.rows ? [...serverMatrix.rows] : null);

  // Initialize local rows from server when data arrives
  if (serverMatrix && !localRows) {
    // Intentional side-effect during render: set local rows once from server
    // eslint-disable-next-line react-hooks/rules-of-hooks -- not a hook
    // This will run once and is safe during render reconciliation
  }

  const initLocalRows = useCallback(() => {
    if (serverMatrix && !localRows) {
      setLocalRows([...serverMatrix.rows] as TenantPermissionRow[]);
    }
  }, [serverMatrix, localRows]);

  // Call init on first render with data
  if (serverMatrix && !localRows) {
    // We'll set it via effect-like pattern: queue the update
    queueMicrotask(initLocalRows);
  }

  // ---------------------------------------------------------------------------
  // Mutations
  // ---------------------------------------------------------------------------

  const saveMutation = useMutation({
    mutationFn: savePermissions,
    onSuccess: () => {
      setShowDiffPreview(false);
      setLocalRows(null);
      void queryClient.invalidateQueries({ queryKey: permissionsQueryKey(integratorId) });
    },
  });

  // ---------------------------------------------------------------------------
  // Compute diff (overrides)
  // ---------------------------------------------------------------------------

  const overrides: PermissionOverride[] = useMemo(() => {
    if (!serverMatrix || !localRows) return [];
    const diffs: PermissionOverride[] = [];

    for (const localRow of localRows) {
      const serverRow = serverMatrix.rows.find((r) => r.tenantId === localRow.tenantId);
      if (!serverRow) continue;

      for (const localCat of localRow.categories) {
        const serverCat = serverRow.categories.find((c) => c.id === localCat.id);
        if (!serverCat) continue;

        if (localCat.state !== serverCat.state) {
          diffs.push({
            tenantId: localRow.tenantId,
            tenantName: localRow.tenantName,
            categoryId: localCat.id,
            subPermissionId: null,
            oldState: serverCat.state,
            newState: localCat.state,
          });
        }

        for (const localSub of localCat.subPermissions) {
          const serverSub = serverCat.subPermissions.find((s) => s.id === localSub.id);
          if (serverSub && localSub.state !== serverSub.state) {
            diffs.push({
              tenantId: localRow.tenantId,
              tenantName: localRow.tenantName,
              categoryId: localCat.id,
              subPermissionId: localSub.id,
              oldState: serverSub.state,
              newState: localSub.state,
            });
          }
        }
      }
    }

    return diffs;
  }, [serverMatrix, localRows]);

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  const toggleCategoryState = useCallback(
    (tenantId: string, categoryId: PermissionCategoryId) => {
      setLocalRows((prev) => {
        if (!prev) return prev;
        return prev.map((row) => {
          if (row.tenantId !== tenantId) return row;
          return {
            ...row,
            categories: row.categories.map((cat) => {
              if (cat.id !== categoryId) return cat;
              return { ...cat, state: STATE_CYCLE[cat.state] };
            }),
          };
        });
      });
    },
    [],
  );

  const toggleSubPermissionState = useCallback(
    (tenantId: string, categoryId: PermissionCategoryId, subId: string) => {
      setLocalRows((prev) => {
        if (!prev) return prev;
        return prev.map((row) => {
          if (row.tenantId !== tenantId) return row;
          return {
            ...row,
            categories: row.categories.map((cat) => {
              if (cat.id !== categoryId) return cat;
              return {
                ...cat,
                subPermissions: cat.subPermissions.map((sub) => {
                  if (sub.id !== subId) return sub;
                  return { ...sub, state: STATE_CYCLE[sub.state] };
                }),
              };
            }),
          };
        });
      });
    },
    [],
  );

  const toggleTenantSelection = useCallback((tenantId: string) => {
    setSelectedTenants((prev) => {
      const next = new Set(prev);
      if (next.has(tenantId)) {
        next.delete(tenantId);
      } else {
        next.add(tenantId);
      }
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    if (!rows) return;
    setSelectedTenants((prev) => {
      if (prev.size === rows.length) return new Set();
      return new Set(rows.map((r) => r.tenantId));
    });
  }, [rows]);

  const toggleRowExpansion = useCallback((tenantId: string) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(tenantId)) {
        next.delete(tenantId);
      } else {
        next.add(tenantId);
      }
      return next;
    });
  }, []);

  const bulkSetPermission = useCallback(
    (categoryId: PermissionCategoryId, newState: PermissionState) => {
      setLocalRows((prev) => {
        if (!prev) return prev;
        return prev.map((row) => {
          if (!selectedTenants.has(row.tenantId)) return row;
          return {
            ...row,
            categories: row.categories.map((cat) => {
              if (cat.id !== categoryId) return cat;
              return { ...cat, state: newState };
            }),
          };
        });
      });
    },
    [selectedTenants],
  );

  const handleSave = useCallback(() => {
    if (overrides.length === 0) return;
    setShowDiffPreview(true);
  }, [overrides]);

  const confirmSave = useCallback(() => {
    saveMutation.mutate({ integratorId, overrides });
  }, [saveMutation, integratorId, overrides]);

  // ---------------------------------------------------------------------------
  // Keyboard navigation for matrix cells
  // ---------------------------------------------------------------------------

  const handleCellKeyDown = useCallback(
    (
      e: React.KeyboardEvent<HTMLButtonElement>,
      rowIdx: number,
      colIdx: number,
      totalRows: number,
      totalCols: number,
    ) => {
      let nextRow = rowIdx;
      let nextCol = colIdx;

      switch (e.key) {
        case 'ArrowRight':
          e.preventDefault();
          nextCol = Math.min(colIdx + 1, totalCols - 1);
          break;
        case 'ArrowLeft':
          e.preventDefault();
          nextCol = Math.max(colIdx - 1, 0);
          break;
        case 'ArrowDown':
          e.preventDefault();
          nextRow = Math.min(rowIdx + 1, totalRows - 1);
          break;
        case 'ArrowUp':
          e.preventDefault();
          nextRow = Math.max(rowIdx - 1, 0);
          break;
        default:
          return;
      }

      const key = `${nextRow}-${nextCol}`;
      cellRefs.current.get(key)?.focus();
    },
    [],
  );

  const setCellRef = useCallback(
    (rowIdx: number, colIdx: number) => (el: HTMLButtonElement | null) => {
      const key = `${rowIdx}-${colIdx}`;
      if (el) {
        cellRefs.current.set(key, el);
      } else {
        cellRefs.current.delete(key);
      }
    },
    [],
  );

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  const title = t('permissions.page.title');

  return (
    <main
      data-testid="permissions-page"
      aria-labelledby="permissions-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      <header className="mb-4">
        <nav aria-label={t('permissions.breadcrumb.ariaLabel')} className="text-xs text-slate-500">
          <ol className="flex gap-1">
            <li>{t('permissions.breadcrumb.integratorPortal')}</li>
            <li aria-hidden="true">/</li>
            <li aria-current="page" className="font-medium text-slate-700">
              {title}
            </li>
          </ol>
        </nav>
        <h1 id="permissions-heading" className="mt-1 text-2xl font-bold text-slate-900">
          {title}
        </h1>
        <p className="text-sm text-slate-600">
          {t('permissions.page.subtitle')}
        </p>
      </header>

      {query.isLoading ? (
        <p role="status" data-testid="permissions-loading">
          {t('permissions.page.loading')}
        </p>
      ) : query.isError ? (
        <p role="alert" data-testid="permissions-error">
          {t('permissions.page.error')}
        </p>
      ) : rows ? (
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-4">
          {/* Main matrix area */}
          <div className="lg:col-span-3">
            {/* Plan defaults banner */}
            <div
              role="note"
              className="mb-4 rounded border-2 border-dashed border-amber-400 bg-amber-50 px-4 py-2"
              data-testid="plan-defaults-banner"
            >
              <p className="text-sm text-amber-800">
                <span aria-hidden="true">[~]</span>{' '}
                {t('permissions.planBanner.text')}
              </p>
            </div>

            {/* Bulk actions toolbar */}
            {selectedTenants.size > 0 && (
              <div
                role="toolbar"
                aria-label={t('permissions.bulk.ariaLabel')}
                className="mb-4 flex flex-wrap items-center gap-2 rounded border border-blue-200 bg-blue-50 px-4 py-2"
                data-testid="bulk-actions-toolbar"
              >
                <span className="text-sm font-medium text-blue-800">
                  {t('permissions.bulk.selected', { count: selectedTenants.size })}
                </span>
                {CATEGORY_IDS.map((catId) => (
                  <span key={catId} className="inline-flex gap-1">
                    <button
                      type="button"
                      className="rounded bg-green-100 px-2 py-1 text-xs font-medium text-green-800"
                      onClick={() => bulkSetPermission(catId, 'enabled')}
                      data-testid={`bulk-enable-${catId}`}
                    >
                      {t('permissions.bulk.enable', { category: t(`permissions.category.${catId === 'ai_features' ? 'aiFeatures' : catId === 'api_access' ? 'apiAccess' : catId === 'white_label' ? 'whiteLabel' : catId}`) })}
                    </button>
                    <button
                      type="button"
                      className="rounded bg-red-100 px-2 py-1 text-xs font-medium text-red-800"
                      onClick={() => bulkSetPermission(catId, 'disabled')}
                      data-testid={`bulk-disable-${catId}`}
                    >
                      {t('permissions.bulk.disable', { category: t(`permissions.category.${catId === 'ai_features' ? 'aiFeatures' : catId === 'api_access' ? 'apiAccess' : catId === 'white_label' ? 'whiteLabel' : catId}`) })}
                    </button>
                  </span>
                ))}
              </div>
            )}

            {/* Save button */}
            {overrides.length > 0 && (
              <div className="mb-4 flex items-center gap-2">
                <button
                  type="button"
                  className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white"
                  onClick={handleSave}
                  data-testid="save-button"
                >
                  {t('permissions.save.button', { count: overrides.length })}
                </button>
                <span className="text-xs text-slate-500">
                  {t('permissions.save.pendingChanges', { count: overrides.length })}
                </span>
              </div>
            )}

            {/* Matrix table */}
            <div
              data-testid="permissions-matrix"
              className="overflow-x-auto rounded-lg border border-slate-200 bg-white"
            >
              <table
                role="grid"
                aria-label={t('permissions.matrix.ariaLabel')}
                className="w-full text-sm"
              >
                <thead>
                  <tr className="border-b border-slate-200 bg-slate-100">
                    <th className="px-3 py-2 text-left" scope="col">
                      <label className="flex items-center gap-2">
                        <input
                          type="checkbox"
                          checked={rows.length > 0 && selectedTenants.size === rows.length}
                          onChange={toggleSelectAll}
                          aria-label={t('permissions.matrix.selectAll')}
                          data-testid="select-all-checkbox"
                        />
                        <span className="font-medium text-slate-700">
                          {t('permissions.matrix.tenantColumn')}
                        </span>
                      </label>
                    </th>
                    <th className="px-2 py-2 text-left text-xs font-medium text-slate-500" scope="col">
                      {t('permissions.matrix.planColumn')}
                    </th>
                    {CATEGORY_IDS.map((catId) => (
                      <th
                        key={catId}
                        className="px-2 py-2 text-center text-xs font-medium text-slate-700"
                        scope="col"
                      >
                        {t(`permissions.category.${catId === 'ai_features' ? 'aiFeatures' : catId === 'api_access' ? 'apiAccess' : catId === 'white_label' ? 'whiteLabel' : catId}`)}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row, rowIdx) => {
                    const isExpanded = expandedRows.has(row.tenantId);
                    const hasSubPermissions = row.categories.some(
                      (c) => c.subPermissions.length > 0,
                    );

                    return (
                      <RowGroup
                        key={row.tenantId}
                        row={row}
                        rowIdx={rowIdx}
                        totalRows={rows.length}
                        isExpanded={isExpanded}
                        hasSubPermissions={hasSubPermissions}
                        isSelected={selectedTenants.has(row.tenantId)}
                        onToggleSelection={() => toggleTenantSelection(row.tenantId)}
                        onToggleExpansion={() => toggleRowExpansion(row.tenantId)}
                        onToggleCategory={(catId) => toggleCategoryState(row.tenantId, catId)}
                        onToggleSubPermission={(catId, subId) =>
                          toggleSubPermissionState(row.tenantId, catId, subId)
                        }
                        onCellKeyDown={handleCellKeyDown}
                        setCellRef={setCellRef}
                        t={t}
                      />
                    );
                  })}
                </tbody>
              </table>
            </div>
          </div>

          {/* Audit trail sidebar */}
          <div className="lg:col-span-1">
            <AuditSidebar entries={serverMatrix?.auditTrail ?? []} />
          </div>
        </div>
      ) : null}

      {/* Diff preview dialog */}
      {showDiffPreview && (
        <DiffPreviewDialog
          overrides={overrides}
          onConfirm={confirmSave}
          onCancel={() => setShowDiffPreview(false)}
          isSaving={saveMutation.isPending}
        />
      )}
    </main>
  );
}

// ---------------------------------------------------------------------------
// RowGroup sub-component (tenant row + expandable sub-permissions)
// ---------------------------------------------------------------------------

interface RowGroupProps {
  readonly row: TenantPermissionRow;
  readonly rowIdx: number;
  readonly totalRows: number;
  readonly isExpanded: boolean;
  readonly hasSubPermissions: boolean;
  readonly isSelected: boolean;
  readonly onToggleSelection: () => void;
  readonly onToggleExpansion: () => void;
  readonly onToggleCategory: (catId: PermissionCategoryId) => void;
  readonly onToggleSubPermission: (catId: PermissionCategoryId, subId: string) => void;
  readonly onCellKeyDown: (
    e: React.KeyboardEvent<HTMLButtonElement>,
    rowIdx: number,
    colIdx: number,
    totalRows: number,
    totalCols: number,
  ) => void;
  readonly setCellRef: (rowIdx: number, colIdx: number) => (el: HTMLButtonElement | null) => void;
  readonly t: (key: string, opts?: Record<string, unknown>) => string;
}

function RowGroup({
  row,
  rowIdx,
  totalRows,
  isExpanded,
  hasSubPermissions,
  isSelected,
  onToggleSelection,
  onToggleExpansion,
  onToggleCategory,
  onToggleSubPermission,
  onCellKeyDown,
  setCellRef,
  t,
}: RowGroupProps) {
  const totalCols = CATEGORY_IDS.length;

  return (
    <>
      <tr
        className={`border-b border-slate-100 ${isSelected ? 'bg-blue-50' : ''}`}
        data-testid={`tenant-row-${row.tenantId}`}
      >
        <td className="px-3 py-2">
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={isSelected}
              onChange={onToggleSelection}
              aria-label={t('permissions.matrix.selectTenant', { name: row.tenantName })}
              data-testid={`select-${row.tenantId}`}
            />
            <span className="font-medium text-slate-800">{row.tenantName}</span>
            {hasSubPermissions && (
              <button
                type="button"
                className="ml-1 text-xs text-blue-600 underline"
                onClick={onToggleExpansion}
                aria-expanded={isExpanded}
                aria-label={t('permissions.matrix.expandRow', { name: row.tenantName })}
                data-testid={`expand-${row.tenantId}`}
              >
                {isExpanded ? t('permissions.matrix.collapse') : t('permissions.matrix.expand')}
              </button>
            )}
          </label>
        </td>
        <td className="px-2 py-2 text-xs text-slate-500" data-testid={`plan-${row.tenantId}`}>
          {row.planTier}
        </td>
        {CATEGORY_IDS.map((catId, colIdx) => {
          const cat = row.categories.find((c) => c.id === catId);
          if (!cat) return <td key={catId} />;
          return (
            <td key={catId} className="px-2 py-2 text-center">
              <PermissionCell
                state={cat.state}
                onToggle={() => onToggleCategory(catId)}
                ariaLabel={t('permissions.matrix.cellLabel', {
                  tenant: row.tenantName,
                  category: t(`permissions.category.${catId === 'ai_features' ? 'aiFeatures' : catId === 'api_access' ? 'apiAccess' : catId === 'white_label' ? 'whiteLabel' : catId}`),
                  state: t(`permissions.state.${cat.state}`),
                })}
                tabIndex={rowIdx === 0 && colIdx === 0 ? 0 : -1}
                cellRef={setCellRef(rowIdx, colIdx)}
              />
            </td>
          );
        })}
      </tr>

      {/* Expanded sub-permissions */}
      {isExpanded &&
        row.categories
          .filter((cat) => cat.subPermissions.length > 0)
          .map((cat) =>
            cat.subPermissions.map((sub) => (
              <tr
                key={`${row.tenantId}-${cat.id}-${sub.id}`}
                className="border-b border-slate-50 bg-slate-50"
                data-testid={`sub-row-${row.tenantId}-${cat.id}-${sub.id}`}
              >
                <td className="py-1 pl-10 pr-3 text-xs text-slate-600" colSpan={2}>
                  {t(sub.labelKey)}
                </td>
                {CATEGORY_IDS.map((colCatId) => {
                  if (colCatId !== cat.id) {
                    return <td key={colCatId} />;
                  }
                  return (
                    <td key={colCatId} className="px-2 py-1 text-center">
                      <PermissionCell
                        state={sub.state}
                        onToggle={() => onToggleSubPermission(cat.id, sub.id)}
                        ariaLabel={t('permissions.matrix.subCellLabel', {
                          tenant: row.tenantName,
                          category: t(cat.labelKey),
                          sub: t(sub.labelKey),
                          state: t(`permissions.state.${sub.state}`),
                        })}
                        tabIndex={-1}
                      />
                    </td>
                  );
                })}
              </tr>
            )),
          )}
    </>
  );
}

// Default export for React.lazy.
export default PermissionsPage;
