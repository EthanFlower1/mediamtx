// KAI-325: Delete user confirmation dialog — requires typing the user name.
//
// Renders in: customer admin only.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { TenantUser } from '@/api/users';

interface DeleteUserConfirmProps {
  open: boolean;
  user: TenantUser | null;
  onClose: () => void;
  onConfirm: () => void;
}

export function DeleteUserConfirm({
  open,
  user,
  onClose,
  onConfirm,
}: DeleteUserConfirmProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [typed, setTyped] = useState('');

  if (!open || !user) return null;

  const confirmed = typed === user.displayName;

  const handleClose = () => {
    setTyped('');
    onClose();
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t('users.delete.title', { name: user.displayName })}
      data-testid="delete-user-dialog"
    >
      <h2>{t('users.delete.title', { name: user.displayName })}</h2>
      <p>{t('users.delete.warning', { name: user.displayName })}</p>
      <label htmlFor="delete-type-input">
        {t('users.delete.typeToConfirm', { name: user.displayName })}
      </label>
      <input
        id="delete-type-input"
        type="text"
        value={typed}
        onChange={(e) => setTyped(e.target.value)}
        data-testid="delete-type-input"
        aria-describedby="delete-type-hint"
      />
      <div>
        <button
          type="button"
          onClick={handleClose}
          data-testid="delete-cancel"
        >
          {t('users.delete.cancel')}
        </button>
        <button
          type="button"
          disabled={!confirmed}
          aria-disabled={!confirmed}
          onClick={() => {
            if (confirmed) {
              setTyped('');
              onConfirm();
            }
          }}
          data-testid="delete-confirm"
        >
          {t('users.delete.confirm')}
        </button>
      </div>
    </div>
  );
}
