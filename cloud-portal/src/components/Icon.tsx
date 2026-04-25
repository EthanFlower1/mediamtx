import * as Lucide from 'lucide-react';
import type { CSSProperties } from 'react';

interface IconProps {
  name: string;
  size?: number;
  color?: string;
  style?: CSSProperties;
}

function toPascal(name: string): string {
  return name.split('-').map(p => p.charAt(0).toUpperCase() + p.slice(1)).join('');
}

export function Icon({ name, size = 14, color, style }: IconProps) {
  const Cmp = (Lucide as unknown as Record<string, React.ComponentType<{ size?: number; color?: string; strokeWidth?: number; style?: CSSProperties }>>)[toPascal(name)];
  if (!Cmp) {
    return (
      <span
        style={{ display: 'inline-flex', width: size, height: size, color, lineHeight: 0, flexShrink: 0, ...style }}
        aria-hidden
      >
        <svg xmlns="http://www.w3.org/2000/svg" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
          <rect x={4} y={4} width={16} height={16} rx={2} />
        </svg>
      </span>
    );
  }
  return (
    <span
      style={{ display: 'inline-flex', width: size, height: size, color, lineHeight: 0, flexShrink: 0, ...style }}
      aria-hidden
    >
      <Cmp size={size} color={color} strokeWidth={2} />
    </span>
  );
}
