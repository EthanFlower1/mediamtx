import { useEffect, useRef, useState } from 'react';
import { NavLink } from 'react-router-dom';
import logo from '../assets/logo.png';
import { useSession } from '../auth/SessionContext';
import type { AlertItem, Scope, Site } from '../types';
import { Icon } from './primitives';

function initials(name: string, email: string): string {
  const src = (name || email || '').trim();
  if (!src) return '??';
  const parts = src.split(/[\s@.]+/).filter(Boolean);
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
  return src.slice(0, 2).toUpperCase();
}

export function scopeLabel(scope: Scope, sites: Site[]): string {
  if (scope.kind === 'all') return `All sites · ${sites.length}`;
  if (scope.kind === 'site') {
    const s = sites.find(x => x.id === scope.id);
    return s ? `${s.code} · ${s.name}` : 'Site';
  }
  return `${scope.label} · ${scope.ids.length} sites`;
}

export function sitesInScope(sites: Site[], scope: Scope): Site[] {
  if (scope.kind === 'all') return sites;
  if (scope.kind === 'site') return sites.filter(s => s.id === scope.id);
  return sites.filter(s => scope.ids.includes(s.id));
}

interface TopBarProps {
  scope: Scope;
  setScope: (s: Scope) => void;
  sites: Site[];
  alerts: AlertItem[];
  onOpenAlerts: () => void;
  onOpenCmd?: () => void;
}

export function TopBar({ scope, setScope, sites, alerts, onOpenAlerts, onOpenCmd }: TopBarProps) {
  const [pickerOpen, setPickerOpen] = useState(false);
  const [, setOrgOpen] = useState(false);
  const [userMenuOpen, setUserMenuOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const userRef = useRef<HTMLDivElement>(null);
  const { user, tenant, signOut } = useSession();

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setPickerOpen(false);
      if (userRef.current && !userRef.current.contains(e.target as Node)) setUserMenuOpen(false);
    };
    window.addEventListener('mousedown', handler);
    return () => window.removeEventListener('mousedown', handler);
  }, []);

  const unack = alerts.filter(a => !a.ack).length;
  const displayName = user?.name?.trim() || user?.email || 'Signed in';
  const orgName = tenant?.name || 'Organization';
  const orgInitial = orgName.trim().charAt(0).toUpperCase() || 'R';
  const avatar = user ? initials(user.name, user.email) : 'RA';

  return (
    <header style={{
      height: 52, background: 'var(--bg-secondary)', borderBottom: '1px solid var(--border)',
      display: 'flex', alignItems: 'center', padding: '0 16px', gap: 14, flexShrink: 0, position: 'relative', zIndex: 50,
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, paddingRight: 12, borderRight: '1px solid var(--border)', height: '100%' }}>
        <img src={logo} alt="" style={{ width: 22, height: 22 }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, letterSpacing: 3, fontWeight: 600, color: 'var(--text-primary)' }}>RAIKADA</span>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1, color: 'var(--text-muted)', padding: '2px 5px', border: '1px solid var(--border)', borderRadius: 2 }}>CLOUD</span>
      </div>

      <button onClick={() => setOrgOpen(o => !o)} style={{
        height: 32, padding: '0 10px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
        display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', color: 'var(--text-primary)',
      }}>
        <div style={{
          width: 18, height: 18, background: 'var(--accent)', color: '#0A0A0A', borderRadius: 2,
          fontFamily: 'var(--font-mono)', fontSize: 10, fontWeight: 700,
          display: 'flex', alignItems: 'center', justifyContent: 'center',
        }}>{orgInitial}</div>
        <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, fontWeight: 500 }}>{orgName}</span>
        <Icon name="chevrons-up-down" size={12} color="var(--text-muted)" />
      </button>

      <div ref={ref} style={{ position: 'relative' }}>
        <button onClick={() => setPickerOpen(o => !o)} style={{
          height: 32, padding: '0 10px 0 8px',
          background: scope.kind !== 'all' ? 'var(--accent-13)' : 'var(--bg-tertiary)',
          border: `1px solid ${scope.kind !== 'all' ? 'var(--accent-27)' : 'var(--border)'}`, borderRadius: 4,
          display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer',
          color: scope.kind !== 'all' ? 'var(--accent)' : 'var(--text-primary)',
        }}>
          <Icon name="crosshair" size={13} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 1, textTransform: 'uppercase' }}>SCOPE</span>
          <span style={{ width: 1, height: 14, background: 'var(--border)' }} />
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, fontWeight: 500 }}>{scopeLabel(scope, sites)}</span>
          <Icon name="chevron-down" size={12} style={{ marginLeft: 2 }} />
        </button>
        {pickerOpen && (
          <ScopeMenu
            sites={sites}
            setScope={(s) => { setScope(s); setPickerOpen(false); }}
          />
        )}
      </div>

      <div style={{ flex: 1 }} />

      <button onClick={onOpenCmd} style={{
        height: 32, width: 280, padding: '0 10px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
        display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', color: 'var(--text-secondary)',
      }}>
        <Icon name="search" size={13} />
        <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12 }}>Search sites, cameras, events…</span>
        <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', padding: '1px 5px', border: '1px solid var(--border)', borderRadius: 2 }}>⌘K</span>
      </button>

      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '0 10px', height: 32, background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4 }}>
        <span style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--success)', boxShadow: 'var(--glow-success)' }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-secondary)', textTransform: 'uppercase' }}>SYNC · 14:32:07 UTC</span>
      </div>

      <button onClick={onOpenAlerts} style={{
        height: 32, width: 32, background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
        display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', color: 'var(--text-primary)',
        position: 'relative',
      }}>
        <Icon name="bell" size={14} />
        {unack > 0 && (
          <span style={{
            position: 'absolute', top: -3, right: -3, minWidth: 14, height: 14, padding: '0 3px',
            background: 'var(--danger)', color: '#0A0A0A',
            fontFamily: 'var(--font-mono)', fontSize: 9, fontWeight: 600,
            borderRadius: 7, display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>{unack}</span>
        )}
      </button>

      <div ref={userRef} style={{ position: 'relative', height: '100%', display: 'flex' }}>
        <button onClick={() => setUserMenuOpen(o => !o)} style={{
          display: 'flex', alignItems: 'center', gap: 8, padding: '0 8px 0 10px',
          borderLeft: '1px solid var(--border)', borderTop: 'none', borderRight: 'none', borderBottom: 'none',
          background: 'transparent', cursor: 'pointer', height: '100%',
        }}>
          <div style={{
            width: 28, height: 28, borderRadius: '50%', background: 'var(--bg-tertiary)', border: '1px solid var(--border)',
            display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)',
          }}>{avatar}</div>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-start' }}>
            <span style={{ fontFamily: 'var(--font-sans)', fontSize: 11, fontWeight: 500, color: 'var(--text-primary)' }}>{displayName}</span>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1, color: 'var(--text-muted)' }}>SIGNED IN</span>
          </div>
          <Icon name="chevron-down" size={12} color="var(--text-muted)" />
        </button>
        {userMenuOpen && (
          <div style={{
            position: 'absolute', top: 50, right: 4, width: 200,
            background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6,
            boxShadow: '0 10px 24px rgba(0,0,0,0.6)', zIndex: 100, padding: 4,
          }}>
            <button onClick={() => { setUserMenuOpen(false); void signOut(); }} style={{
              width: '100%', padding: '8px 12px', background: 'transparent', border: 'none', cursor: 'pointer',
              display: 'flex', alignItems: 'center', gap: 8, color: 'var(--text-primary)',
              fontFamily: 'var(--font-sans)', fontSize: 12, textAlign: 'left',
            }}>
              <Icon name="log-out" size={13} />
              Sign out
            </button>
          </div>
        )}
      </div>
    </header>
  );
}

interface ScopeMenuProps {
  sites: Site[];
  setScope: (s: Scope) => void;
}

function ScopeMenu({ sites, setScope }: ScopeMenuProps) {
  const groups: Array<{ label: string; kind: 'all' | 'group'; ids: string[] }> = [
    { label: 'All sites', kind: 'all', ids: [] },
    { label: 'Logistics', kind: 'group', ids: sites.filter(s => s.kind === 'logistics').map(s => s.id) },
    { label: 'Retail', kind: 'group', ids: sites.filter(s => s.kind === 'retail').map(s => s.id) },
    { label: 'Residential', kind: 'group', ids: sites.filter(s => s.kind === 'residential').map(s => s.id) },
    { label: 'Industrial', kind: 'group', ids: sites.filter(s => s.kind === 'industrial').map(s => s.id) },
  ];

  return (
    <div style={{
      position: 'absolute', top: 38, left: 0, width: 420, background: 'var(--bg-secondary)',
      border: '1px solid var(--border)', borderRadius: 6, boxShadow: '0 10px 24px rgba(0,0,0,0.6)', zIndex: 100,
      maxHeight: 540, overflow: 'auto',
    }}>
      <div style={{ padding: 10, borderBottom: '1px solid var(--border)' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 2, color: 'var(--text-muted)', textTransform: 'uppercase' }}>GROUPS</span>
      </div>
      {groups.map(g => (
        <button key={g.label} onClick={() => setScope(g.kind === 'all' ? { kind: 'all' } : { kind: 'group', label: g.label, ids: g.ids })}
          style={{
            width: '100%', padding: '8px 12px', background: 'transparent', border: 'none', cursor: 'pointer',
            display: 'flex', alignItems: 'center', justifyContent: 'space-between', color: 'var(--text-primary)',
            borderBottom: '1px solid var(--border)',
          }}>
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 13 }}>{g.label}</span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{g.kind === 'all' ? sites.length : g.ids.length}</span>
        </button>
      ))}
      <div style={{ padding: 10, borderBottom: '1px solid var(--border)' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 2, color: 'var(--text-muted)', textTransform: 'uppercase' }}>SITES</span>
      </div>
      {sites.map(s => (
        <button key={s.id} onClick={() => setScope({ kind: 'site', id: s.id })}
          style={{
            width: '100%', padding: '8px 12px', background: 'transparent', border: 'none', cursor: 'pointer',
            display: 'flex', alignItems: 'center', justifyContent: 'space-between', color: 'var(--text-primary)',
          }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%',
              background: s.status === 'online' ? 'var(--success)' : s.status === 'degraded' ? 'var(--warning)' : 'var(--danger)' }} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)', minWidth: 56 }}>{s.code}</span>
            <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12 }}>{s.name}</span>
          </div>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{s.online}/{s.cameras}</span>
        </button>
      ))}
    </div>
  );
}

interface NavItem { path: string; icon: string; label: string; }

export function LeftRail() {
  const top: NavItem[] = [
    { path: '/overview', icon: 'layout-dashboard', label: 'Overview' },
    { path: '/live', icon: 'grid-2x2', label: 'Live Wall' },
    { path: '/playback', icon: 'play-circle', label: 'Playback' },
    { path: '/search', icon: 'search-code', label: 'Search' },
    { path: '/sites', icon: 'map-pin', label: 'Sites' },
    { path: '/devices', icon: 'cctv', label: 'Devices' },
    { path: '/rules', icon: 'workflow', label: 'Recording Rules' },
  ];
  const mid: NavItem[] = [
    { path: '/users', icon: 'users', label: 'Users & Roles' },
    { path: '/remote', icon: 'globe', label: 'Remote Access' },
    { path: '/alerts', icon: 'bell-ring', label: 'Alerts' },
    { path: '/audit', icon: 'scroll-text', label: 'Audit Log' },
  ];
  const bottom: NavItem[] = [
    { path: '/billing', icon: 'credit-card', label: 'Billing' },
    { path: '/onboarding', icon: 'plus-square', label: 'Add Site' },
  ];

  const NavBtn = ({ it }: { it: NavItem }) => (
    <NavLink
      to={it.path}
      title={it.label}
      style={({ isActive }) => ({
        width: 44, height: 38, position: 'relative',
        background: isActive ? 'var(--accent-13)' : 'transparent',
        border: isActive ? '1px solid var(--accent-27)' : '1px solid transparent',
        borderRadius: 4, cursor: 'pointer', textDecoration: 'none',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        color: isActive ? 'var(--accent)' : 'var(--text-secondary)',
      })}
    >
      {({ isActive }) => (
        <>
          {isActive && <div style={{
            position: 'absolute', left: -8, top: 6, bottom: 6, width: 3, background: 'var(--accent)',
            boxShadow: '0 0 6px rgba(249,115,22,0.6)',
          }} />}
          <Icon name={it.icon} size={17} />
        </>
      )}
    </NavLink>
  );

  return (
    <aside style={{
      width: 60, background: 'var(--bg-secondary)', borderRight: '1px solid var(--border)',
      display: 'flex', flexDirection: 'column', alignItems: 'center', padding: '12px 0',
      flexShrink: 0,
    }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, alignItems: 'center' }}>
        {top.map(it => <NavBtn key={it.path} it={it} />)}
      </div>
      <div style={{ width: 28, height: 1, background: 'var(--border)', margin: '12px 0' }} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, alignItems: 'center' }}>
        {mid.map(it => <NavBtn key={it.path} it={it} />)}
      </div>
      <div style={{ flex: 1 }} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, alignItems: 'center' }}>
        {bottom.map(it => <NavBtn key={it.path} it={it} />)}
      </div>
    </aside>
  );
}
