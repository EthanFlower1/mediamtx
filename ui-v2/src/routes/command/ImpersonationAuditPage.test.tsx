import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { ImpersonationAuditPage } from './ImpersonationAuditPage';
import { i18n } from '@/i18n';

// KAI-467 tests: ImpersonationAuditPage
//   - Renders page with heading and breadcrumb
//   - Contains the SessionAuditTable

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/command/impersonation-audit']}>
          <ImpersonationAuditPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('ImpersonationAuditPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders page with heading', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId('impersonation-audit-page')).toBeInTheDocument();
    });

    expect(
      screen.getByRole('heading', { level: 1, name: /impersonation audit/i }),
    ).toBeInTheDocument();
  });

  it('contains breadcrumb navigation', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId('breadcrumb-portal')).toBeInTheDocument();
    });
  });

  it('contains the session audit table', async () => {
    renderPage();

    await waitFor(() => {
      expect(screen.getByTestId('session-audit-table')).toBeInTheDocument();
    });
  });
});
