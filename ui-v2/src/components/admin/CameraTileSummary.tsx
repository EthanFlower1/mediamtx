import { useTranslation } from 'react-i18next';
import type { CameraSummary } from '@/api/dashboard';

// KAI-320: Camera tile summary.
//
// Presents online / offline / warning counts plus recent additions in
// a single group of stat tiles. All strings flow through i18n (seam #8).

export interface CameraTileSummaryProps {
  summary: CameraSummary;
}

export function CameraTileSummary({ summary }: CameraTileSummaryProps): JSX.Element {
  const { t } = useTranslation();
  const tiles: Array<{ key: string; label: string; value: number; tone: string }> = [
    {
      key: 'total',
      label: t('dashboard.cameras.total'),
      value: summary.total,
      tone: 'neutral',
    },
    {
      key: 'online',
      label: t('dashboard.cameras.online'),
      value: summary.online,
      tone: 'ok',
    },
    {
      key: 'offline',
      label: t('dashboard.cameras.offline'),
      value: summary.offline,
      tone: 'bad',
    },
    {
      key: 'warning',
      label: t('dashboard.cameras.warning'),
      value: summary.warning,
      tone: 'warn',
    },
    {
      key: 'recentlyAdded',
      label: t('dashboard.cameras.recentlyAdded'),
      value: summary.recentlyAdded,
      tone: 'neutral',
    },
  ];

  return (
    <section
      aria-label={t('dashboard.cameras.sectionLabel')}
      className="admin-dashboard__camera-summary"
    >
      <h2>{t('dashboard.cameras.heading')}</h2>
      <ul className="admin-dashboard__tile-grid" role="list">
        {tiles.map((tile) => (
          <li
            key={tile.key}
            data-tone={tile.tone}
            data-testid={`camera-tile-${tile.key}`}
            className="admin-dashboard__tile"
          >
            <span className="admin-dashboard__tile-label">{tile.label}</span>
            <span className="admin-dashboard__tile-value" aria-live="polite">
              {tile.value}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
