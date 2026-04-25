import { useRef, type CSSProperties, type ReactNode } from 'react';
import { Icon } from './Icon';

export function hex2rgb(h: string): string {
  const x = h.replace('#', '');
  return [parseInt(x.slice(0, 2), 16), parseInt(x.slice(2, 4), 16), parseInt(x.slice(4, 6), 16)].join(',');
}

type BtnKind = 'primary' | 'secondary' | 'danger' | 'tactical' | 'ghost';

interface BtnProps {
  kind?: BtnKind;
  children?: ReactNode;
  onClick?: () => void;
  style?: CSSProperties;
}

export function Btn({ kind = 'primary', children, onClick, style }: BtnProps) {
  const base: CSSProperties = {
    fontFamily: 'var(--font-sans)', fontWeight: 600, fontSize: 12,
    height: 32, padding: '0 14px', borderRadius: 4, border: '1px solid transparent',
    cursor: 'pointer', display: 'inline-flex', alignItems: 'center', gap: 6,
    transition: 'background 150ms var(--ease-out)',
  };
  const kinds: Record<BtnKind, CSSProperties> = {
    primary:   { background: '#F97316', color: '#0A0A0A' },
    secondary: { background: 'var(--bg-tertiary)', color: 'var(--text-primary)', borderColor: 'var(--border)' },
    danger:    { background: 'rgba(239,68,68,0.13)', color: '#EF4444', borderColor: 'rgba(239,68,68,0.27)' },
    tactical:  { background: 'var(--bg-tertiary)', color: '#F97316', borderColor: 'rgba(249,115,22,0.27)',
                 fontFamily: 'var(--font-mono)', textTransform: 'uppercase', letterSpacing: 1, fontSize: 11, fontWeight: 500 },
    ghost:     { background: 'transparent', color: 'var(--text-secondary)', borderColor: 'var(--border)' },
  };
  return <button style={{ ...base, ...kinds[kind], ...style }} onClick={onClick}>{children}</button>;
}

type BadgeKind = 'live' | 'rec' | 'motion' | 'online' | 'offline' | 'degraded';

interface StatusBadgeProps { kind?: BadgeKind; label?: string; }

export function StatusBadge({ kind = 'live', label }: StatusBadgeProps) {
  const tones: Record<BadgeKind, { c: string; l: string }> = {
    live:     { c: '#22C55E', l: 'LIVE' },
    rec:      { c: '#EF4444', l: 'REC' },
    motion:   { c: '#F97316', l: 'MOTION' },
    online:   { c: '#22C55E', l: 'ONLINE' },
    offline:  { c: '#737373', l: 'OFFLINE' },
    degraded: { c: '#EAB308', l: 'DEGRADED' },
  };
  const t = tones[kind];
  const text = label ?? t.l;
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6,
      padding: '3px 7px', borderRadius: 3,
      fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: '.5px', textTransform: 'uppercase',
      background: `rgba(${hex2rgb(t.c)},0.07)`,
      border: `1px solid rgba(${hex2rgb(t.c)},0.27)`,
      color: t.c,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: '50%', background: t.c, boxShadow: `0 0 6px ${t.c}` }} />
      {text}
    </span>
  );
}

type SegOption = string | { value: string; label: string };

interface SegmentedProps { options: SegOption[]; value: string; onChange?: (v: string) => void; }

export function Segmented({ options, value, onChange }: SegmentedProps) {
  return (
    <div style={{
      display: 'inline-flex', border: '1px solid var(--border)', borderRadius: 4,
      overflow: 'hidden', background: 'var(--bg-primary)',
    }}>
      {options.map((opt, i) => {
        const v = typeof opt === 'string' ? opt : opt.value;
        const lbl = typeof opt === 'string' ? opt : opt.label;
        const active = v === value;
        return (
          <button key={v} onClick={() => onChange?.(v)} style={{
            fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 1,
            padding: '6px 12px', background: active ? 'rgba(249,115,22,0.13)' : 'transparent',
            border: 'none', color: active ? '#F97316' : 'var(--text-muted)',
            cursor: 'pointer',
            borderRight: i < options.length - 1 ? '1px solid var(--border)' : 'none',
            textTransform: 'uppercase',
          }}>{lbl}</button>
        );
      })}
    </div>
  );
}

interface ToggleProps { on: boolean; onChange?: (v: boolean) => void; label?: boolean; }

export function Toggle({ on, onChange, label }: ToggleProps) {
  return (
    <button onClick={() => onChange?.(!on)} style={{
      background: 'transparent', border: 'none', cursor: 'pointer',
      display: 'inline-flex', flexDirection: 'column', alignItems: 'center', gap: 4, padding: 0,
    }}>
      <div style={{
        width: 42, height: 22, borderRadius: 999, background: 'var(--bg-tertiary)',
        border: on ? '2px solid #F97316' : '2px solid var(--border)',
        boxShadow: on ? '0 0 12px rgba(249,115,22,0.55)' : 'none',
        position: 'relative', transition: 'all 150ms var(--ease-out)',
      }}>
        <div style={{
          position: 'absolute', top: '50%', transform: 'translateY(-50%)',
          width: 12, height: 12, borderRadius: '50%',
          background: on ? '#F97316' : '#404040',
          boxShadow: on ? '0 0 8px rgba(249,115,22,0.7)' : 'none',
          left: on ? 24 : 4,
          transition: 'left 150ms var(--ease-out)',
        }} />
      </div>
      {label && (
        <span style={{
          fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1,
          color: on ? '#F97316' : 'var(--text-muted)', textTransform: 'uppercase',
        }}>{on ? 'ON' : 'OFF'}</span>
      )}
    </button>
  );
}

interface SliderProps {
  label: string; value: number; onChange?: (v: number) => void;
  min?: number; max?: number; suffix?: string; ticks?: number;
}

export function Slider({ label, value, onChange, min = 0, max = 100, suffix = '%', ticks = 11 }: SliderProps) {
  const pct = ((value - min) / (max - min)) * 100;
  const trackRef = useRef<HTMLDivElement>(null);
  const onDown = (e: React.MouseEvent) => {
    const move = (ev: MouseEvent) => {
      const r = trackRef.current?.getBoundingClientRect();
      if (!r) return;
      const p = Math.max(0, Math.min(1, (ev.clientX - r.left) / r.width));
      onChange?.(Math.round(min + p * (max - min)));
    };
    const up = () => { window.removeEventListener('mousemove', move); window.removeEventListener('mouseup', up); };
    window.addEventListener('mousemove', move);
    window.addEventListener('mouseup', up);
    move(e.nativeEvent);
  };
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-end' }}>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-secondary)', textTransform: 'uppercase' }}>{label}</span>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: '#F97316' }}>{value}{suffix}</span>
      </div>
      <div ref={trackRef} onMouseDown={onDown} style={{
        position: 'relative', height: 5, background: 'var(--bg-tertiary)',
        border: '1px solid var(--border)', borderRadius: 3, cursor: 'pointer',
      }}>
        <div style={{ position: 'absolute', inset: 0, width: pct + '%',
          background: 'linear-gradient(90deg, #F97316, rgba(249,115,22,0.4))', borderRadius: 3 }} />
        <div style={{
          position: 'absolute', left: pct + '%', top: '50%', transform: 'translate(-50%,-50%)',
          width: 16, height: 16, borderRadius: '50%', background: 'var(--bg-tertiary)',
          border: '2px solid #F97316', boxShadow: '0 0 8px rgba(249,115,22,0.6)',
        }} />
      </div>
      <div style={{ display: 'flex', justifyContent: 'space-between' }}>
        {Array.from({ length: ticks }).map((_, i) => <div key={i} style={{ width: 1, height: 4, background: 'var(--border)' }} />)}
      </div>
    </div>
  );
}

interface SectionHeaderProps { children: ReactNode; right?: ReactNode; }

export function SectionHeader({ children, right }: SectionHeaderProps) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, letterSpacing: 2, fontWeight: 500, color: 'var(--text-secondary)', textTransform: 'uppercase' }}>{children}</span>
      {right}
    </div>
  );
}

interface PageHeaderProps {
  title: string;
  subtitle?: string;
  actions?: ReactNode;
  badges?: ReactNode;
}

export function PageHeader({ title, subtitle, actions, badges }: PageHeaderProps) {
  return (
    <header style={{
      height: 60, padding: '0 24px', borderBottom: '1px solid var(--border)',
      display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexShrink: 0, background: 'var(--bg-primary)',
    }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
          <span style={{ fontFamily: 'var(--font-sans)', fontSize: 16, fontWeight: 600, color: 'var(--text-primary)' }}>{title}</span>
          {subtitle && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1, color: 'var(--text-muted)', textTransform: 'uppercase' }}>{subtitle}</span>}
        </div>
        {badges}
      </div>
      <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>{actions}</div>
    </header>
  );
}

interface StatProps {
  label: string;
  value: ReactNode;
  accent?: string;
  sub?: ReactNode;
  glow?: boolean;
}

export function Stat({ label, value, accent, sub, glow }: StatProps) {
  return (
    <div style={{
      padding: '14px 16px', background: 'var(--bg-secondary)', border: '1px solid var(--border)', borderRadius: 6,
      display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0,
    }}>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 1.5, color: 'var(--text-muted)', textTransform: 'uppercase' }}>{label}</span>
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 22, fontWeight: 500, color: accent ?? 'var(--text-primary)',
        textShadow: glow ? '0 0 12px rgba(249,115,22,0.5)' : 'none', fontVariantNumeric: 'tabular-nums' }}>{value}</span>
      {sub && <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, color: 'var(--text-secondary)' }}>{sub}</span>}
    </div>
  );
}

import type { Site } from '../types';

interface SiteChipProps { site: Site; compact?: boolean; }

export function SiteChip({ site, compact }: SiteChipProps) {
  const c = site.status === 'online' ? 'var(--success)' : site.status === 'degraded' ? 'var(--warning)' : 'var(--danger)';
  return (
    <span style={{
      display: 'inline-flex', alignItems: 'center', gap: 6, padding: compact ? '2px 6px' : '4px 8px',
      background: 'var(--bg-tertiary)', border: '1px solid var(--border)', borderRadius: 3,
    }}>
      <span style={{ width: 5, height: 5, borderRadius: '50%', background: c, boxShadow: `0 0 6px ${c}` }} />
      <span style={{ fontFamily: 'var(--font-mono)', fontSize: 10, letterSpacing: 0.5, color: 'var(--accent)' }}>{site.code}</span>
      {!compact && <span style={{ fontFamily: 'var(--font-sans)', fontSize: 11, color: 'var(--text-primary)' }}>{site.name.replace(/^[^—]+— /, '')}</span>}
    </span>
  );
}

export { Icon };
