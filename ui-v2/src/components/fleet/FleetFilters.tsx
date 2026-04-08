import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import type { CustomerTier, FleetFilters } from '@/api/fleet';

// KAI-308: Fleet filter bar.
//
// Uses plain HTML <select> elements for three reasons:
//  1. Native keyboard + screen reader support out of the box.
//  2. Zero bundle cost on top of React.
//  3. Trivial to drive in vitest without userEvent combobox dance.
//
// Every option label comes from i18n — no hardcoded strings.

export interface FleetFilterBarProps {
  readonly filters: FleetFilters;
  readonly onChange: (next: FleetFilters) => void;
  readonly subResellers: readonly { id: string; name: string }[];
  readonly labels: readonly string[];
}

const TIER_OPTIONS: readonly CustomerTier[] = ['platinum', 'gold', 'silver', 'bronze'];

export function FleetFiltersBar({
  filters,
  onChange,
  subResellers,
  labels,
}: FleetFilterBarProps): JSX.Element {
  const { t } = useTranslation();
  const srId = useId();
  const labelId = useId();
  const tierId = useId();

  return (
    <form
      role="search"
      aria-label={t('fleet.filters.ariaLabel')}
      className="flex flex-wrap items-end gap-3 rounded-lg border border-slate-200 bg-white p-3 shadow-sm"
      onSubmit={(e) => e.preventDefault()}
    >
      <div className="flex flex-col">
        <label htmlFor={srId} className="text-xs font-medium text-slate-700">
          {t('fleet.filters.subReseller')}
        </label>
        <select
          id={srId}
          data-testid="filter-sub-reseller"
          value={filters.subResellerId ?? ''}
          onChange={(e) =>
            onChange({ ...filters, subResellerId: e.target.value || null })
          }
          className="mt-1 rounded border border-slate-300 bg-white px-2 py-1 text-sm"
        >
          <option value="">{t('fleet.filters.allSubResellers')}</option>
          {subResellers.map((sr) => (
            <option key={sr.id} value={sr.id}>
              {sr.name}
            </option>
          ))}
        </select>
      </div>

      <div className="flex flex-col">
        <label htmlFor={labelId} className="text-xs font-medium text-slate-700">
          {t('fleet.filters.label')}
        </label>
        <select
          id={labelId}
          data-testid="filter-label"
          value={filters.label ?? ''}
          onChange={(e) => onChange({ ...filters, label: e.target.value || null })}
          className="mt-1 rounded border border-slate-300 bg-white px-2 py-1 text-sm"
        >
          <option value="">{t('fleet.filters.allLabels')}</option>
          {labels.map((l) => (
            <option key={l} value={l}>
              {t(`fleet.labels.${l}`, { defaultValue: l })}
            </option>
          ))}
        </select>
      </div>

      <div className="flex flex-col">
        <label htmlFor={tierId} className="text-xs font-medium text-slate-700">
          {t('fleet.filters.tier')}
        </label>
        <select
          id={tierId}
          data-testid="filter-tier"
          value={filters.tier ?? ''}
          onChange={(e) =>
            onChange({ ...filters, tier: (e.target.value || null) as CustomerTier | null })
          }
          className="mt-1 rounded border border-slate-300 bg-white px-2 py-1 text-sm"
        >
          <option value="">{t('fleet.filters.allTiers')}</option>
          {TIER_OPTIONS.map((tier) => (
            <option key={tier} value={tier}>
              {t(`fleet.tier.${tier}`)}
            </option>
          ))}
        </select>
      </div>

      <button
        type="button"
        data-testid="filter-reset"
        onClick={() => onChange({})}
        className="rounded border border-slate-300 bg-white px-3 py-1 text-sm text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        {t('fleet.filters.reset')}
      </button>
    </form>
  );
}
