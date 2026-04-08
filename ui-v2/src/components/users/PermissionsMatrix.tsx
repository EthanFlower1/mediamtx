// KAI-325: Role × action permissions matrix.
//
// Renders in: customer admin only (Permissions tab).
// Calls updateRolePermission which stubs to PUT /api/v1/permissions/roles
// (KAI-225 Casbin). See api/users.ts for the TODO.

import { useTranslation } from 'react-i18next';
import type { RolePermission, UserRole, ResourceAction } from '@/api/users';

const ROLES: UserRole[] = ['admin', 'operator', 'viewer', 'auditor'];

const ACTIONS: ResourceAction[] = [
  'view.live',
  'view.playback',
  'ptz.control',
  'recording.configure',
  'users.manage',
  'audit.view',
  'cameras.manage',
  'recorders.manage',
  'integrations.manage',
];

interface PermissionsMatrixProps {
  permissions: RolePermission[];
  onToggle: (role: UserRole, action: ResourceAction, allowed: boolean) => void;
  isSaving: boolean;
  saveError: boolean;
}

export function PermissionsMatrix({
  permissions,
  onToggle,
  isSaving,
  saveError,
}: PermissionsMatrixProps): JSX.Element {
  const { t } = useTranslation('users');

  const getPermission = (role: UserRole, action: ResourceAction): RolePermission | undefined =>
    permissions.find((p) => p.role === role && p.action === action);

  return (
    <section
      aria-label={t('permissions.sectionLabel')}
      data-testid="permissions-matrix"
    >
      <h2>{t('permissions.heading')}</h2>
      <p>{t('permissions.description')}</p>
      {saveError && <p role="alert">{t('permissions.saveError')}</p>}
      {isSaving && <p role="status" aria-live="polite">{t('permissions.saving')}</p>}
      <div style={{ overflowX: 'auto' }}>
        <table>
          <caption className="sr-only">{t('permissions.sectionLabel')}</caption>
          <thead>
            <tr>
              <th scope="col">{/* action column header — intentionally blank */}</th>
              {ROLES.map((role) => (
                <th key={role} scope="col">
                  {t(`users.role.${role}`)}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {ACTIONS.map((action) => (
              <tr key={action}>
                <th scope="row">
                  {t(`permissions.action.${action}`)}
                </th>
                {ROLES.map((role) => {
                  const perm = getPermission(role, action);
                  const allowed = perm?.allowed ?? false;
                  const inherited = perm?.inherited ?? false;
                  const isAdminLocked = role === 'admin';

                  return (
                    <td
                      key={role}
                      data-testid={`perm-cell-${role}-${action}`}
                    >
                      <input
                        type="checkbox"
                        checked={allowed}
                        disabled={isAdminLocked}
                        aria-label={t('permissions.cell.ariaLabel', {
                          role: t(`users.role.${role}`),
                          action: t(`permissions.action.${action}`),
                          state: allowed ? 'allowed' : 'denied',
                        })}
                        onChange={(e) => onToggle(role, action, e.target.checked)}
                        data-testid={`perm-toggle-${role}-${action}`}
                      />
                      {inherited && (
                        <span
                          className="sr-only"
                          aria-label={t('permissions.inherited')}
                          data-testid={`perm-inherited-${role}-${action}`}
                        >
                          {t('permissions.inherited')}
                        </span>
                      )}
                    </td>
                  );
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
