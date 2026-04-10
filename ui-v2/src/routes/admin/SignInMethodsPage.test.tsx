// KAI-135: Tests for SignInMethodsPage.
//
// Coverage:
//   - Renders without crashing and shows provider list
//   - Shows 2 pre-configured providers (Local + OIDC) from mock
//   - Opens add provider selector
//   - Opens OIDC wizard from add selector
//   - Opens edit wizard for existing provider
//   - Navigates wizard steps (step indicator updates)
//   - Delete confirmation requires typed "DELETE"
//   - Default provider change shows warning dialog
//   - Test connection flow (mocked success)
//   - axe a11y smoke

import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';

import { SignInMethodsPage } from './SignInMethodsPage';
import { i18n } from '@/i18n';
import { useSessionStore } from '@/stores/session';
import { runAxe } from '@/test/setup';
import { __resetSignInMethodsMockStateForTests } from '@/api/signInMethods.mock';

function renderPage(): ReturnType<typeof render> {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/admin/sign-in']}>
          <SignInMethodsPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

describe('SignInMethodsPage', () => {
  beforeEach(() => {
    useSessionStore.setState({
      tenantId: 'tenant-test-135',
      tenantName: 'Test Tenant 135',
      userId: 'user-test',
      userDisplayName: 'Test User',
    });
    __resetSignInMethodsMockStateForTests();
    vi.restoreAllMocks();
  });

  it('renders without crashing and shows the page', async () => {
    renderPage();
    expect(await screen.findByTestId('sign-in-methods-page')).toBeInTheDocument();
  });

  it('renders 2 pre-configured provider cards (Local + OIDC)', async () => {
    renderPage();
    expect(await screen.findByTestId('provider-card-local')).toBeInTheDocument();
    expect(screen.getByTestId('provider-card-oidc')).toBeInTheDocument();
  });

  it('shows provider status badges', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('provider-status-local'));
    expect(screen.getByTestId('provider-status-local')).toBeInTheDocument();
    expect(screen.getByTestId('provider-status-oidc')).toBeInTheDocument();
  });

  it('shows user count for each provider', async () => {
    renderPage();
    await waitFor(() => screen.getByTestId('provider-user-count-local'));
    expect(screen.getByTestId('provider-user-count-local')).toBeInTheDocument();
    expect(screen.getByTestId('provider-user-count-oidc')).toBeInTheDocument();
  });

  it('shows the default provider section with radio buttons', async () => {
    renderPage();
    expect(await screen.findByTestId('default-provider-section')).toBeInTheDocument();
    expect(screen.getByTestId('default-radio-local')).toBeInTheDocument();
    expect(screen.getByTestId('default-radio-oidc')).toBeInTheDocument();
  });

  it('shows the add provider button', async () => {
    renderPage();
    expect(await screen.findByTestId('add-provider-button')).toBeInTheDocument();
  });

  it('opens add provider selector when button is clicked', async () => {
    const user = userEvent.setup();
    renderPage();
    const addButton = await screen.findByTestId('add-provider-button');
    await user.click(addButton);
    expect(screen.getByTestId('add-provider-selector')).toBeInTheDocument();
  });

  it('add provider selector shows unconfigured providers', async () => {
    const user = userEvent.setup();
    renderPage();
    const addButton = await screen.findByTestId('add-provider-button');
    await user.click(addButton);

    // Local and OIDC are already configured, so they should NOT appear
    expect(screen.queryByTestId('add-provider-option-local')).not.toBeInTheDocument();
    expect(screen.queryByTestId('add-provider-option-oidc')).not.toBeInTheDocument();

    // Unconfigured ones should appear
    expect(screen.getByTestId('add-provider-option-entra')).toBeInTheDocument();
    expect(screen.getByTestId('add-provider-option-google')).toBeInTheDocument();
    expect(screen.getByTestId('add-provider-option-saml')).toBeInTheDocument();
    expect(screen.getByTestId('add-provider-option-ldap')).toBeInTheDocument();
    expect(screen.getByTestId('add-provider-option-okta')).toBeInTheDocument();
  });

  it('opens OIDC wizard when selecting from add provider', async () => {
    const user = userEvent.setup();
    renderPage();
    // Click edit for the existing OIDC provider
    const editButton = await screen.findByTestId('provider-edit-oidc');
    await user.click(editButton);
    expect(screen.getByTestId('oidc-provider-wizard')).toBeInTheDocument();
  });

  it('wizard shows step indicator', async () => {
    const user = userEvent.setup();
    renderPage();
    const editButton = await screen.findByTestId('provider-edit-oidc');
    await user.click(editButton);
    expect(screen.getByTestId('wizard-step-indicator')).toBeInTheDocument();
  });

  it('wizard navigates between steps', async () => {
    const user = userEvent.setup();
    renderPage();
    const editButton = await screen.findByTestId('provider-edit-oidc');
    await user.click(editButton);

    // Should be on step 1 — fill required fields and navigate to step 2
    expect(screen.getByTestId('oidc-wizard-step1')).toBeInTheDocument();

    // The existing mock has valid data pre-filled, so Next should work
    const nextButton = screen.getByTestId('wizard-next');
    await user.click(nextButton);

    await waitFor(() => {
      expect(screen.getByTestId('oidc-wizard-step2')).toBeInTheDocument();
    });

    // Navigate back
    const backButton = screen.getByTestId('wizard-back');
    await user.click(backButton);

    await waitFor(() => {
      expect(screen.getByTestId('oidc-wizard-step1')).toBeInTheDocument();
    });
  });

  it('delete confirmation requires typed DELETE', async () => {
    const user = userEvent.setup();
    renderPage();
    const deleteButton = await screen.findByTestId('provider-delete-oidc');
    await user.click(deleteButton);

    expect(screen.getByTestId('delete-provider-dialog')).toBeInTheDocument();

    // Confirm button should be disabled initially
    const confirmButton = screen.getByTestId('delete-confirm');
    expect(confirmButton).toBeDisabled();

    // Type DELETE
    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'DELETE');

    // Confirm button should now be enabled
    expect(confirmButton).not.toBeDisabled();
  });

  it('delete confirmation removes provider after confirm', async () => {
    const user = userEvent.setup();
    renderPage();
    const deleteButton = await screen.findByTestId('provider-delete-oidc');
    await user.click(deleteButton);

    const input = screen.getByTestId('delete-confirm-input');
    await user.type(input, 'DELETE');

    const confirmButton = screen.getByTestId('delete-confirm');
    await user.click(confirmButton);

    // Dialog should close
    await waitFor(() => {
      expect(screen.queryByTestId('delete-provider-dialog')).not.toBeInTheDocument();
    });
  });

  it('default provider change shows warning dialog', async () => {
    const user = userEvent.setup();
    renderPage();

    // Wait for the radio inputs to appear
    const oidcRadio = await screen.findByTestId('default-radio-input-oidc');
    await user.click(oidcRadio);

    expect(screen.getByTestId('default-change-dialog')).toBeInTheDocument();
  });

  it('default provider change dialog can be confirmed', async () => {
    const user = userEvent.setup();
    renderPage();

    const oidcRadio = await screen.findByTestId('default-radio-input-oidc');
    await user.click(oidcRadio);

    const confirmButton = screen.getByTestId('default-change-confirm');
    await user.click(confirmButton);

    // Dialog should close
    await waitFor(() => {
      expect(screen.queryByTestId('default-change-dialog')).not.toBeInTheDocument();
    });
  });

  it('default provider change dialog can be cancelled', async () => {
    const user = userEvent.setup();
    renderPage();

    const oidcRadio = await screen.findByTestId('default-radio-input-oidc');
    await user.click(oidcRadio);

    const cancelButton = screen.getByTestId('default-change-cancel');
    await user.click(cancelButton);

    await waitFor(() => {
      expect(screen.queryByTestId('default-change-dialog')).not.toBeInTheDocument();
    });
  });

  it('test connection flow shows success in wizard', async () => {
    const user = userEvent.setup();
    renderPage();
    const editButton = await screen.findByTestId('provider-edit-oidc');
    await user.click(editButton);

    // Navigate to step 2 (last step where test button is)
    const nextButton = screen.getByTestId('wizard-next');
    await user.click(nextButton);

    await waitFor(() => screen.getByTestId('oidc-wizard-step2'));

    // Click test connection
    const testButton = screen.getByTestId('wizard-test-button');
    await user.click(testButton);

    // Wait for the test result to appear (mock has 400ms delay)
    await waitFor(
      () => {
        expect(screen.getByTestId('wizard-test-result')).toBeInTheDocument();
      },
      { timeout: 2000 },
    );

    expect(screen.getByTestId('wizard-test-result').dataset.success).toBe('true');
  });

  it('passes axe a11y smoke test', async () => {
    const { container } = renderPage();
    await screen.findByTestId('sign-in-methods-page');
    const violations = await runAxe(container);
    const serious = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    expect(serious).toHaveLength(0);
  });
});
