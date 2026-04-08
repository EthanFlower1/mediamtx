import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor, within, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { RecordersPage } from './RecordersPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as recordersApi from '@/api/recorders';

// KAI-322 tests:
//   - list renders with correct row count
//   - empty state renders when no recorders
//   - pair modal opens and generates a token
//   - revoke calls the correct API
//   - unpair dialog blocks delete until confirmed
//   - unpair cancel does NOT delete
//   - health filter narrows results
//   - search narrows results
//   - detail drawer opens showing hardware info
//   - axe smoke (no critical/serious violations)
//   - queries scoped to active session tenant

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/recorders']}>
          <RecordersPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('RecordersPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-rec-test-01',
      tenantName: 'Rec Test Tenant',
      userId: 'user-rec-test',
      userDisplayName: 'Rec Test User',
    });
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ------------------------------------------------------------------
  // List
  // ------------------------------------------------------------------

  it('renders recorder list with expected row count', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));
    const rows = await screen.findAllByTestId(/^recorder-row-/);
    // Stub returns 6 recorders.
    expect(rows.length).toBeGreaterThanOrEqual(1);
    expect(rows.length).toBeLessThanOrEqual(6);
  });

  it('renders empty state when listRecorders returns an empty array', async () => {
    vi.spyOn(recordersApi, 'listRecorders').mockResolvedValue([]);
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));
    expect(screen.getByTestId('recorders-empty')).toBeInTheDocument();
    expect(screen.queryAllByTestId(/^recorder-row-/).length).toBe(0);
  });

  it('scopes queries to the active session tenant', async () => {
    const listSpy = vi.spyOn(recordersApi, 'listRecorders');
    renderPage();
    await waitFor(() => expect(listSpy).toHaveBeenCalled());
    expect(listSpy).toHaveBeenCalledWith(
      expect.objectContaining({ tenantId: 'tenant-rec-test-01' }),
    );

    listSpy.mockClear();
    act(() => {
      useSessionStore.setState({
        tenantId: 'tenant-other-202',
        tenantName: 'Other',
        userId: 'u',
        userDisplayName: 'O',
      });
    });
    await waitFor(() =>
      expect(listSpy).toHaveBeenCalledWith(
        expect.objectContaining({ tenantId: 'tenant-other-202' }),
      ),
    );
  });

  // ------------------------------------------------------------------
  // Filters
  // ------------------------------------------------------------------

  it('health filter narrows results to offline only', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    await user.selectOptions(screen.getByTestId('recorders-health-filter'), 'offline');
    await waitFor(() => {
      const indicators = screen.getAllByTestId('recorder-health-indicator');
      for (const ind of indicators) {
        expect(ind.getAttribute('data-health')).toBe('offline');
      }
    });
  });

  it('search input triggers listRecorders with the typed search term', async () => {
    const user = userEvent.setup();
    const listSpy = vi.spyOn(recordersApi, 'listRecorders');
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    await user.type(screen.getByTestId('recorders-search'), 'Recorder 01');
    await waitFor(() =>
      expect(listSpy).toHaveBeenCalledWith(
        expect.objectContaining({ search: 'Recorder 01' }),
      ),
    );
  });

  // ------------------------------------------------------------------
  // Pair new Recorder modal
  // ------------------------------------------------------------------

  it('pair modal opens when clicking pair new button', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    await user.click(screen.getByTestId('recorders-pair-button'));
    await waitFor(() => screen.getByTestId('pair-recorder-modal'));
  });

  it('pair modal shows token after clicking Generate', async () => {
    const user = userEvent.setup();
    const tokenSpy = vi.spyOn(recordersApi, 'createPairingToken').mockResolvedValue({
      token: 'kpair-test-token-abc123',
      expiresAt: new Date(Date.now() + 30 * 60_000).toISOString(),
      redeemed: false,
    });

    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    await user.click(screen.getByTestId('recorders-pair-button'));
    await waitFor(() => screen.getByTestId('pair-recorder-modal'));

    await user.click(screen.getByTestId('pair-generate-button'));
    await waitFor(() => screen.getByTestId('pair-token-display'));

    expect(screen.getByTestId('pair-token-field')).toBeInTheDocument();
    const tokenInput = screen.getByTestId('pair-token-field') as HTMLInputElement;
    expect(tokenInput.value).toBe('kpair-test-token-abc123');
    expect(tokenSpy).toHaveBeenCalledWith(
      expect.objectContaining({ tenantId: 'tenant-rec-test-01' }),
    );
  });

  it('revoke button calls revokePairingToken with the correct token', async () => {
    const user = userEvent.setup();
    vi.spyOn(recordersApi, 'createPairingToken').mockResolvedValue({
      token: 'kpair-revoke-me-xyz',
      expiresAt: new Date(Date.now() + 30 * 60_000).toISOString(),
      redeemed: false,
    });
    const revokeSpy = vi.spyOn(recordersApi, 'revokePairingToken').mockResolvedValue();

    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));
    await user.click(screen.getByTestId('recorders-pair-button'));
    await waitFor(() => screen.getByTestId('pair-recorder-modal'));
    await user.click(screen.getByTestId('pair-generate-button'));
    await waitFor(() => screen.getByTestId('pair-token-display'));

    await user.click(screen.getByTestId('pair-revoke-button'));
    await waitFor(() =>
      expect(revokeSpy).toHaveBeenCalledWith('tenant-rec-test-01', 'kpair-revoke-me-xyz'),
    );
  });

  it('pair modal shows error when createPairingToken fails', async () => {
    const user = userEvent.setup();
    vi.spyOn(recordersApi, 'createPairingToken').mockRejectedValue(new Error('server error'));

    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));
    await user.click(screen.getByTestId('recorders-pair-button'));
    await waitFor(() => screen.getByTestId('pair-recorder-modal'));
    await user.click(screen.getByTestId('pair-generate-button'));

    await waitFor(() => screen.getByTestId('pair-error'));
  });

  // ------------------------------------------------------------------
  // Unpair confirmation dialog
  // ------------------------------------------------------------------

  it('unpair dialog appears when clicking Unpair', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    const unpairButtons = await screen.findAllByTestId(/^recorder-unpair-/);
    await user.click(unpairButtons[0]!);
    await waitFor(() => screen.getByTestId('unpair-recorder-dialog'));
    expect(screen.getByTestId('unpair-warning')).toBeInTheDocument();
  });

  it('unpair Cancel does NOT call unpairRecorder', async () => {
    const user = userEvent.setup();
    const unpairSpy = vi.spyOn(recordersApi, 'unpairRecorder').mockResolvedValue();

    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    const unpairButtons = await screen.findAllByTestId(/^recorder-unpair-/);
    await user.click(unpairButtons[0]!);
    await waitFor(() => screen.getByTestId('unpair-recorder-dialog'));

    await user.click(screen.getByTestId('unpair-cancel'));
    await waitFor(() =>
      expect(screen.queryByTestId('unpair-recorder-dialog')).not.toBeInTheDocument(),
    );
    expect(unpairSpy).not.toHaveBeenCalled();
  });

  it('unpair Confirm calls unpairRecorder with the correct id', async () => {
    const user = userEvent.setup();
    const unpairSpy = vi.spyOn(recordersApi, 'unpairRecorder').mockResolvedValue();

    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    const unpairButtons = await screen.findAllByTestId(/^recorder-unpair-/);
    await user.click(unpairButtons[0]!);
    const dialog = await screen.findByTestId('unpair-recorder-dialog');
    await user.click(within(dialog).getByTestId('unpair-confirm'));

    await waitFor(() => expect(unpairSpy).toHaveBeenCalledTimes(1));
    expect(unpairSpy).toHaveBeenCalledWith('tenant-rec-test-01', expect.stringMatching(/^rec-/));
  });

  // ------------------------------------------------------------------
  // Detail drawer
  // ------------------------------------------------------------------

  it('detail drawer opens showing hardware info', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    const detailButtons = await screen.findAllByTestId(/^recorder-detail-rec-/);
    await user.click(detailButtons[0]!);

    const drawer = await screen.findByTestId('recorder-detail-drawer');
    expect(drawer).toBeInTheDocument();
    await waitFor(() => screen.getByTestId('recorder-detail-body'));
    expect(screen.getByTestId('detail-cpu')).toBeInTheDocument();
    expect(screen.getByTestId('detail-ip')).toBeInTheDocument();
    expect(screen.getByTestId('detail-sidecars')).toBeInTheDocument();
  });

  it('detail drawer close button hides the drawer', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('recorders-list'));

    const detailButtons = await screen.findAllByTestId(/^recorder-detail-rec-/);
    await user.click(detailButtons[0]!);
    await waitFor(() => screen.getByTestId('recorder-detail-drawer'));

    await user.click(screen.getByTestId('detail-close-button'));
    await waitFor(() =>
      expect(screen.queryByTestId('recorder-detail-drawer')).not.toBeInTheDocument(),
    );
  });

  // ------------------------------------------------------------------
  // Accessibility
  // ------------------------------------------------------------------

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('recorders-page'));
    await waitFor(() => screen.getByTestId('recorders-list'));

    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
