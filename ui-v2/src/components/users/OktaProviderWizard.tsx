// KAI-325: Okta OIDC wizard.
//
// Fields: domain, client_id, client_secret (masked), authorization_server_id, test.
// Renders in: customer admin only.

import { useState, useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { oktaProviderSchema, type OktaProviderFormValues } from '@/lib/authProviderSchemas';
import { PasswordField } from '@/components/PasswordField';
import { WizardShell } from './WizardShell';
import type { OktaProviderConfig, TestProviderResult } from '@/api/users';

const MASK_SENTINEL = '••••••••';

interface OktaProviderWizardProps {
  open: boolean;
  config: OktaProviderConfig;
  onClose: () => void;
  onTest: (config: OktaProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: OktaProviderConfig) => Promise<void>;
}

export function OktaProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: OktaProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);
  const isMasked = config.clientSecret === MASK_SENTINEL;

  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<OktaProviderFormValues>({
    resolver: zodResolver(oktaProviderSchema),
    defaultValues: {
      domain: config.domain,
      clientId: config.clientId,
      clientSecret: isMasked ? '' : config.clientSecret,
      authorizationServerId: config.authorizationServerId,
    },
  });

  const buildConfig = useCallback((values: OktaProviderFormValues): OktaProviderConfig => ({
    kind: 'okta',
    enabled: true,
    domain: values.domain,
    clientId: values.clientId,
    clientSecret: isMasked && !values.clientSecret ? MASK_SENTINEL : values.clientSecret,
    authorizationServerId: values.authorizationServerId,
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
      aria-label={t('auth.okta.step1.sectionLabel')}
      data-testid="okta-wizard-step1"
    >
      <h3>{t('auth.okta.step1.heading')}</h3>

      <div>
        <label htmlFor="okta-domain">{t('auth.okta.fields.domain')}</label>
        <input
          id="okta-domain"
          type="text"
          placeholder="acme.okta.com"
          aria-invalid={errors.domain ? 'true' : undefined}
          data-testid="okta-field-domain"
          {...register('domain')}
        />
        {errors.domain && (
          <p role="alert" data-testid="okta-error-domain">
            {t(errors.domain.message ?? 'auth.okta.errors.domainRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="okta-client-id">{t('auth.okta.fields.clientId')}</label>
        <input
          id="okta-client-id"
          type="text"
          aria-invalid={errors.clientId ? 'true' : undefined}
          data-testid="okta-field-clientId"
          {...register('clientId')}
        />
        {errors.clientId && (
          <p role="alert" data-testid="okta-error-clientId">
            {t(errors.clientId.message ?? 'auth.okta.errors.clientIdRequired')}
          </p>
        )}
      </div>

      <PasswordField
        id="okta-client-secret"
        label={t('auth.okta.fields.clientSecret')}
        masked={isMasked}
        error={errors.clientSecret ? t(errors.clientSecret.message ?? 'auth.okta.errors.clientSecretRequired') : undefined}
        data-testid="okta-field-clientSecret"
        {...register('clientSecret')}
      />

      <div>
        <label htmlFor="okta-auth-server">{t('auth.okta.fields.authorizationServerId')}</label>
        <input
          id="okta-auth-server"
          type="text"
          placeholder="default"
          aria-invalid={errors.authorizationServerId ? 'true' : undefined}
          data-testid="okta-field-authorizationServerId"
          {...register('authorizationServerId')}
        />
        {errors.authorizationServerId && (
          <p role="alert" data-testid="okta-error-authorizationServerId">
            {t(errors.authorizationServerId.message ?? 'auth.okta.errors.authorizationServerIdRequired')}
          </p>
        )}
      </div>
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.okta.name')}
      steps={[{ label: t('auth.okta.step1.sectionLabel'), content: stepContent }]}
      currentStep={0}
      onBack={() => {}}
      onNext={() => {}}
      onClose={onClose}
      onTest={handleTest}
      onSave={handleSave}
      isTesting={isTesting}
      testResult={testResult}
      isSaveDisabled={testResult === null || !testResult.success}
      testid="okta-provider-wizard"
    />
  );
}
