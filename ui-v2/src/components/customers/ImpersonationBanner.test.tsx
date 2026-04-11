import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { ImpersonationBanner } from './ImpersonationBanner';
import { useImpersonationStore } from '@/stores/impersonation';
import { i18n } from '@/i18n';
import * as impersonationApi from '@/api/impersonation';

// KAI-467 tests: ImpersonationBanner
//   - Does not render when no session active
//   - Renders banner with customer name when session active
//   - Shows countdown timer
//   - End button terminates session
//   - Auto-clears after session timeout

function renderBanner(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <ImpersonationBanner />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

const MOCK_SESSION: impersonationApi.ImpersonationSessionDetail = {
  sessionId: 'test-sess-001',
  mode: 'integrator',
  impersonatingUserId: 'user-001',
  impersonatingUserName: 'Jane',
  impersonatingTenantId: 'integrator-001',
  impersonatedTenantId: 'cust-001',
  impersonatedTenantName: 'Acme Corp',
  scopedPermissions: ['view.live'],
  status: 'active',
  reason: 'Testing camera issue',
  createdAtIso: new Date().toISOString(),
  expiresAtIso: new Date(Date.now() + 15 * 60 * 1000).toISOString(),
  terminatedAtIso: null,
  terminatedBy: null,
};

describe('ImpersonationBanner', () => {
  beforeEach(() => {
    useImpersonationStore.setState({ session: null, _timeoutId: null });
    vi.restoreAllMocks();
  });

  afterEach(() => {
    useImpersonationStore.getState().endSession();
  });

  it('does not render when no session is active', () => {
    renderBanner();
    expect(screen.queryByTestId('impersonation-banner')).not.toBeInTheDocument();
  });

  it('renders banner with customer name when session is active', () => {
    useImpersonationStore.getState().startSession(MOCK_SESSION);
    renderBanner();

    const banner = screen.getByTestId('impersonation-banner');
    expect(banner).toBeInTheDocument();
    expect(screen.getByTestId('impersonation-banner-text')).toHaveTextContent(
      'Acme Corp',
    );
  });

  it('shows countdown timer', () => {
    useImpersonationStore.getState().startSession(MOCK_SESSION);
    renderBanner();

    const countdown = screen.getByTestId('impersonation-banner-countdown');
    expect(countdown).toBeInTheDocument();
    // Should show approximately 15 minutes remaining.
    expect(countdown.textContent).toMatch(/1[45]:\d{2}\s*remaining/);
  });

  it('end button terminates session via API and clears store', async () => {
    const terminateSpy = vi
      .spyOn(impersonationApi, 'terminateSession')
      .mockResolvedValue(undefined);

    useImpersonationStore.getState().startSession(MOCK_SESSION);
    renderBanner();

    const endButton = screen.getByTestId('impersonation-end-button');
    expect(endButton).toBeInTheDocument();

    await userEvent.click(endButton);

    await waitFor(() => {
      expect(terminateSpy).toHaveBeenCalledWith('test-sess-001');
    });

    await waitFor(() => {
      expect(useImpersonationStore.getState().session).toBeNull();
    });
  });

  it('has role="alert" for screen reader announcement', () => {
    useImpersonationStore.getState().startSession(MOCK_SESSION);
    renderBanner();

    const banner = screen.getByTestId('impersonation-banner');
    expect(banner).toHaveAttribute('role', 'alert');
  });
});
