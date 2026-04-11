import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listKeys,
  createKey,
  rotateKey,
  revokeKey,
  getKeyAuditLog,
  apiKeysQueryKeys,
  AVAILABLE_SCOPES,
  __TEST__,
} from '@/api/apiKeys';
import type {
  APIKeyRecord,
  KeyStatus,
  CreateKeyResult,
  RotateKeyResult,
  AuditLogEntry,
} from '@/api/apiKeys';

// KAI-319: Integrator Portal API Keys Management page.
//
// Lets integrators manage API keys for programmatic access:
//   - Table with status badges (icon+text+border, never color alone)
//   - Generate key dialog (name, scopes, expiry) with one-time display
//   - Rotate dialog with configurable grace period
//   - Revoke confirmation with type-to-confirm
//   - Per-key audit log drawer
//   - Section 508/WCAG 2.1 AA compliant

const CURRENT_TENANT_ID = __TEST__.CURRENT_TENANT_ID;

// ---------------------------------------------------------------------------
// Status badge -- icon+text+border (severity encoding, never color alone)
// ---------------------------------------------------------------------------

const STATUS_STYLES: Record<KeyStatus, { border: string; bg: string; text: string; label: string }> = {
  active: { border: 'border-green-600', bg: 'bg-green-50', text: 'text-green-800', label: 'Active' },
  expiring: { border: 'border-yellow-600', bg: 'bg-yellow-50', text: 'text-yellow-800', label: 'Expiring Soon' },
  expired: { border: 'border-slate-500', bg: 'bg-slate-100', text: 'text-slate-700', label: 'Expired' },
  revoked: { border: 'border-red-600', bg: 'bg-red-50', text: 'text-red-800', label: 'Revoked' },
  grace: { border: 'border-amber-600', bg: 'bg-amber-50', text: 'text-amber-800', label: 'Grace Period' },
};

function StatusBadge({ status }: { readonly status: KeyStatus }): JSX.Element {
  const { t } = useTranslation();
  const s = STATUS_STYLES[status];
  return (
    <span
      data-testid={`status-badge-${status}`}
      className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${s.border} ${s.bg} ${s.text}`}
    >
      {t(`apiKeys.status.${status}`)}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Scope tags
// ---------------------------------------------------------------------------

function ScopeTags({ scopes }: { readonly scopes: readonly string[] }): JSX.Element {
  const { t } = useTranslation();
  if (scopes.length === 0) {
    return (
      <span className="text-xs text-slate-500 italic">{t('apiKeys.fullAccess')}</span>
    );
  }
  return (
    <div className="flex flex-wrap gap-1">
      {scopes.map((scope) => (
        <span
          key={scope}
          className="inline-flex rounded bg-slate-100 px-1.5 py-0.5 text-xs font-mono text-slate-700"
        >
          {scope}
        </span>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// One-time key display (used after create and rotate)
// ---------------------------------------------------------------------------

interface KeyDisplayProps {
  readonly rawKey: string;
  readonly onDismiss: () => void;
}

function OneTimeKeyDisplay({ rawKey, onDismiss }: KeyDisplayProps): JSX.Element {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(rawKey);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback: select the text in the input.
      const el = document.getElementById('raw-key-display') as HTMLInputElement | null;
      el?.select();
    }
  }, [rawKey]);

  return (
    <div
      role="alert"
      data-testid="one-time-key-display"
      className="rounded-lg border-2 border-amber-500 bg-amber-50 p-4"
    >
      <p className="text-sm font-semibold text-amber-900">
        {t('apiKeys.oneTimeWarning')}
      </p>
      <div className="mt-2 flex items-center gap-2">
        <input
          id="raw-key-display"
          data-testid="raw-key-value"
          type="text"
          readOnly
          value={rawKey}
          className="flex-1 rounded border border-amber-300 bg-white px-3 py-2 font-mono text-sm text-slate-900"
          aria-label={t('apiKeys.rawKeyLabel')}
        />
        <button
          type="button"
          data-testid="copy-key-btn"
          onClick={() => void handleCopy()}
          className="rounded-md bg-amber-600 px-3 py-2 text-sm font-medium text-white hover:bg-amber-700"
        >
          {copied ? t('apiKeys.copied') : t('apiKeys.copy')}
        </button>
      </div>
      <div className="mt-3 flex justify-end">
        <button
          type="button"
          data-testid="dismiss-key-display"
          onClick={onDismiss}
          className="rounded-md border border-amber-400 px-3 py-1.5 text-sm text-amber-800 hover:bg-amber-100"
        >
          {t('apiKeys.dismiss')}
        </button>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Generate Key Dialog
// ---------------------------------------------------------------------------

interface GenerateKeyDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onCreated: (result: CreateKeyResult) => void;
}

function GenerateKeyDialog({ open, onClose, onCreated }: GenerateKeyDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const [selectedScopes, setSelectedScopes] = useState<string[]>([]);
  const [expiryDays, setExpiryDays] = useState<string>('90');
  const [noExpiry, setNoExpiry] = useState(false);
  const [nameError, setNameError] = useState('');

  const mutation = useMutation({
    mutationFn: () =>
      createKey({
        tenantId: CURRENT_TENANT_ID,
        name,
        scopes: selectedScopes,
        expiresInDays: noExpiry ? null : parseInt(expiryDays, 10) || 90,
      }),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: apiKeysQueryKeys.all(CURRENT_TENANT_ID) });
      onCreated(result);
      setName('');
      setSelectedScopes([]);
      setExpiryDays('90');
      setNoExpiry(false);
      setNameError('');
      onClose();
    },
  });

  const handleSubmit = useCallback(() => {
    if (!name.trim()) {
      setNameError(t('apiKeys.generate.nameError'));
      return;
    }
    setNameError('');
    mutation.mutate();
  }, [name, t, mutation]);

  const toggleScope = useCallback((scope: string) => {
    setSelectedScopes((prev) =>
      prev.includes(scope) ? prev.filter((s) => s !== scope) : [...prev, scope],
    );
  }, []);

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="generate-dialog-title"
      data-testid="generate-key-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-lg rounded-lg bg-white p-6 shadow-xl">
        <h2 id="generate-dialog-title" className="text-lg font-bold text-slate-900">
          {t('apiKeys.generate.title')}
        </h2>

        <div className="mt-4 space-y-4">
          {/* Name */}
          <div>
            <label htmlFor="key-name" className="block text-sm font-medium text-slate-700">
              {t('apiKeys.generate.nameLabel')}
            </label>
            <input
              id="key-name"
              data-testid="key-name-input"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('apiKeys.generate.namePlaceholder')}
              className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
              aria-describedby={nameError ? 'key-name-error' : undefined}
              aria-invalid={nameError ? 'true' : undefined}
            />
            {nameError && (
              <p id="key-name-error" data-testid="key-name-error" role="alert" className="mt-1 text-xs text-red-700">
                {nameError}
              </p>
            )}
          </div>

          {/* Scopes */}
          <fieldset>
            <legend className="text-sm font-medium text-slate-700">
              {t('apiKeys.generate.scopesLabel')}
            </legend>
            <p className="text-xs text-slate-500 mt-0.5">{t('apiKeys.generate.scopesHint')}</p>
            <div className="mt-2 grid grid-cols-2 gap-2" data-testid="scope-checkboxes">
              {AVAILABLE_SCOPES.map((scope) => (
                <label
                  key={scope.value}
                  className="flex items-center gap-2 text-sm text-slate-700"
                >
                  <input
                    type="checkbox"
                    checked={selectedScopes.includes(scope.value)}
                    onChange={() => toggleScope(scope.value)}
                    className="rounded border-slate-300"
                    data-testid={`scope-${scope.value}`}
                  />
                  <span className="font-mono text-xs">{scope.value}</span>
                </label>
              ))}
            </div>
          </fieldset>

          {/* Expiry */}
          <div>
            <label htmlFor="key-expiry" className="block text-sm font-medium text-slate-700">
              {t('apiKeys.generate.expiryLabel')}
            </label>
            <div className="mt-1 flex items-center gap-3">
              <input
                id="key-expiry"
                data-testid="key-expiry-input"
                type="number"
                min="1"
                max="365"
                value={expiryDays}
                onChange={(e) => setExpiryDays(e.target.value)}
                disabled={noExpiry}
                className="w-24 rounded-md border border-slate-300 px-3 py-2 text-sm disabled:opacity-50"
                aria-label={t('apiKeys.generate.expiryDaysLabel')}
              />
              <span className="text-sm text-slate-600">{t('apiKeys.generate.days')}</span>
              <label className="flex items-center gap-1.5 text-sm text-slate-700">
                <input
                  type="checkbox"
                  data-testid="no-expiry-checkbox"
                  checked={noExpiry}
                  onChange={(e) => setNoExpiry(e.target.checked)}
                  className="rounded border-slate-300"
                />
                {t('apiKeys.generate.noExpiry')}
              </label>
            </div>
          </div>
        </div>

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="generate-cancel"
            onClick={onClose}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('apiKeys.generate.cancel')}
          </button>
          <button
            type="button"
            data-testid="generate-submit"
            onClick={handleSubmit}
            disabled={mutation.isPending}
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {mutation.isPending ? t('apiKeys.generate.generating') : t('apiKeys.generate.submit')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Rotate Key Dialog
// ---------------------------------------------------------------------------

interface RotateDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly keyRecord: APIKeyRecord | null;
  readonly onRotated: (result: RotateKeyResult) => void;
}

function RotateKeyDialog({ open, onClose, keyRecord, onRotated }: RotateDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [graceHours, setGraceHours] = useState('24');

  const mutation = useMutation({
    mutationFn: () =>
      rotateKey({
        tenantId: CURRENT_TENANT_ID,
        keyId: keyRecord!.id,
        gracePeriodHours: parseInt(graceHours, 10) || 24,
      }),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: apiKeysQueryKeys.all(CURRENT_TENANT_ID) });
      onRotated(result);
      setGraceHours('24');
      onClose();
    },
  });

  if (!open || !keyRecord) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="rotate-dialog-title"
      data-testid="rotate-key-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h2 id="rotate-dialog-title" className="text-lg font-bold text-slate-900">
          {t('apiKeys.rotate.title')}
        </h2>
        <p className="mt-2 text-sm text-slate-600">
          {t('apiKeys.rotate.description', { name: keyRecord.name, prefix: keyRecord.keyPrefix })}
        </p>

        <div className="mt-4">
          <label htmlFor="grace-period" className="block text-sm font-medium text-slate-700">
            {t('apiKeys.rotate.graceLabel')}
          </label>
          <p className="text-xs text-slate-500 mt-0.5">{t('apiKeys.rotate.graceHint')}</p>
          <div className="mt-1 flex items-center gap-2">
            <input
              id="grace-period"
              data-testid="grace-period-input"
              type="number"
              min="1"
              max="168"
              value={graceHours}
              onChange={(e) => setGraceHours(e.target.value)}
              className="w-24 rounded-md border border-slate-300 px-3 py-2 text-sm"
            />
            <span className="text-sm text-slate-600">{t('apiKeys.rotate.hours')}</span>
          </div>
        </div>

        <div className="mt-4 rounded-md border border-amber-300 bg-amber-50 p-3">
          <p className="text-xs text-amber-800">
            {t('apiKeys.rotate.warning')}
          </p>
        </div>

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="rotate-cancel"
            onClick={onClose}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('apiKeys.rotate.cancel')}
          </button>
          <button
            type="button"
            data-testid="rotate-submit"
            onClick={() => mutation.mutate()}
            disabled={mutation.isPending}
            className="rounded-md bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-700 disabled:opacity-50"
          >
            {mutation.isPending ? t('apiKeys.rotate.rotating') : t('apiKeys.rotate.submit')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Revoke Confirmation Dialog
// ---------------------------------------------------------------------------

interface RevokeConfirmProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly keyRecord: APIKeyRecord | null;
}

function RevokeConfirmDialog({ open, onClose, keyRecord }: RevokeConfirmProps): JSX.Element | null {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [confirmText, setConfirmText] = useState('');

  const mutation = useMutation({
    mutationFn: () => revokeKey(CURRENT_TENANT_ID, keyRecord!.id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: apiKeysQueryKeys.all(CURRENT_TENANT_ID) });
      setConfirmText('');
      onClose();
    },
  });

  if (!open || !keyRecord) return null;

  const confirmWord = 'REVOKE';
  const canSubmit = confirmText === confirmWord;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="revoke-dialog-title"
      data-testid="revoke-confirm-dialog"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
        <h2 id="revoke-dialog-title" className="text-lg font-bold text-red-900">
          {t('apiKeys.revoke.title')}
        </h2>
        <p className="mt-2 text-sm text-slate-600">
          {t('apiKeys.revoke.description', { name: keyRecord.name, prefix: keyRecord.keyPrefix })}
        </p>

        <div className="mt-4 rounded-md border border-red-300 bg-red-50 p-3">
          <p className="text-xs text-red-800">
            {t('apiKeys.revoke.warning')}
          </p>
        </div>

        <div className="mt-4">
          <label htmlFor="revoke-confirm" className="block text-sm font-medium text-slate-700">
            {t('apiKeys.revoke.confirmLabel', { word: confirmWord })}
          </label>
          <input
            id="revoke-confirm"
            data-testid="revoke-confirm-input"
            type="text"
            value={confirmText}
            onChange={(e) => setConfirmText(e.target.value)}
            className="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm"
            autoComplete="off"
          />
        </div>

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            data-testid="revoke-cancel"
            onClick={() => {
              setConfirmText('');
              onClose();
            }}
            className="rounded-md border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('apiKeys.revoke.cancel')}
          </button>
          <button
            type="button"
            data-testid="revoke-submit"
            onClick={() => mutation.mutate()}
            disabled={!canSubmit || mutation.isPending}
            className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
          >
            {mutation.isPending ? t('apiKeys.revoke.revoking') : t('apiKeys.revoke.submit')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Audit Log Drawer
// ---------------------------------------------------------------------------

const AUDIT_ACTION_LABELS: Record<AuditLogEntry['action'], string> = {
  create: 'Created',
  rotate: 'Rotated',
  revoke: 'Revoked',
  authenticate: 'Authenticated',
  auth_fail: 'Auth Failed',
};

interface AuditDrawerProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly keyRecord: APIKeyRecord | null;
}

function AuditLogDrawer({ open, onClose, keyRecord }: AuditDrawerProps): JSX.Element | null {
  const { t } = useTranslation();

  const auditQuery = useQuery({
    queryKey: apiKeysQueryKeys.auditLog(CURRENT_TENANT_ID, keyRecord?.id ?? ''),
    queryFn: () => getKeyAuditLog(CURRENT_TENANT_ID, keyRecord?.id ?? ''),
    enabled: open && !!keyRecord,
  });

  if (!open || !keyRecord) return null;

  const entries: readonly AuditLogEntry[] = auditQuery.data ?? [];

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="audit-drawer-title"
      data-testid="audit-log-drawer"
      className="fixed inset-0 z-50 flex justify-end bg-black/40"
    >
      <div className="h-full w-full max-w-lg overflow-y-auto bg-white p-6 shadow-xl">
        <div className="flex items-center justify-between">
          <h2 id="audit-drawer-title" className="text-lg font-bold text-slate-900">
            {t('apiKeys.audit.title', { name: keyRecord.name })}
          </h2>
          <button
            type="button"
            data-testid="audit-close"
            onClick={onClose}
            className="rounded-md p-1 text-slate-500 hover:text-slate-700"
            aria-label={t('apiKeys.audit.close')}
          >
            [X]
          </button>
        </div>

        <p className="mt-1 text-xs text-slate-500 font-mono">
          {keyRecord.keyPrefix}... | {keyRecord.id}
        </p>

        {auditQuery.isLoading && (
          <p className="mt-4 text-sm text-slate-500" role="status">{t('apiKeys.audit.loading')}</p>
        )}

        {entries.length === 0 && !auditQuery.isLoading && (
          <p className="mt-4 text-sm text-slate-500">{t('apiKeys.audit.empty')}</p>
        )}

        <ul className="mt-4 space-y-3" data-testid="audit-entries">
          {entries.map((entry) => (
            <li
              key={entry.id}
              data-testid={`audit-entry-${entry.id}`}
              className="rounded-md border border-slate-200 p-3"
            >
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium text-slate-900">
                  {AUDIT_ACTION_LABELS[entry.action] ?? entry.action}
                </span>
                <time className="text-xs text-slate-500" dateTime={entry.createdAt}>
                  {new Date(entry.createdAt).toLocaleString()}
                </time>
              </div>
              <p className="mt-1 text-xs text-slate-600">
                {t('apiKeys.audit.actor')}: {entry.actorId}
              </p>
              {entry.ipAddress && (
                <p className="text-xs text-slate-500">
                  IP: {entry.ipAddress}
                </p>
              )}
            </li>
          ))}
        </ul>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Key List Table
// ---------------------------------------------------------------------------

interface KeyListTableProps {
  readonly keys: readonly APIKeyRecord[];
  readonly onRotate: (key: APIKeyRecord) => void;
  readonly onRevoke: (key: APIKeyRecord) => void;
  readonly onViewAudit: (key: APIKeyRecord) => void;
}

function KeyListTable({ keys, onRotate, onRevoke, onViewAudit }: KeyListTableProps): JSX.Element {
  const { t } = useTranslation();

  if (keys.length === 0) {
    return (
      <div className="rounded-lg border-2 border-dashed border-slate-300 p-8 text-center" data-testid="empty-keys">
        <p className="text-sm text-slate-500">{t('apiKeys.table.empty')}</p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-left text-sm" data-testid="api-keys-table">
        <thead>
          <tr className="border-b border-slate-200 bg-slate-50">
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">{t('apiKeys.table.name')}</th>
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">{t('apiKeys.table.prefix')}</th>
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">{t('apiKeys.table.scopes')}</th>
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">{t('apiKeys.table.status')}</th>
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">{t('apiKeys.table.lastUsed')}</th>
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">{t('apiKeys.table.created')}</th>
            <th scope="col" className="px-4 py-3 font-medium text-slate-700">
              <span className="sr-only">{t('apiKeys.table.actions')}</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {keys.map((key) => {
            const isRevoked = key.status === 'revoked';
            return (
              <tr
                key={key.id}
                data-testid={`key-row-${key.id}`}
                className={`border-b border-slate-100 ${isRevoked ? 'opacity-60' : ''}`}
              >
                <td className="px-4 py-3">
                  <div>
                    <span className="font-medium text-slate-900">{key.name}</span>
                    {key.rotatedFromId && (
                      <span className="ml-1 text-xs text-slate-500">{t('apiKeys.table.rotatedTag')}</span>
                    )}
                  </div>
                  <span className="text-xs text-slate-500">{key.createdBy}</span>
                </td>
                <td className="px-4 py-3 font-mono text-xs text-slate-600">
                  {key.keyPrefix}...
                </td>
                <td className="px-4 py-3">
                  <ScopeTags scopes={key.scopes} />
                </td>
                <td className="px-4 py-3">
                  <StatusBadge status={key.status} />
                </td>
                <td className="px-4 py-3 text-xs text-slate-600">
                  {key.lastUsedAt
                    ? new Date(key.lastUsedAt).toLocaleDateString()
                    : t('apiKeys.table.never')}
                </td>
                <td className="px-4 py-3 text-xs text-slate-600">
                  {new Date(key.createdAt).toLocaleDateString()}
                </td>
                <td className="px-4 py-3">
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      data-testid={`audit-btn-${key.id}`}
                      onClick={() => onViewAudit(key)}
                      className="rounded px-2 py-1 text-xs text-blue-600 hover:bg-blue-50"
                      aria-label={t('apiKeys.table.viewAudit', { name: key.name })}
                    >
                      {t('apiKeys.table.audit')}
                    </button>
                    {!isRevoked && key.status !== 'grace' && (
                      <button
                        type="button"
                        data-testid={`rotate-btn-${key.id}`}
                        onClick={() => onRotate(key)}
                        className="rounded px-2 py-1 text-xs text-amber-600 hover:bg-amber-50"
                        aria-label={t('apiKeys.table.rotateLabel', { name: key.name })}
                      >
                        {t('apiKeys.table.rotate')}
                      </button>
                    )}
                    {!isRevoked && (
                      <button
                        type="button"
                        data-testid={`revoke-btn-${key.id}`}
                        onClick={() => onRevoke(key)}
                        className="rounded px-2 py-1 text-xs text-red-600 hover:bg-red-50"
                        aria-label={t('apiKeys.table.revokeLabel', { name: key.name })}
                      >
                        {t('apiKeys.table.revoke')}
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// APIKeysPage (exported as default for lazy-loading)
// ---------------------------------------------------------------------------

export function APIKeysPage(): JSX.Element {
  const { t } = useTranslation();

  // Dialogs
  const [generateOpen, setGenerateOpen] = useState(false);
  const [rotateTarget, setRotateTarget] = useState<APIKeyRecord | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<APIKeyRecord | null>(null);
  const [auditTarget, setAuditTarget] = useState<APIKeyRecord | null>(null);

  // One-time key display (after create or rotate)
  const [displayedKey, setDisplayedKey] = useState<string | null>(null);

  // Include revoked keys toggle
  const [showRevoked, setShowRevoked] = useState(false);

  const keysQuery = useQuery({
    queryKey: apiKeysQueryKeys.list(CURRENT_TENANT_ID),
    queryFn: () => listKeys(CURRENT_TENANT_ID),
  });

  const allKeys: readonly APIKeyRecord[] = keysQuery.data ?? [];
  const visibleKeys = showRevoked ? allKeys : allKeys.filter((k) => k.status !== 'revoked');

  const handleCreated = useCallback((result: CreateKeyResult) => {
    setDisplayedKey(result.rawKey);
  }, []);

  const handleRotated = useCallback((result: RotateKeyResult) => {
    setDisplayedKey(result.rawKey);
  }, []);

  return (
    <div className="mx-auto max-w-6xl px-4 py-6" data-testid="api-keys-page">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">{t('apiKeys.title')}</h1>
          <p className="mt-1 text-sm text-slate-500">{t('apiKeys.subtitle')}</p>
        </div>
        <button
          type="button"
          data-testid="generate-key-btn"
          onClick={() => setGenerateOpen(true)}
          className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
        >
          {t('apiKeys.generateButton')}
        </button>
      </div>

      {/* One-time key display */}
      {displayedKey && (
        <div className="mt-4">
          <OneTimeKeyDisplay rawKey={displayedKey} onDismiss={() => setDisplayedKey(null)} />
        </div>
      )}

      {/* Filter */}
      <div className="mt-4 flex items-center gap-3">
        <label className="flex items-center gap-1.5 text-sm text-slate-700">
          <input
            type="checkbox"
            data-testid="show-revoked-checkbox"
            checked={showRevoked}
            onChange={(e) => setShowRevoked(e.target.checked)}
            className="rounded border-slate-300"
          />
          {t('apiKeys.showRevoked')}
        </label>
        <span className="text-xs text-slate-500">
          {t('apiKeys.keyCount', { count: visibleKeys.length })}
        </span>
      </div>

      {/* Loading */}
      {keysQuery.isLoading && (
        <p className="mt-6 text-sm text-slate-500" role="status">{t('apiKeys.loading')}</p>
      )}

      {/* Error */}
      {keysQuery.isError && (
        <div className="mt-6 rounded-md border border-red-300 bg-red-50 p-4" role="alert">
          <p className="text-sm text-red-800">{t('apiKeys.error')}</p>
        </div>
      )}

      {/* Table */}
      {!keysQuery.isLoading && !keysQuery.isError && (
        <div className="mt-4">
          <KeyListTable
            keys={visibleKeys}
            onRotate={(key) => setRotateTarget(key)}
            onRevoke={(key) => setRevokeTarget(key)}
            onViewAudit={(key) => setAuditTarget(key)}
          />
        </div>
      )}

      {/* Dialogs */}
      <GenerateKeyDialog
        open={generateOpen}
        onClose={() => setGenerateOpen(false)}
        onCreated={handleCreated}
      />
      <RotateKeyDialog
        open={!!rotateTarget}
        onClose={() => setRotateTarget(null)}
        keyRecord={rotateTarget}
        onRotated={handleRotated}
      />
      <RevokeConfirmDialog
        open={!!revokeTarget}
        onClose={() => setRevokeTarget(null)}
        keyRecord={revokeTarget}
      />
      <AuditLogDrawer
        open={!!auditTarget}
        onClose={() => setAuditTarget(null)}
        keyRecord={auditTarget}
      />
    </div>
  );
}

export default APIKeysPage;
