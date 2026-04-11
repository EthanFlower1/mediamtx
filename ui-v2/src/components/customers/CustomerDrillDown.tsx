import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  beginImpersonation,
  customerQueryKey,
  getCustomer,
} from '@/api/customers';
import { useImpersonationStore } from '@/stores/impersonation';
import { ImpersonateConfirmDialog } from './ImpersonateConfirmDialog';

// KAI-309: Customer drill-down page.
//
// Tabs: Overview / Cameras / Users / Billing / Activity.
// Only Overview is implemented; the others are "coming soon" stubs
// that link to the v1 ticket tracking them. This keeps the shell
// real and discoverable while unblocking downstream work.

type TabId = 'overview' | 'cameras' | 'users' | 'billing' | 'activity';

const TABS: readonly TabId[] = ['overview', 'cameras', 'users', 'billing', 'activity'];

const TAB_TICKETS: Record<Exclude<TabId, 'overview'>, string> = {
  cameras: 'KAI-321',
  users: 'KAI-310',
  billing: 'KAI-311',
  activity: 'KAI-312',
};

function formatCents(cents: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

export function CustomerDrillDown(): JSX.Element {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const params = useParams<{ customerId: string }>();
  const customerId = params.customerId ?? '';
  const [tab, setTab] = useState<TabId>('overview');
  const [impersonating, setImpersonating] = useState(false);
  const startImpersonationSession = useImpersonationStore((s) => s.startSession);

  const query = useQuery({
    queryKey: customerQueryKey(customerId),
    queryFn: () => getCustomer(customerId),
    enabled: Boolean(customerId),
  });

  const impersonateMutation = useMutation({
    mutationFn: (reason: string) => beginImpersonation(customerId, reason),
    onSuccess: (session) => {
      setImpersonating(false);
      // KAI-467: Set session in store so ImpersonationBanner renders.
      startImpersonationSession({
        sessionId: session.sessionId,
        mode: 'integrator',
        impersonatingUserId: session.integratorUserId,
        impersonatingUserName: 'Current User',
        impersonatingTenantId: 'integrator-001',
        impersonatedTenantId: session.customerId,
        impersonatedTenantName: customer?.name ?? session.customerId,
        scopedPermissions: ['view.live', 'view.playback', 'cameras.edit'],
        status: 'active',
        reason: session.reason,
        createdAtIso: session.beganAtIso,
        expiresAtIso: new Date(Date.now() + 30 * 60 * 1000).toISOString(),
        terminatedAtIso: null,
        terminatedBy: null,
      });
      navigate('/admin');
    },
  });

  const customer = query.data ?? null;

  const kpis = useMemo(() => {
    if (!customer) return null;
    return [
      {
        id: 'cameras',
        label: t('customers.drillDown.kpis.cameras'),
        value: customer.camerasManaged.toLocaleString(i18n.language),
      },
      {
        id: 'mrr',
        label: t('customers.drillDown.kpis.mrr'),
        value: formatCents(customer.monthlyRecurringRevenueCents, i18n.language),
      },
      {
        id: 'uptime',
        label: t('customers.drillDown.kpis.uptime'),
        value: `${customer.uptimePercent.toFixed(1)}%`,
      },
      {
        id: 'incidents',
        label: t('customers.drillDown.kpis.activeIncidents'),
        value: String(customer.activeIncidents),
      },
    ];
  }, [customer, i18n.language, t]);

  return (
    <main
      data-testid="customer-drill-down"
      aria-labelledby="customer-drill-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      <nav aria-label={t('customers.drillDown.breadcrumbAriaLabel')} className="text-xs text-slate-500">
        <ol className="flex gap-1">
          <li>
            <button
              type="button"
              data-testid="breadcrumb-customers"
              onClick={() => navigate('/command/customers')}
              className="hover:underline focus:outline-none focus:ring-2 focus:ring-blue-500"
            >
              {t('customers.list.title')}
            </button>
          </li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="font-medium text-slate-700">
            {customer?.name ?? customerId}
          </li>
        </ol>
      </nav>

      <header className="mt-2 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 id="customer-drill-heading" className="text-2xl font-bold text-slate-900">
            {customer?.name ?? t('customers.drillDown.loadingName')}
          </h1>
          {customer ? (
            <p className="text-xs text-slate-500">
              {t('customers.drillDown.subtitle', {
                status: t(`customers.status.${customer.status}`),
                tier: t(`fleet.tier.${customer.tier}`),
              })}
            </p>
          ) : null}
        </div>
        <button
          type="button"
          data-testid="impersonate-open"
          aria-describedby="impersonate-help"
          onClick={() => setImpersonating(true)}
          disabled={!customer}
          className="rounded border border-red-300 bg-white px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50 focus:outline-none focus:ring-2 focus:ring-red-500 disabled:opacity-40"
        >
          {t('customers.drillDown.impersonateButton')}
        </button>
        <span id="impersonate-help" className="sr-only">
          {t('customers.drillDown.impersonateHelp')}
        </span>
      </header>

      <div role="tablist" aria-label={t('customers.drillDown.tabsAriaLabel')} className="mt-4 flex gap-1 border-b border-slate-200">
        {TABS.map((id) => {
          const selected = tab === id;
          return (
            <button
              key={id}
              type="button"
              role="tab"
              id={`tab-${id}`}
              aria-selected={selected}
              aria-controls={`tabpanel-${id}`}
              tabIndex={selected ? 0 : -1}
              data-testid={`drill-tab-${id}`}
              onClick={() => setTab(id)}
              className={`-mb-px border-b-2 px-3 py-1.5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-blue-500 ${
                selected
                  ? 'border-blue-600 text-blue-700'
                  : 'border-transparent text-slate-600 hover:text-slate-900'
              }`}
            >
              {t(`customers.drillDown.tabs.${id}`)}
            </button>
          );
        })}
      </div>

      {query.isLoading ? (
        <p role="status" data-testid="drill-loading" className="mt-4 text-sm text-slate-500">
          {t('customers.drillDown.loading')}
        </p>
      ) : !customer ? (
        <p role="alert" data-testid="drill-not-found" className="mt-4 text-sm text-red-700">
          {t('customers.drillDown.notFound')}
        </p>
      ) : (
        <div className="mt-4">
          {tab === 'overview' && (
            <div
              role="tabpanel"
              id="tabpanel-overview"
              aria-labelledby="tab-overview"
              data-testid="drill-panel-overview"
              className="space-y-4"
            >
              <section
                aria-label={t('customers.drillDown.kpis.regionLabel')}
                className="grid grid-cols-2 gap-3 sm:grid-cols-4"
              >
                {kpis?.map((k) => (
                  <div
                    key={k.id}
                    data-testid={`drill-kpi-${k.id}`}
                    className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm"
                  >
                    <dt className="text-xs text-slate-500">{k.label}</dt>
                    <dd className="mt-1 text-xl font-semibold text-slate-900">{k.value}</dd>
                  </div>
                ))}
              </section>

              <section
                aria-label={t('customers.drillDown.contact.regionLabel')}
                className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm"
              >
                <h2 className="text-sm font-semibold text-slate-800">
                  {t('customers.drillDown.contact.heading')}
                </h2>
                <dl className="mt-2 grid grid-cols-1 gap-1 text-sm text-slate-700 sm:grid-cols-2">
                  <dt className="font-medium">{t('customers.drillDown.contact.name')}</dt>
                  <dd>{customer.contact.name}</dd>
                  <dt className="font-medium">{t('customers.drillDown.contact.email')}</dt>
                  <dd>{customer.contact.email}</dd>
                  <dt className="font-medium">{t('customers.drillDown.contact.phone')}</dt>
                  <dd>{customer.contact.phone}</dd>
                  <dt className="font-medium">{t('customers.drillDown.contact.timezone')}</dt>
                  <dd>{customer.timezone}</dd>
                </dl>
              </section>

              <section
                aria-label={t('customers.drillDown.activity.regionLabel')}
                className="rounded-lg border border-slate-200 bg-white p-3 shadow-sm"
              >
                <h2 className="text-sm font-semibold text-slate-800">
                  {t('customers.drillDown.activity.heading')}
                </h2>
                <ul data-testid="drill-activity-feed" className="mt-2 space-y-1 text-sm text-slate-700">
                  {customer.recentActivity.map((entry) => (
                    <li key={entry.id} className="flex items-baseline gap-2">
                      <span className="text-xs text-slate-500">
                        {new Date(entry.iso).toLocaleString(i18n.language)}
                      </span>
                      <span>
                        {t(entry.detailKey)} — {entry.actor}
                      </span>
                    </li>
                  ))}
                </ul>
              </section>
            </div>
          )}

          {tab !== 'overview' && (
            <div
              role="tabpanel"
              id={`tabpanel-${tab}`}
              aria-labelledby={`tab-${tab}`}
              data-testid={`drill-panel-${tab}`}
              className="rounded-lg border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-600"
            >
              <p>{t('customers.drillDown.comingSoon.body')}</p>
              <p className="mt-1 text-xs text-slate-500">
                {t('customers.drillDown.comingSoon.tracking', {
                  ticket: TAB_TICKETS[tab as Exclude<TabId, 'overview'>],
                })}
              </p>
            </div>
          )}
        </div>
      )}

      {customer && impersonating ? (
        <ImpersonateConfirmDialog
          customer={customer}
          open={impersonating}
          onCancel={() => setImpersonating(false)}
          onConfirm={async (reason) => {
            await impersonateMutation.mutateAsync(reason);
          }}
        />
      ) : null}
    </main>
  );
}

export default CustomerDrillDown;
