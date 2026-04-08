import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within, act } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { CamerasPage } from './CamerasPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as camerasApi from '@/api/cameras';

// KAI-321 tests:
//   - list renders + filter + sort
//   - add wizard ONVIF candidates pickable
//   - manual entry validation (bad RTSP URL)
//   - edit pre-fills current camera
//   - move dialog mutation called with target recorder
//   - delete name-confirm gating
//   - bulk delete with 3 selected rows
//   - axe smoke (no critical/serious violations)
//   - tenant scope: queries fired with current session tenantId

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/cameras']}>
          <CamerasPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('CamerasPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-77',
      tenantName: 'Test Tenant 77',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    vi.restoreAllMocks();
  });

  it('renders the camera list with windowed rows (30 cameras backing the view)', async () => {
    const listSpy = vi.spyOn(camerasApi, 'listCameras');
    renderPage();
    await waitFor(() =>
      expect(screen.getByTestId('cameras-list')).toBeInTheDocument(),
    );
    // The data layer holds all 30 sample cameras for the tenant.
    await waitFor(() => {
      const lastResult = listSpy.mock.results[listSpy.mock.results.length - 1];
      expect(lastResult).toBeDefined();
    });
    const lastResult = listSpy.mock.results[listSpy.mock.results.length - 1]!;
    const data = await (lastResult.value as Promise<unknown[]>);
    expect(data.length).toBe(30);

    // The virtualized table renders a windowed subset (>= 10 visible rows).
    const rendered = await screen.findAllByTestId(/^camera-row-/);
    expect(rendered.length).toBeGreaterThanOrEqual(10);
    expect(rendered.length).toBeLessThanOrEqual(30);
  });

  it('filters by status (offline only)', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    await user.selectOptions(screen.getByTestId('cameras-status-filter'), 'offline');

    await waitFor(() => {
      const rows = screen.queryAllByTestId(/^camera-row-/);
      // Status pattern repeats every 7 (5 online, 1 warning, 1 offline) → 30/7 ≈ 4 offline.
      expect(rows.length).toBeGreaterThan(0);
      expect(rows.length).toBeLessThan(30);
      // Every visible row's status indicator must read "offline".
      const indicators = screen.getAllByTestId('camera-status-indicator');
      for (const ind of indicators) {
        expect(ind.getAttribute('data-status')).toBe('offline');
      }
    });
  });

  it('search input narrows the visible list', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    await user.type(screen.getByTestId('cameras-search'), 'Camera 001');

    await waitFor(() => {
      const rows = screen.getAllByTestId(/^camera-row-/);
      expect(rows.length).toBe(1);
    });
  });

  it('toggles column sort direction when clicking the name header button', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    const nameHeader = screen.getByTestId('cameras-header-name');
    expect(nameHeader.getAttribute('aria-sort')).toBe('none');

    await user.click(within(nameHeader).getByRole('button'));
    await waitFor(() =>
      expect(screen.getByTestId('cameras-header-name').getAttribute('aria-sort')).toBe(
        'ascending',
      ),
    );

    await user.click(within(screen.getByTestId('cameras-header-name')).getByRole('button'));
    await waitFor(() =>
      expect(screen.getByTestId('cameras-header-name').getAttribute('aria-sort')).toBe(
        'descending',
      ),
    );
  });

  it('add wizard ONVIF flow lists discovered candidates', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    await user.click(screen.getByTestId('cameras-add-button'));
    await waitFor(() => screen.getByTestId('add-camera-wizard'));

    await user.click(screen.getByTestId('wizard-method-onvif'));
    await user.click(screen.getByTestId('wizard-next'));

    await waitFor(() => screen.getByTestId('onvif-candidates'));
    const candidates = within(screen.getByTestId('onvif-candidates')).getAllByRole(
      'radio',
    );
    expect(candidates.length).toBe(3);
  });

  it('manual entry rejects an invalid RTSP URL', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    await user.click(screen.getByTestId('cameras-add-button'));
    await waitFor(() => screen.getByTestId('add-camera-wizard'));
    // Default method is 'manual'.
    await user.click(screen.getByTestId('wizard-next'));

    await waitFor(() => screen.getByTestId('wizard-field-rtsp'));
    await user.type(screen.getByTestId('wizard-field-rtsp'), 'http://not-rtsp.example');
    await user.click(screen.getByTestId('wizard-next'));

    expect(await screen.findByTestId('wizard-url-error')).toBeInTheDocument();
  });

  it('password field uses type=password (never plain text)', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    await user.click(screen.getByTestId('cameras-add-button'));
    await waitFor(() => screen.getByTestId('add-camera-wizard'));
    await user.click(screen.getByTestId('wizard-next'));

    const password = await screen.findByTestId('wizard-field-password');
    expect(password.getAttribute('type')).toBe('password');
  });

  it('edit modal pre-fills the selected camera', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    const editButtons = await screen.findAllByTestId(/^camera-edit-/);
    await user.click(editButtons[0]!);

    const modal = await screen.findByTestId('edit-camera-modal');
    const nameInput = within(modal).getByTestId('edit-field-name') as HTMLInputElement;
    expect(nameInput.value).toBe('Camera 001');
  });

  it('move dialog calls moveCamera with the chosen recorder id', async () => {
    const user = userEvent.setup();
    const moveSpy = vi.spyOn(camerasApi, 'moveCamera');
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    const moveButtons = await screen.findAllByTestId(/^camera-move-/);
    await user.click(moveButtons[0]!);

    const dialog = await screen.findByTestId('move-camera-dialog');
    const select = within(dialog).getByTestId('move-target-recorder') as HTMLSelectElement;
    // Pick the first non-current recorder option.
    const options = Array.from(select.options).filter((o) => o.value !== '');
    expect(options.length).toBeGreaterThan(0);
    await user.selectOptions(select, options[0]!.value);
    await user.click(within(dialog).getByTestId('move-confirm'));

    await waitFor(() => {
      expect(moveSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          tenantId: 'tenant-test-77',
          targetRecorderId: options[0]!.value,
        }),
      );
    });
  });

  it('delete confirm requires typing the camera name', async () => {
    const user = userEvent.setup();
    const deleteSpy = vi.spyOn(camerasApi, 'deleteCamera');
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    const deleteButtons = await screen.findAllByTestId(/^camera-delete-/);
    await user.click(deleteButtons[0]!);

    const dialog = await screen.findByTestId('delete-camera-dialog');
    const confirm = within(dialog).getByTestId('delete-confirm') as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    // Wrong text — still disabled.
    await user.type(within(dialog).getByTestId('delete-type-input'), 'wrong');
    expect((within(dialog).getByTestId('delete-confirm') as HTMLButtonElement).disabled).toBe(
      true,
    );

    // Clear and type the exact camera name.
    const input = within(dialog).getByTestId('delete-type-input') as HTMLInputElement;
    await user.clear(input);
    await user.type(input, 'Camera 001');
    await waitFor(() =>
      expect(
        (within(dialog).getByTestId('delete-confirm') as HTMLButtonElement).disabled,
      ).toBe(false),
    );

    await user.click(within(dialog).getByTestId('delete-confirm'));
    await waitFor(() =>
      expect(deleteSpy).toHaveBeenCalledWith(
        expect.objectContaining({ tenantId: 'tenant-test-77' }),
      ),
    );
  });

  it('bulk delete fires deleteCamera once per selected row (3 selected)', async () => {
    const user = userEvent.setup();
    const deleteSpy = vi.spyOn(camerasApi, 'deleteCamera').mockResolvedValue();
    renderPage();
    await waitFor(() => screen.getByTestId('cameras-list'));

    const checkboxes = await screen.findAllByTestId(/^camera-select-/);
    await user.click(checkboxes[0]!);
    await user.click(checkboxes[1]!);
    await user.click(checkboxes[2]!);

    await user.click(screen.getByTestId('cameras-bulk-delete'));
    await waitFor(() => expect(deleteSpy).toHaveBeenCalledTimes(3));
  });

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await waitFor(() => screen.getByTestId('cameras-page'));
    // Wait for list to be in DOM so axe sees real content.
    await waitFor(() => screen.getByTestId('cameras-list'));

    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });

  it('scopes camera queries to the active session tenant', async () => {
    const listSpy = vi.spyOn(camerasApi, 'listCameras');
    renderPage();
    await waitFor(() => expect(listSpy).toHaveBeenCalled());
    expect(listSpy).toHaveBeenCalledWith(
      expect.objectContaining({ tenantId: 'tenant-test-77' }),
    );

    // Switching tenants triggers a re-fetch with the new id.
    listSpy.mockClear();
    act(() => {
      useSessionStore.setState({
        tenantId: 'tenant-other-99',
        tenantName: 'Other Tenant',
        userId: 'u',
        userDisplayName: 'O',
      });
    });
    await waitFor(() =>
      expect(listSpy).toHaveBeenCalledWith(
        expect.objectContaining({ tenantId: 'tenant-other-99' }),
      ),
    );
  });
});
