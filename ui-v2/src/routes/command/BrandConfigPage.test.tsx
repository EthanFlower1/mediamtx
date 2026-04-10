import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { i18n } from '@/i18n';
import { BrandConfigPage } from './BrandConfigPage';
import { runAxe } from '@/test/setup';

// KAI-310: Brand Configuration page tests.
//
// Covers:
//   1. Renders brand settings form with mock data
//   2. Save draft mutation fires and shows success
//   3. Publish opens confirmation dialog, confirm triggers mutation
//   4. Domain validation button triggers check + status badge
//   5. Axe smoke (zero critical/serious)

function renderPage(): HTMLElement {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/command/brand']}>
          <BrandConfigPage />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return container;
}

async function waitForForm(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByTestId('brand-config-page')).toBeInTheDocument();
    expect(screen.getByLabelText(/company name/i)).toBeInTheDocument();
  });
}

describe('BrandConfigPage', () => {
  beforeEach(() => {
    void i18n.changeLanguage('en');
  });

  it('renders page header, brand settings form, and preview panel', async () => {
    renderPage();

    expect(
      screen.getByRole('heading', { level: 1, name: /brand configuration/i }),
    ).toBeInTheDocument();

    await waitForForm();

    // Company name input populated from mock.
    const nameInput = screen.getByLabelText(/company name/i) as HTMLInputElement;
    expect(nameInput.value).toBe('SecureVision Pro');

    // Tagline populated.
    const taglineInput = screen.getByLabelText(/tagline/i) as HTMLInputElement;
    expect(taglineInput.value).toBe('Enterprise surveillance, simplified.');

    // Preview panel present.
    expect(screen.getByTestId('brand-preview')).toBeInTheDocument();
    expect(screen.getByTestId('preview-header')).toBeInTheDocument();

    // Domain section present.
    expect(screen.getByRole('textbox', { name: /custom domain/i })).toBeInTheDocument();

    // Email templates present (3 types).
    expect(screen.getByTestId('email-template-welcome')).toBeInTheDocument();
    expect(screen.getByTestId('email-template-alert')).toBeInTheDocument();
    expect(screen.getByTestId('email-template-report')).toBeInTheDocument();

    // Mobile app config present.
    expect(screen.getByLabelText(/app name/i)).toBeInTheDocument();
  });

  it('save draft button triggers mutation and shows success', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForForm();

    const saveBtn = screen.getByTestId('save-draft-btn');
    await user.click(saveBtn);

    await waitFor(() => {
      expect(screen.getByTestId('save-success')).toBeInTheDocument();
    });
  });

  it('publish button opens confirmation dialog, cancel closes it', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForForm();

    // Dialog not shown initially.
    expect(screen.queryByTestId('publish-dialog')).not.toBeInTheDocument();

    // Click publish.
    await user.click(screen.getByTestId('publish-btn'));

    // Dialog opens.
    const dialog = screen.getByTestId('publish-dialog');
    expect(dialog).toBeInTheDocument();
    expect(dialog).toHaveAttribute('role', 'dialog');
    expect(dialog).toHaveAttribute('aria-modal', 'true');

    // Cancel closes.
    await user.click(screen.getByTestId('publish-cancel-btn'));
    expect(screen.queryByTestId('publish-dialog')).not.toBeInTheDocument();
  });

  it('publish confirm triggers publish mutation', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForForm();

    await user.click(screen.getByTestId('publish-btn'));
    expect(screen.getByTestId('publish-dialog')).toBeInTheDocument();

    await user.click(screen.getByTestId('publish-confirm-btn'));

    // Dialog closes after publish completes.
    await waitFor(() => {
      expect(screen.queryByTestId('publish-dialog')).not.toBeInTheDocument();
    });
  });

  it('domain validation button triggers check and displays status badge', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForForm();

    // Status badge present from mock data (valid).
    const badge = screen.getByTestId('domain-status-badge');
    expect(badge).toBeInTheDocument();

    // Click validate.
    const validateBtn = screen.getByTestId('validate-domain-btn');
    await user.click(validateBtn);

    // Badge remains after re-validation.
    await waitFor(() => {
      expect(screen.getByTestId('domain-status-badge')).toBeInTheDocument();
    });
  });

  it('passes an axe-core smoke scan with no critical or serious violations', async () => {
    const container = renderPage();
    await waitForForm();

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
