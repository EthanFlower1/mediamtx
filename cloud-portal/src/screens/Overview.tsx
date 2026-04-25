import type { AlertItem, Camera, Route, SavedView, Scope, Site } from '../types';
import { Btn, Icon, PageHeader, SectionHeader, Stat } from '../components/primitives';
import { scopeLabel, sitesInScope } from '../components/Shell';

interface OverviewProps {
  scope: Scope;
  sites: Site[];
  cameras: Camera[];
  alerts: AlertItem[];
  savedViews: SavedView[];
  onNavigate: (r: Route) => void;
  setScope: (s: Scope) => void;
}

export function Overview({ scope, sites, cameras, alerts, savedViews, onNavigate, setScope }: OverviewProps) {
  const inScope = sitesInScope(sites, scope);
  const camsInScope = cameras.filter(c => inScope.find(s => s.id === c.siteId));

  const totals = {
    sites: inScope.length,
    cameras: camsInScope.length,
    online: camsInScope.filter(c => c.status === 'online').length,
    offline: camsInScope.filter(c => c.status === 'offline').length,
    degraded: camsInScope.filter(c => c.status === 'degraded').length,
    recording: camsInScope.filter(c => c.recording).length,
    alerts: alerts.filter(a => !a.ack && (scope.kind === 'all' || inScope.find(s => s.code === a.site))).length,
    storageUsed: inScope.reduce((a, s) => a + s.storage.used, 0),
    storageTotal: inScope.reduce((a, s) => a + s.storage.total, 0),
    bandwidth: inScope.reduce((a, s) => a + s.bandwidth, 0),
  };

  const onlinePct = totals.cameras ? totals.online / totals.cameras : 0;
  const degradedPct = totals.cameras ? totals.degraded / totals.cameras : 0;
  const offlinePct = totals.cameras ? totals.offline / totals.cameras : 0;

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'auto', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Overview"
        subtitle={`SCOPE · ${scopeLabel(scope, sites).toUpperCase()}`}
        actions={<>
          <Btn kind="ghost" onClick={() => onNavigate('alerts')}><Icon name="bell" size={13} /> Alerts</Btn>
          <Btn kind="primary" onClick={() => onNavigate('live')}><Icon name="grid-2x2" size={13} /> Open Live Wall</Btn>
        </>}
      />

      <div style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 20 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(6, 1fr)', gap: 12 }}>
          <Stat label="SITES" value={totals.sites} sub={`${inScope.filter(s => s.status === 'online').length} online`} />
          <Stat label="CAMERAS" value={totals.cameras} sub={`${totals.online} streaming`} />
          <Stat label="RECORDING" value={totals.recording} accent="var(--accent)" glow sub={`${Math.round((totals.recording / Math.max(totals.cameras, 1)) * 100)}% of fleet`} />
          <Stat label="OPEN ALERTS" value={totals.alerts} accent={totals.alerts ? 'var(--danger)' : 'var(--text-primary)'} sub={totals.alerts ? `${alerts.filter(a => a.severity === 'danger' && !a.ack).length} critical` : 'all clear'} />
          <Stat label="STORAGE" value={`${totals.storageUsed} GB`} sub={`/ ${totals.storageTotal} GB`} />
          <Stat label="BANDWIDTH" value={`${totals.bandwidth.toFixed(1)} Mb/s`} sub="up · 24h avg" />
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1.5fr 1fr', gap: 16 }}>
          <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 16 }}>
            <SectionHeader>FLEET HEALTH SPECTRUM</SectionHeader>
            <div style={{ marginTop: 18 }}>
              <div style={{ display: 'flex', height: 32, borderRadius: 2, overflow: 'hidden', border: '1px solid var(--border)' }}>
                <div style={{ width: `${onlinePct * 100}%`, background: 'rgba(34,197,94,0.6)', display: 'flex', alignItems: 'center', justifyContent: 'flex-start', paddingLeft: 8 }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#0A0A0A', fontWeight: 600 }}>{totals.online} ONLINE</span>
                </div>
                <div style={{ width: `${degradedPct * 100}%`, background: 'rgba(234,179,8,0.6)', display: 'flex', alignItems: 'center', paddingLeft: 6 }}>
                  {degradedPct > 0.04 && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#0A0A0A', fontWeight: 600 }}>{totals.degraded}</span>}
                </div>
                <div style={{ width: `${offlinePct * 100}%`, background: 'rgba(239,68,68,0.6)', display: 'flex', alignItems: 'center', paddingLeft: 6 }}>
                  {offlinePct > 0.04 && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: '#0A0A0A', fontWeight: 600 }}>{totals.offline} OFF</span>}
                </div>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1 }}>
                <span>0%</span><span>25%</span><span>50%</span><span>75%</span><span>100%</span>
              </div>
            </div>

            <div style={{ marginTop: 24 }}>
              <SectionHeader>PER-SITE STATUS</SectionHeader>
              <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 6 }}>
                {inScope.slice(0, 8).map(s => {
                  const onlineSitePct = s.online / Math.max(s.cameras, 1);
                  const c = s.status === 'online' ? 'var(--success)' : s.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
                  return (
                    <button key={s.id} onClick={() => setScope({ kind: 'site', id: s.id })} style={{
                      display: 'grid', gridTemplateColumns: '70px 1fr 200px 60px', gap: 12, alignItems: 'center',
                      padding: '6px 8px', background: 'transparent', border: '1px solid transparent', borderRadius: 4, cursor: 'pointer',
                      width: '100%', textAlign: 'left',
                    }}
                      onMouseOver={(e) => { e.currentTarget.style.background = 'var(--bg-tertiary)'; }}
                      onMouseOut={(e) => { e.currentTarget.style.background = 'transparent'; }}>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{s.code}</span>
                      <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.name}</span>
                      <div style={{ position: 'relative', height: 6, background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 1 }}>
                        <div style={{ position: 'absolute', inset: 0, width: `${onlineSitePct * 100}%`, background: c, opacity: 0.7 }} />
                      </div>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)', textAlign: 'right' }}>{s.online}/{s.cameras}</span>
                    </button>
                  );
                })}
              </div>
            </div>
          </div>

          <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 16, display: 'flex', flexDirection: 'column' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <SectionHeader>RECENT ALERTS</SectionHeader>
              <button onClick={() => onNavigate('alerts')} style={{
                background: 'transparent', border: 'none', cursor: 'pointer',
                fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--accent)', textTransform: 'uppercase',
              }}>VIEW ALL →</button>
            </div>
            <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 6, flex: 1, overflow: 'auto', minHeight: 0 }}>
              {alerts.slice(0, 6).map(a => {
                const sevC = a.severity === 'danger' ? 'var(--danger)' : a.severity === 'warning' ? 'var(--warning)' : 'var(--accent)';
                return (
                  <div key={a.id} style={{
                    display: 'grid', gridTemplateColumns: '6px 60px 1fr 60px', gap: 10, alignItems: 'center',
                    padding: 8, background: a.ack ? 'transparent' : 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
                  }}>
                    <span style={{ width: 6, height: 6, borderRadius: '50%', background: sevC, boxShadow: a.ack ? 'none' : `0 0 6px ${sevC}` }} />
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{a.site}</span>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 1, minWidth: 0 }}>
                      <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{a.text}</span>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 0.5 }}>{a.camName.toUpperCase()} · {a.cam}</span>
                    </div>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)', textAlign: 'right', fontVariantNumeric: 'tabular-nums' }}>{a.time}</span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '2fr 1fr', gap: 16 }}>
          <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 16 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <SectionHeader>SAVED LIVE WALLS</SectionHeader>
              <button onClick={() => onNavigate('live')} style={{
                background: 'transparent', border: 'none', cursor: 'pointer',
                fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--accent)', textTransform: 'uppercase',
              }}>+ NEW WALL</button>
            </div>
            <div style={{ marginTop: 12, display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 10 }}>
              {savedViews.map(v => (
                <button key={v.id} onClick={() => onNavigate('live')} style={{
                  padding: 12, background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4, cursor: 'pointer',
                  display: 'flex', flexDirection: 'column', gap: 8, textAlign: 'left',
                }}>
                  <div style={{ aspectRatio: '16/10', background: '#0A0A0A', position: 'relative', overflow: 'hidden' }}>
                    <div style={{ position: 'absolute', inset: 4, display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 2 }}>
                      {Array.from({ length: 9 }).map((_, i) => (
                        <div key={i} style={{ background: 'linear-gradient(135deg, #1a1410, #0d0c0a)', position: 'relative' }}>
                          <span style={{ position: 'absolute', top: 0, left: 0, width: 4, height: 4, borderTop: '1px solid var(--accent)', borderLeft: '1px solid var(--accent)', opacity: 0.5 }} />
                          <span style={{ position: 'absolute', bottom: 0, right: 0, width: 4, height: 4, borderBottom: '1px solid var(--accent)', borderRight: '1px solid var(--accent)', opacity: 0.5 }} />
                        </div>
                      ))}
                    </div>
                  </div>
                  <div>
                    <div style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 1, color: 'var(--accent)' }}>{v.name}</div>
                    <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 0.5, marginTop: 2 }}>{v.cams} CAMS · {v.sites} SITES · {v.lastUsed}</div>
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
            <SectionHeader>QUICK ACTIONS</SectionHeader>
            {([
              { id: 'onboarding', icon: 'plus-square', label: 'Add a new site', sub: 'Claim NVR · invite admins' },
              { id: 'rules', icon: 'workflow', label: 'Configure recording rules', sub: '6 active · 0 drafts' },
              { id: 'users', icon: 'user-plus', label: 'Invite a user', sub: 'Operator, Site Admin, Viewer' },
              { id: 'remote', icon: 'key', label: 'Issue remote access token', sub: 'Time-limited share link' },
            ] as Array<{ id: Route; icon: string; label: string; sub: string }>).map(a => (
              <button key={a.id} onClick={() => onNavigate(a.id)} style={{
                display: 'flex', gap: 10, padding: 10, background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
                cursor: 'pointer', textAlign: 'left', alignItems: 'center',
              }}>
                <div style={{
                  width: 30, height: 30, borderRadius: 4, background: 'var(--bg-primary)', border: '1px solid var(--accent-27)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--accent)',
                }}>
                  <Icon name={a.icon} size={14} />
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div style={{ fontFamily: 'var(--font-sans)', fontSize: 12, fontWeight: 500, color: 'var(--text-primary)' }}>{a.label}</div>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)', letterSpacing: 0.5 }}>{a.sub}</div>
                </div>
                <Icon name="chevron-right" size={14} color="var(--text-muted)" />
              </button>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
