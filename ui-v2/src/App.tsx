import { Navigate, Route, Routes } from 'react-router-dom';
import { AdminHome } from './routes/admin/Home';
import { AdminSettings } from './routes/admin/Settings';
import { CommandHome } from './routes/command/Home';
import { CommandCustomers } from './routes/command/Customers';
import { NotFound } from './routes/NotFound';

// KAI-307: top-level router. Two runtime contexts share one
// build; route prefix selects context.
//   /admin/*    -> Customer Admin
//   /command/*  -> Integrator Portal
export function App(): JSX.Element {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/admin" replace />} />
      <Route path="/admin" element={<AdminHome />} />
      <Route path="/admin/settings" element={<AdminSettings />} />
      <Route path="/command" element={<CommandHome />} />
      <Route path="/command/customers" element={<CommandCustomers />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
