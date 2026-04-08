import { useTranslation } from 'react-i18next';
import type { SystemHealth, HealthStatus } from '@/api/dashboard';

// KAI-320: System health widget — 4 tiles: storage, network, sidecars, recorder.

export interface SystemHealthWidgetProps {
  health: SystemHealth;
}

function formatBytes(bytes: number): string {
  const gb = bytes / 1_000_000_000;
  if (gb >= 1000) return `${(gb / 1000).toFixed(1)} TB`;
  return `${gb.toFixed(0)} GB`;
}

function minutesSince(iso: string): number {
  return Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60_000));
}

interface Tile {
  key: string;
  label: string;
  primary: string;
  secondary: string;
  status: HealthStatus;
}

export function SystemHealthWidget({ health }: SystemHealthWidgetProps): JSX.Element {
  const { t } = useTranslation();

  const storagePct = Math.round((health.storage.usedBytes / health.storage.totalBytes) * 100);

  const tiles: Tile[] = [
    {
      key: 'storage',
      label: t('dashboard.health.storage.label'),
      primary: `${storagePct}%`,
      secondary: t('dashboard.health.storage.detail', {
        used: formatBytes(health.storage.usedBytes),
        total: formatBytes(health.storage.totalBytes),
      }),
      status: health.storage.status,
    },
    {
      key: 'network',
      label: t('dashboard.health.network.label'),
      primary: t('dashboard.health.network.primary', {
        mbps: health.network.uplinkMbps,
      }),
      secondary: t('dashboard.health.network.detail', {
        loss: health.network.packetLossPct,
      }),
      status: health.network.status,
    },
    {
      key: 'sidecars',
      label: t('dashboard.health.sidecars.label'),
      primary: `${health.sidecars.healthySidecars}/${health.sidecars.totalSidecars}`,
      secondary: t('dashboard.health.sidecars.detail'),
      status: health.sidecars.status,
    },
    {
      key: 'recorder',
      label: t('dashboard.health.recorder.label'),
      primary: t('dashboard.health.recorder.primary', {
        minutes: minutesSince(health.recorder.lastCheckinAt),
      }),
      secondary: t('dashboard.health.recorder.detail'),
      status: health.recorder.status,
    },
  ];

  return (
    <section
      aria-label={t('dashboard.health.sectionLabel')}
      className="admin-dashboard__health"
    >
      <h2>{t('dashboard.health.heading')}</h2>
      <ul className="admin-dashboard__tile-grid" role="list">
        {tiles.map((tile) => (
          <li
            key={tile.key}
            data-testid={`health-tile-${tile.key}`}
            data-status={tile.status}
            className="admin-dashboard__tile"
          >
            <span className="admin-dashboard__tile-label">{tile.label}</span>
            <span className="admin-dashboard__tile-value">{tile.primary}</span>
            <span className="admin-dashboard__tile-detail">{tile.secondary}</span>
            <span className="sr-only">
              {t(`dashboard.health.status.${tile.status}`)}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}
