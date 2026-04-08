import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  listRecorders,
  unpairRecorder,
  recordersQueryKeys,
  type Recorder,
  type RecorderHealth,
} from '@/api/recorders';
import { useSessionStore } from '@/stores/session';
import { RecorderList } from '@/components/recorders/RecorderList';
import { PairRecorderDrawer } from '@/components/recorders/PairRecorderDrawer';
import { RecorderDetailDrawer } from '@/components/recorders/RecorderDetailDrawer';
import { UnpairRecorderConfirm } from '@/components/recorders/UnpairRecorderConfirm';

// KAI-322: Customer Admin Recorders page.
//
// ONLY rendered in the customer admin runtime context (/admin/recorders).
// The integrator portal has its own route tree under /command/* and MUST
// NOT import this page. During customer impersonation (KAI-224), the
// impersonating integrator staff will land here and see the customer's
// Recorders — the session store already holds the impersonated tenantId,
// so no additional guard is needed at the component level.
//
// Real-time updates: the spec calls for WebSocket subscription. No WS
// setup exists on main yet, so we poll every 10 s.
// TODO(KAI-320-ws): swap refetchInterval for a WS subscription once the
// WebSocket pattern lands from KAI-320.
//
// White-label: no hardcoded colors or brand assets. CSS variables from
// the brand config system (KAI-310) are used via class names in the
// shared stylesheet. This component depends on KAI-310 being shipped to
// get correct brand colors at runtime; current CSS variable defaults are
// safe placeholders.

type HealthFilter = RecorderHealth | 'all';

export function RecordersPage(): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const queryClient = useQueryClient();

  const [search, setSearch] = useState('');
  const [healthFilter, setHealthFilter] = useState<HealthFilter>('all');

  const [pairOpen, setPairOpen] = useState(false);
  const [detailTarget, setDetailTarget] = useState<Recorder | null>(null);
  const [unpairTarget, setUnpairTarget] = useState<Recorder | null>(null);

  const filters = useMemo(
    () => ({ search, health: healthFilter }),
    [search, healthFilter],
  );

  const query = useQuery<Recorder[]>({
    queryKey: recordersQueryKeys.list(tenantId, filters),
    queryFn: () => listRecorders({ tenantId, ...filters }),
    // TODO(KAI-320-ws): replace with WebSocket subscription.
    refetchInterval: 10_000,
  });

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: recordersQueryKeys.all(tenantId) });
  }, [queryClient, tenantId]);

  const unpairMutation = useMutation({
    mutationFn: (id: string) => unpairRecorder(tenantId, id),
    onSuccess: invalidate,
  });

  const recorders = query.data ?? [];

  const handleDetail = useCallback((recorder: Recorder) => {
    setDetailTarget(recorder);
  }, []);

  const handleUnpairRequest = useCallback((recorder: Recorder) => {
    setUnpairTarget(recorder);
  }, []);

  const handleUnpairConfirm = useCallback(() => {
    if (unpairTarget) {
      unpairMutation.mutate(unpairTarget.id);
      setUnpairTarget(null);
    }
  }, [unpairTarget, unpairMutation]);

  return (
    <main
      aria-label={t('recorders.page.label')}
      data-testid="recorders-page"
      className="recorders-page"
    >
      <nav aria-label={t('recorders.breadcrumb.ariaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('recorders.page.title')}</li>
        </ol>
      </nav>

      <header className="recorders-page__header">
        <h1>{t('recorders.page.title')}</h1>
        <button
          type="button"
          onClick={() => setPairOpen(true)}
          data-testid="recorders-pair-button"
        >
          {t('recorders.actions.pairNew')}
        </button>
      </header>

      <section
        className="recorders-page__toolbar"
        aria-label={t('recorders.toolbar.ariaLabel')}
      >
        <label>
          <span className="sr-only">{t('recorders.toolbar.searchLabel')}</span>
          <input
            type="search"
            placeholder={t('recorders.toolbar.searchPlaceholder')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            data-testid="recorders-search"
            aria-label={t('recorders.toolbar.searchLabel')}
          />
        </label>
        <label>
          <span className="sr-only">{t('recorders.toolbar.healthFilterLabel')}</span>
          <select
            value={healthFilter}
            onChange={(e) => setHealthFilter(e.target.value as HealthFilter)}
            data-testid="recorders-health-filter"
            aria-label={t('recorders.toolbar.healthFilterLabel')}
          >
            <option value="all">{t('recorders.filter.all')}</option>
            <option value="online">{t('recorders.health.online')}</option>
            <option value="degraded">{t('recorders.health.degraded')}</option>
            <option value="offline">{t('recorders.health.offline')}</option>
          </select>
        </label>
      </section>

      {query.isLoading && (
        <p role="status" aria-live="polite">
          {t('recorders.list.loading')}
        </p>
      )}
      {query.isError && (
        <p role="alert">{t('recorders.list.error')}</p>
      )}
      {query.isSuccess && (
        <RecorderList
          recorders={recorders}
          onDetail={handleDetail}
          onUnpair={handleUnpairRequest}
        />
      )}

      <PairRecorderDrawer
        open={pairOpen}
        tenantId={tenantId}
        onClose={() => setPairOpen(false)}
      />

      <RecorderDetailDrawer
        open={detailTarget !== null}
        recorder={detailTarget}
        tenantId={tenantId}
        onClose={() => setDetailTarget(null)}
      />

      <UnpairRecorderConfirm
        open={unpairTarget !== null}
        recorder={unpairTarget}
        onClose={() => setUnpairTarget(null)}
        onConfirm={handleUnpairConfirm}
      />
    </main>
  );
}

export default RecordersPage;
