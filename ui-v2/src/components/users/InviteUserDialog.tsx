// KAI-325: Invite user dialog.
//
// Renders in: customer admin only (Users tab).

import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { inviteUserSchema, type InviteUserFormValues } from '@/lib/authProviderSchemas';
import type { UserRole } from '@/api/users';

interface InviteUserDialogProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (values: InviteUserFormValues) => void;
  isPending: boolean;
}

const ROLES: UserRole[] = ['admin', 'operator', 'viewer', 'auditor'];

export function InviteUserDialog({
  open,
  onClose,
  onSubmit,
  isPending,
}: InviteUserDialogProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const {
    register,
    handleSubmit,
    reset,
    formState: { errors },
  } = useForm<InviteUserFormValues>({
    resolver: zodResolver(inviteUserSchema),
    defaultValues: { email: '', role: 'viewer', groups: '' },
  });

  if (!open) return null;

  const handleClose = () => {
    reset();
    onClose();
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={t('users.invite.title')}
      data-testid="invite-user-dialog"
    >
      <h2>{t('users.invite.title')}</h2>
      <form
        onSubmit={handleSubmit((values) => {
          onSubmit(values);
          reset();
        })}
        noValidate
      >
        <div>
          <label htmlFor="invite-email">{t('users.invite.emailLabel')}</label>
          <input
            id="invite-email"
            type="email"
            placeholder={t('users.invite.emailPlaceholder')}
            aria-invalid={errors.email ? 'true' : undefined}
            aria-describedby={errors.email ? 'invite-email-error' : undefined}
            data-testid="invite-field-email"
            {...register('email')}
          />
          {errors.email && (
            <p id="invite-email-error" role="alert" data-testid="invite-email-error">
              {t(errors.email.message ?? 'users.invite.errors.emailInvalid')}
            </p>
          )}
        </div>

        <div>
          <label htmlFor="invite-role">{t('users.invite.roleLabel')}</label>
          <select
            id="invite-role"
            aria-invalid={errors.role ? 'true' : undefined}
            data-testid="invite-field-role"
            {...register('role')}
          >
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {t(`users.role.${r}`)}
              </option>
            ))}
          </select>
          {errors.role && (
            <p role="alert" data-testid="invite-role-error">
              {t(errors.role.message ?? 'users.invite.errors.roleRequired')}
            </p>
          )}
        </div>

        <div>
          <label htmlFor="invite-groups">{t('users.invite.groupsLabel')}</label>
          <input
            id="invite-groups"
            type="text"
            placeholder={t('users.invite.groupsPlaceholder')}
            data-testid="invite-field-groups"
            {...register('groups')}
          />
        </div>

        <div>
          <button
            type="button"
            onClick={handleClose}
            data-testid="invite-cancel"
          >
            {t('users.invite.cancel')}
          </button>
          <button
            type="submit"
            disabled={isPending}
            aria-disabled={isPending}
            data-testid="invite-submit"
          >
            {t('users.invite.submit')}
          </button>
        </div>
      </form>
    </div>
  );
}
