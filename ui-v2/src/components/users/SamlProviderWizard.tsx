// KAI-325: SAML 2.0 wizard — 2 steps.
//
// Step 1: metadata URL OR raw XML paste (at least one required).
// Step 2: entity ID, ACS URL (read-only), signing cert, attribute mappings.
// Renders in: customer admin only.

import { useState, useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { samlProviderSchema, type SamlProviderFormValues } from '@/lib/authProviderSchemas';

// Step 1 field names for partial validation.
const SAML_STEP1_FIELDS: (keyof SamlProviderFormValues)[] = ['metadataUrl', 'metadataXml'];
const ACS_URL = `${window.location.origin}/api/v1/auth/saml/acs`;
import { WizardShell } from './WizardShell';
import type { SamlProviderConfig, TestProviderResult } from '@/api/users';

interface SamlProviderWizardProps {
  open: boolean;
  config: SamlProviderConfig;
  onClose: () => void;
  onTest: (config: SamlProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: SamlProviderConfig) => Promise<void>;
}

export function SamlProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: SamlProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [step, setStep] = useState(0);
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);

  const {
    register,
    handleSubmit,
    trigger,
    formState: { errors },
  } = useForm<SamlProviderFormValues>({
    resolver: zodResolver(samlProviderSchema),
    defaultValues: {
      metadataUrl: config.metadataUrl,
      metadataXml: config.metadataXml,
      entityId: config.entityId,
      signingCert: config.signingCert,
      attrEmail: config.attributeMappings.email,
      attrName: config.attributeMappings.name,
      attrGroups: config.attributeMappings.groups,
    },
  });

  const buildConfig = useCallback((values: SamlProviderFormValues): SamlProviderConfig => ({
    kind: 'saml',
    enabled: true,
    metadataUrl: values.metadataUrl,
    metadataXml: values.metadataXml,
    entityId: values.entityId,
    acsUrl: ACS_URL,
    signingCert: values.signingCert,
    attributeMappings: {
      email: values.attrEmail,
      name: values.attrName,
      groups: values.attrGroups,
    },
  }), []);

  const handleNext = useCallback(async () => {
    const valid = await trigger(SAML_STEP1_FIELDS);
    if (valid) setStep(1);
  }, [trigger]);

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
    <section aria-label={t('auth.saml.step1.sectionLabel')} data-testid="saml-wizard-step1">
      <h3>{t('auth.saml.step1.heading')}</h3>

      <div>
        <label htmlFor="saml-metadata-url">{t('auth.saml.fields.metadataUrl')}</label>
        <input
          id="saml-metadata-url"
          type="url"
          placeholder={t('auth.saml.fields.metadataUrlPlaceholder')}
          aria-invalid={errors.metadataUrl ? 'true' : undefined}
          data-testid="saml-field-metadataUrl"
          {...register('metadataUrl')}
        />
        {errors.metadataUrl && (
          <p role="alert" data-testid="saml-error-metadataUrl">
            {t(errors.metadataUrl.message ?? 'auth.saml.errors.metadataRequired')}
          </p>
        )}
      </div>

      <p>{t('auth.saml.fields.metadataXmlLabel')}</p>

      <div>
        <label htmlFor="saml-metadata-xml">{t('auth.saml.fields.metadataXml')}</label>
        <textarea
          id="saml-metadata-xml"
          rows={8}
          aria-invalid={errors.metadataXml ? 'true' : undefined}
          data-testid="saml-field-metadataXml"
          {...register('metadataXml')}
        />
      </div>
    </section>
  );

  const step2Content = (
    <section aria-label={t('auth.saml.step2.sectionLabel')} data-testid="saml-wizard-step2">
      <h3>{t('auth.saml.step2.heading')}</h3>

      <div>
        <label htmlFor="saml-entity-id">{t('auth.saml.fields.entityId')}</label>
        <input
          id="saml-entity-id"
          type="text"
          aria-invalid={errors.entityId ? 'true' : undefined}
          data-testid="saml-field-entityId"
          {...register('entityId')}
        />
        {errors.entityId && (
          <p role="alert" data-testid="saml-error-entityId">
            {t(errors.entityId.message ?? 'auth.saml.errors.entityIdRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="saml-acs-url">{t('auth.saml.fields.acsUrl')}</label>
        <input
          id="saml-acs-url"
          type="text"
          value={ACS_URL}
          readOnly
          aria-readonly="true"
          data-testid="saml-field-acsUrl"
        />
      </div>

      <div>
        <label htmlFor="saml-signing-cert">{t('auth.saml.fields.signingCert')}</label>
        <textarea
          id="saml-signing-cert"
          rows={6}
          placeholder="-----BEGIN CERTIFICATE-----"
          data-testid="saml-field-signingCert"
          {...register('signingCert')}
        />
      </div>

      <div>
        <label htmlFor="saml-attr-email">{t('auth.saml.fields.attrEmail')}</label>
        <input id="saml-attr-email" type="text" data-testid="saml-field-attrEmail" {...register('attrEmail')} />
      </div>
      <div>
        <label htmlFor="saml-attr-name">{t('auth.saml.fields.attrName')}</label>
        <input id="saml-attr-name" type="text" data-testid="saml-field-attrName" {...register('attrName')} />
      </div>
      <div>
        <label htmlFor="saml-attr-groups">{t('auth.saml.fields.attrGroups')}</label>
        <input id="saml-attr-groups" type="text" data-testid="saml-field-attrGroups" {...register('attrGroups')} />
      </div>
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.saml.name')}
      steps={[
        { label: t('auth.saml.step1.sectionLabel'), content: step1Content },
        { label: t('auth.saml.step2.sectionLabel'), content: step2Content },
      ]}
      currentStep={step}
      onBack={() => setStep(0)}
      onNext={() => { void handleNext(); }}
      onClose={onClose}
      onTest={handleTest}
      onSave={handleSave}
      isTesting={isTesting}
      testResult={testResult}
      isSaveDisabled={testResult === null || !testResult.success}
      testid="saml-provider-wizard"
    />
  );
}
