// KAI-324: Component tests for the Customer Admin Events page.
//
// Coverage:
//   - Renders virtualized event list from mock listEvents()
//   - Filter narrows the visible list (severity dimension)
//   - Search input narrows results
//   - Semantic search toggle flips the semantic flag on the query
//   - Tenant isolation: query scoped to active session tenantId
//   - CSV export helper round-trip (buildEventsCsv)
//   - Jump to playback navigates to /admin/playback/:eventId
//   - axe-core smoke (no critical/serious violations)

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { EventsPage } from './EventsPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as eventsApi from '@/api/events';
import { buildEventsCsv, type AiEvent } from '@/api/events';

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/events']}>
          <Routes>
            <Route path="/admin/events" element={<EventsPage />} />
            <Route
              path="/admin/playback/:eventId"
              element={<div data-testid="playback-stub" />}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('EventsPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-324',
      tenantName: 'Test Tenant 324',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // Render + tenant scoping
  // ---------------------------------------------------------------------------

  it('renders the virtualized event list from mock data', async () => {
    const listSpy = vi.spyOn(eventsApi, 'listEvents');
    renderPage();
    await waitFor(() =>
      expect(screen.getByTestId('events-page')).toBeInTheDocument(),
    );
    await waitFor(() =>
      expect(screen.getByTestId('events-list-scroll')).toBeInTheDocument(),
    );
    expect(listSpy).toHaveBeenCalled();
    // Tenant-scoped: first call includes the active tenant id.
    expect(listSpy.mock.calls[0]?.[0].tenantId).toBe('tenant-test-324');
    // At least one row should be visible in the default viewport.
    expect(screen.getAllByTestId(/^event-row-/).length).toBeGreaterThan(0);
  });

  it('rescopes queries when the active tenant changes', async () => {
    const listSpy = vi.spyOn(eventsApi, 'listEvents');
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-scroll'));

    listSpy.mockClear();
    act(() => {
      useSessionStore.setState({
        tenantId: 'tenant-other-999',
        tenantName: 'Other',
        userId: 'u',
        userDisplayName: 'O',
      });
    });
    await waitFor(() => {
      const calls = listSpy.mock.calls.map((c) => c[0].tenantId);
      expect(calls).toContain('tenant-other-999');
    });
  });

  // ---------------------------------------------------------------------------
  // Filters
  // ---------------------------------------------------------------------------

  it('severity filter narrows the visible event count', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-count'));

    const beforeText = screen.getByTestId('events-list-count').textContent ?? '';
    const beforeCount = parseInt(beforeText.replace(/\D+/g, ''), 10);
    expect(beforeCount).toBeGreaterThan(0);

    // Restrict to critical only — mock data has a known critical slice.
    await user.click(screen.getByTestId('events-filter-severity-critical'));

    await waitFor(() => {
      const text = screen.getByTestId('events-list-count').textContent ?? '';
      const n = parseInt(text.replace(/\D+/g, ''), 10);
      expect(n).toBeLessThan(beforeCount);
      expect(n).toBeGreaterThan(0);
    });
  });

  it('reset button clears filters back to the unfiltered count', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-count'));
    const beforeText = screen.getByTestId('events-list-count').textContent ?? '';

    await user.click(screen.getByTestId('events-filter-severity-critical'));
    await waitFor(() => {
      expect(screen.getByTestId('events-list-count').textContent).not.toBe(
        beforeText,
      );
    });

    await user.click(screen.getByTestId('events-filters-reset'));
    await waitFor(() => {
      expect(screen.getByTestId('events-list-count').textContent).toBe(beforeText);
    });
  });

  // ---------------------------------------------------------------------------
  // Search + semantic toggle
  // ---------------------------------------------------------------------------

  it('search input narrows the visible results', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-count'));
    const beforeText = screen.getByTestId('events-list-count').textContent ?? '';
    const beforeCount = parseInt(beforeText.replace(/\D+/g, ''), 10);

    await user.type(screen.getByTestId('events-search-input'), 'vehicle');
    await waitFor(() => {
      const text = screen.getByTestId('events-list-count').textContent ?? '';
      const n = parseInt(text.replace(/\D+/g, ''), 10);
      expect(n).toBeLessThan(beforeCount);
    });
  });

  it('semantic toggle flips the semantic flag on listEvents', async () => {
    const listSpy = vi.spyOn(eventsApi, 'listEvents');
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-count'));

    await user.click(screen.getByTestId('events-search-semantic'));
    // After the toggle the query re-runs and the page re-enters the
    // loading branch briefly; wait for the input to come back before
    // submitting a new search value. Use fireEvent.change (single
    // atomic update) instead of user.type so the input isn't unmounted
    // mid-keystroke when each letter triggers its own refetch.
    const searchInput = await waitFor(() =>
      screen.getByTestId('events-search-input'),
    );
    fireEvent.change(searchInput, { target: { value: 'person' } });

    await waitFor(() => {
      const semanticCall = listSpy.mock.calls.find(
        (c) => c[0].semantic === true && c[0].search === 'person',
      );
      expect(semanticCall).toBeDefined();
    });
  });

  it('semantic checkbox is disabled when ai.semantic_search entitlement is missing', async () => {
    useSessionStore.setState({ entitlements: {} });
    renderPage();
    await waitFor(() => screen.getByTestId('events-search-semantic'));
    const checkbox = screen.getByTestId('events-search-semantic') as HTMLInputElement;
    expect(checkbox.disabled).toBe(true);
    expect(screen.getByTestId('events-search-semantic-entitlement-hint')).toBeInTheDocument();
  });

  // ---------------------------------------------------------------------------
  // Jump to playback
  // ---------------------------------------------------------------------------

  it('Enter key on an event row navigates to playback', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-scroll'));

    const rows = screen.getAllByTestId(/^event-row-/);
    const first = rows[0]!;
    first.focus();
    await user.keyboard('{Enter}');

    await waitFor(() =>
      expect(screen.getByTestId('playback-stub')).toBeInTheDocument(),
    );
  });

  it('jump-to-playback button navigates to the playback route', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('events-list-scroll'));

    const jumpButtons = screen.getAllByTestId(/^event-jump-/);
    await user.click(jumpButtons[0]!);

    await waitFor(() =>
      expect(screen.getByTestId('playback-stub')).toBeInTheDocument(),
    );
  });

  // ---------------------------------------------------------------------------
  // CSV helper — pure function round-trip
  // ---------------------------------------------------------------------------

  it('buildEventsCsv emits header + one row per event with CSV escaping', () => {
    const events: readonly AiEvent[] = [
      {
        id: 'evt-1',
        tenantId: 'tenant-test-324',
        cameraId: 'cam-a',
        cameraName: 'Camera, 001',
        type: 'person.detected',
        severity: 'critical',
        timestamp: '2026-04-08T12:00:00.000Z',
        summary: 'He said "hi"',
        thumbnailUrl: '/thumb/a.jpg',
        clipStartSec: 0,
        clipDurationSec: 12,
      },
    ];
    const csv = buildEventsCsv(events);
    const lines = csv.split('\n');
    expect(lines).toHaveLength(2);
    expect(lines[0]).toBe(
      'id,timestamp,camera,type,severity,summary,clipStartSec,clipDurationSec',
    );
    // Comma in camera name forces quoting, embedded quotes are doubled.
    expect(lines[1]).toContain('"Camera, 001"');
    expect(lines[1]).toContain('"He said ""hi"""');
  });

  // ---------------------------------------------------------------------------
  // a11y smoke
  // ---------------------------------------------------------------------------

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('events-page'));
    await waitFor(() => screen.getByTestId('events-list-scroll'));
    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
