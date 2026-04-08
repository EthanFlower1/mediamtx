import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { Camera } from '@/api/cameras';
import { Modal } from './Modal';

// KAI-321: Delete Camera confirm-intent dialog.
//
// Requires the user to type the camera's exact name to enable the
// confirm button, and announces the retention impact via
// aria-describedby.

export interface DeleteCameraConfirmProps {
  open: boolean;
  camera: Camera | null;
  onClose: () => void;
  onConfirm: () => void;
}

export function DeleteCameraConfirm({
  open,
  camera,
  onClose,
  onConfirm,
}: DeleteCameraConfirmProps): JSX.Element | null {
  const { t } = useTranslation();
  const [typed, setTyped] = useState('');

  useEffect(() => {
    if (!open) setTyped('');
  }, [open]);

  if (!open || !camera) return null;

  const matches = typed === camera.name;

  return (
    <Modal
      open={open}
      onClose={onClose}
      titleId="delete-camera-title"
      descriptionId="delete-camera-warning"
      testId="delete-camera-dialog"
    >
      <h2 id="delete-camera-title">{t('cameras.delete.title', { name: camera.name })}</h2>
      <p id="delete-camera-warning" role="alert" data-testid="delete-warning">
        {t('cameras.delete.warning', { name: camera.name })}
      </p>
      <label>
        {t('cameras.delete.typeToConfirm', { name: camera.name })}
        <input
          type="text"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          data-testid="delete-type-input"
          aria-describedby="delete-camera-warning"
        />
      </label>
      <div className="modal-actions">
        <button type="button" onClick={onClose}>
          {t('cameras.wizard.cancel')}
        </button>
        <button
          type="button"
          disabled={!matches}
          aria-disabled={!matches}
          aria-describedby="delete-camera-warning"
          onClick={onConfirm}
          data-testid="delete-confirm"
        >
          {t('cameras.delete.confirm')}
        </button>
      </div>
    </Modal>
  );
}
