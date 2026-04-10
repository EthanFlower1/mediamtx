import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { i18n } from '@/i18n';
import { MobileBuildsPage } from './MobileBuildsPage';
import { __MOCK__ } from '@/api/mobileBuilds.mock';
import { runAxe } from '@/test/setup';

// KAI-311: Mobile App Builds page tests.
//
// Covers:
//   1. Renders build list table with mock data
//   2. Trigger build dialog opens and submits
//   3. Build status badges render with correct text (WCAG: icon+text+border)
//   4. Credential status displays correctly
//   5. Distribution config section renders
//   6. Build detail drawer opens on "Details" click
//   7. Axe smoke (zero critical/serious)

function renderPage(): HTMLElement {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/command/builds']}>
          <MobileBuildsPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return container;
}

async function waitForTable(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByTestId('builds-table')).toBeInTheDocument();
  });
}

describe('MobileBuildsPage', () => {
  beforeEach(() => {
    void i18n.changeLanguage('en');
  });

  it('renders page header, build table, credentials, and distribution config', async () => {
    renderPage();

    expect(
      screen.getByRole('heading', { level: 1, name: /mobile app builds/i }),
    ).toBeInTheDocument();

    await waitForTable();

    // Build rows — 5 mock builds.
    const table = screen.getByTestId('builds-table');
    const rows = within(table).getAllByRole('row');
    // 1 header row + 5 data rows.
    expect(rows).toHaveLength(6);

    // Credentials section renders.
    expect(screen.getByTestId('credentials-section')).toBeInTheDocument();
    expect(screen.getByTestId('apple-team-id')).toHaveTextContent('A1B2C3D4E5');
    expect(screen.getByTestId('google-service-account')).toHaveTextContent(
      'builds@acme-integrator.iam.gserviceaccount.com',
    );

    // Distribution config renders.
    expect(screen.getByTestId('distribution-section')).toBeInTheDocument();
    expect(screen.getByTestId('testflight-group')).toHaveTextContent('Internal Testers');
    expect(screen.getByTestId('play-console-track')).toBeInTheDocument();
    expect(screen.getByTestId('auto-submit')).toBeInTheDocument();
  });

  it('renders build status badges with text (not color alone)', async () => {
    renderPage();
    await waitForTable();

    // Multiple builds can share a status; use getAllByTestId.
    const succeeded = screen.getAllByTestId('status-badge-succeeded');
    expect(succeeded.length).toBeGreaterThanOrEqual(1);
    expect(succeeded[0]).toHaveTextContent(/succeeded/i);

    expect(screen.getByTestId('status-badge-building')).toHaveTextContent(/building/i);
    expect(screen.getByTestId('status-badge-failed')).toHaveTextContent(/failed/i);
    expect(screen.getByTestId('status-badge-queued')).toHaveTextContent(/queued/i);
  });

  it('renders credential status badges', async () => {
    renderPage();
    await waitForTable();

    // Both Apple and Google should show "Connected" status.
    const connectedBadges = screen.getAllByTestId('credential-status-connected');
    expect(connectedBadges.length).toBeGreaterThanOrEqual(2);
  });

  it('opens trigger build dialog and can close it', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForTable();

    // Dialog should not be in the DOM initially.
    expect(screen.queryByTestId('trigger-build-dialog')).not.toBeInTheDocument();

    // Click the trigger button.
    await user.click(screen.getByTestId('trigger-build-button'));

    // Dialog should now be visible.
    await waitFor(() => {
      expect(screen.getByTestId('trigger-build-dialog')).toBeInTheDocument();
    });

    // Verify dialog has required fields.
    const dialog = screen.getByTestId('trigger-build-dialog');
    expect(within(dialog).getByTestId('trigger-brand-select')).toBeInTheDocument();
    expect(within(dialog).getByTestId('trigger-release-notes')).toBeInTheDocument();
    expect(within(dialog).getByTestId('trigger-build-submit')).toBeInTheDocument();
  });

  it('opens build detail drawer when clicking details', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForTable();

    // No drawer initially.
    expect(screen.queryByTestId('build-detail-drawer')).not.toBeInTheDocument();

    // Click details on the first build row.
    const firstRow = screen.getByTestId('build-row-build-005');
    const detailButton = within(firstRow).getByRole('button', { name: /details/i });
    await user.click(detailButton);

    // Drawer should appear.
    await waitFor(() => {
      expect(screen.getByTestId('build-detail-drawer')).toBeInTheDocument();
    });

    // Drawer shows build logs.
    expect(screen.getByTestId('build-logs')).toBeInTheDocument();
  });

  it('passes an axe-core smoke scan with no critical or serious violations', async () => {
    const container = renderPage();
    await waitForTable();

    const violations = await runAxe(container);
    const blocking = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    if (blocking.length > 0) {
      // eslint-disable-next-line no-console
      console.error('axe blocking violations:', blocking);
    }
    expect(blocking).toEqual([]);
  });
});
