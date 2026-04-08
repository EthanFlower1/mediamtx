import { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CustomerSummary } from '@/api/customers';

// KAI-309: Impersonation confirmation dialog.
//
// Destructive/sensitive action → requires:
//  - Focus trap while open
//  - Required reason field (aria-describedby wired to the warning)
//  - Audit-log warning text visible before the user can confirm
//  - Clear language that the customer will see an impersonation banner
//
// The actual API call lives one layer up so the route can navigate
// on success. This component is purely view + validation.

export interface ImpersonateConfirmDialogProps {
  readonly customer: CustomerSummary;
  readonly open: boolean;
  readonly onCancel: () => void;
  readonly onConfirm: (reason: string) => void | Promise<void>;
}

export function ImpersonateConfirmDialog({
  customer,
  open,
  onCancel,
  onConfirm,
}: ImpersonateConfirmDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const headingId = useId();
  const warningId = useId();
  const reasonId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const reasonRef = useRef<HTMLTextAreaElement>(null);
  const [reason, setReason] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) {
      setReason('');
      setError(null);
      setSubmitting(false);
      return;
    }
    // Focus the reason field when opening, for quick input and to
    // seed the focus trap.
    const id = window.setTimeout(() => reasonRef.current?.focus(), 0);
    return () => window.clearTimeout(id);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault();
        onCancel();
        return;
      }
      if (e.key !== 'Tab') return;
      // Focus trap inside the dialog.
      const root = dialogRef.current;
      if (!root) return;
      const focusables = root.querySelectorAll<HTMLElement>(
        'button, textarea, input, select, [tabindex]:not([tabindex="-1"])',
      );
      if (focusables.length === 0) return;
      const first = focusables[0]!;
      const last = focusables[focusables.length - 1]!;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onCancel]);

  if (!open) return null;

  async function submit() {
    const trimmed = reason.trim();
    if (!trimmed) {
      setError(t('customers.impersonate.errors.reasonRequired'));
      reasonRef.current?.focus();
      return;
    }
    setSubmitting(true);
    try {
      await onConfirm(trimmed);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      data-testid="impersonate-backdrop"
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={headingId}
        aria-describedby={warningId}
        data-testid="impersonate-dialog"
        className="w-full max-w-md rounded-lg bg-white p-5 shadow-xl"
      >
        <h2 id={headingId} className="text-lg font-semibold text-slate-900">
          {t('customers.impersonate.title', { name: customer.name })}
        </h2>
        <p id={warningId} className="mt-2 rounded border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900">
          <strong className="block font-semibold">
            {t('customers.impersonate.warningTitle')}
          </strong>
          <span>{t('customers.impersonate.warningBody')}</span>
        </p>
        <p className="mt-2 text-sm text-slate-700">
          {t('customers.impersonate.bannerNotice')}
        </p>
        <div className="mt-4">
          <label htmlFor={reasonId} className="block text-sm font-medium text-slate-800">
            {t('customers.impersonate.reasonLabel')}
            <span aria-hidden="true" className="text-red-600">
              *
            </span>
          </label>
          <textarea
            id={reasonId}
            ref={reasonRef}
            data-testid="impersonate-reason"
            required
            rows={3}
            aria-required="true"
            aria-invalid={error ? 'true' : 'false'}
            aria-describedby={warningId}
            value={reason}
            onChange={(e) => {
              setReason(e.target.value);
              if (error) setError(null);
            }}
            className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          {error ? (
            <p
              role="alert"
              data-testid="impersonate-reason-error"
              className="mt-1 text-xs text-red-700"
            >
              {error}
            </p>
          ) : null}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            data-testid="impersonate-cancel"
            onClick={onCancel}
            className="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            {t('customers.impersonate.cancel')}
          </button>
          <button
            type="button"
            data-testid="impersonate-confirm"
            aria-describedby={warningId}
            onClick={submit}
            disabled={submitting}
            className="rounded bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-500 disabled:opacity-60"
          >
            {submitting
              ? t('customers.impersonate.submitting')
              : t('customers.impersonate.confirm')}
          </button>
        </div>
      </div>
    </div>
  );
}
