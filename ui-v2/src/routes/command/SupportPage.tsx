import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { ScreenSharePanel } from '@/components/support/ScreenSharePanel';
import { TicketCreator } from '@/components/support/TicketCreator';
import { TicketIntegrationConfig } from '@/components/support/TicketIntegrationConfig';
import {
  getTicketHookConfig,
  ticketHookConfigQueryKey,
  type CreateTicketResult,
  type ScreenShareSession,
} from '@/api/screenShare';
import { __TEST__ as CUSTOMERS_TEST } from '@/api/customers';

// KAI-469: Integrator support page — screen sharing + ticket integration.
//
// Three-panel layout:
//   1. ScreenSharePanel — initiate/manage screen-share sessions
//   2. TicketCreator — create tickets from customer context (shown on demand)
//   3. TicketIntegrationConfig — configure provider settings
//
// The page accepts an integratorId (defaults to the current mock integrator).
// In production this comes from the session/auth context.

interface SupportPageProps {
  readonly integratorId?: string;
}

export function SupportPage({
  integratorId = CUSTOMERS_TEST.CURRENT_INTEGRATOR_ID,
}: SupportPageProps): JSX.Element {
  const { t } = useTranslation();

  // Customer context for creating tickets / screen shares.
  const [selectedCustomerId, setSelectedCustomerId] = useState('cust-integrator-001-000');
  const [selectedCustomerName, setSelectedCustomerName] = useState('Acme 0 Retail');
  const [selectedRecorderId, setSelectedRecorderId] = useState('rec-001');

  // Ticket creator visibility.
  const [showTicketCreator, setShowTicketCreator] = useState(false);
  const [ticketSession, setTicketSession] = useState<ScreenShareSession | undefined>();
  const [activeTab, setActiveTab] = useState<'sessions' | 'config'>('sessions');

  const hookConfigQuery = useQuery({
    queryKey: ticketHookConfigQueryKey(integratorId),
    queryFn: () => getTicketHookConfig(integratorId),
  });

  function handleCreateTicketFromSession(session: ScreenShareSession) {
    setTicketSession(session);
    setSelectedCustomerId(session.customerId);
    setSelectedCustomerName(session.customerName);
    setSelectedRecorderId(session.recorderId);
    setShowTicketCreator(true);
  }

  function handleTicketCreated(_result: CreateTicketResult) {
    setShowTicketCreator(false);
    setTicketSession(undefined);
  }

  return (
    <div data-testid="support-page" className="min-h-screen bg-slate-50">
      {/* Header */}
      <header className="border-b border-slate-200 bg-white px-6 py-4">
        <h1 className="text-lg font-bold text-slate-900">
          {t('support.page.title')}
        </h1>
        <p className="text-sm text-slate-500">{t('support.page.subtitle')}</p>
      </header>

      {/* Tabs */}
      <nav
        role="tablist"
        aria-label={t('support.page.tabsAriaLabel')}
        className="flex gap-1 border-b border-slate-200 bg-white px-6"
      >
        <button
          role="tab"
          aria-selected={activeTab === 'sessions'}
          data-testid="tab-sessions"
          onClick={() => setActiveTab('sessions')}
          className={`px-3 py-2 text-sm font-medium border-b-2 ${
            activeTab === 'sessions'
              ? 'border-blue-600 text-blue-600'
              : 'border-transparent text-slate-500 hover:text-slate-700'
          }`}
        >
          {t('support.page.tabs.sessions')}
        </button>
        <button
          role="tab"
          aria-selected={activeTab === 'config'}
          data-testid="tab-config"
          onClick={() => setActiveTab('config')}
          className={`px-3 py-2 text-sm font-medium border-b-2 ${
            activeTab === 'config'
              ? 'border-blue-600 text-blue-600'
              : 'border-transparent text-slate-500 hover:text-slate-700'
          }`}
        >
          {t('support.page.tabs.config')}
        </button>
      </nav>

      <div className="mx-auto max-w-5xl p-6 space-y-6">
        {activeTab === 'sessions' && (
          <>
            {/* Customer context selector */}
            <div
              data-testid="customer-context"
              className="flex flex-wrap gap-3 items-end rounded-lg border border-slate-200 bg-white p-4"
            >
              <label className="block space-y-1">
                <span className="text-xs font-medium text-slate-600">
                  {t('support.page.customerIdLabel')}
                </span>
                <input
                  type="text"
                  data-testid="context-customer-id"
                  value={selectedCustomerId}
                  onChange={(e) => setSelectedCustomerId(e.target.value)}
                  className="block rounded border border-slate-300 px-2.5 py-1.5 text-sm"
                />
              </label>
              <label className="block space-y-1">
                <span className="text-xs font-medium text-slate-600">
                  {t('support.page.customerNameLabel')}
                </span>
                <input
                  type="text"
                  data-testid="context-customer-name"
                  value={selectedCustomerName}
                  onChange={(e) => setSelectedCustomerName(e.target.value)}
                  className="block rounded border border-slate-300 px-2.5 py-1.5 text-sm"
                />
              </label>
              <label className="block space-y-1">
                <span className="text-xs font-medium text-slate-600">
                  {t('support.page.recorderIdLabel')}
                </span>
                <input
                  type="text"
                  data-testid="context-recorder-id"
                  value={selectedRecorderId}
                  onChange={(e) => setSelectedRecorderId(e.target.value)}
                  className="block rounded border border-slate-300 px-2.5 py-1.5 text-sm"
                />
              </label>
              <button
                type="button"
                data-testid="create-ticket-standalone"
                onClick={() => {
                  setTicketSession(undefined);
                  setShowTicketCreator(true);
                }}
                className="rounded border border-slate-300 px-3 py-1.5 text-sm font-medium text-slate-700 hover:bg-slate-50"
              >
                {t('support.page.createTicketButton')}
              </button>
            </div>

            {/* Screen share panel */}
            <ScreenSharePanel
              integratorId={integratorId}
              customerId={selectedCustomerId}
              customerName={selectedCustomerName}
              recorderId={selectedRecorderId}
              onCreateTicket={handleCreateTicketFromSession}
            />

            {/* Ticket creator (shown when needed) */}
            {showTicketCreator && (
              <TicketCreator
                integratorId={integratorId}
                customerId={selectedCustomerId}
                customerName={selectedCustomerName}
                recorderId={selectedRecorderId}
                session={ticketSession}
                provider={hookConfigQuery.data?.provider ?? 'internal'}
                onCreated={handleTicketCreated}
                onCancel={() => setShowTicketCreator(false)}
              />
            )}
          </>
        )}

        {activeTab === 'config' && (
          <TicketIntegrationConfig integratorId={integratorId} />
        )}
      </div>
    </div>
  );
}

export default SupportPage;
