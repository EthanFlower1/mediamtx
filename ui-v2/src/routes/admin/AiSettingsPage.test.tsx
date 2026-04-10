// KAI-327: Tests for AiSettingsPage.
//
// Coverage:
//   - Renders without crashing
//   - All 6 feature toggles visible
//   - Disable face-recognition → warning modal
//   - Enable face-recognition → ack modal, confirm disabled until checkbox
//   - Face vault section only when face-recognition enabled
//   - Summary card shows mock data
//   - Add enrollment modal validates consent checkbox
//   - Delete requires typed "DELETE"
//   - Emergency purge requires typed PURGE-TENANT-{tenantName}
//   - Search + consent filter narrow the list
//   - axe a11y smoke
//   - Tenant isolation — changing session tenant triggers refetch

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, act, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { AiSettingsPage } from './AiSettingsPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import * as aiSettingsApi from '@/api/aiSettings';
import * as faceVaultApi from '@/api/faceVault';
import { __resetAiSettingsMockStateForTests } from '@/api/aiSettings.mock';
import { __resetFaceVaultMockStateForTests } from '@/api/faceVault.mock';

function renderPage(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/ai-settings']}>
          <AiSettingsPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

async function enableFaceRecognition(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await waitFor(() => screen.getByTestId('feature-toggle-face-recognition'));
  await user.click(screen.getByTestId('feature-toggle-face-recognition'));
  await waitFor(() => screen.getByTestId('enable-face-dialog'));
  await user.click(screen.getByTestId('enable-face-ack-checkbox'));
  await user.click(screen.getByTestId('enable-face-confirm'));
  await waitFor(() => screen.getByTestId('face-vault-section'));
}

describe('AiSettingsPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-327',
      tenantName: 'Test Tenant 327',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    __resetAiSettingsMockStateForTests();
    __resetFaceVaultMockStateForTests();
    vi.restoreAllMocks();
  });

  it('renders without crashing and shows the compliance banner', async () => {
    renderPage();
    expect(await screen.findByTestId('ai-settings-page')).toBeInTheDocument();
    expect(screen.getByTestId('ai-settings-compliance-banner')).toBeInTheDocument();
  });

  it('renders all 6 feature toggles', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('feature-toggle-object-detection'));
    for (const key of [
      'object-detection',
      'face-recognition',
      'lpr',
      'behavioral-analytics',
      'semantic-search',
      'custom-models',
    ]) {
      expect(screen.getByTestId(`feature-card-${key}`)).toBeInTheDocument();
      expect(screen.getByTestId(`feature-toggle-${key}`)).toBeInTheDocument();
    }
  });

  it('scopes ai-settings queries to the active tenant and refetches on tenant switch', async () => {
    const spy = vi.spyOn(aiSettingsApi, 'listAiFeatures');
    renderPage();
    await waitFor(() => expect(spy).toHaveBeenCalledWith('tenant-test-327'));

    spy.mockClear();
    act(() => {
      useSessionStore.setState({
        tenantId: 'tenant-other-999',
        tenantName: 'Other Tenant',
        userId: 'u',
        userDisplayName: 'O',
      });
    });
    await waitFor(() => expect(spy).toHaveBeenCalledWith('tenant-other-999'));
  });

  it('face vault section is hidden until face-recognition is enabled', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('feature-toggle-face-recognition'));
    expect(screen.queryByTestId('face-vault-section')).not.toBeInTheDocument();
    expect(screen.getByTestId('face-vault-disabled-hint')).toBeInTheDocument();
  });

  it('enabling face-recognition requires the ack checkbox before confirm is enabled', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitFor(() => screen.getByTestId('feature-toggle-face-recognition'));

    await user.click(screen.getByTestId('feature-toggle-face-recognition'));
    await waitFor(() => screen.getByTestId('enable-face-dialog'));

    const confirm = screen.getByTestId('enable-face-confirm') as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    await user.click(screen.getByTestId('enable-face-ack-checkbox'));
    expect(
      (screen.getByTestId('enable-face-confirm') as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it('disabling face-recognition opens the warning modal before mutating', async () => {
    const user = userEvent.setup();
    const setSpy = vi.spyOn(aiSettingsApi, 'setAiFeatureEnabled');
    renderPage();
    await enableFaceRecognition(user);
    setSpy.mockClear();

    await user.click(screen.getByTestId('feature-toggle-face-recognition'));
    await waitFor(() => screen.getByTestId('disable-face-dialog'));
    // setAiFeatureEnabled must NOT have been called yet — the dialog gates it.
    expect(setSpy).not.toHaveBeenCalled();

    await user.click(screen.getByTestId('disable-face-confirm'));
    await waitFor(() => expect(setSpy).toHaveBeenCalled());
  });

  it('face vault summary card displays mock data once enabled', async () => {
    const user = userEvent.setup();
    renderPage();
    await enableFaceRecognition(user);

    const summary = await screen.findByTestId('face-vault-summary');
    expect(summary).toBeInTheDocument();
    const total = await screen.findByTestId('face-vault-summary-total');
    expect(Number(total.textContent)).toBeGreaterThan(0);
  });

  it('add enrollment modal requires the consent checkbox for Art 9 explicit consent', async () => {
    const user = userEvent.setup();
    const enrollSpy = vi.spyOn(faceVaultApi, 'enrollFace');
    renderPage();
    await enableFaceRecognition(user);

    // Open Add
    await user.click(await screen.findByTestId('face-vault-add-button'));
    await waitFor(() => screen.getByTestId('add-enrollment-dialog'));

    await user.type(screen.getByTestId('add-enrollment-name'), 'Alice Example');
    await user.type(screen.getByTestId('add-enrollment-identifier'), 'SUBJ-42');

    // Default legal basis is art9-explicit-consent → consent checkbox unchecked
    await user.click(screen.getByTestId('add-enrollment-submit'));
    expect(await screen.findByTestId('add-enrollment-error')).toBeInTheDocument();
    expect(enrollSpy).not.toHaveBeenCalled();

    // Now check consent + submit
    await user.click(screen.getByTestId('add-enrollment-consent'));
    await user.click(screen.getByTestId('add-enrollment-submit'));
    await waitFor(() => expect(enrollSpy).toHaveBeenCalled());
  });

  it('delete enrollment requires typing DELETE before the confirm button enables', async () => {
    const user = userEvent.setup();
    const deleteSpy = vi.spyOn(faceVaultApi, 'deleteFaceEnrollment');
    renderPage();
    await enableFaceRecognition(user);

    // Click the first delete row button
    const deleteButtons = await screen.findAllByTestId(/^face-vault-delete-/);
    await user.click(deleteButtons[0]!);
    await waitFor(() => screen.getByTestId('delete-enrollment-dialog'));

    const confirm = screen.getByTestId('delete-enrollment-confirm') as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    await user.type(screen.getByTestId('delete-enrollment-type-input'), 'delete');
    expect(
      (screen.getByTestId('delete-enrollment-confirm') as HTMLButtonElement).disabled,
    ).toBe(true);

    await user.clear(screen.getByTestId('delete-enrollment-type-input'));
    await user.type(screen.getByTestId('delete-enrollment-type-input'), 'DELETE');
    expect(
      (screen.getByTestId('delete-enrollment-confirm') as HTMLButtonElement).disabled,
    ).toBe(false);

    await user.click(screen.getByTestId('delete-enrollment-confirm'));
    await waitFor(() =>
      expect(deleteSpy).toHaveBeenCalledWith(
        expect.objectContaining({ confirmation: 'DELETE' }),
      ),
    );
  });

  it('emergency purge multi-step dialog requires typed PURGE-TENANT-{tenantName}', async () => {
    const user = userEvent.setup();
    const purgeSpy = vi.spyOn(faceVaultApi, 'purgeFaceVault');
    renderPage();
    await enableFaceRecognition(user);

    await user.click(await screen.findByTestId('face-vault-purge-button'));
    await waitFor(() => screen.getByTestId('purge-dialog'));

    // Step 1 → Step 2
    await user.click(screen.getByTestId('purge-next-1'));
    await waitFor(() => screen.getByTestId('purge-step-2'));

    // Step 2 → Step 3
    await user.click(screen.getByTestId('purge-next-2'));
    await waitFor(() => screen.getByTestId('purge-step-3'));

    // Step 3: typed confirmation — "Next" is disabled until sentinel matches
    const next3 = screen.getByTestId('purge-next-3') as HTMLButtonElement;
    expect(next3.disabled).toBe(true);

    await user.type(screen.getByTestId('purge-type-input'), 'wrong-string');
    expect(
      (screen.getByTestId('purge-next-3') as HTMLButtonElement).disabled,
    ).toBe(true);

    await user.clear(screen.getByTestId('purge-type-input'));
    await user.type(
      screen.getByTestId('purge-type-input'),
      'PURGE-TENANT-Test Tenant 327',
    );
    expect(
      (screen.getByTestId('purge-next-3') as HTMLButtonElement).disabled,
    ).toBe(false);

    // Step 3 → Step 4: two-person approval
    await user.click(screen.getByTestId('purge-next-3'));
    await waitFor(() => screen.getByTestId('purge-step-4'));

    // Confirm button disabled until a different admin approves
    const confirm = screen.getByTestId('purge-confirm') as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);

    // Same-principal error: typing the current user's own ID
    fireEvent.change(screen.getByTestId('purge-approver-input'), {
      target: { value: 'user-test' },
    });
    expect(screen.getByTestId('purge-same-principal-error')).toBeTruthy();
    expect(
      (screen.getByTestId('purge-confirm') as HTMLButtonElement).disabled,
    ).toBe(true);

    // Different admin approves
    fireEvent.change(screen.getByTestId('purge-approver-input'), {
      target: { value: 'admin-approver-2' },
    });
    expect(
      (screen.getByTestId('purge-confirm') as HTMLButtonElement).disabled,
    ).toBe(false);

    await user.click(screen.getByTestId('purge-confirm'));
    await waitFor(() =>
      expect(purgeSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          scope: 'tenant',
          confirmation: 'PURGE-TENANT-Test Tenant 327',
          proposerId: 'user-test',
          approverId: 'admin-approver-2',
        }),
      ),
    );
  });

  it('search + consent-status filter narrow the enrollment list', async () => {
    const user = userEvent.setup();
    const listSpy = vi.spyOn(faceVaultApi, 'listFaceEnrollments');
    renderPage();
    await enableFaceRecognition(user);

    await waitFor(() => screen.getByTestId('face-vault-table'));
    listSpy.mockClear();

    // Change search → new call with updated filter
    const searchInput = screen.getByTestId('face-vault-search');
    await user.type(searchInput, 'Subject 1');
    await waitFor(() =>
      expect(listSpy).toHaveBeenCalledWith(
        expect.objectContaining({ search: expect.stringContaining('Subject') }),
      ),
    );

    // Change consent filter → new call with updated filter
    listSpy.mockClear();
    const select = screen.getByTestId('face-vault-consent-filter') as HTMLSelectElement;
    await user.selectOptions(select, 'granted');
    await waitFor(() =>
      expect(listSpy).toHaveBeenCalledWith(
        expect.objectContaining({ consentStatus: 'granted' }),
      ),
    );
  });

  it('has no critical or serious axe violations', async () => {
    renderPage();
    const page = await screen.findByTestId('ai-settings-page');
    await waitFor(() => screen.getByTestId('feature-toggle-object-detection'));
    const violations = await runAxe(page);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toEqual([]);
  });
});
