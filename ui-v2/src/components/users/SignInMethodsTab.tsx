// KAI-325: Sign-in Methods tab — orchestrates 6 SSO wizard cards.
//
// Renders in: customer admin only.
// All provider operations go through the IdentityProvider interface
// (KAI-222) via the stub API in api/users.ts. No Zitadel-specific
// knowledge here.

import { useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  authProvidersQueryKeys,
  listAuthProviders,
  testAuthProvider,
  saveAuthProvider,
  type AuthProvider,
  type ProviderConfig,
  type LocalProviderConfig,
  type EntraProviderConfig,
  type GoogleProviderConfig,
  type OktaProviderConfig,
  type OidcProviderConfig,
  type SamlProviderConfig,
  type LdapProviderConfig,
  type TestProviderResult,
} from '@/api/users';
import { LocalProviderWizard } from './LocalProviderWizard';
import { EntraProviderWizard } from './EntraProviderWizard';
import { GoogleProviderWizard } from './GoogleProviderWizard';
import { OktaProviderWizard } from './OktaProviderWizard';
import { OidcProviderWizard } from './OidcProviderWizard';
import { SamlProviderWizard } from './SamlProviderWizard';
import { LdapProviderWizard } from './LdapProviderWizard';

interface SignInMethodsTabProps {
  tenantId: string;
}

type OpenWizard = AuthProvider['kind'] | null;

export function SignInMethodsTab({ tenantId }: SignInMethodsTabProps): JSX.Element {
  const { t } = useTranslation('users');
  const queryClient = useQueryClient();
  const [openWizard, setOpenWizard] = useState<OpenWizard>(null);

  const query = useQuery<AuthProvider[]>({
    queryKey: authProvidersQueryKeys.list(tenantId),
    queryFn: () => listAuthProviders(tenantId),
  });

  const invalidate = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: authProvidersQueryKeys.all(tenantId) });
  }, [queryClient, tenantId]);

  const saveMutation = useMutation({
    mutationFn: (args: { config: ProviderConfig }) =>
      saveAuthProvider({ tenantId, kind: args.config.kind, config: args.config }),
    onSuccess: invalidate,
  });

  const handleTest = useCallback(
    async (config: ProviderConfig): Promise<TestProviderResult> =>
      testAuthProvider({ tenantId, kind: config.kind, config }),
    [tenantId],
  );

  const handleSave = useCallback(
    async (config: ProviderConfig): Promise<void> => {
      await saveMutation.mutateAsync({ config });
    },
    [saveMutation],
  );

  if (query.isLoading) {
    return <p role="status" aria-live="polite">{t('auth.providers.loading')}</p>;
  }
  if (query.isError || !query.data) {
    return <p role="alert">{t('auth.providers.error')}</p>;
  }

  const providers = query.data;

  const getProvider = (kind: AuthProvider['kind']): AuthProvider | undefined =>
    providers.find((p) => p.kind === kind);

  return (
    <section
      aria-label={t('auth.providers.sectionLabel')}
      data-testid="sign-in-methods-tab"
    >
      <h2>{t('auth.providers.heading')}</h2>
      <p>{t('auth.providers.description')}</p>

      <ul style={{ listStyle: 'none', padding: 0 }}>
        {(
          [
            { kind: 'local', nameKey: 'auth.providers.local.name', descKey: 'auth.providers.local.description' },
            { kind: 'entra', nameKey: 'auth.providers.entra.name', descKey: 'auth.providers.entra.description' },
            { kind: 'google', nameKey: 'auth.providers.google.name', descKey: 'auth.providers.google.description' },
            { kind: 'okta', nameKey: 'auth.providers.okta.name', descKey: 'auth.providers.okta.description' },
            { kind: 'oidc', nameKey: 'auth.providers.oidc.name', descKey: 'auth.providers.oidc.description' },
            { kind: 'saml', nameKey: 'auth.providers.saml.name', descKey: 'auth.providers.saml.description' },
            { kind: 'ldap', nameKey: 'auth.providers.ldap.name', descKey: 'auth.providers.ldap.description' },
          ] as const
        ).map(({ kind, nameKey, descKey }) => {
          const provider = getProvider(kind);
          return (
            <li key={kind} data-testid={`provider-card-${kind}`}>
              <strong>{t(nameKey)}</strong>
              <p>{t(descKey)}</p>
              <span data-testid={`provider-status-${kind}`}>
                {provider?.enabled
                  ? t('auth.providers.enabled')
                  : t('auth.providers.disabled')}
              </span>
              <button
                type="button"
                onClick={() => setOpenWizard(kind)}
                data-testid={`provider-configure-${kind}`}
              >
                {provider?.enabled ? t('auth.providers.edit') : t('auth.providers.configure')}
              </button>
            </li>
          );
        })}
      </ul>

      {/* Wizards — only one open at a time */}
      {(() => {
        const local = getProvider('local');
        if (!local) return null;
        return (
          <LocalProviderWizard
            open={openWizard === 'local'}
            config={local.config as LocalProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}

      {(() => {
        const entra = getProvider('entra');
        if (!entra) return null;
        return (
          <EntraProviderWizard
            open={openWizard === 'entra'}
            config={entra.config as EntraProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}

      {(() => {
        const google = getProvider('google');
        if (!google) return null;
        return (
          <GoogleProviderWizard
            open={openWizard === 'google'}
            config={google.config as GoogleProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}

      {(() => {
        const okta = getProvider('okta');
        if (!okta) return null;
        return (
          <OktaProviderWizard
            open={openWizard === 'okta'}
            config={okta.config as OktaProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}

      {(() => {
        const oidc = getProvider('oidc');
        if (!oidc) return null;
        return (
          <OidcProviderWizard
            open={openWizard === 'oidc'}
            config={oidc.config as OidcProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}

      {(() => {
        const saml = getProvider('saml');
        if (!saml) return null;
        return (
          <SamlProviderWizard
            open={openWizard === 'saml'}
            config={saml.config as SamlProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}

      {(() => {
        const ldap = getProvider('ldap');
        if (!ldap) return null;
        return (
          <LdapProviderWizard
            open={openWizard === 'ldap'}
            config={ldap.config as LdapProviderConfig}
            onClose={() => setOpenWizard(null)}
            onTest={(cfg) => handleTest(cfg)}
            onSave={(cfg) => handleSave(cfg)}
          />
        );
      })()}
    </section>
  );
}
