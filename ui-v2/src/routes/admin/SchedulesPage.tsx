// KAI-326: Customer Admin Recording Schedules + Retention page.
//
// Sub-surfaces:
//   1. Schedule Templates list — table with CRUD
//   2. Create/Edit Schedule modal — type selector, day/time picker, retention, camera assignment
//   3. Retention Overview card — per-tier storage usage
//   4. Bulk camera-to-schedule assignment
//   5. Visual weekly timeline — CSS grid, no canvas
//
// All strings through react-i18next. All queries tenant-scoped.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

import {
  schedulesQueryKeys,
  type CreateScheduleArgs,
  type RecordingSchedule,
  type RetentionOverview,
  type RetentionTier,
  type ScheduleCamera,
  type ScheduleType,
  type WeeklyTimeRange,
} from '@/api/schedules';
import {
  bulkAssignSchedule,
  createSchedule,
  deleteSchedule,
  listRetentionOverview,
  listScheduleCameras,
  listSchedules,
  updateSchedule,
} from '@/api/schedules.mock';
import { useSessionStore } from '@/stores/session';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DAYS = [0, 1, 2, 3, 4, 5, 6] as const;
const RETENTION_TIERS: RetentionTier[] = ['7d', '30d', '90d', '1yr', 'forensic'];
const SCHEDULE_TYPES: ScheduleType[] = ['continuous', 'motion', 'scheduled'];

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${units[i]}`;
}

// ---------------------------------------------------------------------------
// Weekly Timeline component (CSS grid, no canvas)
// ---------------------------------------------------------------------------

interface WeeklyTimelineProps {
  timeRanges: WeeklyTimeRange[];
  type: ScheduleType;
  ariaLabel: string;
}

function WeeklyTimeline({ timeRanges, type, ariaLabel }: WeeklyTimelineProps): JSX.Element {
  const { t } = useTranslation();

  function timeToPercent(time: string): number {
    const [h, m] = time.split(':').map(Number);
    return ((h! * 60 + m!) / 1440) * 100;
  }

  return (
    <div
      role="img"
      aria-label={ariaLabel}
      data-testid="weekly-timeline"
      className="weekly-timeline"
      style={{ display: 'grid', gridTemplateColumns: 'auto 1fr', gap: '2px', fontSize: '0.75rem' }}
    >
      {DAYS.map((day) => {
        const dayRanges = type === 'continuous'
          ? [{ day, startTime: '00:00', endTime: '23:59' }]
          : timeRanges.filter((r) => r.day === day);

        return (
          <div key={day} style={{ display: 'contents' }}>
            <span data-testid={`timeline-day-${day}`} style={{ padding: '2px 4px', textAlign: 'right' }}>
              {t(`schedules.timeline.day.${day}`)}
            </span>
            <div
              style={{
                position: 'relative',
                height: '16px',
                background: '#e5e7eb',
                borderRadius: '2px',
              }}
            >
              {dayRanges.map((range, i) => {
                const left = timeToPercent(range.startTime);
                const right = timeToPercent(range.endTime);
                const width = Math.max(right - left, 0.5);
                return (
                  <div
                    key={i}
                    style={{
                      position: 'absolute',
                      left: `${left}%`,
                      width: `${width}%`,
                      height: '100%',
                      background: type === 'motion' ? '#f59e0b' : '#3b82f6',
                      borderRadius: '2px',
                    }}
                  />
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Retention Overview Card
// ---------------------------------------------------------------------------

interface RetentionCardProps {
  overview: RetentionOverview[];
}

function RetentionCard({ overview }: RetentionCardProps): JSX.Element {
  const { t } = useTranslation();

  return (
    <section
      aria-label={t('schedules.overview.heading')}
      data-testid="retention-overview"
      className="retention-overview"
    >
      <h2>{t('schedules.overview.heading')}</h2>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))', gap: '12px' }}>
        {overview.map((item) => (
          <div
            key={item.tier}
            data-testid={`retention-tier-${item.tier}`}
            style={{ border: '1px solid #d1d5db', borderRadius: '8px', padding: '12px' }}
          >
            <h3>{t(`schedules.retention.${item.tier}`)}</h3>
            <p>
              {t('schedules.overview.storage')}: {formatBytes(item.storageUsedBytes)} / {formatBytes(item.storageTotalBytes)}
            </p>
            <p>
              {t('schedules.overview.cameras')}: {item.cameraCount}
            </p>
            <p>
              {t('schedules.overview.daysRemaining')}: {item.estimatedDaysRemaining}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Create/Edit Schedule Modal
// ---------------------------------------------------------------------------

interface ScheduleFormValues {
  name: string;
  type: ScheduleType;
  timeRanges: WeeklyTimeRange[];
  retentionTier: RetentionTier;
  cameraIds: string[];
}

interface ScheduleModalProps {
  open: boolean;
  editTarget: RecordingSchedule | null;
  cameras: ScheduleCamera[];
  onClose: () => void;
  onSubmit: (values: ScheduleFormValues) => void;
  isPending: boolean;
}

function ScheduleModal({ open, editTarget, cameras, onClose, onSubmit, isPending }: ScheduleModalProps): JSX.Element | null {
  const { t } = useTranslation();
  const [name, setName] = useState(editTarget?.name ?? '');
  const [type, setType] = useState<ScheduleType>(editTarget?.type ?? 'continuous');
  const [retentionTier, setRetentionTier] = useState<RetentionTier>(editTarget?.retentionTier ?? '30d');
  const [selectedCameras, setSelectedCameras] = useState<string[]>(editTarget?.cameraIds ?? []);
  const [timeRanges, setTimeRanges] = useState<WeeklyTimeRange[]>(editTarget?.timeRanges ?? []);
  const [errors, setErrors] = useState<Record<string, boolean>>({});

  // Reset on editTarget change
  useMemo(() => {
    setName(editTarget?.name ?? '');
    setType(editTarget?.type ?? 'continuous');
    setRetentionTier(editTarget?.retentionTier ?? '30d');
    setSelectedCameras(editTarget?.cameraIds ?? []);
    setTimeRanges(editTarget?.timeRanges ?? []);
    setErrors({});
  }, [editTarget]);

  if (!open) return null;

  const handleSubmit = () => {
    const newErrors: Record<string, boolean> = {};
    if (!name.trim()) newErrors['name'] = true;
    if (type === 'scheduled' && timeRanges.length === 0) newErrors['timeRanges'] = true;
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    onSubmit({ name, type, timeRanges, retentionTier, cameraIds: selectedCameras });
  };

  const toggleDay = (day: number) => {
    const existing = timeRanges.find((r) => r.day === day);
    if (existing) {
      setTimeRanges(timeRanges.filter((r) => r.day !== day));
    } else {
      setTimeRanges([...timeRanges, { day, startTime: '00:00', endTime: '23:59' }]);
    }
  };

  const toggleCamera = (cameraId: string) => {
    setSelectedCameras((prev) =>
      prev.includes(cameraId) ? prev.filter((id) => id !== cameraId) : [...prev, cameraId],
    );
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={editTarget ? t('schedules.edit.title') : t('schedules.create.title')}
      data-testid="schedule-modal"
    >
      <h2>{editTarget ? t('schedules.edit.title') : t('schedules.create.title')}</h2>

      {/* Name */}
      <label>
        {t('schedules.create.name')}
        <input
          type="text"
          value={name}
          onChange={(e) => { setName(e.target.value); setErrors((prev) => ({ ...prev, name: false })); }}
          data-testid="schedule-field-name"
          aria-invalid={errors['name'] ?? false}
          aria-required
        />
      </label>
      {errors['name'] && <p data-testid="schedule-error-name" role="alert">{t('schedules.create.nameRequired')}</p>}

      {/* Type */}
      <fieldset>
        <legend>{t('schedules.create.type')}</legend>
        {SCHEDULE_TYPES.map((st) => (
          <label key={st}>
            <input
              type="radio"
              name="schedule-type"
              value={st}
              checked={type === st}
              onChange={() => setType(st)}
              data-testid={`schedule-type-${st}`}
            />
            {t(`schedules.type.${st}`)}
          </label>
        ))}
      </fieldset>

      {/* Time range picker (only for scheduled type) */}
      {type === 'scheduled' && (
        <fieldset data-testid="schedule-time-range-picker">
          <legend>{t('schedules.create.timeRange')}</legend>
          {DAYS.map((day) => {
            const active = timeRanges.some((r) => r.day === day);
            return (
              <label key={day}>
                <input
                  type="checkbox"
                  checked={active}
                  onChange={() => toggleDay(day)}
                  data-testid={`schedule-day-${day}`}
                />
                {t(`schedules.timeline.day.${day}`)}
              </label>
            );
          })}
          {errors['timeRanges'] && (
            <p data-testid="schedule-error-timeRanges" role="alert">{t('schedules.create.timeRangeRequired')}</p>
          )}
        </fieldset>
      )}

      {/* Retention tier */}
      <label>
        {t('schedules.create.retention')}
        <select
          value={retentionTier}
          onChange={(e) => setRetentionTier(e.target.value as RetentionTier)}
          data-testid="schedule-field-retention"
        >
          {RETENTION_TIERS.map((tier) => (
            <option key={tier} value={tier}>{t(`schedules.retention.${tier}`)}</option>
          ))}
        </select>
      </label>

      {/* Camera multi-select */}
      <fieldset data-testid="schedule-camera-select">
        <legend>{t('schedules.create.cameras')}</legend>
        {cameras.map((cam) => (
          <label key={cam.id}>
            <input
              type="checkbox"
              checked={selectedCameras.includes(cam.id)}
              onChange={() => toggleCamera(cam.id)}
              data-testid={`schedule-camera-${cam.id}`}
            />
            {cam.name}
          </label>
        ))}
      </fieldset>

      <div>
        <button
          type="button"
          onClick={onClose}
          data-testid="schedule-cancel"
        >
          {t('schedules.create.cancel')}
        </button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={isPending}
          data-testid="schedule-submit"
        >
          {isPending ? t('schedules.create.submitting') : t('schedules.create.submit')}
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Delete Confirm Modal
// ---------------------------------------------------------------------------

interface DeleteConfirmProps {
  open: boolean;
  schedule: RecordingSchedule | null;
  onClose: () => void;
  onConfirm: () => void;
}

function DeleteConfirm({ open, schedule, onClose, onConfirm }: DeleteConfirmProps): JSX.Element | null {
  const { t } = useTranslation();
  const [typed, setTyped] = useState('');

  if (!open || !schedule) return null;

  const canConfirm = typed === schedule.name;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t('schedules.delete.title')}
      data-testid="delete-schedule-dialog"
    >
      <h2>{t('schedules.delete.title')}</h2>
      <p>{t('schedules.delete.body', { name: schedule.name })}</p>
      <label>
        {t('schedules.delete.typePrompt', { name: schedule.name })}
        <input
          type="text"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          data-testid="delete-type-input"
        />
      </label>
      <div>
        <button type="button" onClick={onClose} data-testid="delete-cancel">
          {t('schedules.delete.cancel')}
        </button>
        <button
          type="button"
          disabled={!canConfirm}
          onClick={onConfirm}
          data-testid="delete-confirm"
        >
          {t('schedules.delete.confirm')}
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Bulk Assign Modal
// ---------------------------------------------------------------------------

interface BulkAssignModalProps {
  open: boolean;
  schedules: RecordingSchedule[];
  selectedCameraIds: string[];
  onClose: () => void;
  onConfirm: (scheduleId: string) => void;
}

function BulkAssignModal({ open, schedules, selectedCameraIds, onClose, onConfirm }: BulkAssignModalProps): JSX.Element | null {
  const { t } = useTranslation();
  const [scheduleId, setScheduleId] = useState('');

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t('schedules.bulkAssign.modal.title')}
      data-testid="bulk-assign-modal"
    >
      <h2>{t('schedules.bulkAssign.modal.title')}</h2>
      <p>{t('schedules.bulkAssign.modal.camerasSelected', { count: selectedCameraIds.length })}</p>
      <label>
        {t('schedules.bulkAssign.modal.schedule')}
        <select
          value={scheduleId}
          onChange={(e) => setScheduleId(e.target.value)}
          data-testid="bulk-assign-schedule-select"
        >
          <option value="">{t('schedules.bulkAssign.modal.selectSchedule')}</option>
          {schedules.map((s) => (
            <option key={s.id} value={s.id}>{s.name}</option>
          ))}
        </select>
      </label>
      <div>
        <button type="button" onClick={onClose} data-testid="bulk-assign-cancel">
          {t('schedules.create.cancel')}
        </button>
        <button
          type="button"
          disabled={!scheduleId}
          onClick={() => onConfirm(scheduleId)}
          data-testid="bulk-assign-confirm"
        >
          {t('schedules.bulkAssign.modal.confirm')}
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export function SchedulesPage(): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const queryClient = useQueryClient();

  // State
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<RecordingSchedule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<RecordingSchedule | null>(null);
  const [bulkAssignOpen, setBulkAssignOpen] = useState(false);
  const [selectedCameraIds, setSelectedCameraIds] = useState<string[]>([]);

  // Queries
  const schedulesQuery = useQuery<RecordingSchedule[]>({
    queryKey: schedulesQueryKeys.list(tenantId),
    queryFn: () => listSchedules(tenantId),
  });

  const retentionQuery = useQuery<RetentionOverview[]>({
    queryKey: schedulesQueryKeys.retention(tenantId),
    queryFn: () => listRetentionOverview(tenantId),
  });

  const camerasQuery = useQuery<ScheduleCamera[]>({
    queryKey: schedulesQueryKeys.cameras(tenantId),
    queryFn: () => listScheduleCameras(tenantId),
  });

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: schedulesQueryKeys.all(tenantId) });
  }, [queryClient, tenantId]);

  // Mutations
  const createMutation = useMutation({
    mutationFn: (args: CreateScheduleArgs) => createSchedule(args),
    onSuccess: invalidate,
  });

  const updateMutation = useMutation({
    mutationFn: (args: Parameters<typeof updateSchedule>[0]) => updateSchedule(args),
    onSuccess: invalidate,
  });

  const deleteMutation = useMutation({
    mutationFn: (args: Parameters<typeof deleteSchedule>[0]) => deleteSchedule(args),
    onSuccess: invalidate,
  });

  const bulkAssignMutation = useMutation({
    mutationFn: (args: Parameters<typeof bulkAssignSchedule>[0]) => bulkAssignSchedule(args),
    onSuccess: invalidate,
  });

  // Handlers
  const handleCreate = useCallback(
    (values: ScheduleFormValues) => {
      createMutation.mutate({
        tenantId,
        name: values.name,
        type: values.type,
        timeRanges: values.timeRanges,
        retentionTier: values.retentionTier,
        cameraIds: values.cameraIds,
      });
      setCreateOpen(false);
    },
    [createMutation, tenantId],
  );

  const handleEdit = useCallback(
    (values: ScheduleFormValues) => {
      if (!editTarget) return;
      updateMutation.mutate({
        tenantId,
        scheduleId: editTarget.id,
        name: values.name,
        type: values.type,
        timeRanges: values.timeRanges,
        retentionTier: values.retentionTier,
        cameraIds: values.cameraIds,
      });
      setEditTarget(null);
    },
    [editTarget, updateMutation, tenantId],
  );

  const handleDelete = useCallback(() => {
    if (!deleteTarget) return;
    deleteMutation.mutate({ tenantId, scheduleId: deleteTarget.id });
    setDeleteTarget(null);
  }, [deleteTarget, deleteMutation, tenantId]);

  const handleBulkAssign = useCallback(
    (scheduleId: string) => {
      bulkAssignMutation.mutate({ tenantId, scheduleId, cameraIds: selectedCameraIds });
      setBulkAssignOpen(false);
      setSelectedCameraIds([]);
    },
    [bulkAssignMutation, tenantId, selectedCameraIds],
  );

  const toggleCameraSelection = useCallback((cameraId: string) => {
    setSelectedCameraIds((prev) =>
      prev.includes(cameraId) ? prev.filter((id) => id !== cameraId) : [...prev, cameraId],
    );
  }, []);

  const schedules = useMemo(() => schedulesQuery.data ?? [], [schedulesQuery.data]);
  const cameras = useMemo(() => camerasQuery.data ?? [], [camerasQuery.data]);

  return (
    <main
      aria-label={t('schedules.page.label')}
      data-testid="schedules-page"
      className="schedules-page"
    >
      <nav aria-label={t('schedules.breadcrumb.ariaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('schedules.breadcrumb')}</li>
        </ol>
      </nav>

      <header>
        <h1>{t('schedules.title')}</h1>
      </header>

      {/* Retention Overview */}
      {retentionQuery.isSuccess && (
        <RetentionCard overview={retentionQuery.data} />
      )}

      {/* Toolbar */}
      <section aria-label={t('schedules.toolbar.ariaLabel')} data-testid="schedules-toolbar">
        <button
          type="button"
          onClick={() => setCreateOpen(true)}
          data-testid="schedules-create-button"
        >
          {t('schedules.create.title')}
        </button>

        <button
          type="button"
          disabled={selectedCameraIds.length === 0}
          aria-disabled={selectedCameraIds.length === 0}
          onClick={() => setBulkAssignOpen(true)}
          data-testid="schedules-bulk-assign-button"
        >
          {t('schedules.bulkAssign.button')}
        </button>
      </section>

      {/* Camera selection for bulk assign */}
      {cameras.length > 0 && (
        <section aria-label={t('schedules.bulkAssign.camerasLabel')} data-testid="camera-selection">
          {cameras.map((cam) => (
            <label key={cam.id}>
              <input
                type="checkbox"
                checked={selectedCameraIds.includes(cam.id)}
                onChange={() => toggleCameraSelection(cam.id)}
                data-testid={`camera-select-${cam.id}`}
              />
              {cam.name}
            </label>
          ))}
        </section>
      )}

      {/* Loading / Error / Empty */}
      {schedulesQuery.isLoading && (
        <p role="status" aria-live="polite">{t('schedules.loading')}</p>
      )}
      {schedulesQuery.isError && (
        <p role="alert">{t('schedules.error')}</p>
      )}
      {schedulesQuery.isSuccess && schedules.length === 0 && (
        <section data-testid="schedules-empty">
          <h2>{t('schedules.empty.title')}</h2>
          <p>{t('schedules.empty.body')}</p>
          <button type="button" onClick={() => setCreateOpen(true)}>
            {t('schedules.empty.cta')}
          </button>
        </section>
      )}

      {/* Schedule Templates Table */}
      {schedulesQuery.isSuccess && schedules.length > 0 && (
        <table data-testid="schedules-table" aria-label={t('schedules.table.ariaLabel')}>
          <thead>
            <tr>
              <th scope="col">{t('schedules.columns.name')}</th>
              <th scope="col">{t('schedules.columns.type')}</th>
              <th scope="col">{t('schedules.columns.cameras')}</th>
              <th scope="col">{t('schedules.columns.retention')}</th>
              <th scope="col">{t('schedules.columns.timeline')}</th>
              <th scope="col">{t('schedules.columns.actions')}</th>
            </tr>
          </thead>
          <tbody>
            {schedules.map((schedule) => (
              <tr key={schedule.id} data-testid={`schedule-row-${schedule.id}`}>
                <td>{schedule.name}</td>
                <td>
                  <span data-testid={`schedule-type-badge-${schedule.id}`}>
                    {t(`schedules.type.${schedule.type}`)}
                  </span>
                </td>
                <td>{schedule.cameraIds.length}</td>
                <td>{t(`schedules.retention.${schedule.retentionTier}`)}</td>
                <td>
                  <WeeklyTimeline
                    timeRanges={schedule.timeRanges}
                    type={schedule.type}
                    ariaLabel={t('schedules.timeline.ariaLabel', { name: schedule.name })}
                  />
                </td>
                <td>
                  <button
                    type="button"
                    onClick={() => setEditTarget(schedule)}
                    data-testid={`schedule-edit-${schedule.id}`}
                    aria-label={t('schedules.actions.editAriaLabel', { name: schedule.name })}
                  >
                    {t('schedules.actions.edit')}
                  </button>
                  <button
                    type="button"
                    onClick={() => setDeleteTarget(schedule)}
                    data-testid={`schedule-delete-${schedule.id}`}
                    aria-label={t('schedules.actions.deleteAriaLabel', { name: schedule.name })}
                  >
                    {t('schedules.actions.delete')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Dialogs */}
      <ScheduleModal
        open={createOpen}
        editTarget={null}
        cameras={cameras}
        onClose={() => setCreateOpen(false)}
        onSubmit={handleCreate}
        isPending={createMutation.isPending}
      />

      <ScheduleModal
        open={editTarget !== null}
        editTarget={editTarget}
        cameras={cameras}
        onClose={() => setEditTarget(null)}
        onSubmit={handleEdit}
        isPending={updateMutation.isPending}
      />

      <DeleteConfirm
        open={deleteTarget !== null}
        schedule={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
      />

      <BulkAssignModal
        open={bulkAssignOpen}
        schedules={schedules}
        selectedCameraIds={selectedCameraIds}
        onClose={() => setBulkAssignOpen(false)}
        onConfirm={handleBulkAssign}
      />
    </main>
  );
}

export default SchedulesPage;
