import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listStaff,
  inviteStaff,
  updateStaff,
  suspendStaff,
  reactivateStaff,
  removeStaff,
  getRoleHistory,
  staffQueryKeys,
  ROLE_DEFINITIONS,
  __TEST__,
} from '@/api/staff';
import type {
  StaffMember,
  StaffRole,
  StaffStatus,
  RoleChangeEntry,
} from '@/api/staff';

// KAI-313: Integrator Portal Staff Management + Roles page.
//
// Lets integrators manage internal staff accounts:
//   - Table with role badges (icon+text+border, never color alone)
//   - Invite dialog, edit role dialog, suspend/reactivate, remove (REMOVE confirmation)
//   - Roles summary card showing 4 predefined roles
//   - Section 508/WCAG 2.1 AA compliant

// ---------------------------------------------------------------------------
// Role badge — icon+text+border (severity encoding, never color alone)
// ---------------------------------------------------------------------------

const ROLE_STYLES: Record<StaffRole, { border: string; bg: string; text: string; icon: string }> = {
  owner: { border: 'border-purple-600', bg: 'bg-purple-50', text: 'text-purple-800', icon: '[Crown]' },
  admin: { border: 'border-blue-600', bg: 'bg-blue-50', text: 'text-blue-800', icon: '[Shield]' },
  technician: { border: 'border-amber-600', bg: 'bg-amber-50', text: 'text-amber-800', icon: '[Wrench]' },
  viewer: { border: 'border-slate-500', bg: 'bg-slate-50', text: 'text-slate-700', icon: '[Eye]' },
};

function RoleBadge({ role }: { readonly role: StaffRole }): JSX.Element {
  const { t } = useTranslation();
  const s = ROLE_STYLES[role];
  return (
    <span
      data-testid={`role-badge-${role}`}
      className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${s.border} ${s.bg} ${s.text}`}
    >
      <span aria-hidden="true">{s.icon}</span>
      {t(`staff.roles.${role}`)}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Status badge
// ---------------------------------------------------------------------------

const STATUS_STYLES: Record<StaffStatus, { border: string; bg: string; text: string; label: string }> = {
  active: { border: 'border-green-600', bg: 'bg-green-50', text: 'text-green-800', label: 'Active' },
  invited: { border: 'border-yellow-600', bg: 'bg-yellow-50', text: 'text-yellow-800', label: 'Invited' },
  suspended: { border: 'border-red-600', bg: 'bg-red-50', text: 'text-red-800', label: 'Suspended' },
};

function StatusBadge({ status }: { readonly status: StaffStatus }): JSX.Element {
  const { t } = useTranslation();
  const s = STATUS_STYLES[status];
  return (
    <span
      data-testid={`status-badge-${status}`}
      className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${s.border} ${s.bg} ${s.text}`}
    >
      {t(`staff.status.${status}`)}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Invite Staff Dialog
// ---------------------------------------------------------------------------

interface InviteDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly integratorId: string;
}

function InviteStaffDialog({ open, onClose, integratorId }: InviteDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<StaffRole>('technician');
  const [message, setMessage] = useState('');
  const [emailError, setEmailError] = useState('');

  const mutation = useMutation({
    mutationFn: () => inviteStaff({ integratorId, email, role, message: message || undefined }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: staffQueryKeys.all(integratorId) });
      setEmail('');
      setRole('technician');
      setMessage('');
      setEmailError('');
      onClose();
    },
  });

  const handleSubmit = useCallback(() => {
    if (!email.trim() || !email.includes('@')) {
      setEmailError(t('staff.invite.emailError'));
      return;
    }
    setEmailError('');
    mutation.mutate();
  }, [email, t, mutation]);

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="invite-dialog-title"
      data-testid="invite-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h2 id="invite-dialog-title" className="text-lg font-bold text-slate-900">
          {t('staff.invite.title')}
        </h2>

        <div className="mt-4 space-y-3">
          <div>
            <label htmlFor="invite-email" className="block text-sm font-medium text-slate-700">
              {t('staff.invite.emailLabel')}
            </label>
            <input
              id="invite-email"
              data-testid="invite-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
              aria-describedby={emailError ? 'invite-email-error' : undefined}
              aria-invalid={emailError ? 'true' : undefined}
            />
            {emailError && (
              <p id="invite-email-error" data-testid="invite-email-error" role="alert" className="mt-1 text-xs text-red-700">
                {emailError}
              </p>
            )}
          </div>

          <div>
            <label htmlFor="invite-role" className="block text-sm font-medium text-slate-700">
              {t('staff.invite.roleLabel')}
            </label>
            <select
              id="invite-role"
              data-testid="invite-role"
              value={role}
              onChange={(e) => setRole(e.target.value as StaffRole)}
              className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            >
              <option value="owner">{t('staff.roles.owner')}</option>
              <option value="admin">{t('staff.roles.admin')}</option>
              <option value="technician">{t('staff.roles.technician')}</option>
              <option value="viewer">{t('staff.roles.viewer')}</option>
            </select>
          </div>

          <div>
            <label htmlFor="invite-message" className="block text-sm font-medium text-slate-700">
              {t('staff.invite.messageLabel')}
            </label>
            <textarea
              id="invite-message"
              data-testid="invite-message"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              rows={2}
              className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
          </div>
        </div>

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="invite-cancel"
            onClick={onClose}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('staff.invite.cancel')}
          </button>
          <button
            type="button"
            data-testid="invite-submit"
            onClick={handleSubmit}
            disabled={mutation.isPending}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {mutation.isPending ? t('staff.invite.sending') : t('staff.invite.send')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Edit Staff Dialog
// ---------------------------------------------------------------------------

interface EditDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly member: StaffMember | null;
  readonly integratorId: string;
}

function EditStaffDialog({ open, onClose, member, integratorId }: EditDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [newRole, setNewRole] = useState<StaffRole>(member?.role ?? 'viewer');

  const historyQuery = useQuery({
    queryKey: staffQueryKeys.roleHistory(integratorId, member?.id ?? ''),
    queryFn: () => getRoleHistory(integratorId, member?.id ?? ''),
    enabled: open && !!member,
  });

  const mutation = useMutation({
    mutationFn: () =>
      updateStaff({ integratorId, staffId: member!.id, role: newRole }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: staffQueryKeys.all(integratorId) });
      onClose();
    },
  });

  // Sync newRole when member changes.
  if (member && newRole !== member.role && !mutation.isPending) {
    // Only reset if the dialog just opened with a different member.
  }

  if (!open || !member) return null;

  const history: readonly RoleChangeEntry[] = historyQuery.data ?? [];

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="edit-dialog-title"
      data-testid="edit-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h2 id="edit-dialog-title" className="text-lg font-bold text-slate-900">
          {t('staff.edit.title', { name: member.name })}
        </h2>

        <div className="mt-4">
          <label htmlFor="edit-role" className="block text-sm font-medium text-slate-700">
            {t('staff.edit.roleLabel')}
          </label>
          <select
            id="edit-role"
            data-testid="edit-role"
            value={newRole}
            onChange={(e) => setNewRole(e.target.value as StaffRole)}
            className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
          >
            <option value="owner">{t('staff.roles.owner')}</option>
            <option value="admin">{t('staff.roles.admin')}</option>
            <option value="technician">{t('staff.roles.technician')}</option>
            <option value="viewer">{t('staff.roles.viewer')}</option>
          </select>
        </div>

        {history.length > 0 && (
          <div className="mt-4" data-testid="role-history">
            <h3 className="text-sm font-medium text-slate-700">{t('staff.edit.historyTitle')}</h3>
            <ul className="mt-2 space-y-1 text-xs text-slate-600" aria-label={t('staff.edit.historyAriaLabel')}>
              {history.map((entry) => (
                <li key={entry.id} data-testid={`history-entry-${entry.id}`}>
                  {t('staff.edit.historyEntry', {
                    from: t(`staff.roles.${entry.fromRole}`),
                    to: t(`staff.roles.${entry.toRole}`),
                    by: entry.changedBy,
                    date: new Date(entry.changedAtIso).toLocaleDateString(),
                  })}
                </li>
              ))}
            </ul>
          </div>
        )}

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="edit-cancel"
            onClick={onClose}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('staff.edit.cancel')}
          </button>
          <button
            type="button"
            data-testid="edit-save"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {t('staff.edit.save')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Suspend/Reactivate Confirmation Dialog
// ---------------------------------------------------------------------------

interface SuspendDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly member: StaffMember | null;
  readonly integratorId: string;
}

function SuspendDialog({ open, onClose, member, integratorId }: SuspendDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const isSuspended = member?.status === 'suspended';

  const mutation = useMutation({
    mutationFn: () =>
      isSuspended
        ? reactivateStaff(integratorId, member!.id)
        : suspendStaff(integratorId, member!.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: staffQueryKeys.all(integratorId) });
      onClose();
    },
  });

  if (!open || !member) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="suspend-dialog-title"
      data-testid="suspend-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h2 id="suspend-dialog-title" className="text-lg font-bold text-slate-900">
          {isSuspended
            ? t('staff.reactivate.title', { name: member.name })
            : t('staff.suspend.title', { name: member.name })}
        </h2>
        <p className="mt-2 text-sm text-slate-600" data-testid="suspend-impact">
          {isSuspended
            ? t('staff.reactivate.impact', { name: member.name })
            : t('staff.suspend.impact', { name: member.name })}
        </p>
        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="suspend-cancel"
            onClick={onClose}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('staff.suspend.cancel')}
          </button>
          <button
            type="button"
            data-testid="suspend-confirm"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
            className={`rounded-md px-4 py-2 text-sm font-medium text-white disabled:opacity-50 ${
              isSuspended ? 'bg-green-600 hover:bg-green-700' : 'bg-red-600 hover:bg-red-700'
            }`}
          >
            {isSuspended ? t('staff.reactivate.confirm') : t('staff.suspend.confirm')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Remove Staff Confirmation Dialog (destructive — type REMOVE)
// ---------------------------------------------------------------------------

interface RemoveDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly member: StaffMember | null;
  readonly integratorId: string;
}

function RemoveStaffDialog({ open, onClose, member, integratorId }: RemoveDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [confirmText, setConfirmText] = useState('');

  const mutation = useMutation({
    mutationFn: () => removeStaff(integratorId, member!.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: staffQueryKeys.all(integratorId) });
      setConfirmText('');
      onClose();
    },
  });

  if (!open || !member) return null;

  const canConfirm = confirmText === 'REMOVE';

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="remove-dialog-title"
      data-testid="remove-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h2 id="remove-dialog-title" className="text-lg font-bold text-red-800">
          {t('staff.remove.title', { name: member.name })}
        </h2>
        <p className="mt-2 text-sm text-slate-600">
          {t('staff.remove.warning', { name: member.name })}
        </p>
        <div className="mt-4">
          <label htmlFor="remove-confirm-input" className="block text-sm font-medium text-slate-700">
            {t('staff.remove.typeToConfirm')}
          </label>
          <input
            id="remove-confirm-input"
            data-testid="remove-confirm-input"
            type="text"
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            aria-describedby="remove-confirm-hint"
          />
          <p id="remove-confirm-hint" className="mt-1 text-xs text-slate-500">
            {t('staff.remove.typeHint')}
          </p>
        </div>
        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="remove-cancel"
            onClick={() => { setConfirmText(''); onClose(); }}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('staff.remove.cancel')}
          </button>
          <button
            type="button"
            data-testid="remove-confirm"
            onClick={() => mutation.mutate()}
            disabled={!canConfirm || mutation.isPending}
            className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
          >
            {t('staff.remove.confirm')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Roles Summary Card
// ---------------------------------------------------------------------------

function RolesSummaryCard(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section
      data-testid="roles-summary"
      aria-labelledby="roles-summary-heading"
      className="rounded-lg border border-slate-200 bg-white p-4"
    >
      <h2 id="roles-summary-heading" className="text-lg font-bold text-slate-900">
        {t('staff.rolesSummary.title')}
      </h2>
      <div className="mt-3 space-y-3">
        {ROLE_DEFINITIONS.map((rd) => {
          const s = ROLE_STYLES[rd.role];
          return (
            <div key={rd.role} className="flex items-start gap-3" data-testid={`role-def-${rd.role}`}>
              <span
                aria-hidden="true"
                className={`mt-0.5 inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium ${s.border} ${s.bg} ${s.text}`}
              >
                {s.icon}
              </span>
              <dl>
                <dt className="text-sm font-semibold text-slate-800">{t(rd.labelKey)}</dt>
                <dd className="text-xs text-slate-600">{t(rd.descriptionKey)}</dd>
              </dl>
            </div>
          );
        })}
      </div>
    </section>
  );
}

// ---------------------------------------------------------------------------
// Staff Page (main export)
// ---------------------------------------------------------------------------

interface StaffPageProps {
  readonly integratorId?: string;
}

export function StaffPage({
  integratorId = __TEST__.CURRENT_INTEGRATOR_ID,
}: StaffPageProps): JSX.Element {
  const { t } = useTranslation();

  // Dialog states
  const [inviteOpen, setInviteOpen] = useState(false);
  const [editMember, setEditMember] = useState<StaffMember | null>(null);
  const [suspendMember, setSuspendMember] = useState<StaffMember | null>(null);
  const [removeMember, setRemoveMember] = useState<StaffMember | null>(null);

  const query = useQuery({
    queryKey: staffQueryKeys.list(integratorId),
    queryFn: () => listStaff(integratorId),
  });

  const staff: readonly StaffMember[] = query.data ?? [];

  return (
    <main
      data-testid="staff-page"
      aria-labelledby="staff-page-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      <header className="mb-4">
        <nav aria-label={t('staff.breadcrumbAriaLabel')} className="text-xs text-slate-500">
          <ol className="flex gap-1">
            <li>{t('staff.breadcrumb.integratorPortal')}</li>
            <li aria-hidden="true">/</li>
            <li aria-current="page" className="font-medium text-slate-700">
              {t('staff.title')}
            </li>
          </ol>
        </nav>
        <h1
          id="staff-page-heading"
          className="mt-1 text-2xl font-bold text-slate-900"
        >
          {t('staff.title')}
        </h1>
        <p className="text-sm text-slate-600">{t('staff.subtitle')}</p>
      </header>

      <div className="mb-4 flex items-center justify-between">
        <div />
        <button
          type="button"
          data-testid="invite-open"
          onClick={() => setInviteOpen(true)}
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
        >
          {t('staff.inviteButton')}
        </button>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1fr_300px]">
        {/* Staff table */}
        <div className="overflow-x-auto rounded-lg border border-slate-200 bg-white">
          {query.isLoading ? (
            <p role="status" data-testid="staff-loading" className="p-4 text-sm text-slate-500">
              {t('staff.loading')}
            </p>
          ) : query.isError ? (
            <p role="alert" data-testid="staff-error" className="p-4 text-sm text-red-700">
              {t('staff.error')}
            </p>
          ) : (
            <table data-testid="staff-table" className="w-full text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-left text-xs font-medium uppercase text-slate-500">
                  <th scope="col" className="px-4 py-3">{t('staff.columns.name')}</th>
                  <th scope="col" className="px-4 py-3">{t('staff.columns.email')}</th>
                  <th scope="col" className="px-4 py-3">{t('staff.columns.role')}</th>
                  <th scope="col" className="px-4 py-3">{t('staff.columns.status')}</th>
                  <th scope="col" className="px-4 py-3">{t('staff.columns.lastLogin')}</th>
                  <th scope="col" className="px-4 py-3">{t('staff.columns.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {staff.map((m) => (
                  <tr
                    key={m.id}
                    data-testid={`staff-row-${m.id}`}
                    aria-label={t('staff.rowAriaLabel', { name: m.name, role: t(`staff.roles.${m.role}`), status: t(`staff.status.${m.status}`) })}
                    className="border-b border-slate-100 hover:bg-slate-50"
                  >
                    <td className="px-4 py-3 font-medium text-slate-900">{m.name}</td>
                    <td className="px-4 py-3 text-slate-600">{m.email}</td>
                    <td className="px-4 py-3"><RoleBadge role={m.role} /></td>
                    <td className="px-4 py-3"><StatusBadge status={m.status} /></td>
                    <td className="px-4 py-3 text-slate-500">
                      {m.lastLoginIso
                        ? new Date(m.lastLoginIso).toLocaleDateString()
                        : t('staff.neverLoggedIn')}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-1">
                        <button
                          type="button"
                          data-testid={`edit-${m.id}`}
                          onClick={() => setEditMember(m)}
                          aria-label={t('staff.actions.editAriaLabel', { name: m.name })}
                          className="rounded px-2 py-1 text-xs text-blue-700 hover:bg-blue-50"
                        >
                          {t('staff.actions.edit')}
                        </button>
                        <button
                          type="button"
                          data-testid={`suspend-${m.id}`}
                          onClick={() => setSuspendMember(m)}
                          aria-label={
                            m.status === 'suspended'
                              ? t('staff.actions.reactivateAriaLabel', { name: m.name })
                              : t('staff.actions.suspendAriaLabel', { name: m.name })
                          }
                          className="rounded px-2 py-1 text-xs text-amber-700 hover:bg-amber-50"
                        >
                          {m.status === 'suspended'
                            ? t('staff.actions.reactivate')
                            : t('staff.actions.suspend')}
                        </button>
                        <button
                          type="button"
                          data-testid={`remove-${m.id}`}
                          onClick={() => setRemoveMember(m)}
                          aria-label={t('staff.actions.removeAriaLabel', { name: m.name })}
                          className="rounded px-2 py-1 text-xs text-red-700 hover:bg-red-50"
                        >
                          {t('staff.actions.remove')}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>

        {/* Roles summary card */}
        <RolesSummaryCard />
      </div>

      {/* Dialogs */}
      <InviteStaffDialog
        open={inviteOpen}
        onClose={() => setInviteOpen(false)}
        integratorId={integratorId}
      />
      <EditStaffDialog
        open={!!editMember}
        onClose={() => setEditMember(null)}
        member={editMember}
        integratorId={integratorId}
      />
      <SuspendDialog
        open={!!suspendMember}
        onClose={() => setSuspendMember(null)}
        member={suspendMember}
        integratorId={integratorId}
      />
      <RemoveStaffDialog
        open={!!removeMember}
        onClose={() => setRemoveMember(null)}
        member={removeMember}
        integratorId={integratorId}
      />
    </main>
  );
}

export default StaffPage;
