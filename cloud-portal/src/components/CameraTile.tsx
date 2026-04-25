import type { Camera } from '../types';
import { Icon, StatusBadge } from './primitives';

function Bracket({ pos }: { pos: 'tl' | 'tr' | 'bl' | 'br' }) {
  const sz = 16, t = 2;
  const base = { position: 'absolute' as const, width: sz, height: sz, opacity: 0.4, borderColor: '#F97316', borderStyle: 'solid' as const };
  const map = {
    tl: { top: 6, left: 6, borderWidth: `${t}px 0 0 ${t}px` },
    tr: { top: 6, right: 6, borderWidth: `${t}px ${t}px 0 0` },
    bl: { bottom: 6, left: 6, borderWidth: `0 0 ${t}px ${t}px` },
    br: { bottom: 6, right: 6, borderWidth: `0 ${t}px ${t}px 0` },
  };
  return <span style={{ ...base, ...map[pos] }} />;
}

interface CameraTileProps {
  camera: Camera;
  focused?: boolean;
  onFocus?: () => void;
  large?: boolean;
}

export function CameraTile({ camera, focused, onFocus, large }: CameraTileProps) {
  const offline = camera.status === 'offline';
  const idCharA = camera.id?.charCodeAt(4) ?? 0;
  const idCharB = camera.id?.charCodeAt(5) ?? 0;
  return (
    <div onClick={onFocus} style={{
      position: 'relative', aspectRatio: '16 / 9', background: 'linear-gradient(135deg, #1a1410, #0d0c0a)',
      cursor: 'pointer', overflow: 'hidden', outline: focused ? '1px solid rgba(249,115,22,0.4)' : 'none',
      opacity: offline ? 0.55 : 1,
    }}>
      <div style={{
        position: 'absolute', inset: 0,
        background: camera.feed ?? `radial-gradient(circle at ${30 + idCharA % 40}% ${40 + idCharB % 30}%, rgba(120,90,40,0.45), transparent 55%), radial-gradient(circle at 70% 30%, rgba(80,60,30,0.5), transparent 60%), linear-gradient(180deg, #211711, #0a0907)`,
      }} />
      <div style={{ position: 'absolute', inset: 0,
        background: 'repeating-linear-gradient(0deg, rgba(255,255,255,0.02) 0 1px, transparent 1px 3px)' }} />
      <div style={{ position: 'absolute', left: 0, right: 0, top: 0, height: 44,
        background: 'linear-gradient(180deg, rgba(10,10,10,0.85), transparent)' }} />
      <div style={{ position: 'absolute', left: 0, right: 0, bottom: 0, height: 44,
        background: 'linear-gradient(0deg, rgba(10,10,10,0.85), transparent)' }} />
      <Bracket pos="tl" /><Bracket pos="tr" /><Bracket pos="bl" /><Bracket pos="br" />
      {offline && (
        <div style={{
          position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center',
          flexDirection: 'column', gap: 8, background: 'rgba(10,10,10,0.5)',
        }}>
          <Icon name="wifi-off" size={28} color="#EF4444" />
          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 1, color: '#EF4444' }}>OFFLINE</span>
        </div>
      )}
      <div style={{ position: 'absolute', top: 10, left: 10 }}>
        {!offline ? <StatusBadge kind="live" /> : <StatusBadge kind="offline" />}
      </div>
      <div style={{ position: 'absolute', top: 10, right: 10, display: 'flex', gap: 6 }}>
        {camera.motion && <StatusBadge kind="motion" />}
        {camera.recording && !offline && <StatusBadge kind="rec" />}
      </div>
      <div style={{ position: 'absolute', bottom: 10, left: 10,
        fontFamily: 'var(--font-sans)', fontSize: large ? 14 : 12, fontWeight: 500, color: '#E5E5E5' }}>
        {camera.name}
      </div>
      {!offline && (
        <div style={{ position: 'absolute', bottom: 10, right: 10,
          fontFamily: 'var(--font-mono)', fontSize: 10, color: '#E5E5E5', display: 'flex', gap: 10 }}>
          <span>{camera.id}</span>
          {large && <span>{camera.timestamp ?? '14:32:07'}</span>}
        </div>
      )}
      {large && camera.detection && (
        <div style={{
          position: 'absolute', left: '32%', top: '38%', width: '24%', height: '36%',
          border: '1.5px solid #F97316', boxShadow: '0 0 12px rgba(249,115,22,0.4)',
        }}>
          <span style={{
            position: 'absolute', top: -22, left: -1,
            background: '#F97316', color: '#0A0A0A',
            fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: '.5px',
            padding: '2px 6px', textTransform: 'uppercase',
          }}>{camera.detection} · 0.92</span>
        </div>
      )}
    </div>
  );
}

export function EmptyTile() {
  return (
    <div style={{
      aspectRatio: '16 / 9', border: '1px dashed var(--border)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      flexDirection: 'column', gap: 8, color: 'var(--text-muted)',
    }}>
      <Icon name="plus" size={22} />
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, textTransform: 'uppercase' }}>DROP HERE</span>
    </div>
  );
}
