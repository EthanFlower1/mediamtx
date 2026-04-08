import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { listFleet, fleetQueryKey, __TEST__ } from '@/api/fleet';
import type { FleetFilters } from '@/api/fleet';
import { KpiCard } from '@/components/fleet/KpiCard';
import { CustomerHealthCard } from '@/components/fleet/CustomerHealthCard';
import { AlertStream } from '@/components/fleet/AlertStream';
import { FleetFiltersBar } from '@/components/fleet/FleetFilters';

// KAI-308: Fleet Dashboard (Integrator Portal shell).
//
// This is the landing page for the /command/* runtime context. It
// aggregates customer health, fleet-wide KPIs, and a cross-customer
// alert stream — all scoped to the signed-in integrator.
//
// Data layer:
//  - TanStack Query, keyed by integrator ID + filters so flipping
//    filters produces a fresh cache entry.
//  - `listFleet()` is a typed mock stub today; real Connect-Go lands
//    when KAI-238 protos generate (future ticket).
//
// Scope enforcement: the mock client filters by integratorId. We
// also assert in tests that customers owned by a different integrator
// never surface in the rendered tree.
//
// Accessibility:
//  - Page is wrapped in <main> with labelled landmarks (role=region).
//  - KPI row and customer grid both support tab-through navigation.
//  - Axe smoke test covers zero critical/serious violations.

interface FleetDashboardProps {
  readonly integratorId?: string;
  readonly integratorDisplayName?: string;
}

function formatCurrencyCents(cents: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

export function FleetDashboard({
  integratorId = __TEST__.CURRENT_INTEGRATOR_ID,
  integratorDisplayName,
}: FleetDashboardProps): JSX.Element {
  const { t, i18n } = useTranslation();
  const [filters, setFilters] = useState<FleetFilters>({});

  const query = useQuery({
    queryKey: fleetQueryKey(integratorId, filters),
    queryFn: () => listFleet(integratorId, filters),
  });

  const snapshot = query.data;

  const title = t('fleet.dashboard.title');
  const displayName = integratorDisplayName ?? t('fleet.dashboard.defaultIntegratorName');

  const kpiCards = useMemo(() => {
    if (!snapshot) return null;
    const mrr = formatCurrencyCents(snapshot.kpis.totalMrrCents, i18n.language);
    return (
      <div
        role="region"
        aria-label={t('fleet.kpis.regionLabel')}
        className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5"
      >
        <KpiCard
          testId="kpi-total-customers"
          label={t('fleet.kpis.totalCustomers')}
          value={String(snapshot.kpis.totalCustomers)}
          icon={<span>◆</span>}
        />
        <KpiCard
          testId="kpi-total-cameras"
          label={t('fleet.kpis.totalCameras')}
          value={String(snapshot.kpis.totalCameras)}
          icon={<span>◉</span>}
        />
        <KpiCard
          testId="kpi-total-mrr"
          label={t('fleet.kpis.totalMrr')}
          value={mrr}
          icon={<span>$</span>}
        />
        <KpiCard
          testId="kpi-active-incidents"
          label={t('fleet.kpis.activeIncidents')}
          value={String(snapshot.kpis.activeIncidents)}
          icon={<span>!</span>}
        />
        <KpiCard
          testId="kpi-uptime"
          label={t('fleet.kpis.uptime')}
          value={`${snapshot.kpis.uptimePercent.toFixed(1)}%`}
          icon={<span>↑</span>}
        />
      </div>
    );
  }, [snapshot, i18n.language, t]);

  return (
    <main
      data-testid="fleet-dashboard"
      aria-labelledby="fleet-dashboard-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      <header className="mb-4">
        <nav aria-label={t('fleet.breadcrumb.ariaLabel')} className="text-xs text-slate-500">
          <ol className="flex gap-1">
            <li>{t('fleet.breadcrumb.integratorPortal')}</li>
            <li aria-hidden="true">/</li>
            <li aria-current="page" className="font-medium text-slate-700">
              {title}
            </li>
          </ol>
        </nav>
        <h1 id="fleet-dashboard-heading" className="mt-1 text-2xl font-bold text-slate-900">
          {title}
        </h1>
        <p className="text-sm text-slate-600">
          {t('fleet.dashboard.subtitle', { integrator: displayName })}
        </p>
      </header>

      <div className="mb-4">
        <FleetFiltersBar
          filters={filters}
          onChange={setFilters}
          subResellers={snapshot?.availableSubResellers ?? []}
          labels={snapshot?.availableLabels ?? []}
        />
      </div>

      {query.isLoading ? (
        <p role="status" data-testid="fleet-loading">
          {t('fleet.dashboard.loading')}
        </p>
      ) : query.isError ? (
        <p role="alert" data-testid="fleet-error">
          {t('fleet.dashboard.error')}
        </p>
      ) : snapshot ? (
        <>
          <div className="mb-4">{kpiCards}</div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
            <section
              role="region"
              aria-label={t('fleet.customerGrid.regionLabel')}
              data-testid="customer-grid"
              className="lg:col-span-2"
            >
              <h2 className="mb-2 text-lg font-semibold text-slate-900">
                {t('fleet.customerGrid.title', { count: snapshot.customers.length })}
              </h2>
              {snapshot.customers.length === 0 ? (
                <p data-testid="customer-grid-empty" className="text-sm text-slate-500">
                  {t('fleet.customerGrid.empty')}
                </p>
              ) : (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
                  {snapshot.customers.map((c) => (
                    <CustomerHealthCard key={c.id} customer={c} />
                  ))}
                </div>
              )}
            </section>

            <aside className="lg:col-span-1">
              <AlertStream alerts={snapshot.alerts} />
            </aside>
          </div>
        </>
      ) : null}
    </main>
  );
}

// Default export so `React.lazy(() => import('./FleetDashboard'))` works.
export default FleetDashboard;
