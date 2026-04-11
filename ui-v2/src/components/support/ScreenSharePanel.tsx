import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  endSession,
  initiateSession,
  listSessions,
  sessionsQueryKey,
  type InitiateSessionSpec,
  type ScreenShareSession,
  type SessionTransport,
} from '@/api/screenShare';

// KAI-469: Screen sharing panel for the integrator support page.
//
// Allows integrator staff to:
//   1. Initiate a screen-share session with a customer recorder
//   2. See active/recent sessions
//   3. End active sessions
//
// Transport selection: WebRTC (default) or Rewind.

interface ScreenSharePanelProps {
  readonly integratorId: string;
  readonly customerId?: string;
  readonly customerName?: string;
  readonly recorderId?: string;
  readonly onSessionCreated?: (session: ScreenShareSession) => void;
  readonly onCreateTicket?: (session: ScreenShareSession) => void;
}

function formatDuration(seconds: number): string {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}m ${s}s`;
}

function statusBadgeClass(status: ScreenShareSession['status']): string {
  switch (status) {
    case 'active':
      return 'bg-green-100 text-green-800';
    case 'pending':
      return 'bg-yellow-100 text-yellow-800';
    case 'completed':
      return 'bg-slate-100 text-slate-600';
    case 'failed':
      return 'bg-red-100 text-red-800';
    case 'cancelled':
      return 'bg-slate-100 text-slate-500';
    default:
      return 'bg-slate-100 text-slate-600';
  }
}

export function ScreenSharePanel({
  integratorId,
  customerId,
  customerName,
  recorderId,
  onSessionCreated,
  onCreateTicket,
}: ScreenSharePanelProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [transport, setTransport] = useState<SessionTransport>('webrtc');
  const [initiating, setInitiating] = useState(false);

  const sessionsQuery = useQuery({
    queryKey: sessionsQueryKey(integratorId, customerId),
    queryFn: () => listSessions(integratorId, customerId),
  });

  const initiateMutation = useMutation({
    mutationFn: (spec: InitiateSessionSpec) => initiateSession(spec),
    onSuccess: (session) => {
      queryClient.invalidateQueries({ queryKey: sessionsQueryKey(integratorId, customerId) });
      setInitiating(false);
      onSessionCreated?.(session);
    },
  });

  const endMutation = useMutation({
    mutationFn: (sessionId: string) => endSession(sessionId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionsQueryKey(integratorId, customerId) });
    },
  });

  const sessions = sessionsQuery.data ?? [];
  const activeSession = sessions.find((s) => s.status === 'active' || s.status === 'pending');

  function handleInitiate() {
    if (!customerId || !recorderId) return;
    initiateMutation.mutate({
      integratorId,
      customerId,
      recorderId,
      transport,
    });
  }

  return (
    <section
      data-testid="screen-share-panel"
      aria-labelledby="screen-share-heading"
      className="rounded-lg border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 id="screen-share-heading" className="text-sm font-semibold text-slate-800">
          {t('support.screenShare.heading')}
        </h2>
        <p className="text-xs text-slate-500">{t('support.screenShare.description')}</p>
      </div>

      <div className="p-4 space-y-4">
        {/* Initiate new session */}
        {!activeSession && (
          <div data-testid="screen-share-initiate" className="space-y-3">
            {customerName && (
              <p className="text-sm text-slate-700">
                {t('support.screenShare.initiateFor', { customer: customerName })}
              </p>
            )}

            <fieldset className="space-y-2">
              <legend className="text-xs font-medium text-slate-600">
                {t('support.screenShare.transportLabel')}
              </legend>
              <div className="flex gap-3">
                <label className="flex items-center gap-1.5 text-sm">
                  <input
                    type="radio"
                    name="transport"
                    value="webrtc"
                    checked={transport === 'webrtc'}
                    onChange={() => setTransport('webrtc')}
                    className="text-blue-600"
                    data-testid="transport-webrtc"
                  />
                  WebRTC
                </label>
                <label className="flex items-center gap-1.5 text-sm">
                  <input
                    type="radio"
                    name="transport"
                    value="rewind"
                    checked={transport === 'rewind'}
                    onChange={() => setTransport('rewind')}
                    className="text-blue-600"
                    data-testid="transport-rewind"
                  />
                  Rewind
                </label>
              </div>
            </fieldset>

            {initiating ? (
              <div className="flex gap-2">
                <button
                  type="button"
                  data-testid="screen-share-confirm"
                  onClick={handleInitiate}
                  disabled={!customerId || !recorderId || initiateMutation.isPending}
                  className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-40"
                >
                  {initiateMutation.isPending
                    ? t('support.screenShare.connecting')
                    : t('support.screenShare.confirmStart')}
                </button>
                <button
                  type="button"
                  data-testid="screen-share-cancel-initiate"
                  onClick={() => setInitiating(false)}
                  className="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-600 hover:bg-slate-50"
                >
                  {t('common.cancel')}
                </button>
              </div>
            ) : (
              <button
                type="button"
                data-testid="screen-share-start"
                onClick={() => setInitiating(true)}
                disabled={!customerId || !recorderId}
                className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-40"
              >
                {t('support.screenShare.startButton')}
              </button>
            )}

            {initiateMutation.isError && (
              <p role="alert" className="text-xs text-red-600">
                {t('support.screenShare.initError')}
              </p>
            )}
          </div>
        )}

        {/* Active session */}
        {activeSession && (
          <div
            data-testid="screen-share-active"
            className="rounded-lg border-2 border-green-300 bg-green-50 p-3 space-y-2"
          >
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-green-800">
                {t('support.screenShare.activeSession')}
              </span>
              <span className={`rounded-full px-2 py-0.5 text-xs font-medium ${statusBadgeClass(activeSession.status)}`}>
                {activeSession.status}
              </span>
            </div>
            <dl className="grid grid-cols-2 gap-1 text-xs text-slate-600">
              <dt className="font-medium">{t('support.screenShare.transport')}</dt>
              <dd>{activeSession.transport}</dd>
              <dt className="font-medium">{t('support.screenShare.customer')}</dt>
              <dd>{activeSession.customerName}</dd>
              <dt className="font-medium">{t('support.screenShare.recorder')}</dt>
              <dd>{activeSession.recorderId}</dd>
            </dl>
            <div className="flex gap-2 pt-1">
              <button
                type="button"
                data-testid="screen-share-end"
                onClick={() => endMutation.mutate(activeSession.sessionId)}
                disabled={endMutation.isPending}
                className="rounded border border-red-300 bg-white px-2.5 py-1 text-xs font-medium text-red-700 hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-500 disabled:opacity-40"
              >
                {t('support.screenShare.endButton')}
              </button>
              {onCreateTicket && (
                <button
                  type="button"
                  data-testid="screen-share-create-ticket"
                  onClick={() => onCreateTicket(activeSession)}
                  className="rounded border border-slate-300 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
                >
                  {t('support.screenShare.createTicketButton')}
                </button>
              )}
            </div>
          </div>
        )}

        {/* Session history */}
        {sessions.length > 0 && (
          <div data-testid="screen-share-history">
            <h3 className="text-xs font-medium text-slate-600 mb-2">
              {t('support.screenShare.recentSessions')}
            </h3>
            <ul className="space-y-1.5">
              {sessions.map((s) => (
                <li
                  key={s.sessionId}
                  data-testid={`session-${s.sessionId}`}
                  className="flex items-center justify-between rounded border border-slate-100 bg-slate-50 px-2.5 py-1.5 text-xs"
                >
                  <div className="flex items-center gap-2">
                    <span className={`rounded-full px-1.5 py-0.5 font-medium ${statusBadgeClass(s.status)}`}>
                      {s.status}
                    </span>
                    <span className="text-slate-700">{s.customerName}</span>
                    <span className="text-slate-400">{s.transport}</span>
                  </div>
                  <div className="flex items-center gap-2 text-slate-500">
                    {s.durationSeconds > 0 && <span>{formatDuration(s.durationSeconds)}</span>}
                    {s.linkedTicketId && (
                      <span className="rounded bg-blue-50 px-1.5 py-0.5 text-blue-700">
                        {s.linkedTicketId}
                      </span>
                    )}
                    <span>{new Date(s.createdAtIso).toLocaleDateString()}</span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        )}

        {sessionsQuery.isLoading && (
          <p role="status" className="text-xs text-slate-500">
            {t('support.screenShare.loading')}
          </p>
        )}
      </div>
    </section>
  );
}
