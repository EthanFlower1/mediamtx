import { useState } from 'react';
import type { Camera, Scope, Site } from '../types';
import { Btn, Icon, PageHeader, Segmented } from '../components/primitives';
import { CameraTile } from '../components/CameraTile';
import { sitesInScope } from '../components/Shell';

interface PlaybackProps {
  scope: Scope;
  sites: Site[];
  cameras: Camera[];
  setScope: (s: Scope) => void;
}

function TBtn({ icon }: { icon: string }) {
  return (
    <button style={{
      width: 28, height: 28, borderRadius: 4, background: 'var(--bg-tertiary)',
      border: '1px solid var(--border)', cursor: 'pointer', color: 'var(--text-primary)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }}>
      <Icon name={icon} size={13} />
    </button>
  );
}

export function Playback({ scope, sites, cameras }: PlaybackProps) {
  const inScope = sitesInScope(sites, scope);
  const camsInScope = cameras.filter(c => inScope.find(s => s.id === c.siteId));
  const [activeCam, setActiveCam] = useState(camsInScope[0]?.id ?? cameras[0].id);
  const [zoom, setZoom] = useState('1H');
  const [playing, setPlaying] = useState(true);
  const [seekPct, setSeekPct] = useState(50);

  const cam = cameras.find(c => c.id === activeCam) ?? cameras[0];
  const site = sites.find(s => s.id === cam.siteId);

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Playback"
        subtitle={`${site?.code} · ${cam.id} · ${cam.name.toUpperCase()}`}
        actions={<>
          <Btn kind="secondary"><Icon name="download" size={13} /> Export Clip</Btn>
          <Btn kind="secondary"><Icon name="bookmark" size={13} /> Bookmark</Btn>
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-muted)', textTransform: 'uppercase' }}>ZOOM</span>
          <Segmented options={['24H', '6H', '1H', '30M', '10M']} value={zoom} onChange={setZoom} />
        </>}
      />
      <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
        <aside style={{ width: 240, background: 'var(--bg-secondary)', borderRight: '1px solid var(--border)', overflow: 'auto' }}>
          {inScope.map(s => (
            <div key={s.id}>
              <div style={{
                padding: '8px 12px', background: 'var(--bg-primary)', borderBottom: '1px solid var(--border)',
                fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)', letterSpacing: 0.5,
              }}>{s.code} · {s.name.replace(/^[^—]+— /, '')}</div>
              {cameras.filter(c => c.siteId === s.id).map(c => (
                <button key={c.id} onClick={() => setActiveCam(c.id)} style={{
                  width: '100%', padding: '8px 12px', display: 'flex', alignItems: 'center', gap: 8,
                  background: activeCam === c.id ? 'var(--accent-13)' : 'transparent',
                  border: 'none', borderBottom: '1px solid var(--border)',
                  borderLeft: activeCam === c.id ? '2px solid var(--accent)' : '2px solid transparent',
                  color: activeCam === c.id ? 'var(--accent)' : 'var(--text-primary)',
                  cursor: 'pointer', textAlign: 'left',
                }}>
                  <span style={{
                    width: 5, height: 5, borderRadius: '50%',
                    background: c.status === 'online' ? 'var(--success)' : c.status === 'degraded' ? 'var(--warning)' : 'var(--danger)',
                  }} />
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10 }}>{c.id}</span>
                  <span style={{ fontFamily: 'var(--font-sans)', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</span>
                </button>
              ))}
            </div>
          ))}
        </aside>

        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          <div style={{ flex: 1, padding: 16, minHeight: 0 }}>
            <CameraTile camera={{ ...cam, recording: true, detection: 'PERSON' }} large />
          </div>

          <div style={{
            borderTop: '1px solid var(--border)', padding: '12px 20px',
            display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 16, background: 'var(--bg-secondary)',
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <TBtn icon="skip-back" /><TBtn icon="chevron-left" />
              <button onClick={() => setPlaying(!playing)} style={{
                width: 36, height: 36, borderRadius: '50%', background: 'var(--accent)', border: 'none',
                display: 'flex', alignItems: 'center', justifyContent: 'center', cursor: 'pointer', color: '#0A0A0A',
              }}><Icon name={playing ? 'pause' : 'play'} size={16} /></button>
              <TBtn icon="chevron-right" /><TBtn icon="skip-forward" />
            </div>
            <div style={{ display: 'flex', gap: 12, alignItems: 'center' }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-muted)' }}>NOW</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 18, color: 'var(--accent)', fontVariantNumeric: 'tabular-nums' }}>14:32:07</span>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>· 2026-04-25</span>
            </div>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-muted)' }}>SPEED</span>
              <Segmented options={['0.5×', '1×', '2×', '4×']} value={'1×'} onChange={() => {}} />
            </div>
          </div>

          <div style={{ height: 100, padding: '10px 20px', borderTop: '1px solid var(--border)', background: 'var(--bg-secondary)', position: 'relative' }}>
            <div
              style={{ position: 'relative', height: 56, border: '1px solid var(--border)', background: 'var(--bg-primary)', overflow: 'hidden' }}
              onClick={(e) => {
                const r = e.currentTarget.getBoundingClientRect();
                setSeekPct(((e.clientX - r.left) / r.width) * 100);
              }}>
              <div style={{ position: 'absolute', inset: 0, display: 'flex', paddingTop: 4 }}>
                {['14:00', '14:10', '14:20', '14:30', '14:40', '14:50', '15:00'].map(t => (
                  <div key={t} style={{ flex: 1, position: 'relative' }}>
                    <span style={{ position: 'absolute', top: 0, left: 4, fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>{t}</span>
                    <div style={{ position: 'absolute', top: 14, left: 0, width: 1, height: 6, background: 'var(--border)' }} />
                  </div>
                ))}
              </div>
              <div style={{ position: 'absolute', left: 0, right: 0, top: 24, height: 14, background: 'var(--accent-20)' }} />
              <div style={{ position: 'absolute', left: '62%', top: 24, width: '6%', height: 14, backgroundImage: 'repeating-linear-gradient(45deg, rgba(64,64,64,0.5) 0 4px, transparent 4px 8px)' }} />
              {[18, 36, 52, 68].map((l, i) => (
                <div key={i} style={{ position: 'absolute', left: l + '%', top: 42, width: '3%', height: 10, background: 'rgba(239,68,68,0.65)' }} />
              ))}
              <div style={{ position: 'absolute', left: seekPct + '%', top: 0, bottom: 0, width: 2, background: 'var(--accent)', boxShadow: '0 0 12px rgba(249,115,22,0.7)' }}>
                <div style={{
                  position: 'absolute', left: '50%', top: -3, transform: 'translateX(-50%)',
                  padding: '1px 5px', background: 'var(--accent)', color: '#0A0A0A',
                  fontFamily: 'var(--font-mono)', fontSize: 9, fontWeight: 600, whiteSpace: 'nowrap',
                }}>14:32:07</div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
