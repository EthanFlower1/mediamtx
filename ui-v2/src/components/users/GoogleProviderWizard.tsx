// KAI-325: Google Workspace OIDC wizard.
//
// Fields: client_id, client_secret (masked), hosted_domain, test button.
// Renders in: customer admin only.

import { useState, useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { googleProviderSchema, type GoogleProviderFormValues } from '@/lib/authProviderSchemas';
import { PasswordField } from '@/components/PasswordField';
import { WizardShell } from './WizardShell';
import type { GoogleProviderConfig, TestProviderResult } from '@/api/users';

const MASK_SENTINEL = '••••••••';

interface GoogleProviderWizardProps {
  open: boolean;
  config: GoogleProviderConfig;
  onClose: () => void;
  onTest: (config: GoogleProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: GoogleProviderConfig) => Promise<void>;
}

export function GoogleProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: GoogleProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);
  const isMasked = config.clientSecret === MASK_SENTINEL;

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<GoogleProviderFormValues>({
    resolver: zodResolver(googleProviderSchema),
    defaultValues: {
      clientId: config.clientId,
      clientSecret: isMasked ? '' : config.clientSecret,
      hostedDomain: config.hostedDomain,
    },
  });

  const buildConfig = useCallback((values: GoogleProviderFormValues): GoogleProviderConfig => ({
    kind: 'google',
    enabled: true,
    clientId: values.clientId,
    clientSecret: isMasked && !values.clientSecret ? MASK_SENTINEL : values.clientSecret,
    hostedDomain: values.hostedDomain,
  }), [isMasked]);

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
  }, [handleSubmit, onTest, buildConfig]);

  const handleSave = useCallback(() => {
    void handleSubmit(async (values) => {
      await onSave(buildConfig(values));
      onClose();
    })();
  }, [handleSubmit, onSave, onClose, buildConfig]);

  const stepContent = (
    <section
      aria-label={t('auth.google.step1.sectionLabel')}
      data-testid="google-wizard-step1"
    >
      <h3>{t('auth.google.step1.heading')}</h3>

      <div>
        <label htmlFor="google-client-id">{t('auth.google.fields.clientId')}</label>
        <input
          id="google-client-id"
          type="text"
          aria-invalid={errors.clientId ? 'true' : undefined}
          data-testid="google-field-clientId"
          {...register('clientId')}
        />
        {errors.clientId && (
          <p role="alert" data-testid="google-error-clientId">
            {t(errors.clientId.message ?? 'auth.google.errors.clientIdRequired')}
          </p>
        )}
      </div>

      <PasswordField
        id="google-client-secret"
        label={t('auth.google.fields.clientSecret')}
        masked={isMasked}
        error={errors.clientSecret ? t(errors.clientSecret.message ?? 'auth.google.errors.clientSecretRequired') : undefined}
        data-testid="google-field-clientSecret"
        {...register('clientSecret')}
      />

      <div>
        <label htmlFor="google-hosted-domain">{t('auth.google.fields.hostedDomain')}</label>
        <input
          id="google-hosted-domain"
          type="text"
          placeholder="acme.com"
          aria-invalid={errors.hostedDomain ? 'true' : undefined}
          data-testid="google-field-hostedDomain"
          {...register('hostedDomain')}
        />
        {errors.hostedDomain && (
          <p role="alert" data-testid="google-error-hostedDomain">
            {t(errors.hostedDomain.message ?? 'auth.google.errors.hostedDomainRequired')}
          </p>
        )}
      </div>
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.google.name')}
      steps={[{ label: t('auth.google.step1.sectionLabel'), content: stepContent }]}
      currentStep={0}
      onBack={() => {}}
      onNext={() => {}}
      onClose={onClose}
      onTest={handleTest}
      onSave={handleSave}
      isTesting={isTesting}
      testResult={testResult}
      isSaveDisabled={testResult === null || !testResult.success}
      testid="google-provider-wizard"
    />
  );
}
