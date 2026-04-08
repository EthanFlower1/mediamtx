import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import {
  isValidRtspUrl,
  listRecorders,
  type Camera,
  type CameraSpec,
  type RetentionTier,
} from '@/api/cameras';
import { Modal } from './Modal';

// KAI-321: Edit Camera modal — reuses wizard Step 2-4 form layout but
// single-pane, pre-filled with the current camera.

export interface EditCameraModalProps {
  open: boolean;
  tenantId: string;
  camera: Camera | null;
  onClose: () => void;
  onSubmit: (patch: Partial<CameraSpec>) => void;
}

const RETENTION_TIERS: RetentionTier[] = ['short', 'standard', 'long', 'forensic'];

export function EditCameraModal({
  open,
  tenantId,
  camera,
  onClose,
  onSubmit,
}: EditCameraModalProps): JSX.Element | null {
  const { t } = useTranslation();
  const [name, setName] = useState('');
  const [rtspUrl, setRtspUrl] = useState('');
  const [recorderId, setRecorderId] = useState('');
  const [retentionTier, setRetentionTier] = useState<RetentionTier>('standard');
  const [profileName, setProfileName] = useState('main');
  const [urlError, setUrlError] = useState<string | null>(null);

  const recordersQuery = useQuery({
    queryKey: ['cameras', tenantId, 'recorders'],
    queryFn: () => listRecorders(tenantId),
    enabled: open,
  });

  useEffect(() => {
    if (camera) {
      setName(camera.name);
      setRtspUrl(camera.rtspUrl);
      setRecorderId(camera.recorderId);
      setRetentionTier(camera.retentionTier);
      setProfileName(camera.profileName);
      setUrlError(null);
    }
  }, [camera]);

  if (!open || !camera) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      titleId="edit-camera-title"
      testId="edit-camera-modal"
    >
      <h2 id="edit-camera-title">{t('cameras.edit.title', { name: camera.name })}</h2>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!isValidRtspUrl(rtspUrl)) {
            setUrlError(t('cameras.wizard.errors.invalidRtsp'));
            return;
          }
          onSubmit({
            name,
            rtspUrl,
            recorderId,
            retentionTier,
            profileName,
          });
        }}
      >
        <label>
          {t('cameras.wizard.fields.name')}
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            data-testid="edit-field-name"
          />
        </label>
        <label>
          {t('cameras.wizard.fields.rtspUrl')}
          <input
            type="url"
            value={rtspUrl}
            onChange={(e) => {
              setRtspUrl(e.target.value);
              setUrlError(null);
            }}
            aria-invalid={urlError ? 'true' : undefined}
            data-testid="edit-field-rtsp"
          />
        </label>
        {urlError && (
          <p role="alert" data-testid="edit-url-error">
            {urlError}
          </p>
        )}
        <label>
          {t('cameras.wizard.fields.recorder')}
          <select
            value={recorderId}
            onChange={(e) => setRecorderId(e.target.value)}
            data-testid="edit-field-recorder"
          >
            {(recordersQuery.data ?? []).map((r) => (
              <option key={r.id} value={r.id}>
                {r.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t('cameras.wizard.fields.retention')}
          <select
            value={retentionTier}
            onChange={(e) => setRetentionTier(e.target.value as RetentionTier)}
            data-testid="edit-field-retention"
          >
            {RETENTION_TIERS.map((tier) => (
              <option key={tier} value={tier}>
                {t(`cameras.retention.${tier}`)}
              </option>
            ))}
          </select>
        </label>
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            {t('cameras.wizard.cancel')}
          </button>
          <button type="submit" data-testid="edit-submit">
            {t('cameras.edit.save')}
          </button>
        </div>
      </form>
    </Modal>
  );
}
