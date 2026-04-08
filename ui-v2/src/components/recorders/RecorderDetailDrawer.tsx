import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import {
  getRecorder,
  recordersQueryKeys,
  type Recorder,
  type RecorderDetail,
  type SidecarStatus,
} from '@/api/recorders';
import { Modal } from '@/components/cameras/Modal';

// KAI-322: Recorder detail drawer.
//
// Opened when a user clicks "Details" on a recorder row. Shows:
//   - Hardware info (CPU, RAM, disk, IP, version)
//   - Paired cameras list
//   - Sidecar health (MediaMTX, Zitadel, step-ca)
//   - Recent log snippets
//
// Renders in: customer admin (/admin/recorders) and on-prem embed
// (Go binary serves /admin/* via //go:embed — same component).
// NOT rendered in integrator portal.

export interface RecorderDetailDrawerProps {
  open: boolean;
  recorder: Recorder | null;
  tenantId: string;
  onClose: () => void;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1e12) return `${(bytes / 1e12).toFixed(1)} TB`;
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`;
  return `${(bytes / 1e6).toFixed(0)} MB`;
}

export function RecorderDetailDrawer({
  open,
  recorder,
  tenantId,
  onClose,
}: RecorderDetailDrawerProps): JSX.Element | null {
  const { t } = useTranslation();

  const query = useQuery<RecorderDetail>({
    queryKey: recordersQueryKeys.detail(tenantId, recorder?.id ?? ''),
    queryFn: () => getRecorder(tenantId, recorder!.id),
    enabled: open && recorder !== null,
  });

  if (!open || !recorder) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      titleId="recorder-detail-title"
      testId="recorder-detail-drawer"
    >
      <h2 id="recorder-detail-title">{t('recorders.detail.drawerTitle')}</h2>
      <p>{recorder.name}</p>

      {query.isLoading && (
        <p role="status" aria-live="polite">
          {t('recorders.detail.loading')}
        </p>
      )}
      {query.isError && (
        <p role="alert">{t('recorders.detail.error')}</p>
      )}
      {query.isSuccess && query.data && (
        <RecorderDetailBody detail={query.data} />
      )}

      <div className="modal-actions" style={{ marginTop: 24 }}>
        <button type="button" onClick={onClose} data-testid="detail-close-button">
          {t('recorders.detail.close')}
        </button>
      </div>
    </Modal>
  );
}

function RecorderDetailBody({ detail }: { detail: RecorderDetail }): JSX.Element {
  const { t } = useTranslation();
  return (
    <div data-testid="recorder-detail-body">
      {/* Hardware */}
      <section aria-labelledby="detail-hardware-heading" style={{ marginTop: 16 }}>
        <h3 id="detail-hardware-heading">{t('recorders.detail.hardware.heading')}</h3>
        <dl>
          <dt>{t('recorders.detail.hardware.cpu')}</dt>
          <dd data-testid="detail-cpu">{detail.hardware.cpuModel} ({detail.hardware.cpuCores} cores)</dd>

          <dt>{t('recorders.detail.hardware.ram')}</dt>
          <dd data-testid="detail-ram">{formatBytes(detail.hardware.ramBytes)}</dd>

          <dt>{t('recorders.detail.hardware.disk')}</dt>
          <dd data-testid="detail-disk">{detail.hardware.diskModel}</dd>

          <dt>{t('recorders.detail.hardware.ip')}</dt>
          <dd data-testid="detail-ip">{detail.ipAddress}</dd>

          <dt>{t('recorders.detail.hardware.version')}</dt>
          <dd data-testid="detail-version">{detail.version}</dd>
        </dl>
      </section>

      {/* Sidecars */}
      <section aria-labelledby="detail-sidecars-heading" style={{ marginTop: 16 }}>
        <h3 id="detail-sidecars-heading">{t('recorders.detail.sidecars.heading')}</h3>
        <ul data-testid="detail-sidecars">
          {detail.sidecars.map((s) => (
            <SidecarRow key={s.name} sidecar={s} />
          ))}
        </ul>
      </section>

      {/* Paired cameras */}
      <section aria-labelledby="detail-cameras-heading" style={{ marginTop: 16 }}>
        <h3 id="detail-cameras-heading">{t('recorders.detail.cameras.heading')}</h3>
        {detail.pairedCameraIds.length === 0 ? (
          <p data-testid="detail-cameras-empty">{t('recorders.detail.cameras.empty')}</p>
        ) : (
          <ul data-testid="detail-cameras">
            {detail.pairedCameraIds.map((id) => (
              <li key={id}>{id}</li>
            ))}
          </ul>
        )}
      </section>

      {/* Recent logs */}
      <section aria-labelledby="detail-logs-heading" style={{ marginTop: 16 }}>
        <h3 id="detail-logs-heading">{t('recorders.detail.logs.heading')}</h3>
        <pre
          data-testid="detail-logs"
          style={{
            maxHeight: 160,
            overflow: 'auto',
            background: 'var(--color-surface-secondary, #f3f4f6)',
            padding: 8,
            fontSize: '0.75em',
            borderRadius: 4,
          }}
        >
          {detail.recentLogs.join('\n')}
        </pre>
      </section>
    </div>
  );
}

function SidecarRow({ sidecar }: { sidecar: SidecarStatus }): JSX.Element {
  const { t } = useTranslation();
  const label = sidecar.healthy
    ? t('recorders.detail.sidecars.healthy')
    : t('recorders.detail.sidecars.unhealthy');
  return (
    <li
      data-testid={`sidecar-${sidecar.name}`}
      style={{ display: 'flex', gap: 8, alignItems: 'center', padding: '4px 0' }}
    >
      <span
        role="img"
        aria-label={label}
        aria-hidden="false"
      >
        {sidecar.healthy ? '●' : '■'}
      </span>
      <strong>{sidecar.name}</strong>
      <span>{sidecar.message}</span>
    </li>
  );
}
