import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AdminSettings } from './routes/admin/Settings';
import { NotFound } from './routes/NotFound';

// KAI-307: top-level router. Two runtime contexts share one build;
// route prefix selects context.
//   /admin/*    -> Customer Admin
//   /command/*  -> Integrator Portal
//
// KAI-320: AdminDashboard is lazy-loaded to keep the customer-admin
// initial bundle small; widgets + virtualization only ship when the
// route is visited.
// KAI-308: FleetDashboard is lazy-loaded for the same reason on the
// integrator portal side.
// KAI-309: CustomersPage and CustomerDrillDown are lazy-loaded so the
// customer-list table, wizard, and drill-down only ship when the
// integrator visits /command/customers.
const AdminDashboard = lazy(() =>
  import('./routes/admin/AdminDashboard').then((m) => ({ default: m.AdminDashboard })),
);
// KAI-321: CamerasPage lazy-loaded so the customer-admin initial bundle
// stays small; the wizard, table, and modals only ship when /admin/cameras
// is visited.
const CamerasPage = lazy(() =>
  import('./routes/admin/CamerasPage').then((m) => ({ default: m.CamerasPage })),
);
const FleetDashboard = lazy(() => import('./routes/command/FleetDashboard'));
const CustomersPage = lazy(() => import('./routes/command/CustomersPage'));
const CustomerDrillDown = lazy(() =>
  import('./components/customers/CustomerDrillDown').then((m) => ({
    default: m.CustomerDrillDown,
  })),
);

export function App(): JSX.Element {
  return (
    <Suspense fallback={<p role="status">Loading…</p>}>
      <Routes>
        <Route path="/" element={<Navigate to="/admin" replace />} />
        <Route path="/admin" element={<AdminDashboard />} />
        <Route path="/admin/cameras" element={<CamerasPage />} />
        <Route path="/admin/settings" element={<AdminSettings />} />
        <Route path="/command" element={<FleetDashboard />} />
        <Route path="/command/customers" element={<CustomersPage />} />
        <Route path="/command/customers/:customerId" element={<CustomerDrillDown />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </Suspense>
  );
}
