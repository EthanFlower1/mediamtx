import { useState } from 'react';
import type { Camera, Role, Rule, Scope, Site, User } from '../types';
import { Btn, Icon, PageHeader, SectionHeader, Segmented, StatusBadge } from '../components/primitives';
import { sitesInScope } from '../components/Shell';

interface DevicesProps {
  scope: Scope;
  sites: Site[];
  cameras: Camera[];
  setScope: (s: Scope) => void;
}

export function Devices({ scope, sites, cameras, setScope }: DevicesProps) {
  const inScope = sitesInScope(sites, scope);
  const camsInScope = cameras.filter(c => inScope.find(s => s.id === c.siteId));
  const [filter, setFilter] = useState('all');
  const filtered = camsInScope.filter(c => filter === 'all' || c.status === filter);

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Devices"
        subtitle={`${camsInScope.length} CAMERAS · ${inScope.length} SITES · ${camsInScope.filter(c => c.status === 'online').length} ONLINE`}
        actions={<>
          <Segmented options={[
            { value: 'all', label: 'ALL' }, { value: 'online', label: 'ONLINE' },
            { value: 'degraded', label: 'DEGRADED' }, { value: 'offline', label: 'OFFLINE' },
          ]} value={filter} onChange={setFilter} />
          <Btn kind="secondary"><Icon name="search" size={13} /> Discover</Btn>
          <Btn kind="primary">+ Add Camera</Btn>
        </>}
      />
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, overflow: 'hidden' }}>
          <div style={{
            display: 'grid', gridTemplateColumns: '90px 70px 1fr 90px 110px 90px 80px 110px 30px', gap: 12, padding: '10px 16px',
            borderBottom: '1px solid var(--border)', fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1.5, color: 'var(--text-muted)', textTransform: 'uppercase',
          }}>
            <span>STATUS</span><span>ID</span><span>NAME / SITE</span><span>RESOLUTION</span><span>CODEC</span><span>FPS</span><span>RETENTION</span><span>CAPABILITIES</span><span></span>
          </div>
          {filtered.slice(0, 80).map((c, i) => {
            const site = sites.find(s => s.id === c.siteId);
            const sc = c.status === 'online' ? 'var(--success)' : c.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
            const rgb = sc === 'var(--success)' ? '34,197,94' : sc === 'var(--warning)' ? '234,179,8' : '239,68,68';
            return (
              <div key={c.id} style={{
                display: 'grid', gridTemplateColumns: '90px 70px 1fr 90px 110px 90px 80px 110px 30px', gap: 12, padding: '10px 16px',
                borderBottom: i < filtered.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'center',
              }}>
                <span style={{
                  display: 'inline-flex', alignItems: 'center', gap: 5, padding: '2px 6px',
                  background: `rgba(${rgb},0.07)`, border: `1px solid rgba(${rgb},0.27)`, borderRadius: 2,
                  fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 0.5, color: sc, textTransform: 'uppercase',
                }}>
                  <span style={{ width: 4, height: 4, borderRadius: '50%', background: sc, boxShadow: `0 0 4px ${sc}` }} />
                  {c.status}
                </span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{c.id}</span>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
                  <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-primary)' }}>{c.name}</span>
                  <button onClick={() => site && setScope({ kind: 'site', id: site.id })} style={{
                    background: 'none', border: 'none', padding: 0, cursor: 'pointer', textAlign: 'left',
                    fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--accent)', letterSpacing: 0.5,
                  }}>{site?.code} · {site?.name.replace(/^[^—]+— /, '')}</button>
                </div>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{c.resolution}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>RTSP/H.264</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{c.fps}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{c.retention}D</span>
                <div style={{ display: 'flex', gap: 3 }}>
                  {c.caps.slice(0, 3).map(cap => (
                    <span key={cap} style={{
                      padding: '1px 5px', fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 0.5,
                      background: 'var(--bg-tertiary)', border: '1px solid var(--border)', color: 'var(--text-secondary)', borderRadius: 2,
                    }}>{cap}</span>
                  ))}
                </div>
                <Icon name="chevron-right" size={14} color="var(--text-muted)" />
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}

interface RulesProps {
  rules: Rule[];
  sites: Site[];
  cameras: Camera[];
}

export function Rules({ rules, sites, cameras }: RulesProps) {
  const [activeId, setActiveId] = useState(rules[1].id);
  const active = rules.find(r => r.id === activeId);

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Recording Rules"
        subtitle={`${rules.filter(r => r.enabled).length} ACTIVE · ${rules.length} TOTAL`}
        actions={<><Btn kind="secondary">Templates</Btn><Btn kind="primary">+ New Rule</Btn></>}
      />
      <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
        <aside style={{ width: 320, background: 'var(--bg-secondary)', borderRight: '1px solid var(--border)', overflow: 'auto' }}>
          {rules.map(r => (
            <button key={r.id} onClick={() => setActiveId(r.id)} style={{
              width: '100%', padding: 14, textAlign: 'left', cursor: 'pointer',
              background: activeId === r.id ? 'var(--bg-tertiary)' : 'transparent',
              border: 'none', borderBottom: '1px solid var(--border)',
              borderLeft: activeId === r.id ? '2px solid var(--accent)' : '2px solid transparent',
            }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--accent)', letterSpacing: 1 }}>{r.id}</span>
                <span style={{
                  width: 32, height: 14, borderRadius: 999, background: 'var(--bg-primary)',
                  border: r.enabled ? '1px solid var(--accent-27)' : '1px solid var(--border)', position: 'relative',
                  boxShadow: r.enabled ? 'var(--glow-accent-sm)' : 'none',
                }}>
                  <span style={{
                    position: 'absolute', top: 1, left: r.enabled ? 18 : 1,
                    width: 10, height: 10, borderRadius: '50%', background: r.enabled ? 'var(--accent)' : 'var(--text-muted)',
                  }} />
                </span>
              </div>
              <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, fontWeight: 500, color: 'var(--text-primary)', marginBottom: 4 }}>{r.name}</div>
              <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, padding: '2px 5px', background: 'var(--bg-primary)', border: '1px solid var(--border)', color: 'var(--text-secondary)', letterSpacing: 0.5 }}>{r.trigger.toUpperCase()}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, padding: '2px 5px', background: 'var(--bg-primary)', border: '1px solid var(--border)', color: 'var(--text-secondary)', letterSpacing: 0.5 }}>{r.cameras} CAMS</span>
              </div>
            </button>
          ))}
        </aside>

        <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
          {active && <RuleEditor rule={active} sites={sites} cameras={cameras} />}
        </div>
      </div>
    </div>
  );
}

function RuleEditor({ rule, sites }: { rule: Rule; sites: Site[]; cameras: Camera[] }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 18, maxWidth: 1100 }}>
      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)', letterSpacing: 1 }}>{rule.id}</span>
          {rule.enabled ? <StatusBadge kind="online" label="ACTIVE" /> : <StatusBadge kind="offline" label="DISABLED" />}
        </div>
        <h2 style={{ fontFamily: 'var(--font-sans)', fontSize: 22, fontWeight: 600, color: 'var(--text-primary)', margin: 0 }}>{rule.name}</h2>
      </div>

      <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 20 }}>
        <SectionHeader>RULE CHAIN</SectionHeader>
        <div style={{ marginTop: 16, display: 'flex', alignItems: 'stretch', gap: 0, flexWrap: 'wrap' }}>
          <ChainCard kind="WHEN" title="TRIGGER" lines={[rule.trigger.toUpperCase(), rule.triggerDetail]} icon="zap" />
          <ChainArrow />
          {rule.conditions.length > 0 && (
            <>
              <ChainCard kind="IF" title="CONDITION" lines={rule.conditions.map(c => c.detail)} icon="filter" />
              <ChainArrow />
            </>
          )}
          <ChainCard kind="THEN" title="ACTIONS" lines={rule.actions.map(a => `${a.kind.toUpperCase()} · ${a.detail}`)} icon="play-circle" multi />
        </div>
      </div>

      <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 20 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <SectionHeader>SCOPE · {rule.cameras} CAMERAS AFFECTED</SectionHeader>
          <Btn kind="tactical">Edit Scope</Btn>
        </div>
        <div style={{ marginTop: 14, display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 6 }}>
          {sites.map(s => {
            const inRule = rule.scope.sites === 'all' || rule.scope.sites === s.kind || (Array.isArray(rule.scope.sites) && rule.scope.sites.includes(s.id));
            return (
              <div key={s.id} style={{
                padding: 10, background: inRule ? 'var(--accent-13)' : 'var(--bg-tertiary)',
                border: inRule ? '1px solid var(--accent-27)' : '1px solid var(--border)',
                borderRadius: 4,
              }}>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: inRule ? 'var(--accent)' : 'var(--text-muted)', letterSpacing: 1 }}>{s.code}</div>
                <div style={{ fontFamily: 'var(--font-sans)', fontSize: 10, color: 'var(--text-primary)', marginTop: 4, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.name.replace(/^[^—]+— /, '')}</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', marginTop: 2 }}>{inRule ? `${s.cameras} cams` : '—'}</div>
              </div>
            );
          })}
        </div>
      </div>

      <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 20 }}>
        <SectionHeader>RECENT FIRINGS · LAST 24H</SectionHeader>
        <div style={{ marginTop: 12, display: 'flex', alignItems: 'flex-end', height: 80, gap: 2 }}>
          {Array.from({ length: 48 }).map((_, i) => {
            const h = 10 + Math.abs(Math.sin(i * 0.7) * 60) + (i % 7) * 4;
            return <div key={i} style={{ flex: 1, height: h, background: i > 40 ? 'var(--accent)' : 'var(--accent-40)', borderRadius: '1px 1px 0 0' }} />;
          })}
        </div>
        <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6, fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 1 }}>
          <span>24H AGO</span><span>12H AGO</span><span>NOW</span>
        </div>
      </div>
    </div>
  );
}

function ChainCard({ kind, title, lines, icon, multi }: { kind: string; title: string; lines: string[]; icon: string; multi?: boolean }) {
  return (
    <div style={{
      flex: multi ? 2 : 1, minWidth: 220, padding: 14, background: 'var(--bg-tertiary)',
      border: '1px solid var(--accent-27)', borderRadius: 4, position: 'relative',
    }}>
      <div style={{
        position: 'absolute', top: -10, left: 12, padding: '1px 6px', background: 'var(--accent)', color: '#0A0A0A',
        fontFamily: 'var(--font-mono)', fontSize: 9, fontWeight: 600, letterSpacing: 1,
      }}>{kind}</div>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 8 }}>
        <Icon name={icon} size={14} color="var(--accent)" />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1.5, color: 'var(--text-secondary)' }}>{title}</span>
      </div>
      {lines.map((l, i) => (
        <div key={i} style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-primary)', padding: '3px 0', borderTop: i ? '1px dashed var(--border)' : 'none' }}>{l}</div>
      ))}
    </div>
  );
}

function ChainArrow() {
  return (
    <div style={{ display: 'flex', alignItems: 'center', padding: '0 8px', color: 'var(--accent)' }}>
      <Icon name="arrow-right" size={16} />
    </div>
  );
}

interface UsersProps {
  users: User[];
  roles: Role[];
  sites: Site[];
}

export function Users({ users, roles }: UsersProps) {
  const [tab, setTab] = useState('users');
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Users & Roles"
        subtitle={`${users.length} MEMBERS · ${roles.length} ROLES`}
        actions={<>
          <Segmented options={[{ value: 'users', label: 'USERS' }, { value: 'roles', label: 'ROLES' }]} value={tab} onChange={setTab} />
          <Btn kind="primary"><Icon name="user-plus" size={13} /> Invite</Btn>
        </>}
      />
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        {tab === 'users' && (
          <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, overflow: 'hidden' }}>
            <div style={{
              display: 'grid', gridTemplateColumns: '40px 1fr 200px 120px 90px 110px 100px 30px', gap: 12, padding: '10px 16px',
              borderBottom: '1px solid var(--border)', fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1.5, color: 'var(--text-muted)', textTransform: 'uppercase',
            }}>
              <span></span><span>USER</span><span>ROLE</span><span>SITES</span><span>MFA</span><span>LAST SEEN</span><span>STATUS</span><span></span>
            </div>
            {users.map((u, i) => (
              <div key={u.id} style={{
                display: 'grid', gridTemplateColumns: '40px 1fr 200px 120px 90px 110px 100px 30px', gap: 12, padding: '12px 16px',
                borderBottom: i < users.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'center',
              }}>
                <div style={{
                  width: 30, height: 30, borderRadius: '50%', background: 'var(--bg-tertiary)', border: '1px solid var(--border)',
                  display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)',
                }}>{u.avatar}</div>
                <div>
                  <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--text-primary)', fontWeight: 500 }}>{u.name}</div>
                  <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{u.email}</div>
                </div>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--accent)', letterSpacing: 0.5, textTransform: 'uppercase' }}>{u.role}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{u.sites === 'all' ? 'ALL · 14' : `${u.sites.length} SITES`}</span>
                {u.mfa ? <StatusBadge kind="online" label="ON" /> : <StatusBadge kind="offline" label="OFF" />}
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{u.lastSeen}</span>
                {u.status === 'invited' ? <StatusBadge kind="degraded" label="INVITED" /> : <StatusBadge kind="online" label="ACTIVE" />}
                <Icon name="more-vertical" size={14} color="var(--text-muted)" />
              </div>
            ))}
          </div>
        )}
        {tab === 'roles' && (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 12 }}>
            {roles.map(r => (
              <div key={r.id} style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 18 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                  <div style={{ fontFamily: 'var(--font-sans)', fontSize: 16, fontWeight: 600, color: 'var(--text-primary)' }}>{r.name}</div>
                  <span style={{
                    fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)', padding: '2px 6px',
                    background: 'var(--accent-13)', border: '1px solid var(--accent-27)', borderRadius: 2,
                  }}>{r.count} MEMBERS</span>
                </div>
                <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.5, margin: '0 0 14px' }}>{r.description}</p>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1, color: 'var(--text-muted)', marginBottom: 6 }}>PERMISSIONS</div>
                <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                  {r.perms.map(p => (
                    <span key={p} style={{
                      fontFamily: 'var(--font-mono)', fontSize: 10, padding: '2px 6px',
                      background: 'var(--bg-tertiary)', border: '1px solid var(--border)',
                      color: 'var(--text-secondary)', borderRadius: 2, letterSpacing: 0.5,
                    }}>{p}</span>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
