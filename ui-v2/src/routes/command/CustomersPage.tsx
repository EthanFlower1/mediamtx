import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  listCustomers,
  customersQueryKey,
  __TEST__ as CUSTOMERS_TEST,
} from '@/api/customers';
import type { CustomerFilters } from '@/api/customers';
import type { CustomerTier, FleetFilters as FleetFilterShape } from '@/api/fleet';
import { __TEST__ as FLEET_TEST } from '@/api/fleet';
import { FleetFiltersBar } from '@/components/fleet/FleetFilters';
import { CustomerList } from '@/components/customers/CustomerList';
import { CreateCustomerWizard } from '@/components/customers/CreateCustomerWizard';

// KAI-309: Integrator Portal customer list page.
//
// Composes the FleetFiltersBar (reused from KAI-308) with the
// customers-specific CustomerList table and the Create New Customer
// wizard launch button. The drill-down lives at a separate route
// (/command/customers/:customerId) — clicking a row navigates there.
//
// Scope enforcement is handled by the API layer (listCustomers filters
// by integratorId). Tests in CustomersPage.test.tsx verify that the
// 10 out-of-scope mock customers never appear in the rendered tree.

interface CustomersPageProps {
  readonly integratorId?: string;
}

export function CustomersPage({
  integratorId = CUSTOMERS_TEST.CURRENT_INTEGRATOR_ID,
}: CustomersPageProps): JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [filters, setFilters] = useState<FleetFilterShape>({});
  const [search, setSearch] = useState('');
  const [wizardOpen, setWizardOpen] = useState(false);

  const apiFilters: CustomerFilters = useMemo(
    () => ({
      subResellerId: filters.subResellerId ?? null,
      label: filters.label ?? null,
      tier: filters.tier ?? null,
      search: search.trim() || null,
    }),
    [filters, search],
  );

  const query = useQuery({
    queryKey: customersQueryKey(integratorId, apiFilters),
    queryFn: () => listCustomers(integratorId, apiFilters),
  });

  const customers = query.data ?? [];

  return (
    <main
      data-testid="customers-page"
      aria-labelledby="customers-page-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      <header className="mb-4">
        <nav
          aria-label={t('customers.list.breadcrumbAriaLabel')}
          className="text-xs text-slate-500"
        >
          <ol className="flex gap-1">
            <li>{t('customers.list.breadcrumb.integratorPortal')}</li>
            <li aria-hidden="true">/</li>
            <li aria-current="page" className="font-medium text-slate-700">
              {t('customers.list.title')}
            </li>
          </ol>
        </nav>
        <h1
          id="customers-page-heading"
          className="mt-1 text-2xl font-bold text-slate-900"
        >
          {t('customers.list.title')}
        </h1>
        <p className="text-sm text-slate-600">{t('customers.list.subtitle')}</p>
      </header>

      <div className="mb-3">
        <FleetFiltersBar
          filters={filters}
          onChange={(next) => setFilters(next)}
          subResellers={FLEET_TEST.SUB_RESELLERS}
          labels={[...FLEET_TEST.LABELS]}
        />
      </div>

      {query.isLoading ? (
        <p role="status" data-testid="customers-loading" className="text-sm text-slate-500">
          {t('customers.list.loading')}
        </p>
      ) : query.isError ? (
        <p role="alert" data-testid="customers-error" className="text-sm text-red-700">
          {t('customers.list.error')}
        </p>
      ) : (
        <CustomerList
          customers={customers}
          search={search}
          onSearchChange={setSearch}
          onCreateClick={() => setWizardOpen(true)}
          onRowActivate={(c) => navigate(`/command/customers/${c.id}`)}
        />
      )}

      <CreateCustomerWizard
        open={wizardOpen}
        onClose={() => setWizardOpen(false)}
      />
    </main>
  );
}

// Re-export the tier type so route consumers don't need to import from fleet.
export type { CustomerTier };

export default CustomersPage;
