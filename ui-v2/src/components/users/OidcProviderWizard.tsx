// KAI-325: Generic OIDC wizard — 2 steps.
//
// Step 1: issuer URL, client_id, client_secret (masked), scopes.
// Step 2: claim mappings (sub, email, name, groups).
// Renders in: customer admin only.

import { useState, useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import {
  oidcProviderSchema,
  type OidcProviderFormValues,
} from '@/lib/authProviderSchemas';

// Step 1 fields for partial validation.
const OIDC_STEP1_FIELDS: (keyof OidcProviderFormValues)[] = [
  'issuerUrl',
  'clientId',
  'clientSecret',
  'scopes',
];
import { PasswordField } from '@/components/PasswordField';
import { WizardShell } from './WizardShell';
import type { OidcProviderConfig, TestProviderResult } from '@/api/users';

const MASK_SENTINEL = '••••••••';

interface OidcProviderWizardProps {
  open: boolean;
  config: OidcProviderConfig;
  onClose: () => void;
  onTest: (config: OidcProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: OidcProviderConfig) => Promise<void>;
}

export function OidcProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: OidcProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [step, setStep] = useState(0);
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);
  const isMasked = config.clientSecret === MASK_SENTINEL;

  const {
    register,
    handleSubmit,
    trigger,
    formState: { errors },
  } = useForm<OidcProviderFormValues>({
    resolver: zodResolver(oidcProviderSchema),
    defaultValues: {
      issuerUrl: config.issuerUrl,
      clientId: config.clientId,
      clientSecret: isMasked ? '' : config.clientSecret,
      scopes: config.scopes,
      claimSub: config.claimMappings.sub,
      claimEmail: config.claimMappings.email,
      claimName: config.claimMappings.name,
      claimGroups: config.claimMappings.groups,
    },
  });

  const buildConfig = useCallback((values: OidcProviderFormValues): OidcProviderConfig => ({
    kind: 'oidc',
    enabled: true,
    issuerUrl: values.issuerUrl,
    clientId: values.clientId,
    clientSecret: isMasked && !values.clientSecret ? MASK_SENTINEL : values.clientSecret,
    scopes: values.scopes,
    claimMappings: {
      sub: values.claimSub,
      email: values.claimEmail,
      name: values.claimName,
      groups: values.claimGroups,
    },
  }), [isMasked]);

  const handleNext = useCallback(async () => {
    const valid = await trigger(OIDC_STEP1_FIELDS);
    if (valid) setStep(1);
  }, [trigger]);

  const handleBack = useCallback(() => setStep(0), []);

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

  const step1Content = (
    <section aria-label={t('auth.oidc.step1.sectionLabel')} data-testid="oidc-wizard-step1">
      <h3>{t('auth.oidc.step1.heading')}</h3>

      <div>
        <label htmlFor="oidc-issuer-url">{t('auth.oidc.fields.issuerUrl')}</label>
        <input
          id="oidc-issuer-url"
          type="url"
          placeholder="https://idp.example.com"
          aria-invalid={errors.issuerUrl ? 'true' : undefined}
          data-testid="oidc-field-issuerUrl"
          {...register('issuerUrl')}
        />
        {errors.issuerUrl && (
          <p role="alert" data-testid="oidc-error-issuerUrl">
            {t(errors.issuerUrl.message ?? 'auth.oidc.errors.issuerUrlRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="oidc-client-id">{t('auth.oidc.fields.clientId')}</label>
        <input
          id="oidc-client-id"
          type="text"
          aria-invalid={errors.clientId ? 'true' : undefined}
          data-testid="oidc-field-clientId"
          {...register('clientId')}
        />
        {errors.clientId && (
          <p role="alert" data-testid="oidc-error-clientId">
            {t(errors.clientId.message ?? 'auth.oidc.errors.clientIdRequired')}
          </p>
        )}
      </div>

      <PasswordField
        id="oidc-client-secret"
        label={t('auth.oidc.fields.clientSecret')}
        masked={isMasked}
        error={errors.clientSecret ? t(errors.clientSecret.message ?? 'auth.oidc.errors.clientSecretRequired') : undefined}
        data-testid="oidc-field-clientSecret"
        {...register('clientSecret')}
      />

      <div>
        <label htmlFor="oidc-scopes">{t('auth.oidc.fields.scopes')}</label>
        <input
          id="oidc-scopes"
          type="text"
          aria-invalid={errors.scopes ? 'true' : undefined}
          data-testid="oidc-field-scopes"
          {...register('scopes')}
        />
        {errors.scopes && (
          <p role="alert" data-testid="oidc-error-scopes">
            {t(errors.scopes.message ?? 'auth.oidc.errors.scopesRequired')}
          </p>
        )}
      </div>
    </section>
  );

  const step2Content = (
    <section aria-label={t('auth.oidc.step2.sectionLabel')} data-testid="oidc-wizard-step2">
      <h3>{t('auth.oidc.step2.heading')}</h3>

      {(
        [
          ['claimSub', 'auth.oidc.fields.claimSub', 'oidc-claim-sub'],
          ['claimEmail', 'auth.oidc.fields.claimEmail', 'oidc-claim-email'],
          ['claimName', 'auth.oidc.fields.claimName', 'oidc-claim-name'],
          ['claimGroups', 'auth.oidc.fields.claimGroups', 'oidc-claim-groups'],
        ] as const
      ).map(([field, labelKey, inputId]) => (
        <div key={field}>
          <label htmlFor={inputId}>{t(labelKey)}</label>
          <input
            id={inputId}
            type="text"
            data-testid={`oidc-field-${field}`}
            {...register(field)}
          />
        </div>
      ))}
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.oidc.name')}
      steps={[
        { label: t('auth.oidc.step1.sectionLabel'), content: step1Content },
        { label: t('auth.oidc.step2.sectionLabel'), content: step2Content },
      ]}
      currentStep={step}
      onBack={handleBack}
      onNext={() => { void handleNext(); }}
      onClose={onClose}
      onTest={handleTest}
      onSave={handleSave}
      isTesting={isTesting}
      testResult={testResult}
      isSaveDisabled={testResult === null || !testResult.success}
      testid="oidc-provider-wizard"
    />
  );
}
