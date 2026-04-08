import { lazy, Suspense } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';
import { AdminSettings } from './routes/admin/Settings';
import { CommandHome } from './routes/command/Home';
import { CommandCustomers } from './routes/command/Customers';
import { NotFound } from './routes/NotFound';

// KAI-307: top-level router. Two runtime contexts share one
// build; route prefix selects context.
//   /admin/*    -> Customer Admin
//   /command/*  -> Integrator Portal
//
// KAI-320: AdminDashboard is lazy-loaded to keep the customer-admin
// initial bundle small; dashboard widgets + virtualization code only
// ship when the route is visited.
const AdminDashboard = lazy(() =>
  import('./routes/admin/AdminDashboard').then((m) => ({ default: m.AdminDashboard })),
);

export function App(): JSX.Element {
  return (
    <Suspense fallback={null}>
      <Routes>
        <Route path="/" element={<Navigate to="/admin" replace />} />
        <Route path="/admin" element={<AdminDashboard />} />
        <Route path="/admin/settings" element={<AdminSettings />} />
        <Route path="/command" element={<CommandHome />} />
        <Route path="/command/customers" element={<CommandCustomers />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </Suspense>
  );
}
