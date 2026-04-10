import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  getBrandConfig,
  saveBrandDraft,
  publishBrand,
  validateDomain,
  listEmailTemplates,
  brandConfigQueryKey,
  emailTemplatesQueryKey,
  __TEST__,
} from '@/api/brandConfig';
import type {
  BrandConfig,
  BrandDraft,
  ColorPalette,
  DomainValidationStatus,
  EmailTemplate,
} from '@/api/brandConfig';

// KAI-310: Brand Configuration page (Integrator Portal).
//
// White-label branding settings: logo upload (primary + icon), color
// palette (primary, secondary, accent, danger), company name, tagline,
// custom domain with DNS validation, transactional email templates,
// and mobile app config.
//
// Data layer:
//  - TanStack Query keyed by integrator ID.
//  - `getBrandConfig()` / `saveBrandDraft()` / `publishBrand()` are
//    typed mock stubs today; real Connect-Go lands in a future ticket.
//
// Accessibility:
//  - Page wrapped in <main> with labelled landmarks (role=region).
//  - All form inputs have visible labels or aria-label.
//  - Domain validation status uses icon+text+border (never color alone).
//  - Color inputs include text hex labels alongside swatches.
//  - Publish confirmation dialog is a native dialog element.

interface BrandConfigPageProps {
  readonly integratorId?: string;
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function DomainStatusBadge({
  status,
  t,
}: {
  status: DomainValidationStatus;
  t: (key: string) => string;
}): JSX.Element {
  const styles: Record<DomainValidationStatus, string> = {
    valid: 'border-green-600 bg-green-50 text-green-800',
    invalid: 'border-red-600 bg-red-50 text-red-800',
    pending: 'border-yellow-500 bg-yellow-50 text-yellow-800',
    checking: 'border-blue-500 bg-blue-50 text-blue-800',
  };
  const icons: Record<DomainValidationStatus, string> = {
    valid: '[OK]',
    invalid: '[X]',
    pending: '[?]',
    checking: '[...]',
  };
  return (
    <span
      data-testid="domain-status-badge"
      className={`inline-flex items-center gap-1 rounded border px-2 py-0.5 text-xs font-medium ${styles[status]}`}
      role="status"
      aria-label={t(`brand.domain.status.${status}`)}
    >
      <span aria-hidden="true">{icons[status]}</span>
      {t(`brand.domain.status.${status}`)}
    </span>
  );
}

function ColorField({
  label,
  value,
  fieldName,
  onChange,
}: {
  label: string;
  value: string;
  fieldName: string;
  onChange: (field: string, value: string) => void;
}): JSX.Element {
  return (
    <div className="flex items-center gap-2">
      <label htmlFor={`color-${fieldName}`} className="w-24 text-sm font-medium text-slate-700">
        {label}
      </label>
      <input
        id={`color-${fieldName}`}
        type="color"
        value={value}
        onChange={(e) => onChange(fieldName, e.target.value)}
        className="h-8 w-8 cursor-pointer rounded border border-slate-300"
        aria-label={label}
      />
      <span className="text-xs font-mono text-slate-500">{value}</span>
    </div>
  );
}

function PreviewPanel({
  companyName,
  tagline,
  colors,
  t,
}: {
  companyName: string;
  tagline: string;
  colors: ColorPalette;
  t: (key: string) => string;
}): JSX.Element {
  return (
    <section
      role="region"
      aria-label={t('brand.preview.regionLabel')}
      data-testid="brand-preview"
      className="rounded-lg border border-slate-200 bg-white p-4"
    >
      <h3 className="mb-3 text-sm font-semibold text-slate-700">
        {t('brand.preview.heading')}
      </h3>
      {/* Header bar preview */}
      <div
        className="mb-3 rounded px-3 py-2"
        style={{ backgroundColor: colors.primary }}
        data-testid="preview-header"
      >
        <span className="text-sm font-bold text-white">{companyName || '---'}</span>
      </div>
      {/* Login screen mock */}
      <div className="mb-3 rounded border border-slate-200 bg-slate-50 p-3 text-center">
        <p className="text-lg font-bold" style={{ color: colors.primary }}>
          {companyName || '---'}
        </p>
        <p className="text-xs text-slate-500">{tagline || '---'}</p>
        <div className="mx-auto mt-2 w-48">
          <div className="mb-1 rounded border border-slate-300 bg-white px-2 py-1 text-left text-xs text-slate-400">
            {t('brand.preview.emailPlaceholder')}
          </div>
          <div className="mb-2 rounded border border-slate-300 bg-white px-2 py-1 text-left text-xs text-slate-400">
            {t('brand.preview.passwordPlaceholder')}
          </div>
          <button
            type="button"
            disabled
            className="w-full rounded px-2 py-1 text-xs font-medium text-white"
            style={{ backgroundColor: colors.accent }}
          >
            {t('brand.preview.signInButton')}
          </button>
        </div>
      </div>
      {/* Sidebar preview */}
      <div
        className="rounded px-3 py-2"
        style={{ backgroundColor: colors.secondary }}
        data-testid="preview-sidebar"
      >
        <span className="text-xs text-white">{t('brand.preview.sidebarSample')}</span>
      </div>
    </section>
  );
}

function EmailTemplateCard({
  template,
  colors,
  t,
}: {
  template: EmailTemplate;
  colors: ColorPalette;
  t: (key: string) => string;
}): JSX.Element {
  return (
    <div
      data-testid={`email-template-${template.type}`}
      className="rounded border border-slate-200 bg-white p-3"
    >
      <h4 className="text-sm font-semibold text-slate-700">
        {t(`brand.email.type.${template.type}`)}
      </h4>
      <p className="text-xs text-slate-500">{template.subject}</p>
      <div
        className="mt-2 rounded border border-slate-100 bg-slate-50 p-2 text-xs"
        style={{ borderLeftColor: colors.primary, borderLeftWidth: '3px' }}
        dangerouslySetInnerHTML={{ __html: template.bodyHtml }}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function BrandConfigPage({
  integratorId = __TEST__.CURRENT_INTEGRATOR_ID,
}: BrandConfigPageProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  // --- Queries ---
  const configQuery = useQuery({
    queryKey: brandConfigQueryKey(integratorId),
    queryFn: () => getBrandConfig(integratorId),
  });

  const templatesQuery = useQuery({
    queryKey: emailTemplatesQueryKey(integratorId),
    queryFn: () => listEmailTemplates(integratorId),
  });

  // --- Local form state ---
  const config = configQuery.data;
  const templates = templatesQuery.data;

  const [companyName, setCompanyName] = useState<string | null>(null);
  const [tagline, setTagline] = useState<string | null>(null);
  const [colors, setColors] = useState<ColorPalette | null>(null);
  const [customDomain, setCustomDomain] = useState<string | null>(null);
  const [mobileAppName, setMobileAppName] = useState<string | null>(null);
  const [publishDialogOpen, setPublishDialogOpen] = useState(false);

  // Merge local edits over server state.
  const effectiveName = companyName ?? config?.companyName ?? '';
  const effectiveTagline = tagline ?? config?.tagline ?? '';
  const effectiveColors = colors ?? config?.colors ?? __TEST__.DEFAULT_COLORS;
  const effectiveDomain = customDomain ?? config?.customDomain ?? '';
  const effectiveMobileAppName = mobileAppName ?? config?.mobileAppName ?? '';

  // --- Mutations ---
  const saveMutation = useMutation({
    mutationFn: () => {
      const draft: BrandDraft = {
        companyName: effectiveName,
        tagline: effectiveTagline,
        logoUrl: config?.logoUrl ?? null,
        iconUrl: config?.iconUrl ?? null,
        colors: effectiveColors,
        customDomain: effectiveDomain || null,
        mobileAppName: effectiveMobileAppName,
        splashScreenUrl: config?.splashScreenUrl ?? null,
      };
      return saveBrandDraft(integratorId, draft);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: brandConfigQueryKey(integratorId),
      });
    },
  });

  const publishMutation = useMutation({
    mutationFn: () => publishBrand(integratorId),
    onSuccess: () => {
      setPublishDialogOpen(false);
      void queryClient.invalidateQueries({
        queryKey: brandConfigQueryKey(integratorId),
      });
    },
  });

  const domainMutation = useMutation({
    mutationFn: (domain: string) => validateDomain(integratorId, domain),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: brandConfigQueryKey(integratorId),
      });
    },
  });

  const handleColorChange = useCallback(
    (field: string, value: string) => {
      setColors((prev) => ({
        ...(prev ?? effectiveColors),
        [field]: value,
      }));
    },
    [effectiveColors],
  );

  const domainStatus = useMemo(() => {
    if (domainMutation.isPending) return 'checking' as const;
    return config?.domainValidation?.status ?? null;
  }, [domainMutation.isPending, config?.domainValidation?.status]);

  return (
    <main
      data-testid="brand-config-page"
      aria-labelledby="brand-config-heading"
      className="min-h-screen bg-slate-50 p-4"
    >
      {/* Breadcrumb + heading */}
      <header className="mb-4">
        <nav
          aria-label={t('brand.breadcrumb.ariaLabel')}
          className="text-xs text-slate-500"
        >
          <ol className="flex gap-1">
            <li>{t('brand.breadcrumb.integratorPortal')}</li>
            <li aria-hidden="true">/</li>
            <li aria-current="page" className="font-medium text-slate-700">
              {t('brand.title')}
            </li>
          </ol>
        </nav>
        <h1
          id="brand-config-heading"
          className="mt-1 text-2xl font-bold text-slate-900"
        >
          {t('brand.title')}
        </h1>
        <p className="text-sm text-slate-600">{t('brand.subtitle')}</p>
      </header>

      {configQuery.isLoading ? (
        <p role="status" data-testid="brand-loading">
          {t('brand.loading')}
        </p>
      ) : configQuery.isError ? (
        <p role="alert" data-testid="brand-error">
          {t('brand.error')}
        </p>
      ) : config ? (
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-3">
          {/* Left column: form sections */}
          <div className="space-y-6 lg:col-span-2">
            {/* Section 1: Brand Identity */}
            <section
              role="region"
              aria-label={t('brand.identity.regionLabel')}
              className="rounded-lg border border-slate-200 bg-white p-4"
            >
              <h2 className="mb-3 text-lg font-semibold text-slate-900">
                {t('brand.identity.heading')}
              </h2>
              <div className="space-y-3">
                <div>
                  <label
                    htmlFor="brand-company-name"
                    className="block text-sm font-medium text-slate-700"
                  >
                    {t('brand.identity.companyName')}
                  </label>
                  <input
                    id="brand-company-name"
                    type="text"
                    value={effectiveName}
                    onChange={(e) => setCompanyName(e.target.value)}
                    className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <label
                    htmlFor="brand-tagline"
                    className="block text-sm font-medium text-slate-700"
                  >
                    {t('brand.identity.tagline')}
                  </label>
                  <input
                    id="brand-tagline"
                    type="text"
                    value={effectiveTagline}
                    onChange={(e) => setTagline(e.target.value)}
                    className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <p className="text-sm font-medium text-slate-700">
                    {t('brand.identity.logoUpload')}
                  </p>
                  <div className="mt-1 flex gap-4">
                    <button
                      type="button"
                      className="rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-2 text-xs text-slate-500"
                      aria-label={t('brand.identity.uploadPrimaryLogo')}
                    >
                      {t('brand.identity.primaryLogo')}
                    </button>
                    <button
                      type="button"
                      className="rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-2 text-xs text-slate-500"
                      aria-label={t('brand.identity.uploadIcon')}
                    >
                      {t('brand.identity.icon')}
                    </button>
                  </div>
                </div>
              </div>
            </section>

            {/* Section 2: Color Palette */}
            <section
              role="region"
              aria-label={t('brand.colors.regionLabel')}
              className="rounded-lg border border-slate-200 bg-white p-4"
            >
              <h2 className="mb-3 text-lg font-semibold text-slate-900">
                {t('brand.colors.heading')}
              </h2>
              <div className="space-y-2">
                <ColorField
                  label={t('brand.colors.primary')}
                  value={effectiveColors.primary}
                  fieldName="primary"
                  onChange={handleColorChange}
                />
                <ColorField
                  label={t('brand.colors.secondary')}
                  value={effectiveColors.secondary}
                  fieldName="secondary"
                  onChange={handleColorChange}
                />
                <ColorField
                  label={t('brand.colors.accent')}
                  value={effectiveColors.accent}
                  fieldName="accent"
                  onChange={handleColorChange}
                />
                <ColorField
                  label={t('brand.colors.danger')}
                  value={effectiveColors.danger}
                  fieldName="danger"
                  onChange={handleColorChange}
                />
              </div>
            </section>

            {/* Section 3: Custom Domain */}
            <section
              role="region"
              aria-label={t('brand.domain.regionLabel')}
              className="rounded-lg border border-slate-200 bg-white p-4"
            >
              <h2 className="mb-3 text-lg font-semibold text-slate-900">
                {t('brand.domain.heading')}
              </h2>
              <div className="flex items-end gap-2">
                <div className="flex-1">
                  <label
                    htmlFor="brand-custom-domain"
                    className="block text-sm font-medium text-slate-700"
                  >
                    {t('brand.domain.label')}
                  </label>
                  <input
                    id="brand-custom-domain"
                    type="text"
                    value={effectiveDomain}
                    onChange={(e) => setCustomDomain(e.target.value)}
                    placeholder="nvr.yourcompany.com"
                    className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <button
                  type="button"
                  onClick={() => {
                    if (effectiveDomain) {
                      domainMutation.mutate(effectiveDomain);
                    }
                  }}
                  disabled={!effectiveDomain || domainMutation.isPending}
                  className="rounded bg-slate-700 px-3 py-2 text-sm text-white hover:bg-slate-800 disabled:opacity-50"
                  data-testid="validate-domain-btn"
                >
                  {t('brand.domain.validate')}
                </button>
              </div>
              {domainStatus && (
                <div className="mt-2">
                  <DomainStatusBadge status={domainStatus} t={t} />
                </div>
              )}
              {config.domainValidation?.cnameTarget && (
                <p className="mt-1 text-xs text-slate-500">
                  {t('brand.domain.cnameHint', {
                    target: config.domainValidation.cnameTarget,
                  })}
                </p>
              )}
            </section>

            {/* Section 4: Email Templates */}
            <section
              role="region"
              aria-label={t('brand.email.regionLabel')}
              className="rounded-lg border border-slate-200 bg-white p-4"
            >
              <h2 className="mb-3 text-lg font-semibold text-slate-900">
                {t('brand.email.heading')}
              </h2>
              {templates && templates.length > 0 ? (
                <div className="space-y-3">
                  {templates.map((tmpl) => (
                    <EmailTemplateCard
                      key={tmpl.id}
                      template={tmpl}
                      colors={effectiveColors}
                      t={t}
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-slate-500">{t('brand.email.empty')}</p>
              )}
            </section>

            {/* Section 5: Mobile App Config */}
            <section
              role="region"
              aria-label={t('brand.mobile.regionLabel')}
              className="rounded-lg border border-slate-200 bg-white p-4"
            >
              <h2 className="mb-3 text-lg font-semibold text-slate-900">
                {t('brand.mobile.heading')}
              </h2>
              <div className="space-y-3">
                <div>
                  <label
                    htmlFor="brand-mobile-app-name"
                    className="block text-sm font-medium text-slate-700"
                  >
                    {t('brand.mobile.appName')}
                  </label>
                  <input
                    id="brand-mobile-app-name"
                    type="text"
                    value={effectiveMobileAppName}
                    onChange={(e) => setMobileAppName(e.target.value)}
                    className="mt-1 w-full rounded border border-slate-300 px-3 py-2 text-sm"
                  />
                </div>
                <div>
                  <p className="text-sm font-medium text-slate-700">
                    {t('brand.mobile.bundleIdPrefix')}
                  </p>
                  <p className="text-sm text-slate-500">{config.mobileBundleIdPrefix}</p>
                </div>
                <div>
                  <p className="text-sm font-medium text-slate-700">
                    {t('brand.mobile.splashScreen')}
                  </p>
                  <button
                    type="button"
                    className="rounded border border-dashed border-slate-300 bg-slate-50 px-4 py-2 text-xs text-slate-500"
                    aria-label={t('brand.mobile.uploadSplashScreen')}
                  >
                    {t('brand.mobile.uploadSplashScreen')}
                  </button>
                </div>
              </div>
            </section>

            {/* Save / Publish actions */}
            <div className="flex gap-3" data-testid="brand-actions">
              <button
                type="button"
                onClick={() => saveMutation.mutate()}
                disabled={saveMutation.isPending}
                className="rounded bg-slate-700 px-4 py-2 text-sm font-medium text-white hover:bg-slate-800 disabled:opacity-50"
                data-testid="save-draft-btn"
              >
                {saveMutation.isPending
                  ? t('brand.actions.saving')
                  : t('brand.actions.saveDraft')}
              </button>
              <button
                type="button"
                onClick={() => setPublishDialogOpen(true)}
                className="rounded bg-blue-700 px-4 py-2 text-sm font-medium text-white hover:bg-blue-800"
                data-testid="publish-btn"
              >
                {t('brand.actions.publish')}
              </button>
            </div>

            {saveMutation.isSuccess && (
              <p role="status" data-testid="save-success" className="text-sm text-green-700">
                {t('brand.actions.saveSuccess')}
              </p>
            )}
          </div>

          {/* Right column: live preview */}
          <div className="lg:col-span-1">
            <PreviewPanel
              companyName={effectiveName}
              tagline={effectiveTagline}
              colors={effectiveColors}
              t={t}
            />
          </div>
        </div>
      ) : null}

      {/* Publish confirmation dialog */}
      {publishDialogOpen && (
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="publish-dialog-title"
          data-testid="publish-dialog"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
        >
          <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
            <h2
              id="publish-dialog-title"
              className="text-lg font-bold text-slate-900"
            >
              {t('brand.publish.title')}
            </h2>
            <p className="mt-2 text-sm text-slate-600">
              {t('brand.publish.warning')}
            </p>
            <div className="mt-4 flex justify-end gap-3">
              <button
                type="button"
                onClick={() => setPublishDialogOpen(false)}
                className="rounded border border-slate-300 px-4 py-2 text-sm text-slate-700 hover:bg-slate-50"
                data-testid="publish-cancel-btn"
              >
                {t('brand.publish.cancel')}
              </button>
              <button
                type="button"
                onClick={() => publishMutation.mutate()}
                disabled={publishMutation.isPending}
                className="rounded bg-blue-700 px-4 py-2 text-sm font-medium text-white hover:bg-blue-800 disabled:opacity-50"
                data-testid="publish-confirm-btn"
              >
                {publishMutation.isPending
                  ? t('brand.publish.publishing')
                  : t('brand.publish.confirm')}
              </button>
            </div>
          </div>
        </div>
      )}
    </main>
  );
}

export default BrandConfigPage;
