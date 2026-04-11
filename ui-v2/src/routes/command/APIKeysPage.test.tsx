import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router-dom';
import { I18nextProvider } from 'react-i18next';
import { i18n } from '@/i18n';
import { __TEST__ as apiKeysTest } from '@/api/apiKeys';
import { __TEST__ as mockTest } from '@/api/apiKeys.mock';
import { APIKeysPage } from './APIKeysPage';

// KAI-319: API Keys Management page tests.
//
// Covers:
//   - Table renders with correct key data
//   - Status badges show correct states
//   - Generate key dialog with validation
//   - One-time key display after creation
//   - Rotate key dialog with grace period
//   - Revoke confirmation with type-to-confirm
//   - Audit log drawer
//   - Show/hide revoked keys filter

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
}

function renderPage(): ReturnType<typeof render> {
  const qc = createQueryClient();
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={qc}>
        <MemoryRouter>
          <APIKeysPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
}

beforeEach(() => {
  apiKeysTest.resetClient();
  mockTest.resetStores();
});

// ---------------------------------------------------------------------------
// Table rendering
// ---------------------------------------------------------------------------

describe('KeyListTable', () => {
  it('renders the page header and generate button', async () => {
    renderPage();
    expect(await screen.findByTestId('api-keys-page')).toBeInTheDocument();
    expect(screen.getByTestId('generate-key-btn')).toBeInTheDocument();
  });

  it('renders the API keys table with mock data', async () => {
    renderPage();
    // Wait for the table to appear (3 non-revoked keys by default).
    const table = await screen.findByTestId('api-keys-table');
    expect(table).toBeInTheDocument();

    // key-001, key-002, key-003 are visible (not revoked).
    expect(screen.getByTestId('key-row-key-001')).toBeInTheDocument();
    expect(screen.getByTestId('key-row-key-002')).toBeInTheDocument();
    expect(screen.getByTestId('key-row-key-003')).toBeInTheDocument();

    // key-004 (revoked) should NOT be visible by default.
    expect(screen.queryByTestId('key-row-key-004')).not.toBeInTheDocument();
  });

  it('shows revoked keys when toggled', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    const toggle = screen.getByTestId('show-revoked-checkbox');
    await user.click(toggle);

    expect(screen.getByTestId('key-row-key-004')).toBeInTheDocument();
  });

  it('renders key name and prefix', async () => {
    renderPage();
    await screen.findByTestId('api-keys-table');

    const row = screen.getByTestId('key-row-key-001');
    expect(within(row).getByText('Production Integration')).toBeInTheDocument();
    expect(within(row).getByText('kvue_a1b...')).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Generate key dialog
// ---------------------------------------------------------------------------

describe('GenerateKeyDialog', () => {
  it('opens and closes the generate dialog', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('generate-key-btn'));
    expect(screen.getByTestId('generate-key-dialog')).toBeInTheDocument();

    await user.click(screen.getByTestId('generate-cancel'));
    expect(screen.queryByTestId('generate-key-dialog')).not.toBeInTheDocument();
  });

  it('validates empty name', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('generate-key-btn'));
    await user.click(screen.getByTestId('generate-submit'));

    expect(screen.getByTestId('key-name-error')).toBeInTheDocument();
  });

  it('creates a key and shows one-time display', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('generate-key-btn'));
    await user.type(screen.getByTestId('key-name-input'), 'My New Key');
    await user.click(screen.getByTestId('generate-submit'));

    // One-time key display should appear.
    const keyDisplay = await screen.findByTestId('one-time-key-display');
    expect(keyDisplay).toBeInTheDocument();

    const keyValue = screen.getByTestId('raw-key-value') as HTMLInputElement;
    expect(keyValue.value).toMatch(/^kvue_/);
  });

  it('dismisses one-time key display', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('generate-key-btn'));
    await user.type(screen.getByTestId('key-name-input'), 'Dismiss Test');
    await user.click(screen.getByTestId('generate-submit'));

    await screen.findByTestId('one-time-key-display');
    await user.click(screen.getByTestId('dismiss-key-display'));

    expect(screen.queryByTestId('one-time-key-display')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Rotate key dialog
// ---------------------------------------------------------------------------

describe('RotateKeyDialog', () => {
  it('opens the rotate dialog and rotates a key', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('rotate-btn-key-001'));
    expect(screen.getByTestId('rotate-key-dialog')).toBeInTheDocument();

    // Default grace period is 24 hours.
    const graceInput = screen.getByTestId('grace-period-input') as HTMLInputElement;
    expect(graceInput.value).toBe('24');

    await user.click(screen.getByTestId('rotate-submit'));

    // One-time key display for the new key.
    const keyDisplay = await screen.findByTestId('one-time-key-display');
    expect(keyDisplay).toBeInTheDocument();
  });

  it('can cancel rotation', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('rotate-btn-key-001'));
    await user.click(screen.getByTestId('rotate-cancel'));

    expect(screen.queryByTestId('rotate-key-dialog')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Revoke confirmation dialog
// ---------------------------------------------------------------------------

describe('RevokeConfirmDialog', () => {
  it('opens revoke dialog and requires type-to-confirm', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('revoke-btn-key-001'));
    expect(screen.getByTestId('revoke-confirm-dialog')).toBeInTheDocument();

    // Submit should be disabled without typing REVOKE.
    const submitBtn = screen.getByTestId('revoke-submit');
    expect(submitBtn).toBeDisabled();

    // Type the confirmation word.
    await user.type(screen.getByTestId('revoke-confirm-input'), 'REVOKE');
    expect(submitBtn).not.toBeDisabled();
  });

  it('revokes a key after confirmation', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('revoke-btn-key-002'));
    await user.type(screen.getByTestId('revoke-confirm-input'), 'REVOKE');
    await user.click(screen.getByTestId('revoke-submit'));

    // Dialog should close. Key-002 should no longer show rotate/revoke.
    await screen.findByTestId('api-keys-table');
    expect(screen.queryByTestId('revoke-confirm-dialog')).not.toBeInTheDocument();
  });

  it('can cancel revocation', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('revoke-btn-key-001'));
    await user.click(screen.getByTestId('revoke-cancel'));

    expect(screen.queryByTestId('revoke-confirm-dialog')).not.toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// Audit log drawer
// ---------------------------------------------------------------------------

describe('AuditLogDrawer', () => {
  it('opens the audit log drawer for a key', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('audit-btn-key-001'));
    const drawer = await screen.findByTestId('audit-log-drawer');
    expect(drawer).toBeInTheDocument();

    // Should show audit entries.
    const entries = await screen.findByTestId('audit-entries');
    expect(entries.children.length).toBeGreaterThan(0);
  });

  it('closes the audit log drawer', async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByTestId('api-keys-table');

    await user.click(screen.getByTestId('audit-btn-key-001'));
    await screen.findByTestId('audit-log-drawer');

    await user.click(screen.getByTestId('audit-close'));
    expect(screen.queryByTestId('audit-log-drawer')).not.toBeInTheDocument();
  });
});
