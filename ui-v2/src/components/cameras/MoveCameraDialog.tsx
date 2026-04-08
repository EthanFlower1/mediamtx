import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { listRecorders, type Camera } from '@/api/cameras';
import { Modal } from './Modal';

// KAI-321: Move Camera dialog — select target recorder and confirm.

export interface MoveCameraDialogProps {
  open: boolean;
  tenantId: string;
  camera: Camera | null;
  onClose: () => void;
  onConfirm: (targetRecorderId: string) => void;
}

export function MoveCameraDialog({
  open,
  tenantId,
  camera,
  onClose,
  onConfirm,
}: MoveCameraDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const [targetRecorderId, setTargetRecorderId] = useState('');

  const recordersQuery = useQuery({
    queryKey: ['cameras', tenantId, 'recorders'],
    queryFn: () => listRecorders(tenantId),
    enabled: open,
  });

  useEffect(() => {
    if (!open) setTargetRecorderId('');
  }, [open]);

  if (!open || !camera) return null;

  const candidates =
    recordersQuery.data?.filter((r) => r.id !== camera.recorderId) ?? [];

  return (
    <Modal
      open={open}
      onClose={onClose}
      titleId="move-camera-title"
      testId="move-camera-dialog"
    >
      <h2 id="move-camera-title">{t('cameras.move.title', { name: camera.name })}</h2>
      <p>
        {t('cameras.move.description', {
          name: camera.name,
          current: camera.recorderName,
        })}
      </p>
      <label>
        {t('cameras.move.targetLabel')}
        <select
          value={targetRecorderId}
          onChange={(e) => setTargetRecorderId(e.target.value)}
          data-testid="move-target-recorder"
        >
          <option value="">{t('cameras.move.pickTarget')}</option>
          {candidates.map((r) => (
            <option key={r.id} value={r.id}>
              {r.name} — {r.cameraCount}/{r.capacity}
            </option>
          ))}
        </select>
      </label>
      <div className="modal-actions">
        <button type="button" onClick={onClose}>
          {t('cameras.wizard.cancel')}
        </button>
        <button
          type="button"
          disabled={!targetRecorderId}
          onClick={() => onConfirm(targetRecorderId)}
          data-testid="move-confirm"
        >
          {t('cameras.move.confirm')}
        </button>
      </div>
    </Modal>
  );
}
