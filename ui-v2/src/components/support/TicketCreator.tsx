import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation } from '@tanstack/react-query';
import {
  createTicketFromContext,
  type CreateTicketRequest,
  type CreateTicketResult,
  type ScreenShareSession,
  type TicketHookProvider,
} from '@/api/screenShare';

// KAI-469: Ticket creator dialog for the integrator support page.
//
// Creates a support ticket pre-populated with customer + recorder context,
// optionally linked to an active screen-share session. The ticket is
// dispatched to the configured provider (Zendesk, Freshdesk, or internal).

interface TicketCreatorProps {
  readonly integratorId: string;
  readonly customerId: string;
  readonly customerName: string;
  readonly recorderId: string;
  readonly session?: ScreenShareSession;
  readonly provider?: TicketHookProvider;
  readonly onCreated?: (result: CreateTicketResult) => void;
  readonly onCancel?: () => void;
}

export function TicketCreator({
  integratorId,
  customerId,
  customerName,
  recorderId,
  session,
  provider = 'internal',
  onCreated,
  onCancel,
}: TicketCreatorProps): JSX.Element {
  const { t } = useTranslation();
  const [subject, setSubject] = useState(
    session
      ? t('support.tickets.defaultSubject', { customer: customerName })
      : '',
  );
  const [description, setDescription] = useState('');
  const [priority, setPriority] = useState<'low' | 'normal' | 'high' | 'urgent'>('normal');

  const createMutation = useMutation({
    mutationFn: (req: CreateTicketRequest) => createTicketFromContext(req),
    onSuccess: (result) => {
      onCreated?.(result);
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    createMutation.mutate({
      integratorId,
      subject,
      priority,
      context: {
        customerId,
        customerName,
        recorderId,
        sessionId: session?.sessionId,
        description,
      },
    });
  }

  return (
    <form
      data-testid="ticket-creator"
      onSubmit={handleSubmit}
      className="rounded-lg border border-slate-200 bg-white p-4 shadow-sm space-y-4"
    >
      <h3 className="text-sm font-semibold text-slate-800">
        {t('support.tickets.createHeading')}
      </h3>

      {/* Context summary */}
      <dl className="grid grid-cols-2 gap-1 text-xs text-slate-600 rounded bg-slate-50 p-2">
        <dt className="font-medium">{t('support.tickets.customer')}</dt>
        <dd>{customerName}</dd>
        <dt className="font-medium">{t('support.tickets.recorder')}</dt>
        <dd>{recorderId}</dd>
        <dt className="font-medium">{t('support.tickets.provider')}</dt>
        <dd className="capitalize">{provider}</dd>
        {session && (
          <>
            <dt className="font-medium">{t('support.tickets.session')}</dt>
            <dd>{session.sessionId}</dd>
          </>
        )}
      </dl>

      {/* Subject */}
      <label className="block space-y-1">
        <span className="text-xs font-medium text-slate-600">
          {t('support.tickets.subjectLabel')}
        </span>
        <input
          type="text"
          data-testid="ticket-subject"
          value={subject}
          onChange={(e) => setSubject(e.target.value)}
          required
          className="block w-full rounded border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder={t('support.tickets.subjectPlaceholder')}
        />
      </label>

      {/* Priority */}
      <fieldset className="space-y-1">
        <legend className="text-xs font-medium text-slate-600">
          {t('support.tickets.priorityLabel')}
        </legend>
        <div className="flex gap-3">
          {(['low', 'normal', 'high', 'urgent'] as const).map((p) => (
            <label key={p} className="flex items-center gap-1.5 text-sm">
              <input
                type="radio"
                name="priority"
                value={p}
                checked={priority === p}
                onChange={() => setPriority(p)}
                className="text-blue-600"
                data-testid={`priority-${p}`}
              />
              {t(`support.tickets.priority.${p}`)}
            </label>
          ))}
        </div>
      </fieldset>

      {/* Description */}
      <label className="block space-y-1">
        <span className="text-xs font-medium text-slate-600">
          {t('support.tickets.descriptionLabel')}
        </span>
        <textarea
          data-testid="ticket-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          rows={4}
          className="block w-full rounded border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder={t('support.tickets.descriptionPlaceholder')}
        />
      </label>

      {/* Actions */}
      <div className="flex gap-2">
        <button
          type="submit"
          data-testid="ticket-submit"
          disabled={!subject.trim() || createMutation.isPending}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-40"
        >
          {createMutation.isPending
            ? t('support.tickets.creating')
            : t('support.tickets.createButton')}
        </button>
        {onCancel && (
          <button
            type="button"
            data-testid="ticket-cancel"
            onClick={onCancel}
            className="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-600 hover:bg-slate-50"
          >
            {t('common.cancel')}
          </button>
        )}
      </div>

      {createMutation.isError && (
        <p role="alert" className="text-xs text-red-600">
          {t('support.tickets.createError')}
        </p>
      )}

      {createMutation.isSuccess && createMutation.data && (
        <div
          data-testid="ticket-success"
          className="rounded bg-green-50 border border-green-200 p-2 text-xs text-green-800"
        >
          <p>
            {t('support.tickets.createSuccess', { ticketId: createMutation.data.ticketId })}
          </p>
          {createMutation.data.url && (
            <a
              href={createMutation.data.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-600 underline"
            >
              {t('support.tickets.openInProvider')}
            </a>
          )}
        </div>
      )}
    </form>
  );
}
