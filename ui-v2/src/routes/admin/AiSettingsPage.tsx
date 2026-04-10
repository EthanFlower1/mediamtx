// KAI-327: Customer Admin AI Settings + Face Vault management page.
//
// Compliance-sensitive scaffold. Face recognition is regulated under:
//   - EU AI Act Art 5/13/27
//   - GDPR Article 9 (special-category biometric data)
//
// Every destructive action requires typed confirmation. Every
// face-recognition enablement requires an explicit GDPR Art 9 /
// EU AI Act Art 27 acknowledgement. Status is always encoded via
// icon + text + border, never colour alone.
//
// TODO(lead-security): this page is marked for lead-security review
// before ready-for-review. Outstanding items:
//   1. Final consent disclosure copy (GDPR Art 9(2)(a) + EU AI Act Art 52)
//   2. Audit event type list
//   3. Thumbnail display policy (full / blurred / embedding-only)
//   4. Two-person purge approval rule vs single-admin strong-confirm

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

import {
  aiSettingsQueryKeys,
  downloadTransparencyReport,
  listAiFeatures,
  setAiFeatureEnabled,
  type AiFeatureKey,
  type AiFeatureState,
} from '@/api/aiSettings';
import {
  deleteFaceEnrollment,
  enrollFace,
  faceVaultQueryKeys,
  getFaceVaultSummary,
  listFaceEnrollments,
  purgeFaceVault,
  revokeConsent,
  type ConsentStatus,
  type FaceEnrollment,
  type FaceVaultSummary,
  type LegalBasis,
  type PurgeScope,
} from '@/api/faceVault';
import { useSessionStore } from '@/stores/session';

// ---------------------------------------------------------------------------
// Feature metadata
// ---------------------------------------------------------------------------

interface FeatureMeta {
  key: AiFeatureKey;
  nameKey: string;
  descriptionKey: string;
}

const FEATURE_META: FeatureMeta[] = [
  {
    key: 'object-detection',
    nameKey: 'aiSettings.features.objectDetection.name',
    descriptionKey: 'aiSettings.features.objectDetection.description',
  },
  {
    key: 'face-recognition',
    nameKey: 'aiSettings.features.faceRecognition.name',
    descriptionKey: 'aiSettings.features.faceRecognition.description',
  },
  {
    key: 'lpr',
    nameKey: 'aiSettings.features.lpr.name',
    descriptionKey: 'aiSettings.features.lpr.description',
  },
  {
    key: 'behavioral-analytics',
    nameKey: 'aiSettings.features.behavioralAnalytics.name',
    descriptionKey: 'aiSettings.features.behavioralAnalytics.description',
  },
  {
    key: 'semantic-search',
    nameKey: 'aiSettings.features.semanticSearch.name',
    descriptionKey: 'aiSettings.features.semanticSearch.description',
  },
  {
    key: 'custom-models',
    nameKey: 'aiSettings.features.customModels.name',
    descriptionKey: 'aiSettings.features.customModels.description',
  },
];

const LEGAL_BASIS_OPTIONS: { value: LegalBasis; labelKey: string }[] = [
  { value: 'consent', labelKey: 'faceVault.legalBasis.consent' },
  { value: 'art9-explicit-consent', labelKey: 'faceVault.legalBasis.art9Explicit' },
  { value: 'legitimate-interest', labelKey: 'faceVault.legalBasis.legitimateInterest' },
  { value: 'vital-interest', labelKey: 'faceVault.legalBasis.vitalInterest' },
  { value: 'legal-obligation', labelKey: 'faceVault.legalBasis.legalObligation' },
  { value: 'public-task', labelKey: 'faceVault.legalBasis.publicTask' },
  { value: 'art9-vital-interest', labelKey: 'faceVault.legalBasis.art9VitalInterest' },
  { value: 'art9-public-interest', labelKey: 'faceVault.legalBasis.art9PublicInterest' },
];

const RETENTION_OPTIONS: number[] = [30, 90, 180, 365];

type ConsentFilter = ConsentStatus | 'all';

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function AiSettingsPage(): JSX.Element {
  const { t } = useTranslation();
  const tenantId = useSessionStore((s) => s.tenantId);
  const tenantName = useSessionStore((s) => s.tenantName);
  const queryClient = useQueryClient();

  // Modal state for feature toggles
  const [enableAckOpen, setEnableAckOpen] = useState(false);
  const [disableWarnOpen, setDisableWarnOpen] = useState(false);
  const [ackChecked, setAckChecked] = useState(false);

  // Face vault local UI state
  const [search, setSearch] = useState('');
  const [consentFilter, setConsentFilter] = useState<ConsentFilter>('all');
  const [addOpen, setAddOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<FaceEnrollment | null>(null);
  const [revokeTarget, setRevokeTarget] = useState<FaceEnrollment | null>(null);
  const [purgeOpen, setPurgeOpen] = useState(false);

  // Queries
  const featuresQuery = useQuery<AiFeatureState[]>({
    queryKey: aiSettingsQueryKeys.features(tenantId),
    queryFn: () => listAiFeatures(tenantId),
  });

  const faceEnabled = useMemo(
    () => featuresQuery.data?.find((f) => f.key === 'face-recognition')?.enabled ?? false,
    [featuresQuery.data],
  );

  const vaultFilters = useMemo(
    () => ({ tenantId, search, consentStatus: consentFilter }),
    [tenantId, search, consentFilter],
  );

  const summaryQuery = useQuery<FaceVaultSummary>({
    queryKey: faceVaultQueryKeys.summary(tenantId),
    queryFn: () => getFaceVaultSummary(tenantId),
    enabled: faceEnabled,
  });

  const enrollmentsQuery = useQuery<FaceEnrollment[]>({
    queryKey: faceVaultQueryKeys.list(tenantId, {
      search,
      consentStatus: consentFilter,
    }),
    queryFn: () => listFaceEnrollments(vaultFilters),
    enabled: faceEnabled,
  });

  // Mutations
  const invalidateFeatures = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: aiSettingsQueryKeys.all(tenantId),
    });
  }, [queryClient, tenantId]);

  const invalidateVault = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: faceVaultQueryKeys.all(tenantId),
    });
  }, [queryClient, tenantId]);

  const toggleMutation = useMutation({
    mutationFn: async (args: {
      key: AiFeatureKey;
      enabled: boolean;
      withAck: boolean;
    }) => {
      if (args.key === 'face-recognition' && args.enabled && args.withAck) {
        return setAiFeatureEnabled(tenantId, args.key, true, {
          lawfulBasisConfirmed: true,
          aiActImpactAssessmentCompleted: true,
          acknowledgedAt: new Date().toISOString(),
          acknowledgedBy: 'mock-admin',
        });
      }
      return setAiFeatureEnabled(tenantId, args.key, args.enabled);
    },
    onSuccess: invalidateFeatures,
  });

  const enrollMutation = useMutation({
    mutationFn: enrollFace,
    onSuccess: invalidateVault,
  });

  const revokeMutation = useMutation({
    mutationFn: (enrollmentId: string) => revokeConsent(tenantId, enrollmentId),
    onSuccess: invalidateVault,
  });

  const deleteMutation = useMutation({
    mutationFn: (enrollmentId: string) =>
      deleteFaceEnrollment({
        tenantId,
        enrollmentId,
        confirmation: 'DELETE',
      }),
    onSuccess: invalidateVault,
  });

  const purgeMutation = useMutation({
    mutationFn: (args: {
      scope: PurgeScope;
      confirmation: string;
      fromDate?: string;
      toDate?: string;
      cameraId?: string;
    }) =>
      purgeFaceVault({
        tenantId,
        scope: args.scope,
        confirmation: args.confirmation,
        fromDate: args.fromDate,
        toDate: args.toDate,
        cameraId: args.cameraId,
      }),
    onSuccess: invalidateVault,
  });

  // Toggle handler — intercepts face-recognition to open modals
  const handleToggleFeature = useCallback(
    (feature: AiFeatureState) => {
      if (feature.key === 'face-recognition') {
        if (feature.enabled) {
          setDisableWarnOpen(true);
        } else {
          setAckChecked(false);
          setEnableAckOpen(true);
        }
        return;
      }
      toggleMutation.mutate({
        key: feature.key,
        enabled: !feature.enabled,
        withAck: false,
      });
    },
    [toggleMutation],
  );

  // Transparency report download
  const [reportHandle, setReportHandle] = useState<string | null>(null);
  const handleDownloadReport = useCallback(async () => {
    const report = await downloadTransparencyReport(tenantId);
    setReportHandle(report.downloadHandle);
  }, [tenantId]);

  return (
    <main
      aria-label={t('aiSettings.pageLabel')}
      data-testid="ai-settings-page"
      className="ai-settings-page"
    >
      <nav aria-label={t('aiSettings.breadcrumbAriaLabel')}>
        <ol>
          <li>{tenantName}</li>
          <li aria-current="page">{t('aiSettings.breadcrumb')}</li>
        </ol>
      </nav>

      <header>
        <h1>{t('aiSettings.title')}</h1>
      </header>

      {/* Compliance banner — always visible */}
      <section
        role="region"
        aria-label={t('aiSettings.complianceBanner.title')}
        data-testid="ai-settings-compliance-banner"
        className="ai-settings-page__banner"
      >
        <span aria-hidden="true" className="ai-settings-page__banner-icon">
          {'\u24D8'}
        </span>
        <div>
          <h2>{t('aiSettings.complianceBanner.title')}</h2>
          <p>{t('aiSettings.complianceBanner.body')}</p>
          <ul>
            <li>
              <a href="/docs/compliance/face-recognition" data-testid="compliance-docs-link">
                {t('aiSettings.complianceBanner.docsLink')}
              </a>
            </li>
            <li>
              <a href="/admin/audit-log?filter=face" data-testid="compliance-audit-link">
                {t('aiSettings.complianceBanner.auditLink')}
              </a>
            </li>
            <li>
              <button
                type="button"
                onClick={handleDownloadReport}
                data-testid="transparency-report-button"
              >
                {t('aiSettings.complianceBanner.downloadReport')}
              </button>
              {reportHandle !== null && (
                <span role="status" data-testid="transparency-report-handle">
                  {reportHandle}
                </span>
              )}
            </li>
          </ul>
        </div>
      </section>

      {/* Section A — Feature toggles */}
      <section
        aria-label={t('aiSettings.sections.features')}
        data-testid="ai-feature-toggles"
      >
        <h2>{t('aiSettings.sections.features')}</h2>
        {featuresQuery.isLoading && (
          <p role="status" aria-live="polite">
            {t('aiSettings.loading')}
          </p>
        )}
        {featuresQuery.isError && <p role="alert">{t('aiSettings.error')}</p>}
        {featuresQuery.isSuccess && (
          <ul className="ai-settings-page__feature-list">
            {FEATURE_META.map((meta) => {
              const state = featuresQuery.data.find((f) => f.key === meta.key);
              if (!state) return null;
              return (
                <FeatureCard
                  key={meta.key}
                  state={state}
                  nameKey={meta.nameKey}
                  descriptionKey={meta.descriptionKey}
                  onToggle={handleToggleFeature}
                />
              );
            })}
          </ul>
        )}
      </section>

      {/* Section B — Face Vault (gated on faceEnabled) */}
      {faceEnabled ? (
        <section
          aria-label={t('aiSettings.sections.faceVault')}
          data-testid="face-vault-section"
        >
          <h2>{t('faceVault.title')}</h2>

          {/* Summary card */}
          {summaryQuery.isLoading && (
            <p role="status" aria-live="polite">
              {t('faceVault.list.loading')}
            </p>
          )}
          {summaryQuery.isSuccess && (
            <dl data-testid="face-vault-summary">
              <div>
                <dt>{t('faceVault.summary.total')}</dt>
                <dd data-testid="face-vault-summary-total">
                  {summaryQuery.data.totalEnrollments}
                </dd>
              </div>
              <div>
                <dt>{t('faceVault.summary.active')}</dt>
                <dd>{summaryQuery.data.activeEnrollments}</dd>
              </div>
              <div>
                <dt>{t('faceVault.summary.expiringSoon')}</dt>
                <dd>{summaryQuery.data.expiringSoonCount}</dd>
              </div>
              <div>
                <dt>{t('faceVault.summary.lastPurge')}</dt>
                <dd>
                  {summaryQuery.data.lastPurgeAt ??
                    t('faceVault.summary.lastPurgeNever')}
                </dd>
              </div>
              <div>
                <dt>{t('faceVault.summary.retentionPolicy')}</dt>
                <dd>
                  {t('faceVault.summary.retentionPolicyValue', {
                    days: summaryQuery.data.retentionPolicyDays,
                  })}
                </dd>
              </div>
            </dl>
          )}

          {/* Toolbar */}
          <div
            role="group"
            aria-label={t('faceVault.toolbar.ariaLabel')}
            className="face-vault__toolbar"
          >
            <label>
              <span className="sr-only">{t('faceVault.toolbar.searchLabel')}</span>
              <input
                type="search"
                placeholder={t('faceVault.toolbar.searchPlaceholder')}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                data-testid="face-vault-search"
                aria-label={t('faceVault.toolbar.searchLabel')}
              />
            </label>
            <label>
              <span className="sr-only">
                {t('faceVault.toolbar.consentFilterLabel')}
              </span>
              <select
                value={consentFilter}
                onChange={(e) => setConsentFilter(e.target.value as ConsentFilter)}
                data-testid="face-vault-consent-filter"
                aria-label={t('faceVault.toolbar.consentFilterLabel')}
              >
                <option value="all">{t('faceVault.toolbar.allConsent')}</option>
                <option value="granted">{t('faceVault.consent.granted')}</option>
                <option value="revoked">{t('faceVault.consent.revoked')}</option>
                <option value="expired">{t('faceVault.consent.expired')}</option>
                <option value="not-required">{t('faceVault.consent.notRequired')}</option>
              </select>
            </label>
            <button
              type="button"
              onClick={() => setAddOpen(true)}
              data-testid="face-vault-add-button"
            >
              {t('faceVault.actions.add')}
            </button>
          </div>

          {/* Enrollment list */}
          {enrollmentsQuery.isLoading && (
            <p role="status" aria-live="polite">
              {t('faceVault.list.loading')}
            </p>
          )}
          {enrollmentsQuery.isError && (
            <p role="alert">{t('faceVault.list.error')}</p>
          )}
          {enrollmentsQuery.isSuccess && enrollmentsQuery.data.length === 0 && (
            <div data-testid="face-vault-empty">
              <p>{t('faceVault.empty.title')}</p>
              <p>{t('faceVault.empty.body')}</p>
            </div>
          )}
          {enrollmentsQuery.isSuccess && enrollmentsQuery.data.length > 0 && (
            <table
              aria-label={t('faceVault.list.sectionLabel')}
              data-testid="face-vault-table"
            >
              <thead>
                <tr>
                  <th scope="col">{t('faceVault.columns.subject')}</th>
                  <th scope="col">{t('faceVault.columns.enrolled')}</th>
                  <th scope="col">{t('faceVault.columns.consent')}</th>
                  <th scope="col">{t('faceVault.columns.retention')}</th>
                  <th scope="col">{t('faceVault.columns.source')}</th>
                  <th scope="col">{t('faceVault.columns.actions')}</th>
                </tr>
              </thead>
              <tbody>
                {enrollmentsQuery.data.map((e) => (
                  <tr key={e.id} data-testid={`face-vault-row-${e.id}`}>
                    <td>
                      <span>{e.subjectName}</span>
                      <small>{e.subjectIdentifier}</small>
                    </td>
                    <td>{new Date(e.enrolledAt).toLocaleDateString()}</td>
                    <td>
                      <ConsentBadge status={e.consentStatus} />
                    </td>
                    <td>{new Date(e.expiresAt).toLocaleDateString()}</td>
                    <td>{t(`faceVault.source.${e.source}`)}</td>
                    <td>
                      <button
                        type="button"
                        aria-label={t('faceVault.actions.revokeConsentAriaLabel', {
                          name: e.subjectName,
                        })}
                        onClick={() => setRevokeTarget(e)}
                        data-testid={`face-vault-revoke-${e.id}`}
                        disabled={e.consentStatus === 'revoked'}
                      >
                        {t('faceVault.actions.revokeConsent')}
                      </button>
                      <button
                        type="button"
                        aria-label={t('faceVault.actions.deleteAriaLabel', {
                          name: e.subjectName,
                        })}
                        onClick={() => setDeleteTarget(e)}
                        data-testid={`face-vault-delete-${e.id}`}
                      >
                        {t('faceVault.actions.delete')}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {/* Section C — Retention (scaffold) */}
          <section
            aria-label={t('aiSettings.sections.retention')}
            data-testid="retention-section"
          >
            <h3>{t('aiSettings.retention.heading')}</h3>
            <label>
              <span>{t('aiSettings.retention.defaultLabel')}</span>
              <select
                defaultValue={90}
                data-testid="retention-default-select"
                aria-label={t('aiSettings.retention.defaultLabel')}
              >
                {RETENTION_OPTIONS.map((d) => (
                  <option key={d} value={d}>
                    {t(`aiSettings.retention.days.${d}`)}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <input
                type="checkbox"
                defaultChecked
                data-testid="retention-auto-expire"
              />
              <span>{t('aiSettings.retention.autoExpire')}</span>
            </label>
            <button type="button" data-testid="retention-apply-all">
              {t('aiSettings.retention.applyAll')}
            </button>
          </section>

          {/* Emergency purge */}
          <div className="face-vault__purge-zone">
            <button
              type="button"
              onClick={() => setPurgeOpen(true)}
              data-testid="face-vault-purge-button"
              className="face-vault__purge-button"
              aria-label={t('faceVault.purge.title')}
            >
              <span aria-hidden="true">{'\u26A0'}</span>
              {t('faceVault.purge.button')}
            </button>
          </div>
        </section>
      ) : (
        <section
          data-testid="face-vault-disabled-hint"
          aria-label={t('aiSettings.sections.faceVault')}
        >
          <p>{t('faceVault.disabledHint')}</p>
        </section>
      )}

      {/* ===== Modals ===== */}

      {enableAckOpen && (
        <EnableFaceRecognitionDialog
          ackChecked={ackChecked}
          onAckChange={setAckChecked}
          onCancel={() => setEnableAckOpen(false)}
          onConfirm={() => {
            toggleMutation.mutate({
              key: 'face-recognition',
              enabled: true,
              withAck: true,
            });
            setEnableAckOpen(false);
          }}
        />
      )}

      {disableWarnOpen && (
        <DisableFaceRecognitionDialog
          onCancel={() => setDisableWarnOpen(false)}
          onConfirm={() => {
            toggleMutation.mutate({
              key: 'face-recognition',
              enabled: false,
              withAck: false,
            });
            setDisableWarnOpen(false);
          }}
        />
      )}

      {addOpen && (
        <AddEnrollmentDialog
          onCancel={() => setAddOpen(false)}
          onSubmit={(values) => {
            enrollMutation.mutate({
              tenantId,
              subjectName: values.subjectName,
              subjectIdentifier: values.subjectIdentifier,
              legalBasis: values.legalBasis,
              consentGranted: values.consentGranted,
              retentionDays: values.retentionDays,
              thumbnailDataUrl: null,
            });
            setAddOpen(false);
          }}
        />
      )}

      {deleteTarget && (
        <DeleteEnrollmentDialog
          enrollment={deleteTarget}
          onCancel={() => setDeleteTarget(null)}
          onConfirm={() => {
            deleteMutation.mutate(deleteTarget.id);
            setDeleteTarget(null);
          }}
        />
      )}

      {revokeTarget && (
        <RevokeConsentDialog
          enrollment={revokeTarget}
          onCancel={() => setRevokeTarget(null)}
          onConfirm={() => {
            revokeMutation.mutate(revokeTarget.id);
            setRevokeTarget(null);
          }}
        />
      )}

      {purgeOpen && (
        <EmergencyPurgeDialog
          tenantName={tenantName}
          onCancel={() => setPurgeOpen(false)}
          onConfirm={(args) => {
            purgeMutation.mutate(args);
            setPurgeOpen(false);
          }}
        />
      )}
    </main>
  );
}

// ---------------------------------------------------------------------------
// FeatureCard — status encoded via icon + text + border
// ---------------------------------------------------------------------------

interface FeatureCardProps {
  state: AiFeatureState;
  nameKey: string;
  descriptionKey: string;
  onToggle: (state: AiFeatureState) => void;
}

function FeatureCard({
  state,
  nameKey,
  descriptionKey,
  onToggle,
}: FeatureCardProps): JSX.Element {
  const { t } = useTranslation();
  const statusLabel = state.enabled
    ? t('aiSettings.status.enabled')
    : t('aiSettings.status.disabled');
  const statusIcon = state.enabled ? '\u2714' : '\u2716';
  const borderClass = state.enabled
    ? 'feature-card--enabled'
    : 'feature-card--disabled';
  const disabled =
    state.requiresEntitlement && !state.entitled && !state.enabled;
  const name = t(nameKey);
  return (
    <li
      data-testid={`feature-card-${state.key}`}
      data-enabled={state.enabled ? 'true' : 'false'}
      className={`feature-card ${borderClass}`}
    >
      <div className="feature-card__header">
        <h3>{name}</h3>
        <span
          role="status"
          aria-label={t('aiSettings.status.statusAriaLabel')}
          data-testid={`feature-status-${state.key}`}
          className={`feature-card__status ${borderClass}`}
        >
          <span aria-hidden="true">{statusIcon}</span>
          <span>{statusLabel}</span>
        </span>
      </div>
      <p>{t(descriptionKey)}</p>
      {state.requiresEntitlement && !state.entitled && (
        <span
          data-testid={`feature-entitlement-${state.key}`}
          className="feature-card__entitlement"
        >
          <span aria-hidden="true">{'\u26A0'}</span>
          {t('aiSettings.status.entitlementRequired')}
        </span>
      )}
      <button
        type="button"
        role="switch"
        aria-checked={state.enabled}
        aria-label={t('aiSettings.toggleAriaLabel', { name })}
        onClick={() => onToggle(state)}
        disabled={disabled}
        aria-disabled={disabled}
        data-testid={`feature-toggle-${state.key}`}
      >
        {state.enabled
          ? t('aiSettings.status.enabled')
          : t('aiSettings.status.disabled')}
      </button>
    </li>
  );
}

function ConsentBadge({ status }: { status: ConsentStatus }): JSX.Element {
  const { t } = useTranslation();
  const map: Record<ConsentStatus, { icon: string; labelKey: string; cls: string }> = {
    granted: {
      icon: '\u2714',
      labelKey: 'faceVault.consent.granted',
      cls: 'consent-badge--granted',
    },
    revoked: {
      icon: '\u2716',
      labelKey: 'faceVault.consent.revoked',
      cls: 'consent-badge--revoked',
    },
    expired: {
      icon: '\u23F3',
      labelKey: 'faceVault.consent.expired',
      cls: 'consent-badge--expired',
    },
    'not-required': {
      icon: '\u2013',
      labelKey: 'faceVault.consent.notRequired',
      cls: 'consent-badge--none',
    },
  };
  const entry = map[status];
  return (
    <span
      className={`consent-badge ${entry.cls}`}
      data-testid={`consent-badge-${status}`}
    >
      <span aria-hidden="true">{entry.icon}</span>
      <span>{t(entry.labelKey)}</span>
    </span>
  );
}

// ---------------------------------------------------------------------------
// Dialog components
// ---------------------------------------------------------------------------

interface EnableFaceRecognitionDialogProps {
  ackChecked: boolean;
  onAckChange: (checked: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void;
}

function EnableFaceRecognitionDialog({
  ackChecked,
  onAckChange,
  onCancel,
  onConfirm,
}: EnableFaceRecognitionDialogProps): JSX.Element {
  const { t } = useTranslation();
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="enable-face-title"
      data-testid="enable-face-dialog"
    >
      <h2 id="enable-face-title">{t('aiSettings.enableFaceRecognitionAck.title')}</h2>
      <p>{t('aiSettings.enableFaceRecognitionAck.body')}</p>
      <label>
        <input
          type="checkbox"
          checked={ackChecked}
          onChange={(e) => onAckChange(e.target.checked)}
          data-testid="enable-face-ack-checkbox"
        />
        <span>{t('aiSettings.enableFaceRecognitionAck.checkbox')}</span>
      </label>
      <div>
        <button type="button" onClick={onCancel} data-testid="enable-face-cancel">
          {t('aiSettings.enableFaceRecognitionAck.cancel')}
        </button>
        <button
          type="button"
          onClick={onConfirm}
          disabled={!ackChecked}
          aria-disabled={!ackChecked}
          data-testid="enable-face-confirm"
        >
          {t('aiSettings.enableFaceRecognitionAck.confirm')}
        </button>
      </div>
    </div>
  );
}

interface DisableFaceRecognitionDialogProps {
  onCancel: () => void;
  onConfirm: () => void;
}

function DisableFaceRecognitionDialog({
  onCancel,
  onConfirm,
}: DisableFaceRecognitionDialogProps): JSX.Element {
  const { t } = useTranslation();
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="disable-face-title"
      data-testid="disable-face-dialog"
    >
      <h2 id="disable-face-title">
        {t('aiSettings.disableFaceRecognitionWarning.title')}
      </h2>
      <p>{t('aiSettings.disableFaceRecognitionWarning.body')}</p>
      <button type="button" onClick={onCancel} data-testid="disable-face-cancel">
        {t('aiSettings.disableFaceRecognitionWarning.cancel')}
      </button>
      <button type="button" onClick={onConfirm} data-testid="disable-face-confirm">
        {t('aiSettings.disableFaceRecognitionWarning.confirm')}
      </button>
    </div>
  );
}

interface AddEnrollmentValues {
  subjectName: string;
  subjectIdentifier: string;
  legalBasis: LegalBasis;
  consentGranted: boolean;
  retentionDays: number;
}

interface AddEnrollmentDialogProps {
  onCancel: () => void;
  onSubmit: (values: AddEnrollmentValues) => void;
}

function AddEnrollmentDialog({
  onCancel,
  onSubmit,
}: AddEnrollmentDialogProps): JSX.Element {
  const { t } = useTranslation();
  const [subjectName, setSubjectName] = useState('');
  const [subjectIdentifier, setSubjectIdentifier] = useState('');
  const [legalBasis, setLegalBasis] = useState<LegalBasis>('art9-explicit-consent');
  const [consentGranted, setConsentGranted] = useState(false);
  const [retentionDays, setRetentionDays] = useState<number>(90);
  const [error, setError] = useState<string | null>(null);

  const requiresConsent =
    legalBasis === 'consent' || legalBasis === 'art9-explicit-consent';

  const handleSubmit = (): void => {
    if (!subjectName.trim()) {
      setError(t('faceVault.addEnrollment.errors.nameRequired'));
      return;
    }
    if (!subjectIdentifier.trim()) {
      setError(t('faceVault.addEnrollment.errors.identifierRequired'));
      return;
    }
    if (requiresConsent && !consentGranted) {
      setError(t('faceVault.addEnrollment.errors.consentRequired'));
      return;
    }
    setError(null);
    onSubmit({
      subjectName: subjectName.trim(),
      subjectIdentifier: subjectIdentifier.trim(),
      legalBasis,
      consentGranted,
      retentionDays,
    });
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="add-enrollment-title"
      data-testid="add-enrollment-dialog"
    >
      <h2 id="add-enrollment-title">{t('faceVault.addEnrollment.title')}</h2>
      <label>
        <span>{t('faceVault.addEnrollment.name')}</span>
        <input
          type="text"
          value={subjectName}
          onChange={(e) => setSubjectName(e.target.value)}
          data-testid="add-enrollment-name"
        />
      </label>
      <label>
        <span>{t('faceVault.addEnrollment.identifier')}</span>
        <input
          type="text"
          value={subjectIdentifier}
          onChange={(e) => setSubjectIdentifier(e.target.value)}
          data-testid="add-enrollment-identifier"
        />
      </label>
      <label>
        <span>{t('faceVault.addEnrollment.legalBasis')}</span>
        <select
          value={legalBasis}
          onChange={(e) => setLegalBasis(e.target.value as LegalBasis)}
          data-testid="add-enrollment-legal-basis"
        >
          {LEGAL_BASIS_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {t(opt.labelKey)}
            </option>
          ))}
        </select>
      </label>
      {requiresConsent && (
        <label>
          <input
            type="checkbox"
            checked={consentGranted}
            onChange={(e) => setConsentGranted(e.target.checked)}
            data-testid="add-enrollment-consent"
          />
          <span>{t('faceVault.addEnrollment.consent')}</span>
          {/* TODO(lead-security): replace placeholder with approved legal copy */}
          <small data-testid="add-enrollment-consent-placeholder">
            {t('faceVault.addEnrollment.consentPlaceholder')}
          </small>
        </label>
      )}
      <fieldset>
        <legend>{t('faceVault.addEnrollment.image')}</legend>
        <input
          type="file"
          accept="image/*"
          data-testid="add-enrollment-image"
          // Stub only: selecting a file does not upload anywhere.
        />
        <small>{t('faceVault.addEnrollment.imageHint')}</small>
      </fieldset>
      <label>
        <span>{t('faceVault.addEnrollment.retention')}</span>
        <select
          value={retentionDays}
          onChange={(e) => setRetentionDays(Number(e.target.value))}
          data-testid="add-enrollment-retention"
        >
          {RETENTION_OPTIONS.map((d) => (
            <option key={d} value={d}>
              {t(`aiSettings.retention.days.${d}`)}
            </option>
          ))}
        </select>
      </label>
      {error !== null && (
        <p role="alert" data-testid="add-enrollment-error">
          {error}
        </p>
      )}
      <button type="button" onClick={onCancel} data-testid="add-enrollment-cancel">
        {t('faceVault.addEnrollment.cancel')}
      </button>
      <button type="button" onClick={handleSubmit} data-testid="add-enrollment-submit">
        {t('faceVault.addEnrollment.submit')}
      </button>
    </div>
  );
}

interface DeleteEnrollmentDialogProps {
  enrollment: FaceEnrollment;
  onCancel: () => void;
  onConfirm: () => void;
}

function DeleteEnrollmentDialog({
  enrollment,
  onCancel,
  onConfirm,
}: DeleteEnrollmentDialogProps): JSX.Element {
  const { t } = useTranslation();
  const [typed, setTyped] = useState('');
  const ready = typed === 'DELETE';
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-enrollment-title"
      data-testid="delete-enrollment-dialog"
    >
      <h2 id="delete-enrollment-title">
        {t('faceVault.deleteConfirm.title', { name: enrollment.subjectName })}
      </h2>
      <p>{t('faceVault.deleteConfirm.body')}</p>
      <label>
        <span>{t('faceVault.deleteConfirm.typePrompt')}</span>
        <input
          type="text"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          data-testid="delete-enrollment-type-input"
        />
      </label>
      <button type="button" onClick={onCancel} data-testid="delete-enrollment-cancel">
        {t('faceVault.deleteConfirm.cancel')}
      </button>
      <button
        type="button"
        disabled={!ready}
        aria-disabled={!ready}
        onClick={onConfirm}
        data-testid="delete-enrollment-confirm"
      >
        {t('faceVault.deleteConfirm.confirm')}
      </button>
    </div>
  );
}

interface RevokeConsentDialogProps {
  enrollment: FaceEnrollment;
  onCancel: () => void;
  onConfirm: () => void;
}

function RevokeConsentDialog({
  enrollment,
  onCancel,
  onConfirm,
}: RevokeConsentDialogProps): JSX.Element {
  const { t } = useTranslation();
  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="revoke-consent-title"
      data-testid="revoke-consent-dialog"
    >
      <h2 id="revoke-consent-title">
        {t('faceVault.revokeConsent.title', { name: enrollment.subjectName })}
      </h2>
      <p>{t('faceVault.revokeConsent.body')}</p>
      <button type="button" onClick={onCancel} data-testid="revoke-consent-cancel">
        {t('faceVault.revokeConsent.cancel')}
      </button>
      <button type="button" onClick={onConfirm} data-testid="revoke-consent-confirm">
        {t('faceVault.revokeConsent.confirm')}
      </button>
    </div>
  );
}

interface EmergencyPurgeDialogProps {
  tenantName: string;
  onCancel: () => void;
  onConfirm: (args: {
    scope: PurgeScope;
    confirmation: string;
    fromDate?: string;
    toDate?: string;
    cameraId?: string;
  }) => void;
}

function EmergencyPurgeDialog({
  tenantName,
  onCancel,
  onConfirm,
}: EmergencyPurgeDialogProps): JSX.Element {
  const { t } = useTranslation();
  const [step, setStep] = useState<1 | 2 | 3>(1);
  const [scope, setScope] = useState<PurgeScope>('tenant');
  const [cameraId, setCameraId] = useState('');
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const [typed, setTyped] = useState('');
  const sentinel = `PURGE-TENANT-${tenantName}`;
  const ready = typed === sentinel;

  // TODO(lead-security): confirm two-person approval rule vs single-admin
  // strong-confirm per lead-security decision.

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="purge-title"
      data-testid="purge-dialog"
    >
      <h2 id="purge-title">{t('faceVault.purge.title')}</h2>
      <p>{t('faceVault.purge.step', { current: step, total: 3 })}</p>
      {step === 1 && (
        <div data-testid="purge-step-1">
          <p>
            <span aria-hidden="true">{'\u26A0'}</span>
            {t('faceVault.purge.warning1')}
          </p>
          <p>{t('faceVault.purge.warning2')}</p>
          <button
            type="button"
            onClick={() => setStep(2)}
            data-testid="purge-next-1"
          >
            {t('faceVault.purge.next')}
          </button>
        </div>
      )}
      {step === 2 && (
        <div data-testid="purge-step-2">
          <fieldset>
            <legend>{t('faceVault.purge.scopeHeading')}</legend>
            <label>
              <input
                type="radio"
                name="purge-scope"
                value="tenant"
                checked={scope === 'tenant'}
                onChange={() => setScope('tenant')}
                data-testid="purge-scope-tenant"
              />
              <span>{t('faceVault.purge.scope.tenant')}</span>
            </label>
            <label>
              <input
                type="radio"
                name="purge-scope"
                value="camera"
                checked={scope === 'camera'}
                onChange={() => setScope('camera')}
                data-testid="purge-scope-camera"
              />
              <span>{t('faceVault.purge.scope.camera')}</span>
            </label>
            <label>
              <input
                type="radio"
                name="purge-scope"
                value="dateRange"
                checked={scope === 'dateRange'}
                onChange={() => setScope('dateRange')}
                data-testid="purge-scope-date-range"
              />
              <span>{t('faceVault.purge.scope.dateRange')}</span>
            </label>
          </fieldset>
          {scope === 'camera' && (
            <label>
              <span>{t('faceVault.purge.cameraLabel')}</span>
              <input
                type="text"
                value={cameraId}
                onChange={(e) => setCameraId(e.target.value)}
                data-testid="purge-camera-id"
              />
            </label>
          )}
          {scope === 'dateRange' && (
            <>
              <label>
                <span>{t('faceVault.purge.fromLabel')}</span>
                <input
                  type="date"
                  value={fromDate}
                  onChange={(e) => setFromDate(e.target.value)}
                  data-testid="purge-from-date"
                />
              </label>
              <label>
                <span>{t('faceVault.purge.toLabel')}</span>
                <input
                  type="date"
                  value={toDate}
                  onChange={(e) => setToDate(e.target.value)}
                  data-testid="purge-to-date"
                />
              </label>
            </>
          )}
          <button
            type="button"
            onClick={() => setStep(1)}
            data-testid="purge-back-2"
          >
            {t('faceVault.purge.back')}
          </button>
          <button
            type="button"
            onClick={() => setStep(3)}
            data-testid="purge-next-2"
          >
            {t('faceVault.purge.next')}
          </button>
        </div>
      )}
      {step === 3 && (
        <div data-testid="purge-step-3">
          <label>
            <span>
              {t('faceVault.purge.typePrompt', { sentinel })}
            </span>
            <input
              type="text"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              data-testid="purge-type-input"
            />
          </label>
          <button
            type="button"
            onClick={() => setStep(2)}
            data-testid="purge-back-3"
          >
            {t('faceVault.purge.back')}
          </button>
          <button
            type="button"
            disabled={!ready}
            aria-disabled={!ready}
            onClick={() =>
              onConfirm({
                scope,
                confirmation: sentinel,
                fromDate: scope === 'dateRange' ? fromDate : undefined,
                toDate: scope === 'dateRange' ? toDate : undefined,
                cameraId: scope === 'camera' ? cameraId : undefined,
              })
            }
            data-testid="purge-confirm"
          >
            {t('faceVault.purge.confirm')}
          </button>
        </div>
      )}
      <button type="button" onClick={onCancel} data-testid="purge-cancel">
        {t('faceVault.purge.cancel')}
      </button>
    </div>
  );
}

export default AiSettingsPage;
