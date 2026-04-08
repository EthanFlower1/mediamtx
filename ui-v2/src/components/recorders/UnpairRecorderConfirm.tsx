import { useTranslation } from 'react-i18next';

import type { Recorder } from '@/api/recorders';
import { Modal } from '@/components/cameras/Modal';

// KAI-322: Unpair Recorder confirmation dialog.
//
// Requires explicit confirmation (destructive action gate) before
// calling DELETE /api/v1/recorders/:id. Shows camera impact count.
// Escape / Cancel both dismiss without deleting.
//
// Renders in: customer admin (/admin/recorders) and on-prem embed.
// NOT rendered in integrator portal.

export interface UnpairRecorderConfirmProps {
  open: boolean;
  recorder: Recorder | null;
  onClose: () => void;
  onConfirm: () => void;
}

export function UnpairRecorderConfirm({
  open,
  recorder,
  onClose,
  onConfirm,
}: UnpairRecorderConfirmProps): JSX.Element | null {
  const { t } = useTranslation();

  if (!open || !recorder) return null;

  return (
    <Modal
      open={open}
      onClose={onClose}
      titleId="unpair-recorder-title"
      descriptionId="unpair-recorder-warning"
      testId="unpair-recorder-dialog"
    >
      <h2 id="unpair-recorder-title">
        {t('recorders.unpair.title', { name: recorder.name })}
      </h2>
      <p id="unpair-recorder-warning" role="alert" data-testid="unpair-warning">
        {t('recorders.unpair.warning', {
          name: recorder.name,
          count: recorder.cameraCount,
        })}
      </p>
      <div className="modal-actions" style={{ marginTop: 24, display: 'flex', gap: 8 }}>
        <button
          type="button"
          onClick={onClose}
          data-testid="unpair-cancel"
        >
          {t('recorders.unpair.cancel')}
        </button>
        <button
          type="button"
          onClick={onConfirm}
          data-testid="unpair-confirm"
          aria-describedby="unpair-recorder-warning"
        >
          {t('recorders.unpair.confirm')}
        </button>
      </div>
    </Modal>
  );
}
