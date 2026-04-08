import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { AdminDashboard } from './AdminDashboard';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as dashboardApi from '@/api/dashboard';

// KAI-320 tests:
//  - renders with mock data
//  - ack button triggers mutation + updates alert state
//  - keyboard tab order (quick actions -> camera summary -> health
//    -> events -> alerts)
//  - axe smoke (zero critical/serious violations)
//  - tenant scoping: query uses current session tenant ID

function renderDashboard(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin']}>
          <AdminDashboard />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('AdminDashboard', () => {
  beforeEach(() => {
    // Reset session to a known tenant for deterministic scoping tests.
    useSessionStore.setState({
      tenantId: 'tenant-test-42',
      tenantName: 'Test Tenant 42',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
  });

  it('renders dashboard widgets with mock data', async () => {
    renderDashboard();

    await waitFor(() =>
      expect(screen.getByTestId('admin-dashboard')).toBeInTheDocument(),
    );

    expect(screen.getByTestId('dashboard-tenant-name')).toHaveTextContent(
      'Test Tenant 42',
    );
    expect(screen.getByRole('heading', { level: 1, name: /my kaivue/i })).toBeInTheDocument();

    // Camera tile summary: 20 online, 3 offline, 2 warning.
    expect(within(screen.getByTestId('camera-tile-online')).getByText('20')).toBeInTheDocument();
    expect(within(screen.getByTestId('camera-tile-offline')).getByText('3')).toBeInTheDocument();
    expect(within(screen.getByTestId('camera-tile-warning')).getByText('2')).toBeInTheDocument();

    // Health tiles all rendered.
    expect(screen.getByTestId('health-tile-storage')).toBeInTheDocument();
    expect(screen.getByTestId('health-tile-network')).toBeInTheDocument();
    expect(screen.getByTestId('health-tile-sidecars')).toBeInTheDocument();
    expect(screen.getByTestId('health-tile-recorder')).toBeInTheDocument();

    // Event stream log landmark.
    expect(screen.getByRole('log')).toBeInTheDocument();

    // Two active alerts expected from mock.
    expect(screen.getByTestId('alert-alert-001')).toBeInTheDocument();
    expect(screen.getByTestId('alert-alert-002')).toBeInTheDocument();
  });

  it('ack button updates alert state via mutation', async () => {
    const user = userEvent.setup();
    const ackSpy = vi.spyOn(dashboardApi, 'ackAlert');
    renderDashboard();

    await waitFor(() =>
      expect(screen.getByTestId('alert-alert-001')).toBeInTheDocument(),
    );

    await user.click(screen.getByTestId('alert-ack-alert-001'));

    await waitFor(() => {
      expect(screen.queryByTestId('alert-alert-001')).not.toBeInTheDocument();
    });
    expect(ackSpy).toHaveBeenCalledWith(
      expect.objectContaining({ tenantId: 'tenant-test-42', alertId: 'alert-001' }),
    );
  });

  it('tab order moves through quick actions, then camera summary, health, events, alerts', async () => {
    const user = userEvent.setup();
    renderDashboard();

    await waitFor(() =>
      expect(screen.getByTestId('admin-dashboard')).toBeInTheDocument(),
    );

    // First focusable = first quick-action button (seam #6 tab order).
    await user.tab();
    expect(screen.getByTestId('quick-action-addCamera')).toHaveFocus();

    // Walk through remaining quick actions.
    await user.tab();
    expect(screen.getByTestId('quick-action-inviteUser')).toHaveFocus();
    await user.tab();
    expect(screen.getByTestId('quick-action-downloadSupportBundle')).toHaveFocus();
    await user.tab();
    expect(screen.getByTestId('quick-action-viewAuditLog')).toHaveFocus();

    // Next interactive focus target is the event-stream scroller (tabIndex=0),
    // because camera summary & health widget are non-interactive stat tiles.
    await user.tab();
    expect(screen.getByTestId('event-stream-scroller')).toHaveFocus();

    // Then the first alert ack button.
    await user.tab();
    expect(screen.getByTestId('alert-ack-alert-001')).toHaveFocus();
  });

  it('has no critical or serious axe violations', async () => {
    renderDashboard();
    const dashboard = await waitFor(() => screen.getByTestId('admin-dashboard'));

    // Scope axe to the dashboard subtree — jsdom's synthetic document
    // does not ship <title>/lang attributes, which are owned by index.html
    // in the real build and covered by E2E tests.
    const violations = await runAxe(dashboard);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });

  it('scopes TanStack Query to the current session tenant ID', async () => {
    const fetchSpy = vi.spyOn(dashboardApi, 'fetchDashboardSummary');
    renderDashboard();

    await waitFor(() => expect(fetchSpy).toHaveBeenCalled());
    expect(fetchSpy).toHaveBeenCalledWith({ tenantId: 'tenant-test-42' });
  });
});
