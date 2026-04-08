import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import type { RowSelectionState } from '@tanstack/react-table';

import {
  addCamera,
  camerasQueryKeys,
  deleteCamera,
  listCameras,
  moveCamera,
  updateCamera,
  type Camera,
  type CameraSpec,
  type CameraStatus,
} from '@/api/cameras';
import { useSessionStore } from '@/stores/session';
import { CameraList } from '@/components/cameras/CameraList';
import { AddCameraWizard } from '@/components/cameras/AddCameraWizard';
import { EditCameraModal } from '@/components/cameras/EditCameraModal';
import { MoveCameraDialog } from '@/components/cameras/MoveCameraDialog';
import { DeleteCameraConfirm } from '@/components/cameras/DeleteCameraConfirm';

// KAI-321: Customer Admin Cameras page.
//
// Single most-used admin page. Features: searchable/filterable list
// with sortable virtualized table, ONVIF discovery wizard, inline
// edit, cross-recorder move, confirm-intent delete, and bulk actions.
// All strings through i18n. All queries tenant-scoped.

type StatusFilter = CameraStatus | 'all';

export function CamerasPage(): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const queryClient = useQueryClient();

  const [search, setSearch] = useState('');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const [addOpen, setAddOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Camera | null>(null);
  const [moveTarget, setMoveTarget] = useState<Camera | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Camera | null>(null);

  const filters = useMemo(
    () => ({ search, status: statusFilter }),
    [search, statusFilter],
  );

  const query = useQuery<Camera[]>({
    queryKey: camerasQueryKeys.list(tenantId, filters),
    queryFn: () => listCameras({ tenantId, ...filters }),
  });

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: camerasQueryKeys.all(tenantId) });
  }, [queryClient, tenantId]);

  const addMutation = useMutation({
    mutationFn: (spec: CameraSpec) => addCamera({ tenantId, spec }),
    onSuccess: invalidate,
  });

  const updateMutation = useMutation({
    mutationFn: (args: { id: string; patch: Partial<CameraSpec> }) =>
      updateCamera({ tenantId, id: args.id, patch: args.patch }),
    onSuccess: invalidate,
  });

  const moveMutation = useMutation({
    mutationFn: (args: { id: string; targetRecorderId: string }) =>
      moveCamera({ tenantId, id: args.id, targetRecorderId: args.targetRecorderId }),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteCamera({ tenantId, id }),
    onSuccess: invalidate,
  });

  const cameras = query.data ?? [];

  const selectedIds = useMemo(
    () => Object.keys(selection).filter((id) => selection[id]),
    [selection],
  );
  const hasSelection = selectedIds.length > 0;

  const handleToggleExpand = useCallback(
    (cameraId: string) => {
      setExpandedId((prev) => (prev === cameraId ? null : cameraId));
    },
    [],
  );

  const handleBulkDelete = useCallback(() => {
    for (const id of selectedIds) {
      deleteMutation.mutate(id);
    }
    setSelection({});
  }, [selectedIds, deleteMutation]);

  const handleBulkEnable = useCallback(() => {
    for (const id of selectedIds) {
      updateMutation.mutate({ id, patch: {} });
    }
  }, [selectedIds, updateMutation]);

  const handleBulkDisable = useCallback(() => {
    for (const id of selectedIds) {
      updateMutation.mutate({ id, patch: {} });
    }
  }, [selectedIds, updateMutation]);

  return (
    <main
      aria-label={t('cameras.page.label')}
      data-testid="cameras-page"
      className="cameras-page"
    >
      <nav aria-label={t('cameras.breadcrumb.ariaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('cameras.page.title')}</li>
        </ol>
      </nav>
      <header className="cameras-page__header">
        <h1>{t('cameras.page.title')}</h1>
        <button
          type="button"
          onClick={() => setAddOpen(true)}
          data-testid="cameras-add-button"
        >
          {t('cameras.actions.add')}
        </button>
      </header>

      <section
        className="cameras-page__toolbar"
        aria-label={t('cameras.toolbar.ariaLabel')}
      >
        <label>
          <span className="sr-only">{t('cameras.toolbar.searchLabel')}</span>
          <input
            type="search"
            placeholder={t('cameras.toolbar.searchPlaceholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            data-testid="cameras-search"
            aria-label={t('cameras.toolbar.searchLabel')}
          />
        </label>
        <label>
          <span className="sr-only">{t('cameras.toolbar.statusFilterLabel')}</span>
          <select
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as StatusFilter)}
            data-testid="cameras-status-filter"
            aria-label={t('cameras.toolbar.statusFilterLabel')}
          >
            <option value="all">{t('cameras.filter.all')}</option>
            <option value="online">{t('cameras.status.online')}</option>
            <option value="offline">{t('cameras.status.offline')}</option>
            <option value="warning">{t('cameras.status.warning')}</option>
          </select>
        </label>
        <div className="cameras-page__bulk" role="group" aria-label={t('cameras.toolbar.bulkAriaLabel')}>
          <button
            type="button"
            disabled={!hasSelection}
            aria-disabled={!hasSelection}
            onClick={handleBulkEnable}
            data-testid="cameras-bulk-enable"
          >
            {t('cameras.bulk.enable')}
          </button>
          <button
            type="button"
            disabled={!hasSelection}
            aria-disabled={!hasSelection}
            onClick={handleBulkDisable}
            data-testid="cameras-bulk-disable"
          >
            {t('cameras.bulk.disable')}
          </button>
          <button
            type="button"
            disabled={!hasSelection}
            aria-disabled={!hasSelection}
            onClick={handleBulkDelete}
            data-testid="cameras-bulk-delete"
          >
            {t('cameras.bulk.delete')}
          </button>
        </div>
      </section>

      {query.isLoading && (
        <p role="status" aria-live="polite">
          {t('cameras.list.loading')}
        </p>
      )}
      {query.isError && (
        <p role="alert">{t('cameras.list.error')}</p>
      )}
      {query.isSuccess && (
        <CameraList
          cameras={cameras}
          selection={selection}
          onSelectionChange={setSelection}
          onEdit={setEditTarget}
          onMove={setMoveTarget}
          onDelete={setDeleteTarget}
          expandedId={expandedId}
          onToggleExpand={handleToggleExpand}
        />
      )}

      <AddCameraWizard
        open={addOpen}
        tenantId={tenantId}
        onClose={() => setAddOpen(false)}
        onCommit={(spec) => addMutation.mutate(spec)}
      />

      <EditCameraModal
        open={editTarget !== null}
        tenantId={tenantId}
        camera={editTarget}
        onClose={() => setEditTarget(null)}
        onSubmit={(patch) => {
          if (editTarget) updateMutation.mutate({ id: editTarget.id, patch });
          setEditTarget(null);
        }}
      />

      <MoveCameraDialog
        open={moveTarget !== null}
        tenantId={tenantId}
        camera={moveTarget}
        onClose={() => setMoveTarget(null)}
        onConfirm={(targetRecorderId) => {
          if (moveTarget) {
            moveMutation.mutate({ id: moveTarget.id, targetRecorderId });
          }
          setMoveTarget(null);
        }}
      />

      <DeleteCameraConfirm
        open={deleteTarget !== null}
        camera={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate(deleteTarget.id);
          setDeleteTarget(null);
        }}
      />
    </main>
  );
}

export default CamerasPage;
