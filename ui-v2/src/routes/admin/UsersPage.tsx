// KAI-325: Customer Admin Users page — /admin/users.
//
// Three sub-surfaces:
//   1. Users tab — virtualized list with CRUD + bulk actions.
//   2. Permissions tab — Casbin-backed role × action matrix.
//   3. Sign-in Methods tab — 6 SSO provider wizards.
//
// All strings through react-i18next 'users' namespace.
// All queries are scoped to the current tenant (multi-tenant isolation).
// No Zitadel-specific knowledge: provider ops go through IdentityProvider
// interface (KAI-222) via api/users.ts stubs.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import type { OnChangeFn, RowSelectionState, Updater } from '@tanstack/react-table';

import {
  deleteUser,
  inviteUser,
  listPermissions,
  listUsers,
  permissionsQueryKeys,
  suspendUser,
  updateRolePermission,
  updateUser,
  usersQueryKeys,
  type InviteUserArgs,
  type ResourceAction,
  type TenantUser,
  type UserRole,
} from '@/api/users';
import { useSessionStore } from '@/stores/session';

import { UserList } from '@/components/users/UserList';
import { InviteUserDialog } from '@/components/users/InviteUserDialog';
import { DeleteUserConfirm } from '@/components/users/DeleteUserConfirm';
import { PermissionsMatrix } from '@/components/users/PermissionsMatrix';
import { SignInMethodsTab } from '@/components/users/SignInMethodsTab';
import type { InviteUserFormValues } from '@/lib/authProviderSchemas';

type ActiveTab = 'users' | 'permissions' | 'signInMethods';

export function UsersPage(): JSX.Element {
  const { t } = useTranslation('users');
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const queryClient = useQueryClient();

  // Tab state
  const [activeTab, setActiveTab] = useState<ActiveTab>('users');

  // Users tab state
  const [search, setSearch] = useState('');
  const [selection, setSelection] = useState<RowSelectionState>({});

  const handleSelectionChange: OnChangeFn<RowSelectionState> = useCallback(
    (updaterOrValue: Updater<RowSelectionState>) => {
      setSelection((prev) =>
        typeof updaterOrValue === 'function' ? updaterOrValue(prev) : updaterOrValue,
      );
    },
    [],
  );
  const [inviteOpen, setInviteOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<TenantUser | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<TenantUser | null>(null);

  // Users query
  const usersQuery = useQuery<TenantUser[]>({
    queryKey: usersQueryKeys.list(tenantId),
    queryFn: () => listUsers(tenantId),
  });

  const invalidateUsers = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: usersQueryKeys.all(tenantId) });
  }, [queryClient, tenantId]);

  // Users mutations
  const inviteMutation = useMutation({
    mutationFn: (args: InviteUserArgs) => inviteUser(args),
    onSuccess: invalidateUsers,
  });

  const updateMutation = useMutation({
    mutationFn: (args: Parameters<typeof updateUser>[0]) => updateUser(args),
    onSuccess: invalidateUsers,
  });

  const suspendMutation = useMutation({
    mutationFn: (args: { tenantId: string; userId: string }) => suspendUser(args),
    onSuccess: invalidateUsers,
  });

  const deleteMutation = useMutation({
    mutationFn: (args: Parameters<typeof deleteUser>[0]) => deleteUser(args),
    onSuccess: invalidateUsers,
  });

  // Permissions query
  const permissionsQuery = useQuery({
    queryKey: permissionsQueryKeys.matrix(tenantId),
    queryFn: () => listPermissions(tenantId),
    enabled: activeTab === 'permissions',
  });

  const permSaveMutation = useMutation({
    mutationFn: (args: { role: UserRole; action: ResourceAction; allowed: boolean }) =>
      updateRolePermission({ tenantId, ...args }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: permissionsQueryKeys.all(tenantId) });
    },
  });

  // Derived values
  const filteredUsers = useMemo(() => {
    const users = usersQuery.data ?? [];
    if (!search.trim()) return users;
    const q = search.trim().toLowerCase();
    return users.filter(
      (u) =>
        u.displayName.toLowerCase().includes(q) ||
        u.email.toLowerCase().includes(q),
    );
  }, [usersQuery.data, search]);

  const selectedIds = useMemo(
    () => Object.keys(selection).filter((id) => selection[id]),
    [selection],
  );
  const hasSelection = selectedIds.length > 0;

  // Bulk actions
  const handleBulkSuspend = useCallback(() => {
    for (const userId of selectedIds) {
      suspendMutation.mutate({ tenantId, userId });
    }
    setSelection({});
  }, [selectedIds, suspendMutation, tenantId]);

  const handleBulkDelete = useCallback(() => {
    for (const userId of selectedIds) {
      deleteMutation.mutate({ tenantId, userId });
    }
    setSelection({});
  }, [selectedIds, deleteMutation, tenantId]);

  // Invite handler
  const handleInvite = useCallback(
    (values: InviteUserFormValues) => {
      inviteMutation.mutate({
        tenantId,
        email: values.email,
        role: values.role as UserRole,
        groups: values.groups.split(',').map((g) => g.trim()).filter(Boolean),
      });
      setInviteOpen(false);
    },
    [inviteMutation, tenantId],
  );

  // Suspend single user
  const handleSuspend = useCallback(
    (user: TenantUser) => {
      if (user.status === 'active') {
        suspendMutation.mutate({ tenantId, userId: user.id });
      } else {
        updateMutation.mutate({ tenantId, userId: user.id, status: 'active' });
      }
    },
    [suspendMutation, updateMutation, tenantId],
  );

  return (
    <main
      aria-label={t('users.page.label')}
      data-testid="users-page"
      className="users-page"
    >
      <nav aria-label={t('users.breadcrumb.ariaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('users.page.title')}</li>
        </ol>
      </nav>

      <header>
        <h1>{t('users.page.title')}</h1>
      </header>

      {/* Tab navigation */}
      <div role="tablist" aria-label={t('users.tabs.ariaLabel')}>
        {(
          [
            { id: 'users', labelKey: 'users.tab.users' },
            { id: 'permissions', labelKey: 'users.tab.permissions' },
            { id: 'signInMethods', labelKey: 'users.tab.signInMethods' },
          ] as const
        ).map(({ id, labelKey }) => (
          <button
            key={id}
            role="tab"
            aria-selected={activeTab === id}
            aria-controls={`tabpanel-${id}`}
            id={`tab-${id}`}
            type="button"
            onClick={() => setActiveTab(id)}
            data-testid={`tab-${id}`}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>

      {/* Users tab panel */}
      <div
        role="tabpanel"
        id="tabpanel-users"
        aria-labelledby="tab-users"
        hidden={activeTab !== 'users'}
        data-testid="tabpanel-users"
      >
        {/* Toolbar */}
        <section
          aria-label={t('users.toolbar.ariaLabel')}
          className="users-page__toolbar"
        >
          <label>
            <span className="sr-only">{t('users.toolbar.searchLabel')}</span>
            <input
              type="search"
              placeholder={t('users.toolbar.searchPlaceholder')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              data-testid="users-search"
              aria-label={t('users.toolbar.searchLabel')}
            />
          </label>

          <button
            type="button"
            onClick={() => setInviteOpen(true)}
            data-testid="users-invite-button"
          >
            {t('users.actions.invite')}
          </button>

          <div role="group" aria-label={t('users.toolbar.bulkAriaLabel')}>
            <button
              type="button"
              disabled={!hasSelection}
              aria-disabled={!hasSelection}
              onClick={handleBulkSuspend}
              data-testid="users-bulk-suspend"
            >
              {t('users.bulk.suspend')}
            </button>
            <button
              type="button"
              disabled={!hasSelection}
              aria-disabled={!hasSelection}
              onClick={handleBulkDelete}
              data-testid="users-bulk-delete"
            >
              {t('users.bulk.delete')}
            </button>
          </div>
        </section>

        {usersQuery.isLoading && (
          <p role="status" aria-live="polite">{t('users.list.loading')}</p>
        )}
        {usersQuery.isError && (
          <p role="alert">{t('users.list.error')}</p>
        )}
        {usersQuery.isSuccess && (
          <UserList
            users={filteredUsers}
            selection={selection}
            onSelectionChange={handleSelectionChange}
            onEdit={setEditTarget}
            onSuspend={handleSuspend}
            onDelete={setDeleteTarget}
          />
        )}
      </div>

      {/* Permissions tab panel */}
      <div
        role="tabpanel"
        id="tabpanel-permissions"
        aria-labelledby="tab-permissions"
        hidden={activeTab !== 'permissions'}
        data-testid="tabpanel-permissions"
      >
        {permissionsQuery.isLoading && (
          <p role="status" aria-live="polite">{t('permissions.loading')}</p>
        )}
        {permissionsQuery.isError && (
          <p role="alert">{t('permissions.error')}</p>
        )}
        {permissionsQuery.isSuccess && (
          <PermissionsMatrix
            permissions={permissionsQuery.data}
            onToggle={(role, action, allowed) =>
              permSaveMutation.mutate({ role, action, allowed })
            }
            isSaving={permSaveMutation.isPending}
            saveError={permSaveMutation.isError}
          />
        )}
      </div>

      {/* Sign-in Methods tab panel */}
      <div
        role="tabpanel"
        id="tabpanel-signInMethods"
        aria-labelledby="tab-signInMethods"
        hidden={activeTab !== 'signInMethods'}
        data-testid="tabpanel-signInMethods"
      >
        <SignInMethodsTab tenantId={tenantId} />
      </div>

      {/* Dialogs */}
      <InviteUserDialog
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        onSubmit={handleInvite}
        isPending={inviteMutation.isPending}
      />

      {/* TODO(KAI-325): EditUserDialog — role/groups editor */}
      {editTarget && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={t('users.edit.title', { name: editTarget.displayName })}
          data-testid="edit-user-dialog"
        >
          <p>{t('users.edit.title', { name: editTarget.displayName })}</p>
          <button type="button" onClick={() => setEditTarget(null)} data-testid="edit-cancel">
            {t('users.edit.cancel')}
          </button>
        </div>
      )}

      <DeleteUserConfirm
        open={deleteTarget !== null}
        user={deleteTarget}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) deleteMutation.mutate({ tenantId, userId: deleteTarget.id });
          setDeleteTarget(null);
        }}
      />
    </main>
  );
}

export default UsersPage;
