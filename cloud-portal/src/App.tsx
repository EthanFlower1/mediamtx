import { useState } from 'react';
import { BrowserRouter, Navigate, Outlet, Route, Routes, useNavigate, useOutletContext, useParams } from 'react-router-dom';
import { ALERTS, AUDIT, CAMERAS, ROLES, RULES, SAVED_VIEWS, SITES, USERS } from './data/portalData';
import type { Scope, Route as PortalRoute } from './types';
import { LeftRail, TopBar } from './components/Shell';
import { Overview } from './screens/Overview';
import { SiteDetail, Sites } from './screens/Sites';
import { LiveWall } from './screens/LiveWall';
import { Playback } from './screens/Playback';
import { Devices, Rules, Users } from './screens/Modules';
import { Alerts, Audit, Billing, Onboarding, RemoteAccess, Search } from './screens/Extras';
import { Login } from './screens/Login';
import { SessionProvider } from './auth/SessionContext';
import { RequireAuth } from './auth/RequireAuth';

interface PortalOutletContext {
  scope: Scope;
  setScope: (s: Scope) => void;
}

function useScope(): [Scope, (s: Scope) => void] {
  const ctx = useOutletContext<PortalOutletContext>();
  return [ctx.scope, ctx.setScope];
}

function routeToPath(r: PortalRoute): string {
  if (r === 'overview') return '/overview';
  return `/${r}`;
}

function PortalLayout() {
  const [scope, setScope] = useState<Scope>({ kind: 'all' });
  const navigate = useNavigate();
  return (
    <>
      <TopBar
        scope={scope}
        setScope={setScope}
        sites={SITES}
        alerts={ALERTS}
        onOpenAlerts={() => navigate('/alerts')}
      />
      <div className="shell">
        <LeftRail />
        <Outlet context={{ scope, setScope } satisfies PortalOutletContext} />
      </div>
    </>
  );
}

function OverviewRoute() {
  const navigate = useNavigate();
  const [scope, setScope] = useScope();
  return (
    <Overview
      scope={scope}
      sites={SITES}
      cameras={CAMERAS}
      alerts={ALERTS}
      savedViews={SAVED_VIEWS}
      onNavigate={(r) => navigate(routeToPath(r))}
      setScope={setScope}
    />
  );
}

function SitesRoute() {
  const navigate = useNavigate();
  const [scope, setScope] = useScope();
  return (
    <Sites
      scope={scope}
      sites={SITES}
      cameras={CAMERAS}
      setScope={(s) => {
        setScope(s);
        if (s.kind === 'site') navigate(`/sites/${s.id}`);
      }}
      onNavigate={(r) => navigate(routeToPath(r))}
    />
  );
}

function SiteDetailRoute() {
  const navigate = useNavigate();
  const { siteId } = useParams<{ siteId: string }>();
  const [, setScope] = useScope();
  const site = SITES.find((s) => s.id === siteId);
  if (!site) return <Navigate to="/sites" replace />;
  return (
    <SiteDetail
      site={site}
      cameras={CAMERAS}
      setScope={(s) => {
        setScope(s);
        if (s.kind === 'all') navigate('/sites');
        else if (s.kind === 'site') navigate(`/sites/${s.id}`);
      }}
      onNavigate={(r) => navigate(routeToPath(r))}
    />
  );
}

function LiveWallRoute() {
  const [scope, setScope] = useScope();
  return <LiveWall scope={scope} sites={SITES} cameras={CAMERAS} savedViews={SAVED_VIEWS} setScope={setScope} />;
}

function PlaybackRoute() {
  const [scope, setScope] = useScope();
  return <Playback scope={scope} sites={SITES} cameras={CAMERAS} setScope={setScope} />;
}

function SearchRoute() {
  const [scope] = useScope();
  return <Search scope={scope} sites={SITES} cameras={CAMERAS} />;
}

function DevicesRoute() {
  const [scope, setScope] = useScope();
  return <Devices scope={scope} sites={SITES} cameras={CAMERAS} setScope={setScope} />;
}

export function App() {
  return (
    <BrowserRouter>
      <SessionProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route element={<RequireAuth><PortalLayout /></RequireAuth>}>
            <Route index element={<Navigate to="/overview" replace />} />
            <Route path="/overview" element={<OverviewRoute />} />
            <Route path="/live" element={<LiveWallRoute />} />
            <Route path="/playback" element={<PlaybackRoute />} />
            <Route path="/search" element={<SearchRoute />} />
            <Route path="/sites" element={<SitesRoute />} />
            <Route path="/sites/:siteId" element={<SiteDetailRoute />} />
            <Route path="/devices" element={<DevicesRoute />} />
            <Route path="/rules" element={<Rules rules={RULES} sites={SITES} cameras={CAMERAS} />} />
            <Route path="/users" element={<Users users={USERS} roles={ROLES} sites={SITES} />} />
            <Route path="/remote" element={<RemoteAccess />} />
            <Route path="/alerts" element={<Alerts alerts={ALERTS} />} />
            <Route path="/audit" element={<Audit audit={AUDIT} />} />
            <Route path="/billing" element={<Billing />} />
            <Route path="/onboarding" element={<Onboarding />} />
            <Route path="*" element={<Navigate to="/overview" replace />} />
          </Route>
        </Routes>
      </SessionProvider>
    </BrowserRouter>
  );
}
