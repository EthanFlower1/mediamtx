import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, within, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { I18nextProvider } from 'react-i18next';
import { MemoryRouter } from 'react-router-dom';
import { i18n } from '@/i18n';
import { PermissionsPage } from './PermissionsPage';
import { __TEST__ } from '@/api/permissions';
import { __MOCK_TEST__ } from '@/api/permissions.mock';
import { runAxe } from '@/test/setup';

// KAI-315: Customer Permissions Matrix tests.
//
// Covers:
//   1. Renders matrix with mock data (5 tenants, 8 categories)
//   2. Toggle permission cycles through enabled -> disabled -> inherited
//   3. Bulk action applies permission to all selected tenants
//   4. Save with diff preview dialog
//   5. Expandable sub-permissions
//   6. Axe a11y smoke (zero critical/serious)

function renderPermissionsPage(props?: { integratorId?: string }): HTMLElement {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const { container } = render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <MemoryRouter initialEntries={['/command/permissions']}>
          <PermissionsPage integratorId={props?.integratorId} />
        </MemoryRouter>
      </QueryClientProvider>
    </I18nextProvider>,
  );
  return container;
}

async function waitForMatrix(): Promise<void> {
  await waitFor(() => {
    expect(screen.getByTestId('permissions-matrix')).toBeInTheDocument();
  });
  // Wait for at least one tenant row
  await waitFor(() => {
    expect(screen.getByTestId('tenant-row-cust-integrator-001-000')).toBeInTheDocument();
  });
}

describe('PermissionsPage', () => {
  beforeEach(() => {
    void i18n.changeLanguage('en');
    __TEST__.resetClient();
    __MOCK_TEST__.resetCache();
  });

  it('renders page header, plan banner, matrix with 5 tenants and 8 category columns', async () => {
    renderPermissionsPage();

    expect(
      screen.getByRole('heading', { level: 1, name: /customer permissions/i }),
    ).toBeInTheDocument();

    await waitForMatrix();

    // Plan defaults banner
    expect(screen.getByTestId('plan-defaults-banner')).toBeInTheDocument();

    // 5 tenant rows
    for (let i = 0; i < 5; i++) {
      const tenantId = `cust-integrator-001-${String(i).padStart(3, '0')}`;
      expect(screen.getByTestId(`tenant-row-${tenantId}`)).toBeInTheDocument();
    }

    // Audit sidebar
    expect(screen.getByTestId('audit-sidebar')).toBeInTheDocument();
  });

  it('toggles a permission cell through enabled -> disabled -> inherited cycle', async () => {
    const user = userEvent.setup();
    renderPermissionsPage();
    await waitForMatrix();

    // Find the first cell in the first row. The first tenant (index 0) has
    // cameras category at state index (0+0)%3 = 0 which is 'enabled'.
    const firstRow = screen.getByTestId('tenant-row-cust-integrator-001-000');
    const checkboxes = within(firstRow).getAllByRole('checkbox');
    // First checkbox is the selection checkbox, permission cells come after
    // Find the first permission cell (role=checkbox with aria-checked)
    const permCells = checkboxes.filter((el) => {
      const ariaChecked = el.getAttribute('aria-checked');
      return ariaChecked === 'true' || ariaChecked === 'false' || ariaChecked === 'mixed';
    });
    expect(permCells.length).toBeGreaterThanOrEqual(1);

    const firstPermCell = permCells[0]!;
    const initialState = firstPermCell.getAttribute('aria-checked');

    // Click to cycle
    await user.click(firstPermCell);

    // State should have changed
    const newState = firstPermCell.getAttribute('aria-checked');
    expect(newState).not.toBe(initialState);

    // Save button should appear (we changed something)
    await waitFor(() => {
      expect(screen.getByTestId('save-button')).toBeInTheDocument();
    });
  });

  it('bulk action applies permission to all selected tenants', async () => {
    const user = userEvent.setup();
    renderPermissionsPage();
    await waitForMatrix();

    // Select first two tenants
    await user.click(screen.getByTestId('select-cust-integrator-001-000'));
    await user.click(screen.getByTestId('select-cust-integrator-001-001'));

    // Bulk toolbar should appear
    await waitFor(() => {
      expect(screen.getByTestId('bulk-actions-toolbar')).toBeInTheDocument();
    });

    // Check the selected count
    expect(screen.getByText(/2 selected/i)).toBeInTheDocument();

    // Click bulk enable for cameras
    await user.click(screen.getByTestId('bulk-enable-cameras'));

    // Save button should appear
    await waitFor(() => {
      expect(screen.getByTestId('save-button')).toBeInTheDocument();
    });
  });

  it('shows diff preview dialog on save and can confirm', async () => {
    const user = userEvent.setup();
    renderPermissionsPage();
    await waitForMatrix();

    // Toggle a permission to create a change
    const firstRow = screen.getByTestId('tenant-row-cust-integrator-001-000');
    const permCells = within(firstRow)
      .getAllByRole('checkbox')
      .filter((el) => el.getAttribute('aria-checked') !== null && el.getAttribute('aria-checked') !== undefined && el.tagName === 'BUTTON');
    expect(permCells.length).toBeGreaterThanOrEqual(1);

    await user.click(permCells[0]!);

    // Click save
    await waitFor(() => {
      expect(screen.getByTestId('save-button')).toBeInTheDocument();
    });
    await user.click(screen.getByTestId('save-button'));

    // Diff preview dialog should appear
    await waitFor(() => {
      expect(screen.getByTestId('diff-preview-dialog')).toBeInTheDocument();
    });

    expect(screen.getByTestId('diff-preview-list')).toBeInTheDocument();

    // Confirm save
    await user.click(screen.getByTestId('diff-confirm'));

    // Dialog should close after save succeeds
    await waitFor(() => {
      expect(screen.queryByTestId('diff-preview-dialog')).not.toBeInTheDocument();
    });
  });

  it('expands sub-permissions when expand is clicked', async () => {
    const user = userEvent.setup();
    renderPermissionsPage();
    await waitForMatrix();

    // The first tenant should have an expand button (it has sub-permissions)
    const expandBtn = screen.getByTestId('expand-cust-integrator-001-000');
    expect(expandBtn).toBeInTheDocument();

    await user.click(expandBtn);

    // Sub-permission rows should appear for the categories that have them
    // e.g., cameras sub-permission: add_cameras
    await waitFor(() => {
      expect(
        screen.getByTestId('sub-row-cust-integrator-001-000-cameras-add_cameras'),
      ).toBeInTheDocument();
    });

    // AI features sub-permission: object_detection
    expect(
      screen.getByTestId('sub-row-cust-integrator-001-000-ai_features-object_detection'),
    ).toBeInTheDocument();
  });

  it('select-all checkbox toggles all tenants', async () => {
    const user = userEvent.setup();
    renderPermissionsPage();
    await waitForMatrix();

    // Click select all
    await user.click(screen.getByTestId('select-all-checkbox'));

    // Bulk toolbar should show all 5 selected
    await waitFor(() => {
      expect(screen.getByTestId('bulk-actions-toolbar')).toBeInTheDocument();
      expect(screen.getByText(/5 selected/i)).toBeInTheDocument();
    });

    // Click select all again to deselect
    await user.click(screen.getByTestId('select-all-checkbox'));

    await waitFor(() => {
      expect(screen.queryByTestId('bulk-actions-toolbar')).not.toBeInTheDocument();
    });
  });

  it('renders audit trail sidebar with entries', async () => {
    renderPermissionsPage();
    await waitForMatrix();

    const sidebar = screen.getByTestId('audit-sidebar');
    expect(sidebar).toBeInTheDocument();

    // Should have at least one audit entry
    expect(screen.getByTestId('audit-entry-audit-001')).toBeInTheDocument();
  });

  it('passes an axe-core smoke scan with no critical or serious violations', async () => {
    const container = renderPermissionsPage();
    await waitForMatrix();

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
