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
// KAI-322: RecordersPage lazy-loaded — table, pair modal, detail drawer, and
// unpair confirm only ship when /admin/recorders is visited.
const RecordersPage = lazy(() =>
  import('./routes/admin/RecordersPage').then((m) => ({ default: m.RecordersPage })),
);
// KAI-325: UsersPage lazy-loaded; the virtualized user list, 6 SSO wizards,
// and permissions matrix only ship when /admin/users is visited.
const UsersPage = lazy(() =>
  import('./routes/admin/UsersPage').then((m) => ({ default: m.UsersPage })),
);
// KAI-323: LiveViewPage + PlaybackPage lazy-loaded; the grid, picker, and
// playback timeline only ship when /admin/live or /admin/playback is visited.
// Real WebRTC/HLS/RTSP wiring lands with KAI-334.
const LiveViewPage = lazy(() =>
  import('./routes/admin/LiveViewPage').then((m) => ({ default: m.LiveViewPage })),
);
const PlaybackPage = lazy(() =>
  import('./routes/admin/PlaybackPage').then((m) => ({ default: m.PlaybackPage })),
);
// KAI-327: AiSettingsPage lazy-loaded — AI feature toggles + face vault
// management modals (including emergency purge multi-step dialog) only
// ship when /admin/ai-settings is visited.
const AiSettingsPage = lazy(() =>
  import('./routes/admin/AiSettingsPage').then((m) => ({ default: m.AiSettingsPage })),
);
// KAI-326: SchedulesPage lazy-loaded; schedule templates, weekly timeline,
// and retention overview only ship when /admin/schedules is visited.
const SchedulesPage = lazy(() =>
  import('./routes/admin/SchedulesPage').then((m) => ({ default: m.SchedulesPage })),
);
// KAI-324: EventsPage lazy-loaded — the virtualized AI detection list,
// filter bar, semantic search toggle, and CSV/PDF export only ship when
// /admin/events is visited.
const EventsPage = lazy(() =>
  import('./routes/admin/EventsPage').then((m) => ({ default: m.EventsPage })),
);
// KAI-329: SystemHealthPage lazy-loaded — health dashboard, remote access
// controls, and quick settings only ship when /admin/health is visited.
const SystemHealthPage = lazy(() =>
  import('./routes/admin/SystemHealthPage').then((m) => ({ default: m.SystemHealthPage })),
);
const FleetDashboard = lazy(() => import('./routes/command/FleetDashboard'));
const CustomersPage = lazy(() => import('./routes/command/CustomersPage'));
// KAI-310: BrandConfigPage lazy-loaded; brand settings form, preview panel,
// and email template cards only ship when /command/brand is visited.
const BrandConfigPage = lazy(() => import('./routes/command/BrandConfigPage'));
// KAI-311: MobileBuildsPage lazy-loaded — build table, trigger dialog, detail
// drawer, credentials, and distribution config only ship when /command/builds
// is visited.
const MobileBuildsPage = lazy(() => import('./routes/command/MobileBuildsPage'));
// KAI-313: StaffPage lazy-loaded so the staff table, invite/edit/remove
// dialogs, and roles summary only ship when /command/staff is visited.
const StaffPage = lazy(() => import('./routes/command/StaffPage'));
// KAI-315: PermissionsPage lazy-loaded — the permissions matrix, bulk actions,
// diff preview dialog, and audit sidebar only ship when /command/permissions is visited.
const PermissionsPage = lazy(() => import('./routes/command/PermissionsPage'));
// KAI-469: SupportPage lazy-loaded — screen sharing, ticket creator, and
// integration config only ship when /command/support is visited.
const SupportPage = lazy(() => import('./routes/command/SupportPage'));
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
        <Route path="/admin/recorders" element={<RecordersPage />} />
        <Route path="/admin/users" element={<UsersPage />} />
        <Route path="/admin/live" element={<LiveViewPage />} />
        <Route path="/admin/playback/:eventId" element={<PlaybackPage />} />
        <Route path="/admin/ai-settings" element={<AiSettingsPage />} />
        <Route path="/admin/schedules" element={<SchedulesPage />} />
        <Route path="/admin/events" element={<EventsPage />} />
        <Route path="/admin/health" element={<SystemHealthPage />} />
        <Route path="/admin/settings" element={<AdminSettings />} />
        <Route path="/command" element={<FleetDashboard />} />
        <Route path="/command/customers" element={<CustomersPage />} />
        <Route path="/command/customers/:customerId" element={<CustomerDrillDown />} />
        <Route path="/command/brand" element={<BrandConfigPage />} />
        <Route path="/command/builds" element={<MobileBuildsPage />} />
        <Route path="/command/staff" element={<StaffPage />} />
        <Route path="/command/permissions" element={<PermissionsPage />} />
        <Route path="/command/support" element={<SupportPage />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </Suspense>
  );
}
