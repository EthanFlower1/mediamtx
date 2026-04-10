import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { i18n } from '@/i18n';
import { StaffPage } from './StaffPage';
import { __TEST__ as STAFF_TEST } from '@/api/staff.mock';
import { runAxe } from '@/test/setup';

// KAI-313: Staff Management page tests.
//
// Covers:
//  1. Renders staff table with 6 mock rows.
//  2. Role badges display icon+text+border.
//  3. Invite flow: open dialog, fill email, select role, submit.
//  4. Suspend/Reactivate toggle with confirmation.
//  5. Remove with REMOVE confirmation pattern.
//  6. Roles summary card shows 4 predefined roles.
//  7. Axe smoke scan: zero critical/serious violations.
//  8. Lazy-load registration in App.tsx.

function renderPage(): HTMLElement {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/command/staff']}>
          <Routes>
            <Route path="/command/staff" element={<StaffPage />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return container;
}

async function waitForTable(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByTestId('staff-table')).toBeInTheDocument();
  });
}

describe('StaffPage', () => {
  beforeEach(() => {
    STAFF_TEST.resetStores();
    void i18n.changeLanguage('en');
  });

  it('renders 6 staff members in the table', async () => {
    renderPage();
    await waitForTable();
    const rows = STAFF_TEST.MOCK_STAFF;
    expect(rows).toHaveLength(6);
    for (const m of rows) {
      expect(screen.getByTestId(`staff-row-${m.id}`)).toBeInTheDocument();
    }
  });

  it('displays role badges with icon+text+border for each role type', async () => {
    renderPage();
    await waitForTable();

    // Owner badge (Alice Chen)
    const ownerBadge = screen.getAllByTestId('role-badge-owner');
    expect(ownerBadge.length).toBeGreaterThan(0);
    expect(ownerBadge[0]).toHaveTextContent('[Crown]');
    expect(ownerBadge[0]).toHaveTextContent('Owner');

    // Admin badge (Bob Martinez)
    const adminBadges = screen.getAllByTestId('role-badge-admin');
    expect(adminBadges.length).toBeGreaterThan(0);
    expect(adminBadges[0]).toHaveTextContent('[Shield]');

    // Technician badge
    const techBadges = screen.getAllByTestId('role-badge-technician');
    expect(techBadges.length).toBeGreaterThan(0);
    expect(techBadges[0]).toHaveTextContent('[Wrench]');

    // Viewer badge
    const viewerBadges = screen.getAllByTestId('role-badge-viewer');
    expect(viewerBadges.length).toBeGreaterThan(0);
    expect(viewerBadges[0]).toHaveTextContent('[Eye]');
  });

  it('shows roles summary card with 4 predefined roles', async () => {
    renderPage();
    await waitForTable();
    const summary = screen.getByTestId('roles-summary');
    expect(summary).toBeInTheDocument();
    expect(screen.getByTestId('role-def-owner')).toBeInTheDocument();
    expect(screen.getByTestId('role-def-admin')).toBeInTheDocument();
    expect(screen.getByTestId('role-def-technician')).toBeInTheDocument();
    expect(screen.getByTestId('role-def-viewer')).toBeInTheDocument();
  });

  it('invite flow: opens dialog, validates email, submits successfully', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForTable();

    // Open invite dialog
    await user.click(screen.getByTestId('invite-open'));
    expect(screen.getByTestId('invite-dialog')).toBeInTheDocument();

    // Submit without email shows error
    await user.click(screen.getByTestId('invite-submit'));
    expect(screen.getByTestId('invite-email-error')).toBeInTheDocument();

    // Fill valid email and submit
    await user.type(screen.getByTestId('invite-email'), 'newstaff@example.com');
    await user.click(screen.getByTestId('invite-submit'));

    // Dialog closes on success
    await waitFor(() => {
      expect(screen.queryByTestId('invite-dialog')).not.toBeInTheDocument();
    });
  });

  it('suspend/reactivate: opens confirmation dialog and completes action', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForTable();

    // Click suspend on active staff (Carol Davis - staff-003)
    await user.click(screen.getByTestId('suspend-staff-003'));
    expect(screen.getByTestId('suspend-dialog')).toBeInTheDocument();
    expect(screen.getByTestId('suspend-impact')).toHaveTextContent(/Carol Davis/);

    // Confirm suspend
    await user.click(screen.getByTestId('suspend-confirm'));
    await waitFor(() => {
      expect(screen.queryByTestId('suspend-dialog')).not.toBeInTheDocument();
    });

    // Click reactivate on suspended staff (Eva Johnson - staff-005)
    await user.click(screen.getByTestId('suspend-staff-005'));
    expect(screen.getByTestId('suspend-dialog')).toBeInTheDocument();

    // Confirm reactivate
    await user.click(screen.getByTestId('suspend-confirm'));
    await waitFor(() => {
      expect(screen.queryByTestId('suspend-dialog')).not.toBeInTheDocument();
    });
  });

  it('remove: requires typing REMOVE to enable confirm button', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForTable();

    // Click remove on Frank Lee (staff-006)
    await user.click(screen.getByTestId('remove-staff-006'));
    expect(screen.getByTestId('remove-dialog')).toBeInTheDocument();

    // Confirm button is disabled without typing REMOVE
    const confirmBtn = screen.getByTestId('remove-confirm');
    expect(confirmBtn).toBeDisabled();

    // Type wrong text
    await user.type(screen.getByTestId('remove-confirm-input'), 'remove');
    expect(confirmBtn).toBeDisabled();

    // Type correct REMOVE
    await user.clear(screen.getByTestId('remove-confirm-input'));
    await user.type(screen.getByTestId('remove-confirm-input'), 'REMOVE');
    expect(confirmBtn).not.toBeDisabled();

    // Click confirm
    await user.click(confirmBtn);
    await waitFor(() => {
      expect(screen.queryByTestId('remove-dialog')).not.toBeInTheDocument();
    });
  });

  it('axe smoke scan: no critical or serious violations', async () => {
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

  it('StaffPage is registered as a lazy-loaded route in App.tsx', async () => {
    const fs = await import('node:fs/promises');
    const path = await import('node:path');
    const file = path.resolve(__dirname, '../../App.tsx');
    const text = await fs.readFile(file, 'utf-8');
    expect(text).toMatch(/lazy\s*\(\s*\(\)\s*=>\s*import\(['"][^'"]*StaffPage['"]\)/);
    expect(text).toMatch(/\/command\/staff/);
  });
});
