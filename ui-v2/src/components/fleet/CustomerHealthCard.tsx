import { useTranslation } from 'react-i18next';
import type { FleetCustomer, HealthStatus } from '@/api/fleet';
import { cn } from '@/lib/utils';

// KAI-308: Per-customer health card.
//
// Accessibility:
//  - Color is NOT the only signal. Each health state has:
//      - a distinct background / border color (green/yellow/red)
//      - a distinct icon glyph (check / warning / cross)
//      - redundant text label ("Healthy" / "Degraded" / "Critical")
//  - The card is a keyboard-reachable link-like control
//    (role="link", tabIndex=0) so tabbing through the grid works.
//  - aria-label summarises the card contents for screen readers.

export interface CustomerHealthCardProps {
  readonly customer: FleetCustomer;
  readonly onActivate?: (customer: FleetCustomer) => void;
}

const HEALTH_CLASSES: Record<HealthStatus, string> = {
  healthy: 'border-emerald-300 bg-emerald-50',
  degraded: 'border-amber-300 bg-amber-50',
  critical: 'border-red-400 bg-red-50',
};

const HEALTH_ICON: Record<HealthStatus, string> = {
  healthy: '✓',
  degraded: '!',
  critical: '✕',
};

function formatCents(cents: number, locale: string): string {
  return new Intl.NumberFormat(locale, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 0,
  }).format(cents / 100);
}

export function CustomerHealthCard({
  customer,
  onActivate,
}: CustomerHealthCardProps): JSX.Element {
  const { t, i18n } = useTranslation();
  const healthLabel = t(`fleet.health.${customer.health}`);
  const mrr = formatCents(customer.monthlyRecurringRevenueCents, i18n.language);

  const summary = t('fleet.customerCard.ariaSummary', {
    name: customer.name,
    health: healthLabel,
    cameras: customer.cameraCount,
    mrr,
  });

  function handleKey(e: React.KeyboardEvent<HTMLDivElement>): void {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onActivate?.(customer);
    }
  }

  return (
    <div
      role="link"
      tabIndex={0}
      aria-label={summary}
      data-testid={`customer-card-${customer.id}`}
      data-health={customer.health}
      onClick={() => onActivate?.(customer)}
      onKeyDown={handleKey}
      className={cn(
        'cursor-pointer rounded-lg border p-4 shadow-sm',
        'focus:outline-none focus:ring-2 focus:ring-blue-500',
        HEALTH_CLASSES[customer.health],
      )}
    >
      <div className="flex items-start justify-between gap-2">
        <h3 className="text-base font-semibold text-slate-900">{customer.name}</h3>
        <span
          aria-hidden="true"
          className={cn(
            'inline-flex h-6 w-6 items-center justify-center rounded-full text-sm font-bold',
            customer.health === 'healthy' && 'bg-emerald-500 text-white',
            customer.health === 'degraded' && 'bg-amber-500 text-white',
            customer.health === 'critical' && 'bg-red-500 text-white',
          )}
        >
          {HEALTH_ICON[customer.health]}
        </span>
      </div>

      <p className="mt-1 text-sm font-medium text-slate-700">
        <span className="sr-only">{t('fleet.customerCard.statusSrLabel')}: </span>
        {healthLabel}
      </p>

      <dl className="mt-3 grid grid-cols-2 gap-2 text-xs text-slate-600">
        <div>
          <dt className="font-medium">{t('fleet.customerCard.tier')}</dt>
          <dd>{t(`fleet.tier.${customer.tier}`)}</dd>
        </div>
        <div>
          <dt className="font-medium">{t('fleet.customerCard.cameras')}</dt>
          <dd>
            {customer.onlineCameraCount}/{customer.cameraCount}
          </dd>
        </div>
        <div>
          <dt className="font-medium">{t('fleet.customerCard.mrr')}</dt>
          <dd>{mrr}</dd>
        </div>
        <div>
          <dt className="font-medium">{t('fleet.customerCard.uptime')}</dt>
          <dd>{customer.uptimePercent.toFixed(1)}%</dd>
        </div>
      </dl>
    </div>
  );
}
