// KAI-135: Customer Admin Sign-in Methods page — /admin/sign-in.
//
// Standalone page for configuring authentication providers.
// Reuses the existing 7 IdP wizard components from components/users/.
//
// Features:
//   - Provider list with status, user count, last sync
//   - Add provider selector
//   - Per-IdP configuration wizards (Local, Entra, Google, Okta, OIDC, SAML, LDAP)
//   - Default provider selection with change warning
//   - Delete provider with confirmation
//
// All strings via react-i18next. All queries scoped to current tenant.

import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';

import {
  deleteSignInProvider,
  getDefaultProvider,
  listSignInProviders,
  saveSignInProvider,
  setDefaultProvider,
  signInMethodsQueryKeys,
  testSignInProvider,
  type DefaultProviderSelection,
  type LocalProviderConfig,
  type EntraProviderConfig,
  type GoogleProviderConfig,
  type OktaProviderConfig,
  type OidcProviderConfig,
  type SamlProviderConfig,
  type LdapProviderConfig,
  type ProviderConfig,
  type ProviderKind,
  type SignInProviderSummary,
  type TestProviderResult,
} from '@/api/signInMethods';
import { useSessionStore } from '@/stores/session';

import { LocalProviderWizard } from '@/components/users/LocalProviderWizard';
import { EntraProviderWizard } from '@/components/users/EntraProviderWizard';
import { GoogleProviderWizard } from '@/components/users/GoogleProviderWizard';
import { OktaProviderWizard } from '@/components/users/OktaProviderWizard';
import { OidcProviderWizard } from '@/components/users/OidcProviderWizard';
import { SamlProviderWizard } from '@/components/users/SamlProviderWizard';
import { LdapProviderWizard } from '@/components/users/LdapProviderWizard';

// ---------------------------------------------------------------------------
// Provider metadata
// ---------------------------------------------------------------------------

interface ProviderMeta {
  kind: ProviderKind;
  nameKey: string;
  descKey: string;
}

const ALL_PROVIDER_KINDS: ProviderMeta[] = [
  { kind: 'local', nameKey: 'auth.providers.local.name', descKey: 'auth.providers.local.description' },
  { kind: 'entra', nameKey: 'auth.providers.entra.name', descKey: 'auth.providers.entra.description' },
  { kind: 'google', nameKey: 'auth.providers.google.name', descKey: 'auth.providers.google.description' },
  { kind: 'okta', nameKey: 'auth.providers.okta.name', descKey: 'auth.providers.okta.description' },
  { kind: 'oidc', nameKey: 'auth.providers.oidc.name', descKey: 'auth.providers.oidc.description' },
  { kind: 'saml', nameKey: 'auth.providers.saml.name', descKey: 'auth.providers.saml.description' },
  { kind: 'ldap', nameKey: 'auth.providers.ldap.name', descKey: 'auth.providers.ldap.description' },
];

// Default configs for newly-added providers (the wizards expect a config shape).
function defaultConfigForKind(kind: ProviderKind): ProviderConfig {
  switch (kind) {
    case 'local':
      return {
        kind: 'local',
        enabled: false,
        passwordPolicy: {
          minLength: 8,
          requireUppercase: true,
          requireLowercase: true,
          requireDigit: true,
          requireSpecial: false,
          rotationDays: 0,
        },
      } satisfies LocalProviderConfig;
    case 'entra':
      return {
        kind: 'entra',
        enabled: false,
        clientId: '',
        clientSecret: '',
        tenantId: '',
        redirectUri: `${window.location.origin}/auth/callback/entra`,
      } satisfies EntraProviderConfig;
    case 'google':
      return {
        kind: 'google',
        enabled: false,
        clientId: '',
        clientSecret: '',
        hostedDomain: '',
      } satisfies GoogleProviderConfig;
    case 'okta':
      return {
        kind: 'okta',
        enabled: false,
        domain: '',
        clientId: '',
        clientSecret: '',
        authorizationServerId: 'default',
      } satisfies OktaProviderConfig;
    case 'oidc':
      return {
        kind: 'oidc',
        enabled: false,
        issuerUrl: '',
        clientId: '',
        clientSecret: '',
        scopes: 'openid profile email',
        claimMappings: { sub: 'sub', email: 'email', name: 'name', groups: 'groups' },
      } satisfies OidcProviderConfig;
    case 'saml':
      return {
        kind: 'saml',
        enabled: false,
        metadataUrl: '',
        metadataXml: '',
        entityId: '',
        acsUrl: `${window.location.origin}/auth/callback/saml`,
        signingCert: '',
        attributeMappings: { email: 'email', name: 'name', groups: 'groups' },
      } satisfies SamlProviderConfig;
    case 'ldap':
      return {
        kind: 'ldap',
        enabled: false,
        host: '',
        port: 389,
        bindDn: '',
        bindPassword: '',
        baseDn: '',
        userFilter: '(objectClass=person)',
        groupFilter: '(objectClass=group)',
        attributeMappings: { uid: 'uid', email: 'mail', name: 'displayName', memberOf: 'memberOf' },
      } satisfies LdapProviderConfig;
  }
}

// ---------------------------------------------------------------------------
// Status badge helper
// ---------------------------------------------------------------------------

function statusBadgeClasses(status: string): string {
  switch (status) {
    case 'enabled':
      return 'inline-flex items-center rounded-full bg-green-100 px-2.5 py-0.5 text-xs font-medium text-green-800';
    case 'disabled':
      return 'inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-800';
    case 'error':
      return 'inline-flex items-center rounded-full bg-red-100 px-2.5 py-0.5 text-xs font-medium text-red-800';
    default:
      return 'inline-flex items-center rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-800';
  }
}

// ---------------------------------------------------------------------------
// Main page component
// ---------------------------------------------------------------------------

export function SignInMethodsPage(): JSX.Element {
  const { t } = useTranslation('users');
  const { t: tCommon } = useTranslation('common');
  const tenantId = useSessionStore((s) => s.tenantId);
  const queryClient = useQueryClient();

  // Modal / wizard state
  const [openWizard, setOpenWizard] = useState<ProviderKind | null>(null);
  const [addSelectorOpen, setAddSelectorOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<ProviderKind | null>(null);
  const [deleteConfirmText, setDeleteConfirmText] = useState('');
  const [defaultChangeTarget, setDefaultChangeTarget] = useState<ProviderKind | null>(null);

  // Queries
  const providersQuery = useQuery<SignInProviderSummary[]>({
    queryKey: signInMethodsQueryKeys.providers(tenantId),
    queryFn: () => listSignInProviders(tenantId),
  });

  const defaultQuery = useQuery<DefaultProviderSelection>({
    queryKey: signInMethodsQueryKeys.defaultProvider(tenantId),
    queryFn: () => getDefaultProvider(tenantId),
  });

  const invalidateAll = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: signInMethodsQueryKeys.all(tenantId),
    });
  }, [queryClient, tenantId]);

  // Mutations
  const saveMutation = useMutation({
    mutationFn: (args: { kind: ProviderKind; config: ProviderConfig }) =>
      saveSignInProvider(tenantId, args.kind, args.config),
    onSuccess: invalidateAll,
  });

  const deleteMutation = useMutation({
    mutationFn: (kind: ProviderKind) =>
      deleteSignInProvider(tenantId, kind),
    onSuccess: invalidateAll,
  });

  const setDefaultMutation = useMutation({
    mutationFn: (kind: ProviderKind) =>
      setDefaultProvider(tenantId, kind),
    onSuccess: invalidateAll,
  });

  // Wizard handlers
  const handleTest = useCallback(
    async (config: ProviderConfig): Promise<TestProviderResult> =>
      testSignInProvider(tenantId, config.kind, config),
    [tenantId],
  );

  const handleSave = useCallback(
    async (config: ProviderConfig): Promise<void> => {
      await saveMutation.mutateAsync({ kind: config.kind, config });
    },
    [saveMutation],
  );

  const handleDelete = useCallback(() => {
    if (deleteTarget) {
      deleteMutation.mutate(deleteTarget);
      setDeleteTarget(null);
      setDeleteConfirmText('');
    }
  }, [deleteTarget, deleteMutation]);

  const handleDefaultChange = useCallback(() => {
    if (defaultChangeTarget) {
      setDefaultMutation.mutate(defaultChangeTarget);
      setDefaultChangeTarget(null);
    }
  }, [defaultChangeTarget, setDefaultMutation]);

  // Derived data
  const providers = providersQuery.data ?? [];
  const defaultKind = defaultQuery.data?.kind ?? 'local';
  const configuredKinds = useMemo(
    () => new Set(providers.map((p) => p.kind)),
    [providers],
  );
  const unconfiguredKinds = useMemo(
    () => ALL_PROVIDER_KINDS.filter((m) => !configuredKinds.has(m.kind)),
    [configuredKinds],
  );

  const getProviderConfig = (kind: ProviderKind): ProviderConfig => {
    const found = providers.find((p) => p.kind === kind);
    return found?.config ?? defaultConfigForKind(kind);
  };

  // Loading / error states
  if (providersQuery.isLoading || defaultQuery.isLoading) {
    return (
      <main data-testid="sign-in-methods-page">
        <p role="status" aria-live="polite">{tCommon('signIn.loading')}</p>
      </main>
    );
  }

  if (providersQuery.isError || defaultQuery.isError) {
    return (
      <main data-testid="sign-in-methods-page">
        <p role="alert">{tCommon('signIn.error')}</p>
      </main>
    );
  }

  return (
    <main
      className="mx-auto max-w-5xl px-4 py-8 sm:px-6 lg:px-8"
      data-testid="sign-in-methods-page"
    >
      {/* Page header */}
      <nav aria-label={tCommon('signIn.breadcrumbAriaLabel')} className="mb-4">
        <ol className="flex gap-2 text-sm text-gray-500">
          <li>{tCommon('admin.home.title')}</li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="font-medium text-gray-900">
            {tCommon('signIn.title')}
          </li>
        </ol>
      </nav>

      <header className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{tCommon('signIn.title')}</h1>
          <p className="mt-1 text-gray-600">{tCommon('signIn.description')}</p>
        </div>
        {unconfiguredKinds.length > 0 && (
          <button
            type="button"
            onClick={() => setAddSelectorOpen(true)}
            className="rounded-md bg-indigo-600 px-4 py-2 text-sm font-semibold text-white shadow-sm hover:bg-indigo-500"
            data-testid="add-provider-button"
          >
            {tCommon('signIn.addProvider')}
          </button>
        )}
      </header>

      {/* Default provider selection */}
      <section
        aria-label={tCommon('signIn.defaultProvider.sectionLabel')}
        className="mb-8 rounded-lg border border-gray-200 bg-white p-4"
        data-testid="default-provider-section"
      >
        <h2 className="mb-3 text-lg font-semibold">{tCommon('signIn.defaultProvider.heading')}</h2>
        <p className="mb-4 text-sm text-gray-600">{tCommon('signIn.defaultProvider.description')}</p>
        <fieldset>
          <legend className="sr-only">{tCommon('signIn.defaultProvider.legend')}</legend>
          {providers.map((provider) => (
            <label
              key={provider.kind}
              className="mb-2 flex items-center gap-3"
              data-testid={`default-radio-${provider.kind}`}
            >
              <input
                type="radio"
                name="defaultProvider"
                value={provider.kind}
                checked={defaultKind === provider.kind}
                onChange={() => {
                  if (provider.kind !== defaultKind) {
                    setDefaultChangeTarget(provider.kind);
                  }
                }}
                className="h-4 w-4 text-indigo-600"
                data-testid={`default-radio-input-${provider.kind}`}
              />
              <span className="text-sm font-medium">
                {ALL_PROVIDER_KINDS.find((m) => m.kind === provider.kind)
                  ? t(ALL_PROVIDER_KINDS.find((m) => m.kind === provider.kind)!.nameKey)
                  : provider.kind}
              </span>
            </label>
          ))}
        </fieldset>
      </section>

      {/* Provider list */}
      <section
        aria-label={tCommon('signIn.providerList.sectionLabel')}
        data-testid="provider-list-section"
      >
        <h2 className="mb-4 text-lg font-semibold">{tCommon('signIn.providerList.heading')}</h2>

        {providers.length === 0 ? (
          <p data-testid="provider-list-empty" className="text-gray-500">
            {tCommon('signIn.providerList.empty')}
          </p>
        ) : (
          <ul className="space-y-3" style={{ listStyle: 'none', padding: 0 }}>
            {providers.map((provider) => {
              const meta = ALL_PROVIDER_KINDS.find((m) => m.kind === provider.kind);
              return (
                <li
                  key={provider.kind}
                  className="flex items-center justify-between rounded-lg border border-gray-200 bg-white p-4"
                  data-testid={`provider-card-${provider.kind}`}
                >
                  <div className="flex-1">
                    <div className="flex items-center gap-3">
                      <strong className="text-base font-semibold">
                        {meta ? t(meta.nameKey) : provider.displayName}
                      </strong>
                      <span
                        className={statusBadgeClasses(provider.status)}
                        data-testid={`provider-status-${provider.kind}`}
                      >
                        {tCommon(`signIn.status.${provider.status}`)}
                      </span>
                      {defaultKind === provider.kind && (
                        <span
                          className="inline-flex items-center rounded-full bg-indigo-100 px-2.5 py-0.5 text-xs font-medium text-indigo-800"
                          data-testid={`default-badge-${provider.kind}`}
                        >
                          {tCommon('signIn.defaultBadge')}
                        </span>
                      )}
                    </div>
                    <p className="mt-1 text-sm text-gray-500">
                      {meta ? t(meta.descKey) : ''}
                    </p>
                    <div className="mt-2 flex gap-4 text-xs text-gray-400">
                      <span data-testid={`provider-user-count-${provider.kind}`}>
                        {tCommon('signIn.userCount', { count: provider.userCount })}
                      </span>
                      {provider.lastSyncAt && (
                        <span data-testid={`provider-last-sync-${provider.kind}`}>
                          {tCommon('signIn.lastSync', {
                            date: new Date(provider.lastSyncAt).toLocaleString(),
                          })}
                        </span>
                      )}
                    </div>
                  </div>

                  <div className="flex items-center gap-2">
                    <button
                      type="button"
                      onClick={() => setOpenWizard(provider.kind)}
                      className="rounded px-3 py-1.5 text-sm font-medium text-indigo-600 hover:bg-indigo-50"
                      data-testid={`provider-edit-${provider.kind}`}
                    >
                      {tCommon('signIn.actions.edit')}
                    </button>
                    <button
                      type="button"
                      onClick={() => setDeleteTarget(provider.kind)}
                      className="rounded px-3 py-1.5 text-sm font-medium text-red-600 hover:bg-red-50"
                      data-testid={`provider-delete-${provider.kind}`}
                    >
                      {tCommon('signIn.actions.delete')}
                    </button>
                  </div>
                </li>
              );
            })}
          </ul>
        )}
      </section>

      {/* Add provider selector dialog */}
      {addSelectorOpen && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={tCommon('signIn.addProvider')}
          data-testid="add-provider-selector"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
        >
          <div className="w-full max-w-md rounded-lg bg-white p-6 shadow-xl">
            <h2 className="mb-4 text-lg font-semibold">{tCommon('signIn.addProvider')}</h2>
            <ul style={{ listStyle: 'none', padding: 0 }} className="space-y-2">
              {unconfiguredKinds.map(({ kind, nameKey, descKey }) => (
                <li key={kind}>
                  <button
                    type="button"
                    className="w-full rounded-md border border-gray-200 p-3 text-left hover:bg-gray-50"
                    onClick={() => {
                      setAddSelectorOpen(false);
                      setOpenWizard(kind);
                    }}
                    data-testid={`add-provider-option-${kind}`}
                  >
                    <strong className="block text-sm font-semibold">{t(nameKey)}</strong>
                    <span className="text-xs text-gray-500">{t(descKey)}</span>
                  </button>
                </li>
              ))}
            </ul>
            <div className="mt-4 flex justify-end">
              <button
                type="button"
                onClick={() => setAddSelectorOpen(false)}
                className="rounded px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100"
                data-testid="add-provider-cancel"
              >
                {tCommon('signIn.cancel')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Default provider change confirmation dialog */}
      {defaultChangeTarget && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={tCommon('signIn.defaultProvider.changeTitle')}
          data-testid="default-change-dialog"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
        >
          <div className="w-full max-w-sm rounded-lg bg-white p-6 shadow-xl">
            <h2 className="mb-2 text-lg font-semibold">
              {tCommon('signIn.defaultProvider.changeTitle')}
            </h2>
            <p className="mb-4 text-sm text-gray-600">
              {tCommon('signIn.defaultProvider.changeWarning')}
            </p>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setDefaultChangeTarget(null)}
                className="rounded px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100"
                data-testid="default-change-cancel"
              >
                {tCommon('signIn.cancel')}
              </button>
              <button
                type="button"
                onClick={handleDefaultChange}
                className="rounded bg-indigo-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-indigo-500"
                data-testid="default-change-confirm"
              >
                {tCommon('signIn.defaultProvider.changeConfirm')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Delete provider confirmation dialog */}
      {deleteTarget && (
        <div
          role="dialog"
          aria-modal="true"
          aria-label={tCommon('signIn.delete.title')}
          data-testid="delete-provider-dialog"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/30"
        >
          <div className="w-full max-w-sm rounded-lg bg-white p-6 shadow-xl">
            <h2 className="mb-2 text-lg font-semibold">
              {tCommon('signIn.delete.title')}
            </h2>
            <p className="mb-4 text-sm text-gray-600">
              {tCommon('signIn.delete.warning', { kind: deleteTarget })}
            </p>
            <label className="mb-4 block text-sm">
              {tCommon('signIn.delete.typeToConfirm', { word: 'DELETE' })}
              <input
                type="text"
                value={deleteConfirmText}
                onChange={(e) => setDeleteConfirmText(e.target.value)}
                className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-1.5 text-sm"
                data-testid="delete-confirm-input"
              />
            </label>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => {
                  setDeleteTarget(null);
                  setDeleteConfirmText('');
                }}
                className="rounded px-3 py-1.5 text-sm font-medium text-gray-600 hover:bg-gray-100"
                data-testid="delete-cancel"
              >
                {tCommon('signIn.cancel')}
              </button>
              <button
                type="button"
                onClick={handleDelete}
                disabled={deleteConfirmText !== 'DELETE'}
                aria-disabled={deleteConfirmText !== 'DELETE'}
                className="rounded bg-red-600 px-3 py-1.5 text-sm font-semibold text-white hover:bg-red-500 disabled:opacity-50"
                data-testid="delete-confirm"
              >
                {tCommon('signIn.delete.confirm')}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Per-IdP wizard modals — reusing existing wizard components */}
      <LocalProviderWizard
        open={openWizard === 'local'}
        config={getProviderConfig('local') as LocalProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
      <EntraProviderWizard
        open={openWizard === 'entra'}
        config={getProviderConfig('entra') as EntraProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
      <GoogleProviderWizard
        open={openWizard === 'google'}
        config={getProviderConfig('google') as GoogleProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
      <OktaProviderWizard
        open={openWizard === 'okta'}
        config={getProviderConfig('okta') as OktaProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
      <OidcProviderWizard
        open={openWizard === 'oidc'}
        config={getProviderConfig('oidc') as OidcProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
      <SamlProviderWizard
        open={openWizard === 'saml'}
        config={getProviderConfig('saml') as SamlProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
      <LdapProviderWizard
        open={openWizard === 'ldap'}
        config={getProviderConfig('ldap') as LdapProviderConfig}
        onClose={() => setOpenWizard(null)}
        onTest={(cfg) => handleTest(cfg)}
        onSave={(cfg) => handleSave(cfg)}
      />
    </main>
  );
}
