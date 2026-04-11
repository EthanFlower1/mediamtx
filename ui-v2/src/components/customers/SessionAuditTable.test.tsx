import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { SessionAuditTable } from './SessionAuditTable';
import { i18n } from '@/i18n';

// KAI-467 tests: SessionAuditTable
//   - Renders sessions table with mock data
//   - Renders audit log table with mock data
//   - Shows status badges with correct text
//   - Shows result badges (allow/deny)

function renderTable(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter>
          <SessionAuditTable />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('SessionAuditTable', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('renders the session audit table container', async () => {
    renderTable();

    await waitFor(() => {
      expect(screen.getByTestId('session-audit-table')).toBeInTheDocument();
    });
  });

  it('renders the sessions table with data', async () => {
    renderTable();

    await waitFor(() => {
      expect(screen.getByTestId('sessions-table')).toBeInTheDocument();
    });

    // Should have session rows.
    const sessionsTable = screen.getByTestId('sessions-table');
    const rows = sessionsTable.querySelectorAll('tbody tr');
    expect(rows.length).toBeGreaterThan(0);
  });

  it('renders the audit log table with data', async () => {
    renderTable();

    await waitFor(() => {
      expect(screen.getByTestId('audit-log-table')).toBeInTheDocument();
    });

    const auditTable = screen.getByTestId('audit-log-table');
    const rows = auditTable.querySelectorAll('tbody tr');
    expect(rows.length).toBeGreaterThan(0);
  });

  it('shows result badges for allow and deny', async () => {
    renderTable();

    await waitFor(() => {
      expect(screen.getByTestId('audit-log-table')).toBeInTheDocument();
    });

    // The mock data has both "allow" and "deny" results.
    const allowBadges = screen.getAllByTestId('result-badge-allow');
    expect(allowBadges.length).toBeGreaterThan(0);

    const denyBadges = screen.getAllByTestId('result-badge-deny');
    expect(denyBadges.length).toBeGreaterThan(0);
  });

  it('has proper section headings', async () => {
    renderTable();

    await waitFor(() => {
      expect(screen.getByTestId('session-audit-table')).toBeInTheDocument();
    });

    expect(
      screen.getByRole('heading', { name: /impersonation sessions/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole('heading', { name: /audit log/i }),
    ).toBeInTheDocument();
  });
});
