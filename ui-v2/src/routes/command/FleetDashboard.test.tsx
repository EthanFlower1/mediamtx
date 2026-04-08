import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { i18n } from '@/i18n';
import { FleetDashboard } from './FleetDashboard';
import { __TEST__ } from '@/api/fleet';
import { runAxe } from '@/test/setup';

// KAI-308: Fleet Dashboard tests.
//
// Covers the ticket's test requirements:
//   1. Renders with mock data
//   2. Filters update the visible customer set
//   3. Keyboard navigation through KPI + customer cards
//   4. Axe smoke (zero critical/serious)
//   5. Integrator scope enforcement — the 10 customers owned by
//      OTHER_INTEGRATOR_ID must never appear in the tree even
//      though they live in the same mock dataset.

function renderDashboard(props?: { integratorId?: string }): HTMLElement {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/command']}>
          <FleetDashboard integratorId={props?.integratorId} />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return container;
}

async function waitForGrid(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByTestId('customer-grid')).toBeInTheDocument();
  });
  // Wait until at least one card rendered (not the empty state).
  await waitFor(() => {
    expect(screen.queryByTestId('customer-grid-empty')).not.toBeInTheDocument();
  });
}

describe('FleetDashboard', () => {
  beforeEach(() => {
    // Ensure deterministic language for number formatting in tests.
    void i18n.changeLanguage('en');
  });

  it('renders page header, KPIs, and customer grid with mock data', async () => {
    renderDashboard();

    expect(
      screen.getByRole('heading', { level: 1, name: /fleet dashboard/i }),
    ).toBeInTheDocument();

    await waitForGrid();

    // KPI row — five cards.
    expect(screen.getByTestId('kpi-total-customers')).toHaveAttribute(
      'aria-label',
      expect.stringContaining('20'),
    );
    expect(screen.getByTestId('kpi-total-cameras')).toBeInTheDocument();
    expect(screen.getByTestId('kpi-total-mrr')).toBeInTheDocument();
    expect(screen.getByTestId('kpi-active-incidents')).toBeInTheDocument();
    expect(screen.getByTestId('kpi-uptime')).toBeInTheDocument();

    // Twenty customer cards present.
    const grid = screen.getByTestId('customer-grid');
    const cards = within(grid).getAllByRole('link');
    expect(cards).toHaveLength(20);
  });

  it('enforces integrator scope — other integrator customers are hidden', async () => {
    renderDashboard();
    await waitForGrid();

    const grid = screen.getByTestId('customer-grid');
    const cards = within(grid).getAllByRole('link');
    expect(cards).toHaveLength(20);

    // None of the 10 out-of-scope customers should appear by ID.
    const outOfScope = __TEST__.MOCK_CUSTOMERS.filter(
      (c) => c.integratorId === __TEST__.OTHER_INTEGRATOR_ID,
    );
    expect(outOfScope).toHaveLength(10);
    for (const c of outOfScope) {
      expect(screen.queryByTestId(`customer-card-${c.id}`)).not.toBeInTheDocument();
    }

    // Sanity: all 20 in-scope customers ARE rendered.
    const inScope = __TEST__.MOCK_CUSTOMERS.filter(
      (c) => c.integratorId === __TEST__.CURRENT_INTEGRATOR_ID,
    );
    for (const c of inScope) {
      expect(screen.getByTestId(`customer-card-${c.id}`)).toBeInTheDocument();
    }
  });

  it('filters update the visible customer set', async () => {
    const user = userEvent.setup();
    renderDashboard();
    await waitForGrid();

    // Baseline count.
    let cards = within(screen.getByTestId('customer-grid')).getAllByRole('link');
    expect(cards).toHaveLength(20);

    // Pick a tier filter — expect the count to drop to just that tier's cards.
    const tierSelect = screen.getByTestId('filter-tier') as HTMLSelectElement;
    await user.selectOptions(tierSelect, 'platinum');

    await waitFor(() => {
      cards = within(screen.getByTestId('customer-grid')).getAllByRole('link');
      // 20 customers split across 4 tiers → 5 per tier.
      expect(cards).toHaveLength(5);
    });

    // Each remaining card must be platinum.
    for (const card of cards) {
      expect(within(card).getByText(/platinum/i)).toBeInTheDocument();
    }

    // Reset restores all 20.
    await user.click(screen.getByTestId('filter-reset'));
    await waitFor(() => {
      cards = within(screen.getByTestId('customer-grid')).getAllByRole('link');
      expect(cards).toHaveLength(20);
    });
  });

  it('supports keyboard navigation through KPI and customer cards', async () => {
    const user = userEvent.setup();
    renderDashboard();
    await waitForGrid();

    // Tab from <body> forward. Order is not strictly guaranteed across
    // the entire page but we assert that each focusable KPI and at
    // least one customer card can receive focus via keyboard.
    const kpi = screen.getByTestId('kpi-total-customers');
    kpi.focus();
    expect(kpi).toHaveFocus();

    // Tab to the next KPI.
    await user.tab();
    expect(document.activeElement).not.toBe(kpi);

    // Focus the first customer card directly — Enter should fire
    // without throwing and the card must be tabbable.
    const firstCustomer = within(screen.getByTestId('customer-grid')).getAllByRole('link')[0]!;
    firstCustomer.focus();
    expect(firstCustomer).toHaveFocus();
    expect(firstCustomer).toHaveAttribute('tabindex', '0');

    // Tab forward moves focus off the first card.
    await user.tab();
    expect(document.activeElement).not.toBe(firstCustomer);
  });

  it('passes an axe-core smoke scan with no critical or serious violations', async () => {
    const container = renderDashboard();
    await waitForGrid();

    const violations = await runAxe(container);
    const blocking = violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    );
    // Print useful diagnostics if this fails.
    if (blocking.length > 0) {
      console.error('axe blocking violations:', blocking);
    }
    expect(blocking).toEqual([]);
  });
});
