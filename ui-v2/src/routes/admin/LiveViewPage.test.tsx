// KAI-323: Component tests for LiveViewPage.
//
// Coverage:
//   - Renders without crashing
//   - Layout switcher changes tile count (1x1 / 2x2 / 3x3 / 4x4)
//   - Empty state shown when no cameras selected
//   - Camera picker selection populates tiles
//   - Keyboard navigation (Arrow + Enter) navigates to playback
//   - axe-core smoke (no critical/serious violations)

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { LiveViewPage } from './LiveViewPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/live']}>
          <Routes>
            <Route path="/admin/live" element={<LiveViewPage />} />
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

describe('LiveViewPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-323',
      tenantName: 'Test Tenant 323',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
  });

  it('renders the page header and layout switcher', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('live-view-page'));
    expect(screen.getByTestId('live-view-layout-switcher')).toBeInTheDocument();
    expect(screen.getByTestId('live-view-layout-1x1')).toBeInTheDocument();
    expect(screen.getByTestId('live-view-layout-2x2')).toBeInTheDocument();
    expect(screen.getByTestId('live-view-layout-3x3')).toBeInTheDocument();
    expect(screen.getByTestId('live-view-layout-4x4')).toBeInTheDocument();
  });

  it('shows the empty state when no cameras are selected', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('live-view-page'));
    expect(screen.getByTestId('live-view-empty-state')).toBeInTheDocument();
    expect(screen.queryByTestId('live-view-grid')).not.toBeInTheDocument();
  });

  it('layout switcher changes tile count', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('live-view-camera-picker'));

    // Select one camera so the grid renders.
    const firstToggle = screen.getAllByTestId(/^live-view-picker-/)[0];
    expect(firstToggle).toBeDefined();
    fireEvent.click(firstToggle!);

    await waitFor(() => screen.getByTestId('live-view-grid'));

    // Default layout 2x2 → 4 tiles
    expect(screen.getByTestId('live-view-grid').getAttribute('data-tile-count')).toBe('4');

    fireEvent.click(screen.getByTestId('live-view-layout-3x3'));
    expect(screen.getByTestId('live-view-grid').getAttribute('data-tile-count')).toBe('9');

    fireEvent.click(screen.getByTestId('live-view-layout-4x4'));
    expect(screen.getByTestId('live-view-grid').getAttribute('data-tile-count')).toBe('16');

    fireEvent.click(screen.getByTestId('live-view-layout-1x1'));
    expect(screen.getByTestId('live-view-grid').getAttribute('data-tile-count')).toBe('1');
  });

  it('selecting a camera populates a tile', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('live-view-camera-picker'));

    const firstToggle = screen.getAllByTestId(/^live-view-picker-/)[0]!;
    fireEvent.click(firstToggle);

    await waitFor(() => screen.getByTestId('live-view-tile-0'));
    // Tile 0 should have a non-empty status (one of online/offline/warning).
    const tile0 = screen.getByTestId('live-view-tile-0');
    expect(['online', 'offline', 'warning']).toContain(
      tile0.getAttribute('data-status'),
    );
  });

  it('Enter on a tile navigates to playback with ?live=true', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('live-view-camera-picker'));
    const firstToggle = screen.getAllByTestId(/^live-view-picker-/)[0]!;
    fireEvent.click(firstToggle);

    await waitFor(() => screen.getByTestId('live-view-tile-0'));
    const tile0 = screen.getByTestId('live-view-tile-0');
    tile0.focus();
    fireEvent.keyDown(tile0, { key: 'Enter' });

    await waitFor(() => screen.getByTestId('playback-stub'));
  });

  it('Arrow keys move focus between tiles', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('live-view-camera-picker'));

    // Select two cameras so we have at least 2 populated tiles.
    const toggles = screen.getAllByTestId(/^live-view-picker-/);
    fireEvent.click(toggles[0]!);
    fireEvent.click(toggles[1]!);

    await waitFor(() => screen.getByTestId('live-view-tile-0'));
    const tile0 = screen.getByTestId('live-view-tile-0');
    const tile1 = screen.getByTestId('live-view-tile-1');
    tile0.focus();
    expect(document.activeElement).toBe(tile0);
    fireEvent.keyDown(tile0, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(tile1);
    fireEvent.keyDown(tile1, { key: 'ArrowLeft' });
    expect(document.activeElement).toBe(tile0);
  });

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('live-view-page'));
    await waitFor(() => screen.getByTestId('live-view-camera-picker'));
    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
