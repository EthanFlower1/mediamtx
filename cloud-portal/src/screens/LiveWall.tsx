import { useEffect, useState } from 'react';
import type { Camera, SavedView, Scope, Site } from '../types';
import { Btn, Icon, PageHeader, SectionHeader, Segmented } from '../components/primitives';
import { CameraTile } from '../components/CameraTile';
import { scopeLabel, sitesInScope } from '../components/Shell';

interface LiveWallProps {
  scope: Scope;
  sites: Site[];
  cameras: Camera[];
  savedViews: SavedView[];
  setScope: (s: Scope) => void;
}

export function LiveWall({ scope, sites, cameras, savedViews }: LiveWallProps) {
  const [grid, setGrid] = useState('3x3');
  const [activeView, setActiveView] = useState<string | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(true);
  const [picks, setPicks] = useState<Array<string | null>>([]);
  const [dragOver, setDragOver] = useState<number | null>(null);

  const inScope = sitesInScope(sites, scope);
  const camsInScope = cameras.filter(c => inScope.find(s => s.id === c.siteId));

  useEffect(() => {
    if (camsInScope.length > 0) {
      setPicks(camsInScope.slice(0, 9).map(c => c.id));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scope.kind, scope.kind === 'site' ? scope.id : '', scope.kind === 'group' ? scope.ids.join('-') : '']);

  const cols = parseInt(grid.split('x')[0]);
  const slots = cols * cols;
  const tileCams = picks.slice(0, slots).map(id => id ? cameras.find(c => c.id === id) : null);

  const onDragStart = (e: React.DragEvent, camId: string) => { e.dataTransfer.setData('cam', camId); };

  const onDropSlot = (e: React.DragEvent, slotIdx: number) => {
    e.preventDefault();
    const camId = e.dataTransfer.getData('cam');
    const next = [...picks];
    while (next.length < slotIdx + 1) next.push(null);
    const fromIdx = next.indexOf(camId);
    if (fromIdx >= 0) {
      next[fromIdx] = next[slotIdx] ?? null;
      next[slotIdx] = camId;
    } else {
      next[slotIdx] = camId;
    }
    setPicks(next.slice(0, slots));
    setDragOver(null);
  };

  const onRemove = (idx: number) => {
    const next = [...picks];
    next.splice(idx, 1);
    setPicks(next);
  };

  return (
    <div style={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden', background: 'var(--bg-primary)' }}>
      <PageHeader
        title="Live Wall"
        subtitle={`${scopeLabel(scope, sites).toUpperCase()} · ${tileCams.filter(Boolean).length} CAMERAS`}
        actions={<>
          <div style={{ display: 'flex', gap: 4 }}>
            {savedViews.map(v => (
              <button key={v.id} onClick={() => setActiveView(v.id)} style={{
                height: 32, padding: '0 10px',
                background: activeView === v.id ? 'var(--accent-13)' : 'var(--bg-tertiary)',
                border: `1px solid ${activeView === v.id ? 'var(--accent-27)' : 'var(--border)'}`, borderRadius: 4, cursor: 'pointer',
                fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: activeView === v.id ? 'var(--accent)' : 'var(--text-secondary)',
              }}>{v.name}</button>
            ))}
            <button style={{
              width: 32, height: 32, background: 'var(--bg-tertiary)', border: '1px dashed var(--border)', borderRadius: 4, cursor: 'pointer',
              color: 'var(--text-muted)', display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}><Icon name="bookmark-plus" size={13} /></button>
          </div>
          <div style={{ width: 1, height: 24, background: 'var(--border)', margin: '0 4px' }} />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-muted)', textTransform: 'uppercase' }}>GRID</span>
          <Segmented options={['1x1', '2x2', '3x3', '4x4', '5x5']} value={grid} onChange={setGrid} />
          <Btn kind="secondary" onClick={() => setDrawerOpen(o => !o)}>
            <Icon name={drawerOpen ? 'panel-right-close' : 'panel-right-open'} size={13} />
            {drawerOpen ? 'Hide' : 'Cameras'}
          </Btn>
        </>}
      />

      <div style={{ flex: 1, display: 'flex', minHeight: 0 }}>
        <div style={{
          flex: 1, padding: 12, display: 'grid', gap: 6,
          gridTemplateColumns: `repeat(${cols}, 1fr)`, gridAutoRows: 'minmax(0, 1fr)',
          background: 'var(--bg-primary)', minHeight: 0,
        }}>
          {Array.from({ length: slots }).map((_, idx) => {
            const cam = tileCams[idx];
            const site = cam ? sites.find(s => s.id === cam.siteId) : null;
            return (
              <div key={idx}
                onDragOver={(e) => { e.preventDefault(); setDragOver(idx); }}
                onDragLeave={() => setDragOver(null)}
                onDrop={(e) => onDropSlot(e, idx)}
                style={{
                  position: 'relative', minHeight: 0,
                  outline: dragOver === idx ? '2px solid var(--accent)' : 'none', outlineOffset: -2,
                }}>
                {cam ? (
                  <div draggable onDragStart={(e) => onDragStart(e, cam.id)} style={{ height: '100%', position: 'relative' }}>
                    <CameraTile camera={cam} />
                    {site && (
                      <div style={{
                        position: 'absolute', top: 10, left: 80, padding: '3px 6px',
                        background: 'rgba(10,10,10,0.7)', border: '1px solid var(--accent-27)', borderRadius: 2,
                        fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 0.5, color: 'var(--accent)',
                      }}>{site.code}</div>
                    )}
                    <button onClick={() => onRemove(idx)} style={{
                      position: 'absolute', top: 8, right: 8, width: 20, height: 20, borderRadius: 2,
                      background: 'rgba(10,10,10,0.7)', border: '1px solid var(--border)', cursor: 'pointer',
                      display: 'none', alignItems: 'center', justifyContent: 'center', color: 'var(--text-secondary)',
                    }} className="tile-remove">
                      <Icon name="x" size={12} />
                    </button>
                  </div>
                ) : (
                  <div style={{
                    height: '100%', border: '1px dashed var(--border)',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    flexDirection: 'column', gap: 6, color: 'var(--text-muted)',
                    background: dragOver === idx ? 'var(--accent-7)' : 'transparent',
                  }}>
                    <Icon name="plus" size={18} />
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, letterSpacing: 1, textTransform: 'uppercase' }}>SLOT {String(idx + 1).padStart(2, '0')}</span>
                  </div>
                )}
              </div>
            );
          })}
        </div>

        {drawerOpen && (
          <aside style={{
            width: 280, background: 'var(--bg-secondary)', borderLeft: '1px solid var(--border)',
            display: 'flex', flexDirection: 'column', flexShrink: 0, minHeight: 0,
          }}>
            <div style={{ padding: 14, borderBottom: '1px solid var(--border)' }}>
              <SectionHeader>CAMERA SOURCE</SectionHeader>
              <div style={{ marginTop: 8, fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)', letterSpacing: 0.5 }}>
                Drag onto the wall · multi-select with click
              </div>
            </div>
            <div style={{ flex: 1, overflow: 'auto', minHeight: 0 }}>
              {inScope.map(s => {
                const sCams = cameras.filter(c => c.siteId === s.id);
                return (
                  <div key={s.id}>
                    <div style={{
                      padding: '8px 14px', display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                      background: 'var(--bg-primary)', borderBottom: '1px solid var(--border)',
                    }}>
                      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                        <span style={{
                          width: 5, height: 5, borderRadius: '50%',
                          background: s.status === 'online' ? 'var(--success)' : s.status === 'degraded' ? 'var(--warning)' : 'var(--danger)',
                        }} />
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--accent)' }}>{s.code}</span>
                      </div>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)' }}>{sCams.length} CAMS</span>
                    </div>
                    {sCams.map(c => {
                      const used = picks.includes(c.id);
                      return (
                        <div key={c.id} draggable onDragStart={(e) => onDragStart(e, c.id)} style={{
                          padding: '6px 14px', display: 'flex', alignItems: 'center', gap: 8,
                          borderBottom: '1px solid var(--border)', cursor: 'grab',
                          opacity: used ? 0.4 : 1,
                        }}>
                          <div style={{ width: 36, height: 22, background: 'linear-gradient(135deg, #1a1410, #0d0c0a)', position: 'relative', flexShrink: 0 }}>
                            <span style={{ position: 'absolute', top: 1, left: 1, width: 3, height: 3, borderTop: '1px solid var(--accent)', borderLeft: '1px solid var(--accent)', opacity: 0.6 }} />
                            <span style={{ position: 'absolute', bottom: 1, right: 1, width: 3, height: 3, borderBottom: '1px solid var(--accent)', borderRight: '1px solid var(--accent)', opacity: 0.6 }} />
                          </div>
                          <div style={{ flex: 1, minWidth: 0 }}>
                            <div style={{ fontFamily: 'var(--font-sans)', fontSize: 11, color: 'var(--text-primary)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{c.name}</div>
                            <div style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--text-muted)', letterSpacing: 0.5 }}>{c.id}</div>
                          </div>
                          {used && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 9, color: 'var(--accent)' }}>USED</span>}
                          {c.status === 'offline' && <span style={{ width: 5, height: 5, borderRadius: '50%', background: 'var(--danger)' }} />}
                        </div>
                      );
                    })}
                  </div>
                );
              })}
            </div>
          </aside>
        )}
      </div>
    </div>
  );
}
