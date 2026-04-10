import { useTranslation } from 'react-i18next';

// KAI-324: Events search — free-text + CLIP semantic toggle.
//
// The semantic toggle is a user-visible opt-in because semantic
// search will bill against CLIP inference once the real backend
// wires pgvector (KAI-292). Keeping it behind a switch both
// matches the KAI-324 spec and gives us a place to document the
// cost once it lands.

export interface EventSearchProps {
  readonly value: string;
  readonly semantic: boolean;
  /** Whether the tenant has the ai.semantic_search entitlement. */
  readonly semanticEntitled: boolean;
  readonly onChange: (value: string) => void;
  readonly onSemanticChange: (semantic: boolean) => void;
}

export function EventSearch({
  value,
  semantic,
  semanticEntitled,
  onChange,
  onSemanticChange,
}: EventSearchProps): JSX.Element {
  const { t } = useTranslation();

  return (
    <section
      aria-label={t('events.search.sectionLabel')}
      data-testid="events-search"
      className="flex flex-wrap items-end gap-3 rounded-lg border border-slate-200 bg-white p-4 shadow-sm"
    >
      <label className="flex-1">
        <span className="block text-xs font-semibold text-slate-700">
          {t('events.search.label')}
        </span>
        <input
          type="search"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={t('events.search.placeholder')}
          aria-label={t('events.search.label')}
          data-testid="events-search-input"
          className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
      </label>
      <span className="inline-flex items-center gap-2 relative">
        <label className="inline-flex items-center gap-2">
          <input
            type="checkbox"
            checked={semantic}
            disabled={!semanticEntitled}
            onChange={(e) => onSemanticChange(e.target.checked)}
            data-testid="events-search-semantic"
            aria-label={t('events.search.semanticAriaLabel')}
            aria-disabled={!semanticEntitled}
          />
          <span className={`text-xs ${semanticEntitled ? 'text-slate-700' : 'text-slate-400'}`}>
            {t('events.search.semanticLabel')}
          </span>
        </label>
        {!semanticEntitled && (
          <span
            className="text-xs text-slate-400 italic"
            data-testid="events-search-semantic-entitlement-hint"
            title={t('events.search.semanticEntitlementTooltip')}
          >
            {t('events.search.semanticEntitlementTooltip')}
          </span>
        )}
      </span>
      <p className="w-full text-xs text-slate-500">
        {semantic && semanticEntitled
          ? t('events.search.semanticHint')
          : t('events.search.literalHint')}
      </p>
    </section>
  );
}
