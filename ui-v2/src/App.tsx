import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AdminHome } from './routes/admin/Home';
import { AdminSettings } from './routes/admin/Settings';
import { CommandCustomers } from './routes/command/Customers';
import { NotFound } from './routes/NotFound';

// KAI-307: top-level router. Two runtime contexts share one
// build; route prefix selects context.
//   /admin/*    -> Customer Admin
//   /command/*  -> Integrator Portal
//
// KAI-308: /command/ landing page is the Fleet Dashboard. It is
// lazy-loaded via React.lazy so the integrator bundle stays small
// for users who only visit /admin.
const FleetDashboard = lazy(() => import('./routes/command/FleetDashboard'));

export function App(): JSX.Element {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/admin" replace />} />
      <Route path="/admin" element={<AdminHome />} />
      <Route path="/admin/settings" element={<AdminSettings />} />
      <Route
        path="/command"
        element={
          <Suspense fallback={<p role="status">Loading…</p>}>
            <FleetDashboard />
          </Suspense>
        }
      />
      <Route path="/command/customers" element={<CommandCustomers />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
