import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';

import { useSessionStore } from '@/stores/session';
import {
  distinctCameras,
  eventsQueryKeys,
  listEvents,
  type AiEvent,
  type EventListFilters,
} from '@/api/events';

import { EventFilters, type EventFiltersState } from '@/components/events/EventFilters';
import { EventList } from '@/components/events/EventList';
import { EventSearch } from '@/components/events/EventSearch';
import { EventExport } from '@/components/events/EventExport';

// KAI-324: Customer Admin Events page.
//
// Tab order (seam #6 / WCAG 2.1 AA):
//   breadcrumb -> search -> filters -> export -> virtualized list.
// Each interactive element is keyboard-reachable; the list rows
// are focusable articles that dispatch "Jump to playback" on
// Enter or Space (see EventCard). All user-visible strings flow
// through react-i18next under the events.* namespace.

const EMPTY_FILTERS: EventFiltersState = {
  cameraIds: [],
  types: [],
  severities: [],
  fromIso: '',
  toIso: '',
};

export function EventsPage(): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const entitlements = useSessionStore((s) => s.entitlements);

  const [search, setSearch] = useState('');
  const [semantic, setSemantic] = useState(false);
  const [filters, setFilters] = useState<EventFiltersState>(EMPTY_FILTERS);

  const queryFilters: Omit<EventListFilters, 'tenantId'> = useMemo(
    () => ({
      cameraIds: filters.cameraIds,
      types: filters.types,
      severities: filters.severities,
      fromIso: filters.fromIso || undefined,
      toIso: filters.toIso || undefined,
      search: search || undefined,
      semantic,
    }),
    [filters, search, semantic],
  );

  const query = useQuery<AiEvent[]>({
    queryKey: eventsQueryKeys.list(tenantId, queryFilters),
    queryFn: () => listEvents({ tenantId, ...queryFilters }),
  });

  const events = query.data ?? [];

  // All distinct cameras in the current (unfiltered) tenant dataset.
  // Populated from an unfiltered fetch so the camera multi-select
  // stays stable as the user narrows results.
  const allCamerasQuery = useQuery<AiEvent[]>({
    queryKey: eventsQueryKeys.list(tenantId, {}),
    queryFn: () => listEvents({ tenantId }),
  });
  const cameras = useMemo(
    () => distinctCameras(allCamerasQuery.data ?? []),
    [allCamerasQuery.data],
  );

  const handleReset = useCallback(() => {
    setFilters(EMPTY_FILTERS);
    setSearch('');
    setSemantic(false);
  }, []);

  const handleJumpToPlayback = useCallback(
    (event: AiEvent) => {
      // The playback route is registered in KAI-323; until that
      // lands, this is a best-effort navigation that will resolve
      // to the NotFound route in the scaffold. Tests assert the
      // navigate call, not the destination render.
      navigate(`/admin/playback/${event.id}`);
    },
    [navigate],
  );

  if (query.isLoading) {
    return (
      <main aria-busy="true" aria-live="polite">
        <p>{t('events.page.loading')}</p>
      </main>
    );
  }

  if (query.isError) {
    return (
      <main>
        <p role="alert">{t('events.page.error')}</p>
      </main>
    );
  }

  return (
    <main
      aria-label={t('events.page.label')}
      data-testid="events-page"
      className="events-page"
    >
      <nav aria-label={t('events.breadcrumb.ariaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('events.page.title')}</li>
        </ol>
      </nav>
      <header className="events-page__header">
        <h1>{t('events.page.title')}</h1>
      </header>

      <EventSearch
        value={search}
        semantic={semantic}
        semanticEntitled={entitlements['ai.semantic_search'] ?? false}
        onChange={setSearch}
        onSemanticChange={setSemantic}
      />

      <EventFilters
        cameras={cameras}
        value={filters}
        onChange={setFilters}
        onReset={handleReset}
      />

      <EventExport events={events} />

      <EventList events={events} onJumpToPlayback={handleJumpToPlayback} />
    </main>
  );
}

export default EventsPage;
