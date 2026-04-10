import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { SystemHealthPage } from './SystemHealthPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import * as systemHealthApi from '@/api/systemHealth';

// KAI-329 tests:
//   - renders health dashboard with overall status
//   - shows 3 recorder health cards
//   - shows camera status summary
//   - shows storage overview
//   - toggles remote access
//   - shows remote session history table with 5 rows
//   - edits system name
//   - changes timezone

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/health']}>
          <SystemHealthPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('SystemHealthPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-health-test-01',
      tenantName: 'Health Test Tenant',
      userId: 'user-health-test',
      userDisplayName: 'Health Test User',
    });
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ------------------------------------------------------------------
  // Health Dashboard
  // ------------------------------------------------------------------

  it('renders health dashboard with overall status', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('overall-status'));
    expect(screen.getByTestId('overall-status')).toBeInTheDocument();
    expect(screen.getByTestId('overall-status-value')).toBeInTheDocument();
    // Mock has mix of statuses so overall should be degraded
    expect(screen.getByTestId('overall-status').getAttribute('data-status')).toBe('degraded');
  });

  it('shows 3 recorder health cards', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('recorder-health-cards'));
    const cards = screen.getAllByTestId(/^recorder-card-/);
    expect(cards).toHaveLength(3);
  });

  it('shows camera status summary with correct counts', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('camera-summary'));
    expect(screen.getByTestId('cameras-total')).toHaveTextContent('24');
    expect(screen.getByTestId('cameras-online')).toHaveTextContent('18');
    expect(screen.getByTestId('cameras-offline')).toHaveTextContent('3');
    expect(screen.getByTestId('cameras-degraded')).toHaveTextContent('3');
  });

  it('shows storage overview', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('storage-overview'));
    expect(screen.getByTestId('storage-total')).toBeInTheDocument();
    expect(screen.getByTestId('storage-used')).toBeInTheDocument();
    expect(screen.getByTestId('storage-available')).toBeInTheDocument();
    expect(screen.getByTestId('storage-retention-days')).toHaveTextContent('42');
  });

  it('shows network status', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('network-status'));
    expect(screen.getByTestId('network-bandwidth')).toHaveTextContent('62%');
    expect(screen.getByTestId('network-latency')).toHaveTextContent('18 ms');
  });

  // ------------------------------------------------------------------
  // Remote Access
  // ------------------------------------------------------------------

  it('toggles remote access off', async () => {
    const user = userEvent.setup();
    const spy = vi.spyOn(systemHealthApi, 'setRemoteAccessEnabled');

    renderPage();
    await waitFor(() => screen.getByTestId('remote-access-toggle'));

    const toggle = screen.getByTestId('remote-access-toggle') as HTMLInputElement;
    expect(toggle.checked).toBe(true);

    await user.click(toggle);
    await waitFor(() => expect(spy).toHaveBeenCalledWith('tenant-health-test-01', false));
  });

  it('shows remote session history table with 5 rows', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('remote-sessions-table'));
    const rows = screen.getAllByTestId(/^session-row-/);
    expect(rows).toHaveLength(5);
  });

  // ------------------------------------------------------------------
  // Quick Settings
  // ------------------------------------------------------------------

  it('edits system name', async () => {
    const user = userEvent.setup();
    const spy = vi.spyOn(systemHealthApi, 'updateSystemSettings');

    renderPage();
    await waitFor(() => screen.getByTestId('system-name-display'));
    expect(screen.getByTestId('system-name-display')).toHaveTextContent('Main Office NVR System');

    await user.click(screen.getByTestId('system-name-edit'));
    await waitFor(() => screen.getByTestId('system-name-input'));

    const input = screen.getByTestId('system-name-input') as HTMLInputElement;
    await user.clear(input);
    await user.type(input, 'New System Name');

    await user.click(screen.getByTestId('system-name-save'));
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith(
        'tenant-health-test-01',
        expect.objectContaining({ systemName: 'New System Name' }),
      ),
    );
  });

  it('changes timezone', async () => {
    const user = userEvent.setup();
    const spy = vi.spyOn(systemHealthApi, 'updateSystemSettings');

    renderPage();
    await waitFor(() => screen.getByTestId('timezone-select'));

    await user.selectOptions(screen.getByTestId('timezone-select'), 'America/Chicago');
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith(
        'tenant-health-test-01',
        expect.objectContaining({ timezone: 'America/Chicago' }),
      ),
    );
  });

  it('toggles auto-update off', async () => {
    const user = userEvent.setup();
    const spy = vi.spyOn(systemHealthApi, 'updateSystemSettings');

    renderPage();
    await waitFor(() => screen.getByTestId('auto-update-toggle'));

    const toggle = screen.getByTestId('auto-update-toggle') as HTMLInputElement;
    expect(toggle.checked).toBe(true);

    await user.click(toggle);
    await waitFor(() =>
      expect(spy).toHaveBeenCalledWith(
        'tenant-health-test-01',
        expect.objectContaining({ autoUpdateEnabled: false }),
      ),
    );
  });
});
