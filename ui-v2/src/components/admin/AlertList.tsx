import { useTranslation } from 'react-i18next';
import type { Alert } from '@/api/dashboard';

// KAI-320: Active alert list with quick-ack buttons.
//
// Uses aria-live so newly injected alerts are announced to screen
// readers (seam #6, WCAG 2.1 AA).

export interface AlertListProps {
  alerts: Alert[];
  onAck: (alertId: string) => void;
  isAcking?: boolean;
}

export function AlertList({ alerts, onAck, isAcking }: AlertListProps): JSX.Element {
  const { t } = useTranslation();

  const active = alerts.filter((a) => a.state === 'active');

  return (
    <section
      aria-label={t('dashboard.alerts.sectionLabel')}
      className="admin-dashboard__alerts"
    >
      <h2 id="alerts-heading">{t('dashboard.alerts.heading')}</h2>
      <div
        role="region"
        aria-labelledby="alerts-heading"
        aria-live="assertive"
        aria-relevant="additions text"
      >
        {active.length === 0 ? (
          <p data-testid="alerts-empty">{t('dashboard.alerts.empty')}</p>
        ) : (
          <ul role="list" className="admin-dashboard__alert-list">
            {active.map((alert) => (
              <li
                key={alert.id}
                data-testid={`alert-${alert.id}`}
                data-severity={alert.severity}
                className="admin-dashboard__alert"
              >
                <div className="admin-dashboard__alert-body">
                  <strong>{alert.title}</strong>
                  <p>{alert.description}</p>
                </div>
                <button
                  type="button"
                  onClick={() => onAck(alert.id)}
                  disabled={isAcking}
                  aria-label={t('dashboard.alerts.ackAriaLabel', {
                    title: alert.title,
                  })}
                  data-testid={`alert-ack-${alert.id}`}
                >
                  {t('dashboard.alerts.ack')}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}
