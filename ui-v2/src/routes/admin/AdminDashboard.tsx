import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { useSessionStore } from '@/stores/session';
import {
  ackAlert,
  dashboardQueryKeys,
  fetchDashboardSummary,
  type Alert,
  type DashboardSummary,
} from '@/api/dashboard';

import { CameraTileSummary } from '@/components/admin/CameraTileSummary';
import { EventStream } from '@/components/admin/EventStream';
import { SystemHealthWidget } from '@/components/admin/SystemHealthWidget';
import { AlertList } from '@/components/admin/AlertList';
import { QuickActionsRow } from '@/components/admin/QuickActionsRow';

// KAI-320: Customer Admin Dashboard.
//
// Tab order (seam #6 / WCAG 2.1 AA): quick actions -> camera summary
// -> health widgets -> event stream -> alerts. This order is enforced
// by source-order + the event stream scroller being the only
// interactive focus target inside its section.

export function AdminDashboard(): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);

  const queryClient = useQueryClient();

  const query = useQuery<DashboardSummary>({
    queryKey: dashboardQueryKeys.summary(tenantId),
    queryFn: () => fetchDashboardSummary({ tenantId }),
  });

  // Local ack overlay so the UI reflects mutation optimistically without
  // having to thread real invalidation through the mock API.
  const [ackedIds, setAckedIds] = useState<Set<string>>(new Set());

  const ackMutation = useMutation({
    mutationFn: (alertId: string) => ackAlert({ tenantId, alertId }),
    onMutate: (alertId: string) => {
      setAckedIds((prev) => {
        const next = new Set(prev);
        next.add(alertId);
        return next;
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: dashboardQueryKeys.summary(tenantId),
      });
    },
  });

  const handleAck = useCallback(
    (alertId: string) => {
      ackMutation.mutate(alertId);
    },
    [ackMutation],
  );

  if (query.isLoading) {
    return (
      <main aria-busy="true" aria-live="polite">
        <p>{t('dashboard.loading')}</p>
      </main>
    );
  }

  if (query.isError || !query.data) {
    return (
      <main>
        <p role="alert">{t('dashboard.error')}</p>
      </main>
    );
  }

  const data = query.data;
  const visibleAlerts: Alert[] = data.alerts.map((alert) =>
    ackedIds.has(alert.id) ? { ...alert, state: 'acknowledged' } : alert,
  );

  return (
    <main
      className="admin-dashboard"
      aria-label={t('dashboard.pageLabel')}
      data-testid="admin-dashboard"
    >
      <header className="admin-dashboard__header">
        <p className="admin-dashboard__tenant" data-testid="dashboard-tenant-name">
          {tenantName}
        </p>
        <h1>{t('dashboard.title')}</h1>
      </header>

      <QuickActionsRow />
      <CameraTileSummary summary={data.cameras} />
      <SystemHealthWidget health={data.health} />
      <EventStream events={data.events} />
      <AlertList
        alerts={visibleAlerts}
        onAck={handleAck}
        isAcking={ackMutation.isPending}
      />
    </main>
  );
}

export default AdminDashboard;
