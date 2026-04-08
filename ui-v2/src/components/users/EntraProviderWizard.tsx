// KAI-325: Microsoft Entra ID (Azure AD) OIDC wizard.
//
// Single step: client_id, client_secret (masked input), tenant_id,
// redirect_uri (read-only computed). Test button gates Save.
//
// Renders in: customer admin only.
// Architectural seam: no Zitadel-specific knowledge here — all provider
// ops go through the IdentityProvider interface (KAI-222).

import { useState, useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { entraProviderSchema, type EntraProviderFormValues } from '@/lib/authProviderSchemas';
import { PasswordField } from '@/components/PasswordField';
import { WizardShell } from './WizardShell';
import type { EntraProviderConfig, TestProviderResult } from '@/api/users';

const REDIRECT_URI = `${window.location.origin}/api/v1/auth/callback/entra`;
const MASK_SENTINEL = '••••••••';

interface EntraProviderWizardProps {
  open: boolean;
  config: EntraProviderConfig;
  onClose: () => void;
  onTest: (config: EntraProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: EntraProviderConfig) => Promise<void>;
}

export function EntraProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: EntraProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);
  const isMasked = config.clientSecret === MASK_SENTINEL;

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<EntraProviderFormValues>({
    resolver: zodResolver(entraProviderSchema),
    defaultValues: {
      clientId: config.clientId,
      clientSecret: isMasked ? '' : config.clientSecret,
      tenantId: config.tenantId,
    },
  });

  const buildConfig = (values: EntraProviderFormValues): EntraProviderConfig => ({
    kind: 'entra',
    enabled: true,
    clientId: values.clientId,
    clientSecret: isMasked && !values.clientSecret ? MASK_SENTINEL : values.clientSecret,
    tenantId: values.tenantId,
    redirectUri: REDIRECT_URI,
  });

  const handleTest = useCallback(() => {
    void handleSubmit(async (values) => {
      setIsTesting(true);
      try {
        const result = await onTest(buildConfig(values));
        setTestResult(result);
      } finally {
        setIsTesting(false);
      }
    })();
  }, [handleSubmit, onTest]);

  const handleSave = useCallback(() => {
    void handleSubmit(async (values) => {
      await onSave(buildConfig(values));
      onClose();
    })();
  }, [handleSubmit, onSave, onClose]);

  const stepContent = (
    <section
      aria-label={t('auth.entra.step1.sectionLabel')}
      data-testid="entra-wizard-step1"
    >
      <h3>{t('auth.entra.step1.heading')}</h3>

      <div>
        <label htmlFor="entra-client-id">{t('auth.entra.fields.clientId')}</label>
        <input
          id="entra-client-id"
          type="text"
          aria-invalid={errors.clientId ? 'true' : undefined}
          aria-describedby={errors.clientId ? 'entra-client-id-error' : undefined}
          data-testid="entra-field-clientId"
          {...register('clientId')}
        />
        {errors.clientId && (
          <p id="entra-client-id-error" role="alert" data-testid="entra-error-clientId">
            {t(errors.clientId.message ?? 'auth.entra.errors.clientIdRequired')}
          </p>
        )}
      </div>

      <PasswordField
        id="entra-client-secret"
        label={t('auth.entra.fields.clientSecret')}
        masked={isMasked}
        error={errors.clientSecret ? t(errors.clientSecret.message ?? 'auth.entra.errors.clientSecretRequired') : undefined}
        data-testid="entra-field-clientSecret"
        {...register('clientSecret')}
      />

      <div>
        <label htmlFor="entra-tenant-id">{t('auth.entra.fields.tenantId')}</label>
        <input
          id="entra-tenant-id"
          type="text"
          aria-invalid={errors.tenantId ? 'true' : undefined}
          aria-describedby={errors.tenantId ? 'entra-tenant-id-error' : undefined}
          data-testid="entra-field-tenantId"
          {...register('tenantId')}
        />
        {errors.tenantId && (
          <p id="entra-tenant-id-error" role="alert" data-testid="entra-error-tenantId">
            {t(errors.tenantId.message ?? 'auth.entra.errors.tenantIdRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="entra-redirect-uri">{t('auth.entra.fields.redirectUri')}</label>
        <input
          id="entra-redirect-uri"
          type="text"
          value={REDIRECT_URI}
          readOnly
          aria-readonly="true"
          data-testid="entra-field-redirectUri"
        />
      </div>
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.entra.name')}
      steps={[{ label: t('auth.entra.step1.sectionLabel'), content: stepContent }]}
      currentStep={0}
      onBack={() => {}}
      onNext={() => {}}
      onClose={onClose}
      onTest={handleTest}
      onSave={handleSave}
      isTesting={isTesting}
      testResult={testResult}
      isSaveDisabled={testResult === null || !testResult.success}
      testid="entra-provider-wizard"
    />
  );
}
