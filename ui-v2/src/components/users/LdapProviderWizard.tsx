// KAI-325: LDAP / Active Directory wizard — 2 steps.
//
// Step 1: host, port, bind DN, bind password (masked).
// Step 2: base DN, user filter, group filter, attribute mappings.
// Renders in: customer admin only.

import { useState, useCallback } from 'react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { ldapProviderSchema, type LdapProviderFormValues } from '@/lib/authProviderSchemas';

// Step 1 fields for partial validation.
const LDAP_STEP1_FIELDS: (keyof LdapProviderFormValues)[] = [
  'host',
  'port',
  'bindDn',
  'bindPassword',
];
import { PasswordField } from '@/components/PasswordField';
import { WizardShell } from './WizardShell';
import type { LdapProviderConfig, TestProviderResult } from '@/api/users';

const MASK_SENTINEL = '••••••••';

interface LdapProviderWizardProps {
  open: boolean;
  config: LdapProviderConfig;
  onClose: () => void;
  onTest: (config: LdapProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: LdapProviderConfig) => Promise<void>;
}

export function LdapProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: LdapProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [step, setStep] = useState(0);
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);
  const isMasked = config.bindPassword === MASK_SENTINEL;

  const {
    register,
    handleSubmit,
    trigger,
    formState: { errors },
  } = useForm<LdapProviderFormValues>({
    resolver: zodResolver(ldapProviderSchema),
    defaultValues: {
      host: config.host,
      port: config.port,
      bindDn: config.bindDn,
      bindPassword: isMasked ? '' : config.bindPassword,
      baseDn: config.baseDn,
      userFilter: config.userFilter,
      groupFilter: config.groupFilter,
      attrUid: config.attributeMappings.uid,
      attrEmail: config.attributeMappings.email,
      attrName: config.attributeMappings.name,
      attrMemberOf: config.attributeMappings.memberOf,
    },
  });

  const buildConfig = useCallback((values: LdapProviderFormValues): LdapProviderConfig => ({
    kind: 'ldap',
    enabled: true,
    host: values.host,
    port: values.port,
    bindDn: values.bindDn,
    bindPassword: isMasked && !values.bindPassword ? MASK_SENTINEL : values.bindPassword,
    baseDn: values.baseDn,
    userFilter: values.userFilter,
    groupFilter: values.groupFilter,
    attributeMappings: {
      uid: values.attrUid,
      email: values.attrEmail,
      name: values.attrName,
      memberOf: values.attrMemberOf,
    },
  }), [isMasked]);

  const handleNext = useCallback(async () => {
    const valid = await trigger(LDAP_STEP1_FIELDS);
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
    <section aria-label={t('auth.ldap.step1.sectionLabel')} data-testid="ldap-wizard-step1">
      <h3>{t('auth.ldap.step1.heading')}</h3>

      <div>
        <label htmlFor="ldap-host">{t('auth.ldap.fields.host')}</label>
        <input
          id="ldap-host"
          type="text"
          placeholder="ldap.acme.com"
          aria-invalid={errors.host ? 'true' : undefined}
          data-testid="ldap-field-host"
          {...register('host')}
        />
        {errors.host && (
          <p role="alert" data-testid="ldap-error-host">
            {t(errors.host.message ?? 'auth.ldap.errors.hostRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="ldap-port">{t('auth.ldap.fields.port')}</label>
        <input
          id="ldap-port"
          type="number"
          min={1}
          max={65535}
          aria-invalid={errors.port ? 'true' : undefined}
          data-testid="ldap-field-port"
          {...register('port', { valueAsNumber: true })}
        />
        {errors.port && (
          <p role="alert" data-testid="ldap-error-port">
            {t(errors.port.message ?? 'auth.ldap.errors.portInvalid')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="ldap-bind-dn">{t('auth.ldap.fields.bindDn')}</label>
        <input
          id="ldap-bind-dn"
          type="text"
          placeholder={t('auth.ldap.fields.bindDnPlaceholder')}
          aria-invalid={errors.bindDn ? 'true' : undefined}
          data-testid="ldap-field-bindDn"
          {...register('bindDn')}
        />
        {errors.bindDn && (
          <p role="alert" data-testid="ldap-error-bindDn">
            {t(errors.bindDn.message ?? 'auth.ldap.errors.bindDnRequired')}
          </p>
        )}
      </div>

      <PasswordField
        id="ldap-bind-password"
        label={t('auth.ldap.fields.bindPassword')}
        masked={isMasked}
        error={errors.bindPassword ? t(errors.bindPassword.message ?? 'auth.ldap.errors.bindPasswordRequired') : undefined}
        data-testid="ldap-field-bindPassword"
        {...register('bindPassword')}
      />
    </section>
  );

  const step2Content = (
    <section aria-label={t('auth.ldap.step2.sectionLabel')} data-testid="ldap-wizard-step2">
      <h3>{t('auth.ldap.step2.heading')}</h3>

      <div>
        <label htmlFor="ldap-base-dn">{t('auth.ldap.fields.baseDn')}</label>
        <input
          id="ldap-base-dn"
          type="text"
          placeholder={t('auth.ldap.fields.baseDnPlaceholder')}
          aria-invalid={errors.baseDn ? 'true' : undefined}
          data-testid="ldap-field-baseDn"
          {...register('baseDn')}
        />
        {errors.baseDn && (
          <p role="alert" data-testid="ldap-error-baseDn">
            {t(errors.baseDn.message ?? 'auth.ldap.errors.baseDnRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="ldap-user-filter">{t('auth.ldap.fields.userFilter')}</label>
        <input
          id="ldap-user-filter"
          type="text"
          aria-invalid={errors.userFilter ? 'true' : undefined}
          data-testid="ldap-field-userFilter"
          {...register('userFilter')}
        />
        {errors.userFilter && (
          <p role="alert" data-testid="ldap-error-userFilter">
            {t(errors.userFilter.message ?? 'auth.ldap.errors.userFilterRequired')}
          </p>
        )}
      </div>

      <div>
        <label htmlFor="ldap-group-filter">{t('auth.ldap.fields.groupFilter')}</label>
        <input id="ldap-group-filter" type="text" data-testid="ldap-field-groupFilter" {...register('groupFilter')} />
      </div>

      {(
        [
          ['attrUid', 'auth.ldap.fields.attrUid'],
          ['attrEmail', 'auth.ldap.fields.attrEmail'],
          ['attrName', 'auth.ldap.fields.attrName'],
          ['attrMemberOf', 'auth.ldap.fields.attrMemberOf'],
        ] as const
      ).map(([field, labelKey]) => (
        <div key={field}>
          <label htmlFor={`ldap-${field}`}>{t(labelKey)}</label>
          <input
            id={`ldap-${field}`}
            type="text"
            data-testid={`ldap-field-${field}`}
            {...register(field)}
          />
        </div>
      ))}
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.ldap.name')}
      steps={[
        { label: t('auth.ldap.step1.sectionLabel'), content: step1Content },
        { label: t('auth.ldap.step2.sectionLabel'), content: step2Content },
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
      testid="ldap-provider-wizard"
    />
  );
}
