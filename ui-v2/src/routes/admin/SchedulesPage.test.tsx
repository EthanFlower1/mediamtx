// KAI-326: Component tests for SchedulesPage.
//
// Coverage:
//   - Renders default schedule templates
//   - Create modal opens, validates required fields
//   - Delete requires typed confirmation
//   - Retention overview card shows mock data
//   - Weekly timeline renders 7 day labels
//   - Bulk assign flow
//   - axe a11y check
//   - Tenant isolation

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { SchedulesPage } from './SchedulesPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as schedulesMock from '@/api/schedules.mock';

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/schedules']}>
          <SchedulesPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('SchedulesPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-326',
      tenantName: 'Test Tenant 326',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
  });

  // ---------------------------------------------------------------------------
  // Schedule templates list
  // ---------------------------------------------------------------------------

  it('renders default schedule templates in the table', async () => {
    const listSpy = vi.spyOn(schedulesMock, 'listSchedules');
    renderPage();
    await waitFor(() => expect(screen.getByTestId('schedules-table')).toBeInTheDocument());
    expect(listSpy).toHaveBeenCalledWith('tenant-test-326');
    const rows = screen.queryAllByTestId(/^schedule-row-/);
    expect(rows.length).toBe(5);
  });

  // ---------------------------------------------------------------------------
  // Create modal — validation
  // ---------------------------------------------------------------------------

  it('create modal opens and validates required name field', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('schedules-table'));

    await user.click(screen.getByTestId('schedules-create-button'));
    await waitFor(() => screen.getByTestId('schedule-modal'));

    // Submit without filling name
    await user.click(screen.getByTestId('schedule-submit'));
    expect(await screen.findByTestId('schedule-error-name')).toBeInTheDocument();
  });

  it('create modal submits with valid data', async () => {
    const createSpy = vi.spyOn(schedulesMock, 'createSchedule');
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('schedules-table'));

    await user.click(screen.getByTestId('schedules-create-button'));
    await waitFor(() => screen.getByTestId('schedule-modal'));

    await user.type(screen.getByTestId('schedule-field-name'), 'Night Watch');
    await user.click(screen.getByTestId('schedule-type-motion'));
    await user.click(screen.getByTestId('schedule-submit'));

    await waitFor(() =>
      expect(createSpy).toHaveBeenCalledWith(
        expect.objectContaining({ name: 'Night Watch', type: 'motion' }),
      ),
    );
  });

  // ---------------------------------------------------------------------------
  // Delete requires typed confirmation
  // ---------------------------------------------------------------------------

  it('delete requires typing the schedule name before enabling confirm', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('schedules-table'));

    const deleteButtons = screen.getAllByTestId(/^schedule-delete-/);
    await user.click(deleteButtons[0]!);

    const dialog = await screen.findByTestId('delete-schedule-dialog');
    const confirmBtn = within(dialog).getByTestId('delete-confirm') as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);

    // Type wrong name
    await user.type(within(dialog).getByTestId('delete-type-input'), 'wrong-name');
    expect((within(dialog).getByTestId('delete-confirm') as HTMLButtonElement).disabled).toBe(true);

    // Clear and type correct name
    await user.clear(within(dialog).getByTestId('delete-type-input'));
    await user.type(within(dialog).getByTestId('delete-type-input'), '24/7 Continuous');
    expect((within(dialog).getByTestId('delete-confirm') as HTMLButtonElement).disabled).toBe(false);
  });

  // ---------------------------------------------------------------------------
  // Retention overview card
  // ---------------------------------------------------------------------------

  it('retention overview card shows mock data for all tiers', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('retention-overview'));

    for (const tier of ['7d', '30d', '90d', '1yr', 'forensic']) {
      expect(screen.getByTestId(`retention-tier-${tier}`)).toBeInTheDocument();
    }
  });

  // ---------------------------------------------------------------------------
  // Weekly timeline
  // ---------------------------------------------------------------------------

  it('weekly timeline renders 7 day labels', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('schedules-table'));

    for (let day = 0; day <= 6; day++) {
      const labels = screen.getAllByTestId(`timeline-day-${day}`);
      expect(labels.length).toBeGreaterThan(0);
    }
  });

  // ---------------------------------------------------------------------------
  // Bulk assign flow
  // ---------------------------------------------------------------------------

  it('bulk assign flow: select cameras then assign to schedule', async () => {
    const bulkSpy = vi.spyOn(schedulesMock, 'bulkAssignSchedule');
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('schedules-table'));
    await waitFor(() => screen.getByTestId('camera-selection'));

    // Select two cameras
    const camCheckboxes = screen.getAllByTestId(/^camera-select-/);
    await user.click(camCheckboxes[0]!);
    await user.click(camCheckboxes[1]!);

    // Open bulk assign
    await user.click(screen.getByTestId('schedules-bulk-assign-button'));
    await waitFor(() => screen.getByTestId('bulk-assign-modal'));

    // Select a schedule
    await user.selectOptions(screen.getByTestId('bulk-assign-schedule-select'), screen.getAllByRole('option')[1]!);

    await user.click(screen.getByTestId('bulk-assign-confirm'));
    await waitFor(() => expect(bulkSpy).toHaveBeenCalled());
  });

  // ---------------------------------------------------------------------------
  // Tenant isolation
  // ---------------------------------------------------------------------------

  it('scopes schedule queries to the active session tenant', async () => {
    const listSpy = vi.spyOn(schedulesMock, 'listSchedules');
    renderPage();
    await waitFor(() => expect(listSpy).toHaveBeenCalledWith('tenant-test-326'));
  });

  // ---------------------------------------------------------------------------
  // a11y smoke
  // ---------------------------------------------------------------------------

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('schedules-page'));
    await waitFor(() => screen.getByTestId('schedules-table'));
    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
