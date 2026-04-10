import { useMemo, useState, useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listMobileBuilds,
  triggerMobileBuild,
  mobileBuildsQueryKeys,
  __TEST__,
} from '@/api/mobileBuilds';
import type {
  MobileBuild,
  BuildPlatform,
  BuildStatus,
  VersionBumpType,
  CredentialConnectionStatus,
  PlayConsoleTrack,
} from '@/api/mobileBuilds';

// KAI-311: Mobile App Builds page (Integrator Portal).
//
// Lets integrators manage white-label mobile app builds (iOS + Android).
// Sections:
//   1. Build list table
//   2. Trigger build dialog
//   3. Build detail drawer (logs, artifacts, signing, upload status)
//   4. Credentials section (Apple + Google connection status)
//   5. Distribution config (TestFlight group, Play Console track, auto-submit)
//
// Accessibility:
//   - Page wrapped in <main> with labelled landmarks.
//   - Status badges use icon+text+border (never color alone per WCAG 2.1 AA).
//   - All interactive elements are keyboard-accessible.
//   - Axe smoke test in companion .test.tsx file.

interface MobileBuildsPageProps {
  readonly integratorId?: string;
}

// ---------------------------------------------------------------------------
// Status badge — icon + text + border (WCAG: never color alone)
// ---------------------------------------------------------------------------

function BuildStatusBadge({ status }: { readonly status: BuildStatus }): JSX.Element {
  const { t } = useTranslation();
  const config: Record<BuildStatus, { icon: string; label: string; className: string }> = {
    queued: {
      icon: '\u23F3', // hourglass
      label: t('mobileBuilds.status.queued'),
      className: 'border-slate-400 bg-slate-50 text-slate-700',
    },
    building: {
      icon: '\u2699', // gear
      label: t('mobileBuilds.status.building'),
      className: 'border-blue-400 bg-blue-50 text-blue-700',
    },
    succeeded: {
      icon: '\u2713', // check
      label: t('mobileBuilds.status.succeeded'),
      className: 'border-green-500 bg-green-50 text-green-700',
    },
    failed: {
      icon: '\u2717', // X mark
      label: t('mobileBuilds.status.failed'),
      className: 'border-red-500 bg-red-50 text-red-700',
    },
  };
  const c = config[status];
  return (
    <span
      data-testid={`status-badge-${status}`}
      className={`inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs font-medium ${c.className}`}
    >
      <span aria-hidden="true">{c.icon}</span>
      {c.label}
    </span>
  );
}

function CredentialStatusBadge({
  status,
}: {
  readonly status: CredentialConnectionStatus;
}): JSX.Element {
  const { t } = useTranslation();
  const config: Record<
    CredentialConnectionStatus,
    { icon: string; label: string; className: string }
  > = {
    connected: {
      icon: '\u2713',
      label: t('mobileBuilds.credentials.connected'),
      className: 'border-green-500 bg-green-50 text-green-700',
    },
    expired: {
      icon: '\u26A0',
      label: t('mobileBuilds.credentials.expired'),
      className: 'border-amber-500 bg-amber-50 text-amber-700',
    },
    missing: {
      icon: '\u2717',
      label: t('mobileBuilds.credentials.missing'),
      className: 'border-red-500 bg-red-50 text-red-700',
    },
  };
  const c = config[status];
  return (
    <span
      data-testid={`credential-status-${status}`}
      className={`inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs font-medium ${c.className}`}
    >
      <span aria-hidden="true">{c.icon}</span>
      {c.label}
    </span>
  );
}

function PlatformLabel({ platform }: { readonly platform: BuildPlatform }): JSX.Element {
  const { t } = useTranslation();
  const isIos = platform === 'ios';
  return (
    <span className="inline-flex items-center gap-1 text-sm">
      <span aria-hidden="true">{isIos ? '\uF8FF' : '\u{1F4F1}'}</span>
      {isIos ? t('mobileBuilds.platform.ios') : t('mobileBuilds.platform.android')}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Trigger Build Dialog
// ---------------------------------------------------------------------------

interface TriggerBuildDialogProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onSubmit: (args: {
    platform: BuildPlatform | 'both';
    brandConfigName: string;
    versionBump: VersionBumpType;
    releaseNotes: string;
  }) => void;
  readonly brandConfigs: readonly string[];
  readonly isSubmitting: boolean;
}

function TriggerBuildDialog({
  open,
  onClose,
  onSubmit,
  brandConfigs,
  isSubmitting,
}: TriggerBuildDialogProps): JSX.Element | null {
  const { t } = useTranslation();
  const [platform, setPlatform] = useState<BuildPlatform | 'both'>('both');
  const [brandConfig, setBrandConfig] = useState(brandConfigs[0] ?? '');
  const [versionBump, setVersionBump] = useState<VersionBumpType>('patch');
  const [releaseNotes, setReleaseNotes] = useState('');
  useEffect(() => {
    if (brandConfigs.length > 0 && !brandConfig) {
      setBrandConfig(brandConfigs[0]!);
    }
  }, [brandConfigs, brandConfig]);

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      onSubmit({ platform, brandConfigName: brandConfig, versionBump, releaseNotes });
    },
    [platform, brandConfig, versionBump, releaseNotes, onSubmit],
  );

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      data-testid="trigger-build-dialog"
      aria-labelledby="trigger-build-title"
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    >
      <div className="w-full max-w-lg rounded-lg border border-slate-200 bg-white p-6 shadow-xl">
      <form onSubmit={handleSubmit}>
        <h2 id="trigger-build-title" className="mb-4 text-lg font-bold text-slate-900">
          {t('mobileBuilds.triggerDialog.title')}
        </h2>

        <fieldset className="mb-3">
          <legend className="mb-1 text-sm font-medium text-slate-700">
            {t('mobileBuilds.triggerDialog.platformLabel')}
          </legend>
          {(['ios', 'android', 'both'] as const).map((p) => (
            <label key={p} className="mr-4 inline-flex items-center gap-1 text-sm">
              <input
                type="radio"
                name="platform"
                value={p}
                checked={platform === p}
                onChange={() => setPlatform(p)}
              />
              {t(`mobileBuilds.triggerDialog.platform.${p}`)}
            </label>
          ))}
        </fieldset>

        <label className="mb-3 block">
          <span className="mb-1 block text-sm font-medium text-slate-700">
            {t('mobileBuilds.triggerDialog.brandConfigLabel')}
          </span>
          <select
            data-testid="trigger-brand-select"
            value={brandConfig}
            onChange={(e) => setBrandConfig(e.target.value)}
            className="w-full rounded border border-slate-300 px-2 py-1 text-sm"
          >
            {brandConfigs.map((bc) => (
              <option key={bc} value={bc}>
                {bc}
              </option>
            ))}
          </select>
        </label>

        <fieldset className="mb-3">
          <legend className="mb-1 text-sm font-medium text-slate-700">
            {t('mobileBuilds.triggerDialog.versionBumpLabel')}
          </legend>
          {(['patch', 'minor', 'major'] as const).map((v) => (
            <label key={v} className="mr-4 inline-flex items-center gap-1 text-sm">
              <input
                type="radio"
                name="versionBump"
                value={v}
                checked={versionBump === v}
                onChange={() => setVersionBump(v)}
              />
              {t(`mobileBuilds.triggerDialog.versionBump.${v}`)}
            </label>
          ))}
        </fieldset>

        <label className="mb-4 block">
          <span className="mb-1 block text-sm font-medium text-slate-700">
            {t('mobileBuilds.triggerDialog.releaseNotesLabel')}
          </span>
          <textarea
            data-testid="trigger-release-notes"
            value={releaseNotes}
            onChange={(e) => setReleaseNotes(e.target.value)}
            rows={3}
            className="w-full rounded border border-slate-300 px-2 py-1 text-sm"
          />
        </label>

        <div className="flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-50"
          >
            {t('mobileBuilds.triggerDialog.cancel')}
          </button>
          <button
            type="submit"
            disabled={isSubmitting}
            data-testid="trigger-build-submit"
            className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
          >
            {isSubmitting
              ? t('mobileBuilds.triggerDialog.submitting')
              : t('mobileBuilds.triggerDialog.submit')}
          </button>
        </div>
      </form>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Build Detail Drawer
// ---------------------------------------------------------------------------

interface BuildDetailDrawerProps {
  readonly build: MobileBuild | null;
  readonly onClose: () => void;
}

function BuildDetailDrawer({ build, onClose }: BuildDetailDrawerProps): JSX.Element | null {
  const { t } = useTranslation();
  if (!build) return null;

  return (
    <aside
      data-testid="build-detail-drawer"
      role="complementary"
      aria-label={t('mobileBuilds.detail.drawerLabel')}
      className="fixed inset-y-0 right-0 z-40 w-full max-w-lg overflow-y-auto border-l border-slate-200 bg-white p-6 shadow-xl sm:w-[32rem]"
    >
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-lg font-bold text-slate-900">
          {t('mobileBuilds.detail.title', { id: build.id })}
        </h2>
        <button
          onClick={onClose}
          aria-label={t('mobileBuilds.detail.close')}
          className="rounded p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600"
        >
          <span aria-hidden="true">{'\u2715'}</span>
        </button>
      </div>

      <dl className="mb-4 space-y-2 text-sm">
        <div className="flex gap-2">
          <dt className="font-medium text-slate-600">{t('mobileBuilds.columns.platform')}:</dt>
          <dd>
            <PlatformLabel platform={build.platform} />
          </dd>
        </div>
        <div className="flex gap-2">
          <dt className="font-medium text-slate-600">{t('mobileBuilds.columns.version')}:</dt>
          <dd>{build.version}</dd>
        </div>
        <div className="flex gap-2">
          <dt className="font-medium text-slate-600">{t('mobileBuilds.columns.status')}:</dt>
          <dd>
            <BuildStatusBadge status={build.status} />
          </dd>
        </div>
        <div className="flex gap-2">
          <dt className="font-medium text-slate-600">
            {t('mobileBuilds.detail.signingCert')}:
          </dt>
          <dd>
            <CredentialStatusBadge status={build.signingCertStatus} />
          </dd>
        </div>
        <div className="flex gap-2">
          <dt className="font-medium text-slate-600">
            {t('mobileBuilds.detail.uploadStatus')}:
          </dt>
          <dd>{t(`mobileBuilds.detail.uploadValue.${build.distributionUploadStatus}`)}</dd>
        </div>
      </dl>

      {/* Artifacts */}
      {build.artifacts.length > 0 && (
        <section className="mb-4" aria-label={t('mobileBuilds.detail.artifactsLabel')}>
          <h3 className="mb-1 text-sm font-semibold text-slate-900">
            {t('mobileBuilds.detail.artifacts')}
          </h3>
          <ul className="space-y-1">
            {build.artifacts.map((art) => (
              <li key={art.id} className="text-sm">
                <a
                  href={art.downloadUrl}
                  data-testid={`artifact-link-${art.id}`}
                  className="text-blue-600 underline hover:text-blue-800"
                >
                  {art.fileName}
                </a>
                <span className="ml-2 text-slate-500">
                  ({(art.sizeBytes / 1_000_000).toFixed(1)} MB)
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {/* Build Logs */}
      <section aria-label={t('mobileBuilds.detail.logsLabel')}>
        <h3 className="mb-1 text-sm font-semibold text-slate-900">
          {t('mobileBuilds.detail.logs')}
        </h3>
        <pre
          data-testid="build-logs"
          className="max-h-64 overflow-auto rounded border border-slate-200 bg-slate-50 p-3 font-mono text-xs text-slate-700"
        >
          {build.buildLogs}
        </pre>
      </section>
    </aside>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export function MobileBuildsPage({
  integratorId = __TEST__.CURRENT_INTEGRATOR_ID,
}: MobileBuildsPageProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedBuild, setSelectedBuild] = useState<MobileBuild | null>(null);

  const query = useQuery({
    queryKey: mobileBuildsQueryKeys.list(integratorId),
    queryFn: () => listMobileBuilds(integratorId),
  });

  const triggerMutation = useMutation({
    mutationFn: triggerMobileBuild,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: mobileBuildsQueryKeys.all(integratorId),
      });
      setDialogOpen(false);
    },
  });

  const snapshot = query.data;

  const sortedBuilds = useMemo(() => {
    if (!snapshot) return [];
    return [...snapshot.builds].sort(
      (a, b) => new Date(b.createdAtIso).getTime() - new Date(a.createdAtIso).getTime(),
    );
  }, [snapshot]);

  const handleTrigger = useCallback(
    (args: {
      platform: BuildPlatform | 'both';
      brandConfigName: string;
      versionBump: VersionBumpType;
      releaseNotes: string;
    }) => {
      triggerMutation.mutate({
        integratorId,
        ...args,
      });
    },
    [integratorId, triggerMutation],
  );

  const title = t('mobileBuilds.page.title');

  return (
    <main
      data-testid="mobile-builds-page"
      aria-labelledby="mobile-builds-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      {/* Breadcrumb */}
      <header className="mb-4">
        <nav aria-label={t('mobileBuilds.breadcrumb.ariaLabel')} className="text-xs text-slate-500">
          <ol className="flex gap-1">
            <li>{t('mobileBuilds.breadcrumb.integratorPortal')}</li>
            <li aria-hidden="true">/</li>
            <li aria-current="page" className="font-medium text-slate-700">
              {title}
            </li>
          </ol>
        </nav>
        <div className="mt-1 flex items-center justify-between">
          <h1 id="mobile-builds-heading" className="text-2xl font-bold text-slate-900">
            {title}
          </h1>
          <button
            data-testid="trigger-build-button"
            onClick={() => setDialogOpen(true)}
            className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-700"
          >
            {t('mobileBuilds.actions.triggerBuild')}
          </button>
        </div>
        <p className="text-sm text-slate-600">{t('mobileBuilds.page.subtitle')}</p>
      </header>

      {query.isLoading ? (
        <p role="status" data-testid="builds-loading">
          {t('mobileBuilds.page.loading')}
        </p>
      ) : query.isError ? (
        <p role="alert" data-testid="builds-error">
          {t('mobileBuilds.page.error')}
        </p>
      ) : snapshot ? (
        <>
          {/* Build List Table */}
          <section
            role="region"
            aria-label={t('mobileBuilds.table.regionLabel')}
            className="mb-6"
          >
            <h2 className="mb-2 text-lg font-semibold text-slate-900">
              {t('mobileBuilds.table.title', { count: sortedBuilds.length })}
            </h2>
            {sortedBuilds.length === 0 ? (
              <p data-testid="builds-empty" className="text-sm text-slate-500">
                {t('mobileBuilds.table.empty')}
              </p>
            ) : (
              <div className="overflow-x-auto rounded border border-slate-200 bg-white">
                <table className="w-full text-left text-sm" data-testid="builds-table">
                  <thead className="border-b border-slate-200 bg-slate-50">
                    <tr>
                      <th scope="col" className="px-3 py-2 font-medium text-slate-600">
                        {t('mobileBuilds.columns.platform')}
                      </th>
                      <th scope="col" className="px-3 py-2 font-medium text-slate-600">
                        {t('mobileBuilds.columns.version')}
                      </th>
                      <th scope="col" className="px-3 py-2 font-medium text-slate-600">
                        {t('mobileBuilds.columns.status')}
                      </th>
                      <th scope="col" className="px-3 py-2 font-medium text-slate-600">
                        {t('mobileBuilds.columns.brandConfig')}
                      </th>
                      <th scope="col" className="px-3 py-2 font-medium text-slate-600">
                        {t('mobileBuilds.columns.created')}
                      </th>
                      <th scope="col" className="px-3 py-2 font-medium text-slate-600">
                        {t('mobileBuilds.columns.actions')}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedBuilds.map((build) => (
                      <tr
                        key={build.id}
                        data-testid={`build-row-${build.id}`}
                        className="border-b border-slate-100 hover:bg-slate-50"
                      >
                        <td className="px-3 py-2">
                          <PlatformLabel platform={build.platform} />
                        </td>
                        <td className="px-3 py-2 font-mono text-xs">{build.version}</td>
                        <td className="px-3 py-2">
                          <BuildStatusBadge status={build.status} />
                        </td>
                        <td className="px-3 py-2">{build.brandConfigName}</td>
                        <td className="px-3 py-2 text-slate-500">
                          {new Date(build.createdAtIso).toLocaleString()}
                        </td>
                        <td className="px-3 py-2">
                          <button
                            onClick={() => setSelectedBuild(build)}
                            aria-label={t('mobileBuilds.actions.viewDetail', { id: build.id })}
                            className="text-blue-600 underline hover:text-blue-800"
                          >
                            {t('mobileBuilds.actions.details')}
                          </button>
                          {build.artifacts.length > 0 && (
                            <a
                              href={build.artifacts[0]!.downloadUrl}
                              className="ml-3 text-blue-600 underline hover:text-blue-800"
                              aria-label={t('mobileBuilds.actions.downloadAriaLabel', {
                                id: build.id,
                              })}
                            >
                              {t('mobileBuilds.actions.download')}
                            </a>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>

          {/* Credentials Section */}
          <section
            role="region"
            aria-label={t('mobileBuilds.credentials.regionLabel')}
            className="mb-6"
            data-testid="credentials-section"
          >
            <h2 className="mb-2 text-lg font-semibold text-slate-900">
              {t('mobileBuilds.credentials.title')}
            </h2>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* Apple */}
              <div className="rounded border border-slate-200 bg-white p-4">
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-slate-900">
                    {t('mobileBuilds.credentials.apple.title')}
                  </h3>
                  <CredentialStatusBadge status={snapshot.appleCredential.status} />
                </div>
                <dl className="space-y-1 text-sm">
                  <div className="flex gap-2">
                    <dt className="font-medium text-slate-600">
                      {t('mobileBuilds.credentials.apple.teamId')}:
                    </dt>
                    <dd data-testid="apple-team-id">{snapshot.appleCredential.teamId}</dd>
                  </div>
                  <div className="flex gap-2">
                    <dt className="font-medium text-slate-600">
                      {t('mobileBuilds.credentials.apple.teamName')}:
                    </dt>
                    <dd>{snapshot.appleCredential.teamName}</dd>
                  </div>
                </dl>
              </div>

              {/* Google */}
              <div className="rounded border border-slate-200 bg-white p-4">
                <div className="mb-2 flex items-center justify-between">
                  <h3 className="text-sm font-semibold text-slate-900">
                    {t('mobileBuilds.credentials.google.title')}
                  </h3>
                  <CredentialStatusBadge status={snapshot.googleCredential.status} />
                </div>
                <dl className="space-y-1 text-sm">
                  <div className="flex gap-2">
                    <dt className="font-medium text-slate-600">
                      {t('mobileBuilds.credentials.google.serviceAccount')}:
                    </dt>
                    <dd data-testid="google-service-account" className="truncate">
                      {snapshot.googleCredential.serviceAccountEmail}
                    </dd>
                  </div>
                  <div className="flex gap-2">
                    <dt className="font-medium text-slate-600">
                      {t('mobileBuilds.credentials.google.projectId')}:
                    </dt>
                    <dd>{snapshot.googleCredential.projectId}</dd>
                  </div>
                </dl>
              </div>
            </div>
          </section>

          {/* Distribution Config */}
          <section
            role="region"
            aria-label={t('mobileBuilds.distribution.regionLabel')}
            data-testid="distribution-section"
          >
            <h2 className="mb-2 text-lg font-semibold text-slate-900">
              {t('mobileBuilds.distribution.title')}
            </h2>
            <div className="rounded border border-slate-200 bg-white p-4">
              <dl className="grid grid-cols-1 gap-3 text-sm sm:grid-cols-3">
                <div>
                  <dt className="font-medium text-slate-600">
                    {t('mobileBuilds.distribution.testFlightGroup')}
                  </dt>
                  <dd data-testid="testflight-group">
                    {snapshot.distributionConfig.testFlightGroupName}
                  </dd>
                </div>
                <div>
                  <dt className="font-medium text-slate-600">
                    {t('mobileBuilds.distribution.playConsoleTrack')}
                  </dt>
                  <dd data-testid="play-console-track">
                    {t(
                      `mobileBuilds.distribution.track.${snapshot.distributionConfig.playConsoleTrack}`,
                    )}
                  </dd>
                </div>
                <div>
                  <dt className="font-medium text-slate-600">
                    {t('mobileBuilds.distribution.autoSubmit')}
                  </dt>
                  <dd data-testid="auto-submit">
                    {snapshot.distributionConfig.autoSubmit
                      ? t('mobileBuilds.distribution.enabled')
                      : t('mobileBuilds.distribution.disabled')}
                  </dd>
                </div>
              </dl>
            </div>
          </section>

          {/* Trigger Build Dialog */}
          <TriggerBuildDialog
            open={dialogOpen}
            onClose={() => setDialogOpen(false)}
            onSubmit={handleTrigger}
            brandConfigs={snapshot.availableBrandConfigs}
            isSubmitting={triggerMutation.isPending}
          />

          {/* Build Detail Drawer */}
          <BuildDetailDrawer build={selectedBuild} onClose={() => setSelectedBuild(null)} />
        </>
      ) : null}
    </main>
  );
}

// Default export for React.lazy().
export default MobileBuildsPage;
