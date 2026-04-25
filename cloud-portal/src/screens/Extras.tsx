import { useState } from 'react';
import type { AlertItem, AuditEntry, Camera, Scope, Site } from '../types';
import { Btn, Icon, PageHeader, SectionHeader, Segmented, Stat, StatusBadge } from '../components/primitives';
import { CameraTile } from '../components/CameraTile';
import { sitesInScope } from '../components/Shell';

export function RemoteAccess() {
  const tokens = [
    { id: 'TKN-04A2', label: 'Halverson family share', site: 'HSE-21', siteName: 'Halverson Residence', issued: '2026-04-18', expires: '2026-05-25', uses: 12, scope: 'live + playback (7 day)', issuedTo: 'eli@tomasi.family' },
    { id: 'TKN-018F', label: 'On-call ops bridge', site: 'all', siteName: 'All sites', issued: '2026-04-22', expires: '2026-04-29', uses: 4, scope: 'live only · 4 cams', issuedTo: 'on-call@northwall.security' },
    { id: 'TKN-0093', label: 'Insurance adjuster · PCK-01', site: 'PCK-01', siteName: 'Packwell Distribution', issued: '2026-04-21', expires: '2026-04-26', uses: 1, scope: 'playback only · 14:00–18:00 window', issuedTo: 'adjuster@statefarm.example' },
    { id: 'TKN-1124', label: 'Atlas Steel client portal', site: 'ATL-02', siteName: 'Atlas Steel — Yard 2', issued: '2026-03-12', expires: '2026-09-12', uses: 187, scope: 'live + playback (recurring)', issuedTo: 'kai@atlassteel.com' },
  ];
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Remote Access"
        subtitle="TIME-LIMITED TOKENS · CLIENT PORTAL · RELAY CONFIG"
        actions={<><Btn kind="secondary">Relay Settings</Btn><Btn kind="primary">+ Issue Token</Btn></>}
      />
      <div style={{ flex: 1, overflow: 'auto', padding: 24, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12 }}>
          <Stat label="ACTIVE TOKENS" value={tokens.length} sub="3 expiring this week" />
          <Stat label="RELAY STATUS" value="ONLINE" accent="var(--success)" sub="us-west · 18ms p50" />
          <Stat label="SESSIONS · 24H" value="247" sub="from 18 unique users" />
          <Stat label="DATA RELAYED" value="42.8 GB" sub="ingress + egress" />
        </div>

        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, overflow: 'hidden' }}>
          <div style={{ padding: '12px 16px', borderBottom: '1px solid var(--border)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <SectionHeader>ISSUED TOKENS</SectionHeader>
          </div>
          <div style={{
            display: 'grid', gridTemplateColumns: '90px 1fr 110px 110px 110px 70px 30px', gap: 12, padding: '8px 16px',
            borderBottom: '1px solid var(--border)', fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1.5, color: 'var(--text-muted)', textTransform: 'uppercase',
          }}>
            <span>TOKEN</span><span>LABEL / SCOPE</span><span>SITE</span><span>ISSUED</span><span>EXPIRES</span><span>USES</span><span></span>
          </div>
          {tokens.map((t, i) => (
            <div key={t.id} style={{
              display: 'grid', gridTemplateColumns: '90px 1fr 110px 110px 110px 70px 30px', gap: 12, padding: '12px 16px',
              borderBottom: i < tokens.length - 1 ? '1px solid var(--border)' : 'none', alignItems: 'center',
            }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{t.id}</span>
              <div>
                <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--text-primary)' }}>{t.label}</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{t.scope} · {t.issuedTo}</div>
              </div>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{t.site === 'all' ? 'ALL' : t.site}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{t.issued}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{t.expires}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{t.uses}</span>
              <Icon name="more-vertical" size={14} color="var(--text-muted)" />
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export function Alerts({ alerts }: { alerts: AlertItem[] }) {
  const [filter, setFilter] = useState('open');
  const filtered = alerts.filter(a => filter === 'all' || (filter === 'open' && !a.ack) || (filter === 'ack' && a.ack));
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Alerts"
        subtitle={`${alerts.filter(a => !a.ack).length} OPEN · ${alerts.filter(a => a.severity === 'danger' && !a.ack).length} CRITICAL`}
        actions={<>
          <Segmented options={[{ value: 'open', label: 'OPEN' }, { value: 'ack', label: 'ACKED' }, { value: 'all', label: 'ALL' }]} value={filter} onChange={setFilter} />
          <Btn kind="secondary">Acknowledge All</Btn>
        </>}
      />
      <div style={{ flex: 1, overflow: 'auto', padding: 24, display: 'flex', flexDirection: 'column', gap: 8 }}>
        {filtered.map(a => {
          const sev = a.severity === 'danger' ? 'var(--danger)' : a.severity === 'warning' ? 'var(--warning)' : 'var(--accent)';
          return (
            <div key={a.id} style={{
              display: 'grid', gridTemplateColumns: '6px 110px 70px 1fr 110px 110px 100px',
              gap: 14, alignItems: 'center', padding: 14, background: 'var(--bg-secondary)', border: `1px solid var(--border)`,
              borderLeft: `2px solid ${sev}`, borderRadius: 4,
            }}>
              <span style={{ width: 6, height: 6, borderRadius: '50%', background: sev, boxShadow: a.ack ? 'none' : `0 0 6px ${sev}` }} />
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{a.id}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{a.site}</span>
              <div>
                <div style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--text-primary)' }}>{a.text}</div>
                <div style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>{a.siteName} · {a.cam} · {a.camName}</div>
              </div>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)', textTransform: 'uppercase', letterSpacing: 1 }}>{a.kind}</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-primary)' }}>{a.time}</span>
              {a.ack ? <StatusBadge kind="online" label="ACKED" /> : <Btn kind="tactical">Acknowledge</Btn>}
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function Audit({ audit }: { audit: AuditEntry[] }) {
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Audit Log"
        subtitle="LAST 24 HOURS · 247 EVENTS"
        actions={<><Btn kind="secondary">Filter</Btn><Btn kind="secondary"><Icon name="download" size={13} /> Export CSV</Btn></>}
      />
      <div style={{ flex: 1, overflow: 'auto', padding: 24 }}>
        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, fontFamily: 'var(--font-mono)' }}>
          <div style={{
            display: 'grid', gridTemplateColumns: '100px 130px 150px 1fr 1fr 90px', gap: 12, padding: '10px 16px',
            borderBottom: '1px solid var(--border)', fontSize: 9, letterSpacing: 1.5, color: 'var(--text-muted)', textTransform: 'uppercase',
          }}>
            <span>TIME</span><span>USER</span><span>ACTION</span><span>TARGET</span><span>DETAIL</span><span>SITE</span>
          </div>
          {audit.map((e, i) => (
            <div key={i} style={{
              display: 'grid', gridTemplateColumns: '100px 130px 150px 1fr 1fr 90px', gap: 12, padding: '10px 16px',
              borderBottom: i < audit.length - 1 ? '1px solid var(--border)' : 'none', fontSize: 11, alignItems: 'center',
            }}>
              <span style={{ color: 'var(--text-secondary)' }}>{e.time}</span>
              <span style={{ color: e.user === 'system' ? 'var(--text-muted)' : 'var(--text-primary)' }}>{e.user}</span>
              <span style={{ color: 'var(--accent)', letterSpacing: 0.5 }}>{e.action}</span>
              <span style={{ color: 'var(--text-primary)' }}>{e.target}</span>
              <span style={{ color: 'var(--text-secondary)' }}>{e.detail}</span>
              <span style={{ color: 'var(--text-muted)', textTransform: 'uppercase' }}>{e.site}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

export function Billing() {
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'auto', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Billing & Subscription"
        subtitle="INTEGRATOR PLAN · 270 / 500 CAMERAS"
        actions={<><Btn kind="secondary">Invoices</Btn><Btn kind="primary">Manage Plan</Btn></>}
      />
      <div style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 12 }}>
          <Stat label="PLAN" value="INTEGRATOR" accent="var(--accent)" glow sub="annual · paid in full" />
          <Stat label="LICENSE USAGE" value="270 / 500" sub="cameras · 54%" />
          <Stat label="CLOUD STORAGE" value="3.91 TB" sub="of 8 TB included" />
          <Stat label="NEXT INVOICE" value="$2,140.00" sub="due 2026-05-12" />
        </div>

        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 18 }}>
          <SectionHeader>USAGE · CURRENT PERIOD</SectionHeader>
          {([
            { label: 'Camera licenses', used: 270, total: 500, unit: '' },
            { label: 'Cloud storage', used: 3.91, total: 8, unit: ' TB' },
            { label: 'AI detection minutes', used: 184200, total: 250000, unit: ' min' },
            { label: 'Remote access bandwidth', used: 412, total: 1000, unit: ' GB' },
          ]).map(u => {
            const pct = (u.used / u.total) * 100;
            return (
              <div key={u.label} style={{ marginTop: 14 }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 6 }}>
                  <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-primary)' }}>{u.label}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--accent)' }}>{u.used.toLocaleString()}{u.unit} / {u.total.toLocaleString()}{u.unit}</span>
                </div>
                <div style={{ position: 'relative', height: 8, background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 1 }}>
                  <div style={{
                    position: 'absolute', inset: 0, width: pct + '%',
                    background: pct > 80 ? 'var(--warning)' : 'linear-gradient(90deg, var(--accent), var(--accent-40))', borderRadius: 1,
                  }} />
                </div>
              </div>
            );
          })}
        </div>

        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 18 }}>
          <SectionHeader>RECENT INVOICES</SectionHeader>
          <div style={{ marginTop: 12 }}>
            {[
              { id: 'INV-2026-04', date: '2026-04-12', amount: '$2,140.00' },
              { id: 'INV-2026-03', date: '2026-03-12', amount: '$2,140.00' },
              { id: 'INV-2026-02', date: '2026-02-12', amount: '$1,980.00' },
              { id: 'INV-2026-01', date: '2026-01-12', amount: '$1,980.00' },
            ].map((inv, i) => (
              <div key={inv.id} style={{
                display: 'grid', gridTemplateColumns: '160px 130px 1fr 100px 30px', gap: 12, padding: '10px 4px',
                borderTop: i ? '1px solid var(--border)' : 'none', alignItems: 'center',
              }}>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--accent)' }}>{inv.id}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-secondary)' }}>{inv.date}</span>
                <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--text-primary)' }}>{inv.amount}</span>
                <StatusBadge kind="online" label="PAID" />
                <Icon name="download" size={14} color="var(--text-muted)" style={{ cursor: 'pointer' }} />
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <label style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-secondary)', textTransform: 'uppercase' }}>{label}</label>
      <input defaultValue={value} style={{
        height: 36, padding: '0 10px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4,
        color: 'var(--text-primary)', fontFamily: mono ? 'var(--font-mono)' : 'var(--font-sans)', fontSize: 13, outline: 'none',
      }} />
    </div>
  );
}

function Step1() {
  return (
    <div>
      <h2 style={{ fontFamily: 'var(--font-sans)', fontSize: 18, fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 4px' }}>Tell us about the site</h2>
      <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-secondary)', margin: '0 0 20px', lineHeight: 1.5 }}>This information identifies the site across the portal and in audit logs.</p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
        <Field label="Site name" value="Halverson Residence" />
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 14 }}>
          <Field label="Site code" value="HSE-22" mono />
          <Field label="Kind" value="Residential" />
        </div>
        <Field label="Client / owner" value="Halverson" />
        <Field label="City" value="Mercer Island, WA" />
      </div>
    </div>
  );
}

function Step2() {
  return (
    <div>
      <h2 style={{ fontFamily: 'var(--font-sans)', fontSize: 18, fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 4px' }}>Claim the NVR</h2>
      <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-secondary)', margin: '0 0 20px', lineHeight: 1.5 }}>Enter the 6-digit pairing code shown on the NVR's local display, or scan its QR.</p>
      <div style={{ background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 4, padding: 24, display: 'flex', justifyContent: 'center', gap: 6 }}>
        {['8', '3', 'K', 'M', '2', '9'].map((d, i) => (
          <div key={i} style={{
            width: 44, height: 56, background: 'var(--bg-primary)', border: '1px solid var(--accent-27)', borderRadius: 4,
            display: 'flex', alignItems: 'center', justifyContent: 'center', fontFamily: 'var(--font-mono)', fontSize: 24, color: 'var(--accent)',
            textShadow: '0 0 12px rgba(249,115,22,0.5)',
          }}>{d}</div>
        ))}
      </div>
      <div style={{
        marginTop: 16, padding: 12, background: 'var(--bg-tertiary)', border: '1px solid var(--success-27)', borderRadius: 4,
        display: 'flex', alignItems: 'center', gap: 10,
      }}>
        <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--success)', boxShadow: 'var(--glow-success)' }} />
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--success)', letterSpacing: 0.5 }}>NVR-7K2 · MEDIAMTX 2.4.1 · PAIRED</span>
      </div>
    </div>
  );
}

function Step3() {
  return (
    <div>
      <h2 style={{ fontFamily: 'var(--font-sans)', fontSize: 18, fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 4px' }}>Discover cameras</h2>
      <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-secondary)', margin: '0 0 16px', lineHeight: 1.5 }}>ONVIF scan in progress on the site LAN.</p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        {['Front Door · Hikvision DS-2CD2143G2', 'Driveway · Axis P3245', 'Backyard · Reolink RLC-820A', 'Side Gate · (no response)'].map((c, i) => (
          <div key={i} style={{
            display: 'flex', alignItems: 'center', gap: 10, padding: 10, background: 'var(--bg-tertiary)',
            border: '1px solid var(--border)', borderRadius: 4,
          }}>
            <span style={{ width: 6, height: 6, borderRadius: '50%', background: i === 3 ? 'var(--text-muted)' : 'var(--success)', boxShadow: i === 3 ? 'none' : 'var(--glow-success)' }} />
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--accent)' }}>192.168.1.{20 + i}</span>
            <span style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-primary)' }}>{c}</span>
            {i !== 3 && <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--success)' }}>ADDED</span>}
          </div>
        ))}
      </div>
    </div>
  );
}

function Step4() {
  return (
    <div>
      <h2 style={{ fontFamily: 'var(--font-sans)', fontSize: 18, fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 4px' }}>Apply recording rules</h2>
      <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-secondary)', margin: '0 0 16px', lineHeight: 1.5 }}>Pick a template or skip and configure later.</p>
      {['Residential · 24/7 + motion alerts', 'Residential · After-hours only', 'Custom · Build from scratch'].map((t, i) => (
        <div key={t} style={{
          display: 'flex', alignItems: 'center', gap: 12, padding: 14, marginBottom: 8,
          background: i === 0 ? 'var(--accent-13)' : 'var(--bg-tertiary)',
          border: i === 0 ? '1px solid var(--accent-27)' : '1px solid var(--border)', borderRadius: 4,
        }}>
          <div style={{
            width: 14, height: 14, borderRadius: '50%',
            border: i === 0 ? '4px solid var(--accent)' : '1px solid var(--text-muted)', background: 'var(--bg-primary)',
          }} />
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 13, color: 'var(--text-primary)' }}>{t}</span>
        </div>
      ))}
    </div>
  );
}

function Step5() {
  return (
    <div>
      <h2 style={{ fontFamily: 'var(--font-sans)', fontSize: 18, fontWeight: 600, color: 'var(--text-primary)', margin: '0 0 4px' }}>Invite team members</h2>
      <p style={{ fontFamily: 'var(--font-sans)', fontSize: 12, color: 'var(--text-secondary)', margin: '0 0 16px', lineHeight: 1.5 }}>Add people who should have access to this site. You can change this later.</p>
      <Field label="Email" value="eli@halverson.family" />
      <div style={{ marginTop: 14 }}><Field label="Role" value="Viewer" /></div>
      <Btn kind="tactical" style={{ marginTop: 14 }}>+ Add another</Btn>
    </div>
  );
}

export function Onboarding() {
  const [step, setStep] = useState(1);
  const steps = [
    { n: 1, label: 'Site Details' },
    { n: 2, label: 'Claim NVR' },
    { n: 3, label: 'Discover Cameras' },
    { n: 4, label: 'Recording Rules' },
    { n: 5, label: 'Invite Team' },
  ];
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'auto', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Add a New Site"
        subtitle="STEP-BY-STEP · 5 STAGES"
        actions={<>
          <Btn kind="ghost">Cancel</Btn>
          <Btn kind="primary" onClick={() => setStep(Math.min(5, step + 1))}>Continue →</Btn>
        </>}
      />
      <div style={{ padding: '0 24px', borderBottom: '1px solid var(--border)' }}>
        <div style={{ display: 'flex', gap: 0, padding: '16px 0' }}>
          {steps.map((s, i) => (
            <div key={s.n} style={{ display: 'flex', alignItems: 'center', flex: 1 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 10, opacity: step >= s.n ? 1 : 0.4 }}>
                <div style={{
                  width: 28, height: 28, borderRadius: 4,
                  background: step === s.n ? 'var(--accent)' : step > s.n ? 'var(--accent-13)' : 'var(--bg-tertiary)',
                  border: `1px solid ${step >= s.n ? 'var(--accent-27)' : 'var(--border)'}`,
                  display: 'flex', alignItems: 'center', justifyContent: 'center',
                  color: step === s.n ? '#0A0A0A' : 'var(--accent)',
                  fontFamily: 'var(--font-mono)', fontSize: 11, fontWeight: 600,
                }}>
                  {step > s.n ? '✓' : String(s.n).padStart(2, '0')}
                </div>
                <span style={{
                  fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 1,
                  color: step >= s.n ? 'var(--text-primary)' : 'var(--text-muted)', textTransform: 'uppercase',
                }}>{s.label}</span>
              </div>
              {i < steps.length - 1 && (
                <div style={{ flex: 1, height: 1, background: step > s.n ? 'var(--accent-27)' : 'var(--border)', margin: '0 16px' }} />
              )}
            </div>
          ))}
        </div>
      </div>
      <div style={{ padding: 24, display: 'flex', justifyContent: 'center' }}>
        <div style={{ width: 600, background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 24 }}>
          {step === 1 && <Step1 />}
          {step === 2 && <Step2 />}
          {step === 3 && <Step3 />}
          {step === 4 && <Step4 />}
          {step === 5 && <Step5 />}
        </div>
      </div>
    </div>
  );
}

interface SearchProps {
  scope: Scope;
  sites: Site[];
  cameras: Camera[];
}

export function Search({ scope, sites, cameras }: SearchProps) {
  const inScope = sitesInScope(sites, scope);
  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader title="Search" subtitle="AI DETECTIONS · MOTION · ACROSS ALL SITES" />
      <div style={{ padding: 24, display: 'flex', flexDirection: 'column', gap: 14, overflow: 'auto', flex: 1 }}>
        <div style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6, padding: 14, display: 'flex', gap: 10, alignItems: 'center', flexWrap: 'wrap' }}>
          <Icon name="search-code" size={14} color="var(--accent)" />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '4px 8px', background: 'var(--accent-13)', border: '1px solid var(--accent-27)', color: 'var(--accent)', letterSpacing: 0.5, borderRadius: 2 }}>CLASS · PERSON</span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '4px 8px', background: 'var(--accent-13)', border: '1px solid var(--accent-27)', color: 'var(--accent)', letterSpacing: 0.5, borderRadius: 2 }}>WHEN · LAST 6H</span>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, padding: '4px 8px', background: 'var(--bg-tertiary)', border: '1px solid var(--border)', color: 'var(--text-secondary)', letterSpacing: 0.5, borderRadius: 2 }}>+ ADD FILTER</span>
          <span style={{ marginLeft: 'auto', fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-muted)' }}>247 RESULTS · {inScope.length} SITES</span>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px,1fr))', gap: 8 }}>
          {cameras.slice(0, 18).map(c => {
            const site = sites.find(s => s.id === c.siteId);
            return (
              <div key={c.id} style={{ background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 4, overflow: 'hidden' }}>
                <div style={{ position: 'relative' }}>
                  <CameraTile camera={{ ...c, detection: 'PERSON' }} />
                </div>
                <div style={{ padding: '8px 10px', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{site?.code} · {c.id}</span>
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>14:{String(31 - (parseInt(c.id.slice(-2)) % 30)).padStart(2, '0')}:0{(parseInt(c.id.slice(-2)) % 9)}</span>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
