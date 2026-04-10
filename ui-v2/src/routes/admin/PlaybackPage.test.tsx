// KAI-323: Component tests for PlaybackPage.
//
// Coverage:
//   - Renders without crashing on /admin/playback/evt-0000
//   - Loads event via getPlaybackEvent and renders the timeline + markers
//   - ?live=true shows LIVE badge and hides the timeline
//   - Export button calls exportClip and shows the success state
//   - Tenant-scoped query keys (queries scoped to current session tenantId)
//   - axe-core smoke (no critical/serious violations)

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { PlaybackPage } from './PlaybackPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as playbackApi from '@/api/playback';

function renderPage(initialEntry = '/admin/playback/evt-0000'): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route path="/admin/playback/:eventId" element={<PlaybackPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('PlaybackPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-323',
      tenantName: 'Test Tenant 323',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
    playbackApi.resetPlaybackClientForTests();
  });

  it('renders the page header and breadcrumb', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('playback-page'));
    expect(screen.getByRole('heading', { level: 1 })).toBeInTheDocument();
  });

  it('loads the playback event and renders the timeline', async () => {
    const spy = vi.spyOn(playbackApi, 'getPlaybackEvent');
    renderPage();
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith('tenant-test-323', 'evt-0000'),
    );
    await waitFor(() => screen.getByTestId('playback-timeline'));
    expect(screen.getByTestId('playback-video')).toBeInTheDocument();
    // 25 hourly tick marks (0..24).
    expect(screen.getByTestId('playback-timeline-tick-0')).toBeInTheDocument();
    expect(screen.getByTestId('playback-timeline-tick-86400')).toBeInTheDocument();
  });

  it('renders AI markers from listMarkersForWindow', async () => {
    const spy = vi.spyOn(playbackApi, 'listMarkersForWindow');
    renderPage();
    await waitFor(() => expect(spy).toHaveBeenCalled());
    await waitFor(() => {
      const markers = document.querySelectorAll('[data-testid^="playback-marker-"]');
      expect(markers.length).toBeGreaterThan(0);
    });
  });

  it('shows the LIVE badge when ?live=true and hides the timeline', async () => {
    renderPage('/admin/playback/evt-0000?live=true');
    await waitFor(() => screen.getByTestId('playback-page'));
    await waitFor(() => screen.getByTestId('playback-live-badge'));
    expect(screen.getByTestId('playback-live-notice')).toBeInTheDocument();
    expect(screen.queryByTestId('playback-timeline')).not.toBeInTheDocument();
  });

  it('export button triggers exportClip and renders success state', async () => {
    const spy = vi.spyOn(playbackApi, 'exportClip');
    renderPage();
    await waitFor(() => screen.getByTestId('playback-export-button'));
    fireEvent.click(screen.getByTestId('playback-export-button'));
    await waitFor(() => expect(spy).toHaveBeenCalled());
    const callArg = spy.mock.calls[0]?.[0];
    expect(callArg?.tenantId).toBe('tenant-test-323');
    expect(callArg?.eventId).toBe('evt-0000');
    await waitFor(() =>
      expect(screen.getByTestId('playback-export-success')).toBeInTheDocument(),
    );
  });

  it('queries are scoped to the active session tenantId', async () => {
    const spy = vi.spyOn(playbackApi, 'getPlaybackEvent');
    useSessionStore.setState({
      tenantId: 'tenant-other-456',
      tenantName: 'Other Tenant',
      userId: 'u',
      userDisplayName: 'O',
    });
    renderPage();
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith('tenant-other-456', 'evt-0000'),
    );
  });

  it('clicking on the timeline seeks the cursor', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('playback-timeline'));
    const timeline = screen.getByTestId('playback-timeline');
    // Stub getBoundingClientRect for the click ratio math.
    timeline.getBoundingClientRect = () =>
      ({ left: 0, top: 0, width: 1000, height: 50, right: 1000, bottom: 50, x: 0, y: 0, toJSON() {} }) as DOMRect;
    fireEvent.click(timeline, { clientX: 500 });
    await waitFor(() => {
      const aria = timeline.getAttribute('aria-valuenow');
      // 500/1000 * 86400 = 43200
      expect(aria).toBe('43200');
    });
  });

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('playback-page'));
    await waitFor(() => screen.getByTestId('playback-timeline'));
    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
