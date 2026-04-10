import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import {
  getSystemHealth,
  getRemoteAccess,
  setRemoteAccessEnabled,
  getSystemSettings,
  updateSystemSettings,
  systemHealthQueryKeys,
  type SystemHealth,
  type RemoteAccess,
  type SystemSettings,
  type DayOfWeek,
} from '@/api/systemHealth';
import { useSessionStore } from '@/stores/session';

// KAI-329: Customer Admin System Health + Remote Access + Quick Settings page.
//
// ONLY rendered in the customer admin runtime context (/admin/health).
// Three sections: Health Dashboard, Remote Access, Quick Settings.
//
// Real-time updates: polls every 15 s. TODO(KAI-320-ws): swap for WS.

const DAYS_OF_WEEK: DayOfWeek[] = [
  'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday',
];

const TIMEZONES = [
  'America/New_York',
  'America/Chicago',
  'America/Denver',
  'America/Los_Angeles',
  'America/Anchorage',
  'Pacific/Honolulu',
  'Europe/London',
  'Europe/Berlin',
  'Asia/Tokyo',
  'Australia/Sydney',
];

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return `${val.toFixed(1)} ${units[i]}`;
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  if (days > 0) return `${days}d ${hours}h`;
  return `${hours}h`;
}

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function SystemHealthPage(): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const queryClient = useQueryClient();

  // -----------------------------------------------------------------------
  // Queries
  // -----------------------------------------------------------------------

  const healthQuery = useQuery<SystemHealth>({
    queryKey: systemHealthQueryKeys.health(tenantId),
    queryFn: () => getSystemHealth(tenantId),
    refetchInterval: 15_000,
  });

  const remoteQuery = useQuery<RemoteAccess>({
    queryKey: systemHealthQueryKeys.remoteAccess(tenantId),
    queryFn: () => getRemoteAccess(tenantId),
  });

  const settingsQuery = useQuery<SystemSettings>({
    queryKey: systemHealthQueryKeys.settings(tenantId),
    queryFn: () => getSystemSettings(tenantId),
  });

  // -----------------------------------------------------------------------
  // Mutations
  // -----------------------------------------------------------------------

  const toggleRemoteMutation = useMutation({
    mutationFn: (enabled: boolean) => setRemoteAccessEnabled(tenantId, enabled),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: systemHealthQueryKeys.remoteAccess(tenantId),
      });
    },
  });

  const updateSettingsMutation = useMutation({
    mutationFn: (updates: Partial<SystemSettings>) =>
      updateSystemSettings(tenantId, updates),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: systemHealthQueryKeys.settings(tenantId),
      });
    },
  });

  // -----------------------------------------------------------------------
  // Local state for system name editing
  // -----------------------------------------------------------------------

  const [editingName, setEditingName] = useState(false);
  const [nameValue, setNameValue] = useState('');

  const handleStartEditName = useCallback(() => {
    setNameValue(settingsQuery.data?.systemName ?? '');
    setEditingName(true);
  }, [settingsQuery.data]);

  const handleSaveName = useCallback(() => {
    updateSettingsMutation.mutate({ systemName: nameValue });
    setEditingName(false);
  }, [nameValue, updateSettingsMutation]);

  const handleCancelEditName = useCallback(() => {
    setEditingName(false);
  }, []);

  // -----------------------------------------------------------------------
  // Derived data
  // -----------------------------------------------------------------------

  const health = healthQuery.data;
  const remote = remoteQuery.data;
  const settings = settingsQuery.data;

  return (
    <main
      aria-label={t('systemHealth.page.label')}
      data-testid="system-health-page"
      className="system-health-page"
    >
      <nav aria-label={t('systemHealth.breadcrumb.ariaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('systemHealth.page.title')}</li>
        </ol>
      </nav>

      <header className="system-health-page__header">
        <h1>{t('systemHealth.page.title')}</h1>
      </header>

      {/* ================================================================
          SECTION 1: Health Dashboard
          ================================================================ */}
      <section
        aria-label={t('systemHealth.dashboard.ariaLabel')}
        data-testid="health-dashboard"
        className="system-health-page__section"
      >
        <h2>{t('systemHealth.dashboard.title')}</h2>

        {healthQuery.isLoading && (
          <p role="status" aria-live="polite">
            {t('systemHealth.dashboard.loading')}
          </p>
        )}
        {healthQuery.isError && (
          <p role="alert">{t('systemHealth.dashboard.error')}</p>
        )}

        {health && (
          <>
            {/* Overall status */}
            <div
              data-testid="overall-status"
              data-status={health.overallStatus}
              className="system-health-page__overall-status"
            >
              <span>{t('systemHealth.dashboard.overallStatus')}</span>
              <strong data-testid="overall-status-value">
                {t(`systemHealth.status.${health.overallStatus}`)}
              </strong>
            </div>

            {/* Recorder health cards */}
            <div
              data-testid="recorder-health-cards"
              className="system-health-page__recorder-cards"
            >
              {health.recorders.map((rec) => (
                <article
                  key={rec.id}
                  data-testid={`recorder-card-${rec.id}`}
                  className="system-health-page__recorder-card"
                >
                  <h3>{rec.name}</h3>
                  <dl>
                    <dt>{t('systemHealth.recorder.ip')}</dt>
                    <dd data-testid={`recorder-ip-${rec.id}`}>{rec.ipAddress}</dd>

                    <dt>{t('systemHealth.recorder.status')}</dt>
                    <dd
                      data-testid={`recorder-status-${rec.id}`}
                      data-status={rec.status}
                    >
                      {t(`systemHealth.status.${rec.status}`)}
                    </dd>

                    <dt>{t('systemHealth.recorder.uptime')}</dt>
                    <dd data-testid={`recorder-uptime-${rec.id}`}>
                      {formatUptime(rec.uptimeSeconds)}
                    </dd>

                    <dt>{t('systemHealth.recorder.storage')}</dt>
                    <dd data-testid={`recorder-storage-${rec.id}`}>
                      <div className="system-health-page__storage-bar">
                        <div
                          className="system-health-page__storage-bar-fill"
                          style={{
                            width: `${Math.round((rec.storageUsedBytes / rec.storageTotalBytes) * 100)}%`,
                          }}
                          role="progressbar"
                          aria-valuenow={Math.round(
                            (rec.storageUsedBytes / rec.storageTotalBytes) * 100,
                          )}
                          aria-valuemin={0}
                          aria-valuemax={100}
                          aria-label={t('systemHealth.recorder.storageBarLabel')}
                        />
                      </div>
                      <span>
                        {formatBytes(rec.storageUsedBytes)} / {formatBytes(rec.storageTotalBytes)}
                      </span>
                    </dd>

                    <dt>{t('systemHealth.recorder.lastCheckIn')}</dt>
                    <dd data-testid={`recorder-checkin-${rec.id}`}>
                      {new Date(rec.lastCheckIn).toLocaleString()}
                    </dd>
                  </dl>
                </article>
              ))}
            </div>

            {/* Camera status summary */}
            <div
              data-testid="camera-summary"
              className="system-health-page__camera-summary"
            >
              <h3>{t('systemHealth.cameras.title')}</h3>
              <dl>
                <dt>{t('systemHealth.cameras.total')}</dt>
                <dd data-testid="cameras-total">{health.cameras.total}</dd>
                <dt>{t('systemHealth.cameras.online')}</dt>
                <dd data-testid="cameras-online">{health.cameras.online}</dd>
                <dt>{t('systemHealth.cameras.offline')}</dt>
                <dd data-testid="cameras-offline">{health.cameras.offline}</dd>
                <dt>{t('systemHealth.cameras.degraded')}</dt>
                <dd data-testid="cameras-degraded">{health.cameras.degraded}</dd>
              </dl>
            </div>

            {/* Storage overview */}
            <div
              data-testid="storage-overview"
              className="system-health-page__storage-overview"
            >
              <h3>{t('systemHealth.storage.title')}</h3>
              <dl>
                <dt>{t('systemHealth.storage.totalCapacity')}</dt>
                <dd data-testid="storage-total">
                  {formatBytes(health.storage.totalCapacityBytes)}
                </dd>
                <dt>{t('systemHealth.storage.used')}</dt>
                <dd data-testid="storage-used">
                  {formatBytes(health.storage.usedBytes)}
                </dd>
                <dt>{t('systemHealth.storage.available')}</dt>
                <dd data-testid="storage-available">
                  {formatBytes(health.storage.availableBytes)}
                </dd>
                <dt>{t('systemHealth.storage.retentionDays')}</dt>
                <dd data-testid="storage-retention-days">
                  {health.storage.retentionDaysRemaining}
                </dd>
              </dl>
            </div>

            {/* Network status */}
            <div
              data-testid="network-status"
              className="system-health-page__network"
            >
              <h3>{t('systemHealth.network.title')}</h3>
              <dl>
                <dt>{t('systemHealth.network.bandwidth')}</dt>
                <dd data-testid="network-bandwidth">
                  {health.network.bandwidthUtilizationPercent}%
                </dd>
                <dt>{t('systemHealth.network.packetLoss')}</dt>
                <dd data-testid="network-packet-loss">
                  {health.network.packetLossPercent}%
                </dd>
                <dt>{t('systemHealth.network.latency')}</dt>
                <dd data-testid="network-latency">
                  {health.network.latencyMs} ms
                </dd>
              </dl>
            </div>
          </>
        )}
      </section>

      {/* ================================================================
          SECTION 2: Remote Access
          ================================================================ */}
      <section
        aria-label={t('systemHealth.remoteAccess.ariaLabel')}
        data-testid="remote-access-section"
        className="system-health-page__section"
      >
        <h2>{t('systemHealth.remoteAccess.title')}</h2>

        {remoteQuery.isLoading && (
          <p role="status" aria-live="polite">
            {t('systemHealth.remoteAccess.loading')}
          </p>
        )}
        {remoteQuery.isError && (
          <p role="alert">{t('systemHealth.remoteAccess.error')}</p>
        )}

        {remote && (
          <>
            <div className="system-health-page__remote-toggle">
              <label htmlFor="remote-access-toggle">
                {t('systemHealth.remoteAccess.enableLabel')}
              </label>
              <input
                id="remote-access-toggle"
                type="checkbox"
                checked={remote.enabled}
                onChange={(e) => toggleRemoteMutation.mutate(e.target.checked)}
                data-testid="remote-access-toggle"
                aria-label={t('systemHealth.remoteAccess.enableLabel')}
              />
            </div>

            <dl>
              <dt>{t('systemHealth.remoteAccess.portForwarding')}</dt>
              <dd data-testid="port-forwarding-status">
                {remote.portForwardingActive
                  ? t('systemHealth.remoteAccess.active')
                  : t('systemHealth.remoteAccess.inactive')}
              </dd>

              <dt>{t('systemHealth.remoteAccess.vpnTunnel')}</dt>
              <dd data-testid="vpn-tunnel-status">
                {t(`systemHealth.remoteAccess.vpn.${remote.vpnTunnelStatus}`)}
              </dd>
            </dl>

            {/* Remote session history */}
            <h3>{t('systemHealth.remoteAccess.sessionHistory')}</h3>
            <table
              data-testid="remote-sessions-table"
              aria-label={t('systemHealth.remoteAccess.sessionTableAriaLabel')}
            >
              <thead>
                <tr>
                  <th>{t('systemHealth.remoteAccess.columns.user')}</th>
                  <th>{t('systemHealth.remoteAccess.columns.started')}</th>
                  <th>{t('systemHealth.remoteAccess.columns.ended')}</th>
                  <th>{t('systemHealth.remoteAccess.columns.duration')}</th>
                </tr>
              </thead>
              <tbody>
                {remote.recentSessions.map((session) => (
                  <tr key={session.id} data-testid={`session-row-${session.id}`}>
                    <td>{session.userDisplayName}</td>
                    <td>{new Date(session.startedAt).toLocaleString()}</td>
                    <td>
                      {session.endedAt
                        ? new Date(session.endedAt).toLocaleString()
                        : t('systemHealth.remoteAccess.activeSession')}
                    </td>
                    <td>{formatDuration(session.durationSeconds)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        )}
      </section>

      {/* ================================================================
          SECTION 3: Quick Settings
          ================================================================ */}
      <section
        aria-label={t('systemHealth.settings.ariaLabel')}
        data-testid="quick-settings-section"
        className="system-health-page__section"
      >
        <h2>{t('systemHealth.settings.title')}</h2>

        {settingsQuery.isLoading && (
          <p role="status" aria-live="polite">
            {t('systemHealth.settings.loading')}
          </p>
        )}
        {settingsQuery.isError && (
          <p role="alert">{t('systemHealth.settings.error')}</p>
        )}

        {settings && (
          <dl>
            {/* System name */}
            <dt>{t('systemHealth.settings.systemName')}</dt>
            <dd>
              {editingName ? (
                <span className="system-health-page__edit-name">
                  <input
                    type="text"
                    value={nameValue}
                    onChange={(e) => setNameValue(e.target.value)}
                    data-testid="system-name-input"
                    aria-label={t('systemHealth.settings.systemNameInputLabel')}
                  />
                  <button
                    type="button"
                    onClick={handleSaveName}
                    data-testid="system-name-save"
                  >
                    {t('systemHealth.settings.save')}
                  </button>
                  <button
                    type="button"
                    onClick={handleCancelEditName}
                    data-testid="system-name-cancel"
                  >
                    {t('systemHealth.settings.cancel')}
                  </button>
                </span>
              ) : (
                <span>
                  <span data-testid="system-name-display">{settings.systemName}</span>
                  <button
                    type="button"
                    onClick={handleStartEditName}
                    data-testid="system-name-edit"
                  >
                    {t('systemHealth.settings.edit')}
                  </button>
                </span>
              )}
            </dd>

            {/* Timezone */}
            <dt>{t('systemHealth.settings.timezone')}</dt>
            <dd>
              <select
                value={settings.timezone}
                onChange={(e) =>
                  updateSettingsMutation.mutate({ timezone: e.target.value })
                }
                data-testid="timezone-select"
                aria-label={t('systemHealth.settings.timezoneLabel')}
              >
                {TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </select>
            </dd>

            {/* Auto-update */}
            <dt>{t('systemHealth.settings.autoUpdate')}</dt>
            <dd>
              <label htmlFor="auto-update-toggle">
                <input
                  id="auto-update-toggle"
                  type="checkbox"
                  checked={settings.autoUpdateEnabled}
                  onChange={(e) =>
                    updateSettingsMutation.mutate({
                      autoUpdateEnabled: e.target.checked,
                    })
                  }
                  data-testid="auto-update-toggle"
                  aria-label={t('systemHealth.settings.autoUpdateLabel')}
                />
                {settings.autoUpdateEnabled
                  ? t('systemHealth.settings.enabled')
                  : t('systemHealth.settings.disabled')}
              </label>
            </dd>

            {/* Maintenance window */}
            <dt>{t('systemHealth.settings.maintenanceWindow')}</dt>
            <dd>
              <select
                value={settings.maintenanceDay}
                onChange={(e) =>
                  updateSettingsMutation.mutate({
                    maintenanceDay: e.target.value as DayOfWeek,
                  })
                }
                data-testid="maintenance-day-select"
                aria-label={t('systemHealth.settings.maintenanceDayLabel')}
              >
                {DAYS_OF_WEEK.map((day) => (
                  <option key={day} value={day}>
                    {t(`systemHealth.settings.days.${day}`)}
                  </option>
                ))}
              </select>
              <input
                type="time"
                value={settings.maintenanceTime}
                onChange={(e) =>
                  updateSettingsMutation.mutate({
                    maintenanceTime: e.target.value,
                  })
                }
                data-testid="maintenance-time-input"
                aria-label={t('systemHealth.settings.maintenanceTimeLabel')}
              />
            </dd>
          </dl>
        )}
      </section>
    </main>
  );
}

export default SystemHealthPage;
