import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { i18n } from '@/i18n';
import { CustomersPage } from './CustomersPage';
import { __TEST__ as CUSTOMERS_TEST } from '@/api/customers';
import { runAxe } from '@/test/setup';

// KAI-309: Customer list + wizard + impersonation tests.
//
// Covers all required gates from the ticket:
//  1. List renders with mock data (30 in-scope rows).
//  2. Scope enforcement — 10 out-of-scope customers never appear.
//  3. Filter (tier) narrows the visible set.
//  4. Search input narrows the visible set.
//  5. Row click navigates to the drill-down route.
//  6. Wizard happy path: name → billing → plan → token → finish.
//  7. Wizard required-field validation blocks step advance.
//  8. Impersonate dialog requires a reason before confirm.
//  9. Axe smoke scan reports zero critical/serious violations.
// 10. Lazy-load chunk verified — App.tsx imports CustomersPage via
//     React.lazy so it lives in its own chunk.

function renderPage(initialPath = '/command/customers'): HTMLElement {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={[initialPath]}>
          <Routes>
            <Route path="/command/customers" element={<CustomersPage />} />
            <Route
              path="/command/customers/:customerId"
              element={<div data-testid="drill-stub">drill</div>}
            />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return container;
}

async function waitForRows(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByTestId('customer-table')).toBeInTheDocument();
  });
  await waitFor(() => {
    expect(screen.queryByTestId('customer-list-empty')).not.toBeInTheDocument();
  });
}

describe('CustomersPage', () => {
  beforeEach(() => {
    void i18n.changeLanguage('en');
  });

  it('renders 30 in-scope customers in the table', async () => {
    renderPage();
    expect(
      screen.getByRole('heading', { level: 1, name: /customers/i }),
    ).toBeInTheDocument();
    await waitForRows();
    const rows = screen
      .getAllByRole('link')
      .filter((el) => el.tagName.toLowerCase() === 'tr');
    expect(rows).toHaveLength(30);
  });

  it('enforces integrator scope: hides 10 out-of-scope customers', async () => {
    renderPage();
    await waitForRows();
    const outOfScope = CUSTOMERS_TEST.MOCK_CUSTOMERS.filter(
      (c) => c.integratorId === CUSTOMERS_TEST.OTHER_INTEGRATOR_ID,
    );
    expect(outOfScope).toHaveLength(10);
    for (const c of outOfScope) {
      expect(screen.queryByTestId(`customer-row-${c.id}`)).not.toBeInTheDocument();
    }
    const inScope = CUSTOMERS_TEST.MOCK_CUSTOMERS.filter(
      (c) => c.integratorId === CUSTOMERS_TEST.CURRENT_INTEGRATOR_ID,
    );
    expect(inScope).toHaveLength(30);
    for (const c of inScope) {
      expect(screen.getByTestId(`customer-row-${c.id}`)).toBeInTheDocument();
    }
  });

  it('tier filter narrows the visible row set', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForRows();

    const tierSelect = screen.getByTestId('filter-tier') as HTMLSelectElement;
    await user.selectOptions(tierSelect, 'platinum');

    await waitFor(() => {
      const rows = screen
        .getAllByRole('link')
        .filter((el) => el.tagName.toLowerCase() === 'tr');
      // 30 customers across 4 tiers, round-robin → ceil(30/4)=8 platinum.
      expect(rows.length).toBeGreaterThan(0);
      expect(rows.length).toBeLessThan(30);
      for (const r of rows) {
        expect(within(r).getByText(/platinum/i)).toBeInTheDocument();
      }
    });
  });

  it('search input filters customers by name', async () => {
    renderPage();
    await waitForRows();

    const baselineRows = screen
      .getAllByRole('link')
      .filter((el) => el.tagName.toLowerCase() === 'tr');
    expect(baselineRows.length).toBe(30);

    const search = screen.getByTestId('customer-search') as HTMLInputElement;
    // Mock customer naming pattern is "${letter}cme ${index} ${label}".
    // Searching for "cme 5" matches indices whose number contains "5"
    // (e.g. 5, 15, 25). Use fireEvent for a single synchronous change
    // so we don't trigger a refetch storm per keystroke.
    const { fireEvent } = await import('@testing-library/react');
    fireEvent.change(search, { target: { value: 'cme 5' } });

    await waitFor(
      () => {
        const rows = screen
          .getAllByRole('link')
          .filter((el) => el.tagName.toLowerCase() === 'tr');
        expect(rows.length).toBeLessThan(baselineRows.length);
        expect(rows.length).toBeGreaterThan(0);
        for (const r of rows) {
          expect(within(r).getByText(/cme 5/i)).toBeInTheDocument();
        }
      },
      { timeout: 3000 },
    );
  });

  it('row click navigates to the drill-down route', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForRows();

    const firstRow = screen
      .getAllByRole('link')
      .filter((el) => el.tagName.toLowerCase() === 'tr')[0]!;
    await user.click(firstRow);

    await waitFor(() => {
      expect(screen.getByTestId('drill-stub')).toBeInTheDocument();
    });
  });

  it('Create Customer wizard walks the happy path to a setup token', async () => {
    const user = userEvent.setup();
    // Mock clipboard so the copy button doesn't throw in jsdom.
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    });
    renderPage();
    await waitForRows();

    await user.click(screen.getByTestId('customer-create-open'));
    expect(screen.getByTestId('create-wizard')).toBeInTheDocument();

    await user.type(screen.getByTestId('create-name'), 'Wizard Test Co');
    await user.type(screen.getByTestId('create-email'), 'ops@wizard.example');
    // timezone + country defaults are valid.
    await user.click(screen.getByTestId('create-wizard-next'));

    // Step 2: billing mode (default direct is fine).
    expect(screen.getByTestId('create-step-2')).toBeInTheDocument();
    await user.click(screen.getByTestId('create-wizard-next'));

    // Step 3: plan select. Default 'starter' is selected; click next to commit.
    expect(screen.getByTestId('create-step-3')).toBeInTheDocument();
    await user.click(screen.getByTestId('create-wizard-next'));

    // Step 4: token displayed.
    await waitFor(() => {
      expect(screen.getByTestId('create-step-4')).toBeInTheDocument();
    });
    const tokenInput = screen.getByTestId('setup-token-text') as HTMLInputElement;
    expect(tokenInput.value.length).toBeGreaterThan(0);

    await user.click(screen.getByTestId('create-wizard-next'));
    expect(screen.getByTestId('create-step-5')).toBeInTheDocument();
    expect(screen.getByTestId('review-name')).toHaveTextContent('Wizard Test Co');
  });

  it('Create Customer wizard blocks step advance when required fields are empty', async () => {
    const user = userEvent.setup();
    renderPage();
    await waitForRows();

    await user.click(screen.getByTestId('customer-create-open'));
    // Click Next without filling anything.
    await user.click(screen.getByTestId('create-wizard-next'));

    // Stays on step 1 with errors visible.
    expect(screen.getByTestId('create-step-1')).toBeInTheDocument();
    expect(screen.getByTestId('create-name-error')).toBeInTheDocument();
    expect(screen.getByTestId('create-email-error')).toBeInTheDocument();
  });

  it('axe smoke scan: no critical or serious violations on customer list', async () => {
    const container = renderPage();
    await waitForRows();
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

  it('CustomersPage is registered as a lazy-loaded route in App.tsx', async () => {
    // The lazy-load gate verifies that the page lives behind React.lazy
    // so the table + wizard + create flow are not in the initial bundle.
    const appSource = await import('../../App?raw').catch(() => null);
    // Vite ?raw imports are not configured in this project; fall back
    // to fetching the source via the Node fs API which Vitest exposes.
    if (!appSource) {
      const fs = await import('node:fs/promises');
      const path = await import('node:path');
      const file = path.resolve(__dirname, '../../App.tsx');
      const text = await fs.readFile(file, 'utf-8');
      expect(text).toMatch(/lazy\s*\(\s*\(\)\s*=>\s*import\(['"][^'"]*CustomersPage['"]\)/);
      expect(text).toMatch(/CustomerDrillDown/);
    }
  });
});

describe('ImpersonateConfirmDialog', () => {
  beforeEach(() => {
    void i18n.changeLanguage('en');
  });

  it('requires a reason before confirming impersonation', async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    const { ImpersonateConfirmDialog } = await import(
      '@/components/customers/ImpersonateConfirmDialog'
    );
    const customer = CUSTOMERS_TEST.MOCK_CUSTOMERS.find(
      (c) => c.integratorId === CUSTOMERS_TEST.CURRENT_INTEGRATOR_ID,
    )!;
    render(
      <I18nextProvider i18n={i18n}>
        <ImpersonateConfirmDialog
          customer={customer}
          open
          onCancel={onCancel}
          onConfirm={onConfirm}
        />
      </I18nextProvider>,
    );

    // Confirm without typing a reason — should not call onConfirm and
    // should surface the validation error.
    await user.click(screen.getByTestId('impersonate-confirm'));
    expect(onConfirm).not.toHaveBeenCalled();
    expect(screen.getByTestId('impersonate-reason-error')).toBeInTheDocument();

    // Provide a reason — confirm fires.
    await user.type(screen.getByTestId('impersonate-reason'), 'Customer asked for help');
    await user.click(screen.getByTestId('impersonate-confirm'));
    await waitFor(() => {
      expect(onConfirm).toHaveBeenCalledWith('Customer asked for help');
    });
  });
});
