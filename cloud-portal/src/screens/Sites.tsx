import { useState } from 'react';
import type { Camera, Route, Scope, Site } from '../types';
import { Btn, Icon, PageHeader, SectionHeader, Segmented, Stat } from '../components/primitives';
import { CameraTile } from '../components/CameraTile';

interface SitesProps {
  scope: Scope;
  sites: Site[];
  cameras: Camera[];
  setScope: (s: Scope) => void;
  onNavigate: (r: Route) => void;
}

export function Sites({ sites, cameras, setScope, onNavigate }: SitesProps) {
  const [view, setView] = useState<'grid' | 'map' | 'list'>('grid');
  const [filter, setFilter] = useState<string>('all');
  const filtered = sites.filter(s => filter === 'all' || s.kind === filter);

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Sites"
        subtitle={`${sites.length} TOTAL · ${sites.filter(s => s.status === 'online').length} ONLINE`}
        actions={<>
          <Segmented options={[
            { value: 'all', label: 'ALL' }, { value: 'logistics', label: 'LOGISTICS' },
            { value: 'retail', label: 'RETAIL' }, { value: 'residential', label: 'RESIDENTIAL' },
            { value: 'industrial', label: 'INDUSTRIAL' },
          ]} value={filter} onChange={setFilter} />
          <Segmented options={[
            { value: 'grid', label: 'GRID' }, { value: 'map', label: 'MAP' }, { value: 'list', label: 'LIST' },
          ]} value={view} onChange={(v) => setView(v as 'grid' | 'map' | 'list')} />
          <Btn kind="primary" onClick={() => onNavigate('onboarding')}>+ Add Site</Btn>
        </>}
      />

      {view === 'grid' && (
        <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: 12 }}>
            {filtered.map(s => <SiteCard key={s.id} site={s} cameras={cameras} onClick={() => setScope({ kind: 'site', id: s.id })} />)}
          </div>
        </div>
      )}

      {view === 'map' && <SitesMap sites={filtered} setScope={setScope} />}

      {view === 'list' && <SitesList sites={filtered} setScope={setScope} />}
    </div>
  );
}

function SiteCard({ site, cameras, onClick }: { site: Site; cameras: Camera[]; onClick: () => void }) {
  const c = site.status === 'online' ? 'var(--success)' : site.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
  const siteCams = cameras.filter(cam => cam.siteId === site.id);
  return (
    <button onClick={onClick} style={{
      padding: 0, background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, cursor: 'pointer',
      textAlign: 'left', overflow: 'hidden', display: 'flex', flexDirection: 'column',
    }}>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 1, background: 'var(--border)', height: 100 }}>
        {siteCams.slice(0, 4).map((cam, i) => (
          <div key={i} style={{
            position: 'relative',
            background: cam.status === 'offline' ? '#0a0907' : `radial-gradient(circle at ${30 + i * 15}% ${40 + i * 8}%, rgba(120,90,40,0.45), transparent 55%), linear-gradient(180deg, #211711, #0a0907)`,
            opacity: cam.status === 'offline' ? 0.3 : 1,
          }}>
            <span style={{ position: 'absolute', top: 2, left: 2, width: 3, height: 3, borderTop: '1px solid var(--accent)', borderLeft: '1px solid var(--accent)', opacity: 0.5 }} />
            <span style={{ position: 'absolute', bottom: 2, right: 2, width: 3, height: 3, borderBottom: '1px solid var(--accent)', borderRight: '1px solid var(--accent)', opacity: 0.5 }} />
          </div>
        ))}
        {Array.from({ length: Math.max(0, 4 - siteCams.length) }).map((_, i) => <div key={'e' + i} style={{ background: '#0a0907' }} />)}
      </div>
      <div style={{ padding: 12, display: 'flex', flexDirection: 'column', gap: 8 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: c, boxShadow: `0 0 6px ${c}` }} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)', letterSpacing: 0.5 }}>{site.code}</span>
          </div>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1, textTransform: 'uppercase' }}>{site.kind}</span>
        </div>
        <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, fontWeight: 500, color: 'var(--text-primary)' }}>{site.name}</div>
        <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{site.city}</div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 6, marginTop: 4, paddingTop: 8, borderTop: '1px solid var(--border)' }}>
          <div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1 }}>CAMS</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-primary)' }}>{site.online}/{site.cameras}</div>
          </div>
          <div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1 }}>STORAGE</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-primary)' }}>{Math.round((site.storage.used / site.storage.total) * 100)}%</div>
          </div>
          <div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1 }}>ALERTS</div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: site.alerts ? 'var(--danger)' : 'var(--text-primary)' }}>{site.alerts}</div>
          </div>
        </div>
      </div>
    </button>
  );
}

function SitesMap({ sites, setScope }: { sites: Site[]; setScope: (s: Scope) => void }) {
  return (
    <div style={{ flex: 1, position: 'relative', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <svg style={{ position: 'absolute', inset: 0, width: '100%', height: '100%' }}>
        <defs>
          <pattern id="grid" width="60" height="60" patternUnits="userSpaceOnUse">
            <path d="M 60 0 L 0 0 0 60" fill="none" stroke="#1a1a1a" strokeWidth="1" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#grid)" />
      </svg>
      <div style={{
        position: 'absolute', top: 16, left: 16, display: 'flex', flexDirection: 'column', gap: 4,
        fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1, color: 'var(--text-muted)',
      }}>
        <div>NORTHWALL · TACTICAL OVERVIEW</div>
        <div>REGION · US-WEST · 14 SITES</div>
      </div>
      <div style={{ position: 'absolute', bottom: 16, right: 16 }}>
        <svg width="60" height="60" viewBox="0 0 60 60">
          <circle cx="30" cy="30" r="24" fill="none" stroke="#262626" strokeWidth="1" />
          <line x1="30" y1="6" x2="30" y2="54" stroke="#262626" />
          <line x1="6" y1="30" x2="54" y2="30" stroke="#262626" />
          <text x="30" y="14" fill="#F97316" fontSize="9" textAnchor="middle" fontFamily="var(--font-mono)">N</text>
          <text x="30" y="51" fill="#404040" fontSize="9" textAnchor="middle" fontFamily="var(--font-mono)">S</text>
          <text x="9" y="32" fill="#404040" fontSize="9" textAnchor="middle" fontFamily="var(--font-mono)">W</text>
          <text x="51" y="32" fill="#404040" fontSize="9" textAnchor="middle" fontFamily="var(--font-mono)">E</text>
        </svg>
      </div>
      {sites.map(s => {
        const c = s.status === 'online' ? 'var(--success)' : s.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
        return (
          <button key={s.id} onClick={() => setScope({ kind: 'site', id: s.id })} style={{
            position: 'absolute', left: `${s.x * 1.6 + 10}%`, top: `${s.y * 0.95}%`,
            transform: 'translate(-50%, -50%)',
            background: 'transparent', border: 'none', cursor: 'pointer', padding: 0,
            display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4,
          }}>
            <div style={{ position: 'relative', width: 24, height: 24 }}>
              <svg width="24" height="24" viewBox="0 0 24 24">
                <circle cx="12" cy="12" r="4" fill={c} stroke="#0A0A0A" strokeWidth="1" style={{ filter: `drop-shadow(0 0 6px ${c})` }} />
                <circle cx="12" cy="12" r="10" fill="none" stroke={c} strokeWidth="0.5" opacity="0.4" />
                <line x1="12" y1="0" x2="12" y2="6" stroke={c} strokeWidth="1" opacity="0.6" />
                <line x1="12" y1="18" x2="12" y2="24" stroke={c} strokeWidth="1" opacity="0.6" />
                <line x1="0" y1="12" x2="6" y2="12" stroke={c} strokeWidth="1" opacity="0.6" />
                <line x1="18" y1="12" x2="24" y2="12" stroke={c} strokeWidth="1" opacity="0.6" />
              </svg>
            </div>
            <div style={{
              background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 2, padding: '2px 6px',
              display: 'flex', flexDirection: 'column', gap: 1, minWidth: 90, alignItems: 'flex-start',
            }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--accent)', letterSpacing: 0.5 }}>{s.code}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 8, color: 'var(--text-secondary)', letterSpacing: 0.5 }}>{s.online}/{s.cameras} · {s.city.split(',')[0].toUpperCase()}</span>
            </div>
          </button>
        );
      })}
    </div>
  );
}

function SitesList({ sites, setScope }: { sites: Site[]; setScope: (s: Scope) => void }) {
  return (
    <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
      <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, overflow: 'hidden' }}>
        <div style={{
          display: 'grid', gridTemplateColumns: '70px 1fr 140px 120px 110px 90px 90px 30px', gap: 12, padding: '10px 16px',
          borderBottom: '1px solid var(--border)', fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1.5, color: 'var(--text-muted)', textTransform: 'uppercase',
        }}>
          <span>CODE</span><span>SITE</span><span>CITY</span><span>KIND</span><span>CAMERAS</span><span>STORAGE</span><span>ALERTS</span><span></span>
        </div>
        {sites.map((s, i) => {
          const c = s.status === 'online' ? 'var(--success)' : s.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
          return (
            <button key={s.id} onClick={() => setScope({ kind: 'site', id: s.id })} style={{
              display: 'grid', gridTemplateColumns: '70px 1fr 140px 120px 110px 90px 90px 30px', gap: 12, padding: '12px 16px',
              borderBottom: i < sites.length - 1 ? '1px solid var(--border)' : 'none', background: 'transparent', border: 'none',
              borderLeft: '1px solid transparent', cursor: 'pointer', alignItems: 'center', textAlign: 'left', width: '100%',
            }}
              onMouseOver={(e) => { e.currentTarget.style.background = 'var(--bg-tertiary)'; }}
              onMouseOut={(e) => { e.currentTarget.style.background = 'transparent'; }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
                <span style={{ width: 5, height: 5, borderRadius: '50%', background: c, boxShadow: `0 0 6px ${c}` }} />
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--accent)' }}>{s.code}</span>
              </div>
              <span style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--text-primary)' }}>{s.name}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-secondary)' }}>{s.city}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)', letterSpacing: 1, textTransform: 'uppercase' }}>{s.kind}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-primary)' }}>{s.online}/{s.cameras}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-primary)' }}>{Math.round((s.storage.used / s.storage.total) * 100)}%</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: s.alerts ? 'var(--danger)' : 'var(--text-muted)' }}>{s.alerts || '—'}</span>
              <Icon name="chevron-right" size={14} color="var(--text-muted)" />
            </button>
          );
        })}
      </div>
    </div>
  );
}

interface SiteDetailProps {
  site: Site;
  cameras: Camera[];
  setScope: (s: Scope) => void;
  onNavigate: (r: Route) => void;
}

export function SiteDetail({ site, cameras, setScope, onNavigate }: SiteDetailProps) {
  const siteCams = cameras.filter(c => c.siteId === site.id);
  const c = site.status === 'online' ? 'var(--success)' : site.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
  const rgb = c === 'var(--success)' ? '34,197,94' : c === 'var(--warning)' ? '234,179,8' : '239,68,68';
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'auto', background: 'var(--bg-primary)' }}>
      <PageHeader
        title={site.name}
        subtitle={`${site.code} · ${site.city.toUpperCase()} · ${site.kind.toUpperCase()}`}
        badges={<>
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: 6, padding: '4px 8px',
            background: `rgba(${rgb},0.07)`, border: `1px solid rgba(${rgb},0.27)`, borderRadius: 3,
          }}>
            <span style={{ width: 5, height: 5, borderRadius: '50%', background: c, boxShadow: `0 0 6px ${c}` }} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: c, letterSpacing: 1, textTransform: 'uppercase' }}>{site.status}</span>
          </span>
        </>}
        actions={<>
          <Btn kind="ghost" onClick={() => setScope({ kind: 'all' })}><Icon name="x" size={13} /> Exit Site</Btn>
          <Btn kind="secondary" onClick={() => onNavigate('rules')}>Rules</Btn>
          <Btn kind="secondary" onClick={() => onNavigate('devices')}>Devices</Btn>
          <Btn kind="primary" onClick={() => onNavigate('live')}><Icon name="grid-2x2" size={13} /> Live</Btn>
        </>}
      />
      <div style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5, 1fr)', gap: 12 }}>
          <Stat label="CAMERAS" value={`${site.online}/${site.cameras}`} sub={`${site.recording} recording`} />
          <Stat label="STORAGE" value={`${site.storage.used} GB`} sub={`/ ${site.storage.total} GB · ${Math.round((site.storage.used / site.storage.total) * 100)}%`} />
          <Stat label="UPLINK" value={`${site.bandwidth.toFixed(1)} Mb/s`} sub="up · sustained" />
          <Stat label="ALERTS · 24H" value={site.alerts} accent={site.alerts ? 'var(--danger)' : 'var(--text-primary)'} sub="open" />
          <Stat label="LAST SYNC" value={site.lastSync} sub={site.status === 'offline' ? 'site offline' : 'cloud relay'} accent={site.status === 'offline' ? 'var(--danger)' : 'var(--accent)'} />
        </div>

        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 16 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
            <SectionHeader>CAMERAS · {site.cameras}</SectionHeader>
            <Btn kind="tactical" onClick={() => onNavigate('live')}>Open in Live Wall</Btn>
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: 8 }}>
            {siteCams.map(cam => <CameraTile key={cam.id} camera={cam} />)}
          </div>
        </div>
      </div>
    </div>
  );
}
