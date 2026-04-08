import type { ReactNode } from 'react';
import { cn } from '@/lib/utils';

// KAI-308: Generic KPI display card for the Fleet Dashboard.
//
// Accessibility (WCAG 2.1 AA):
//  - The whole card is keyboard focusable (tabIndex=0) so users
//    can tab through the KPI row.
//  - `aria-label` carries the human-readable label + value so
//    screen readers announce "Total customers, 20" rather than
//    just "20".
//  - Visual emphasis does NOT rely on color alone; every KPI has
//    an icon slot for redundant encoding.

export interface KpiCardProps {
  readonly label: string;
  readonly value: string;
  readonly ariaValue?: string;
  readonly icon?: ReactNode;
  readonly trend?: 'up' | 'down' | 'flat';
  readonly trendLabel?: string;
  readonly testId?: string;
}

export function KpiCard({
  label,
  value,
  ariaValue,
  icon,
  trend,
  trendLabel,
  testId,
}: KpiCardProps): JSX.Element {
  const trendSymbol = trend === 'up' ? '▲' : trend === 'down' ? '▼' : trend === 'flat' ? '■' : null;
  return (
    <div
      role="group"
      tabIndex={0}
      aria-label={`${label}, ${ariaValue ?? value}`}
      data-testid={testId}
      className={cn(
        'rounded-lg border border-slate-200 bg-white p-4 shadow-sm',
        'focus:outline-none focus:ring-2 focus:ring-blue-500',
      )}
    >
      <div className="flex items-center gap-2 text-sm font-medium text-slate-600">
        {icon ? (
          <span aria-hidden="true" className="inline-flex">
            {icon}
          </span>
        ) : null}
        <span>{label}</span>
      </div>
      <div className="mt-2 text-2xl font-semibold text-slate-900" aria-hidden="true">
        {value}
      </div>
      {trendSymbol && trendLabel ? (
        <div className="mt-1 text-xs text-slate-500">
          <span aria-hidden="true">{trendSymbol} </span>
          <span>{trendLabel}</span>
        </div>
      ) : null}
    </div>
  );
}
