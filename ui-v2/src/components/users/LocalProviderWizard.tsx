// KAI-325: Local (email + password) provider wizard.
//
// Always-available provider: toggle on/off + NIST 800-63B-aligned
// password policy editor. Single step (no connection test needed for
// local auth; test always returns success).
//
// Renders in: customer admin only.

import { useState, useCallback } from 'react';
import { useForm, Controller } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useTranslation } from 'react-i18next';
import { localProviderSchema, type LocalProviderFormValues } from '@/lib/authProviderSchemas';
import { WizardShell } from './WizardShell';
import type { LocalProviderConfig, TestProviderResult } from '@/api/users';

interface LocalProviderWizardProps {
  open: boolean;
  config: LocalProviderConfig;
  onClose: () => void;
  onTest: (config: LocalProviderConfig) => Promise<TestProviderResult>;
  onSave: (config: LocalProviderConfig) => Promise<void>;
}

export function LocalProviderWizard({
  open,
  config,
  onClose,
  onTest,
  onSave,
}: LocalProviderWizardProps): JSX.Element | null {
  const { t } = useTranslation('users');
  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestProviderResult | null>(null);

  const {
    register,
    handleSubmit,
    control,
    formState: { errors },
  } = useForm<LocalProviderFormValues>({
    resolver: zodResolver(localProviderSchema),
    defaultValues: {
      enabled: config.enabled,
      minLength: config.passwordPolicy.minLength,
      requireUppercase: config.passwordPolicy.requireUppercase,
      requireLowercase: config.passwordPolicy.requireLowercase,
      requireDigit: config.passwordPolicy.requireDigit,
      requireSpecial: config.passwordPolicy.requireSpecial,
      rotationDays: config.passwordPolicy.rotationDays ?? 0,
    },
  });

  const buildConfig = (values: LocalProviderFormValues): LocalProviderConfig => ({
    kind: 'local',
    enabled: values.enabled,
    passwordPolicy: {
      minLength: values.minLength,
      requireUppercase: values.requireUppercase,
      requireLowercase: values.requireLowercase,
      requireDigit: values.requireDigit,
      requireSpecial: values.requireSpecial,
      rotationDays: values.rotationDays === 0 ? null : values.rotationDays,
    },
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
      aria-label={t('auth.local.step1.sectionLabel')}
      data-testid="local-wizard-step1"
    >
      <h3>{t('auth.local.step1.heading')}</h3>

      <div>
        <Controller
          name="enabled"
          control={control}
          render={({ field }) => (
            <label>
              <input
                type="checkbox"
                checked={field.value}
                onChange={field.onChange}
                data-testid="local-field-enabled"
              />
              {t('auth.local.fields.enabled')}
            </label>
          )}
        />
      </div>

      <div>
        <label htmlFor="local-min-length">{t('auth.local.fields.minLength')}</label>
        <input
          id="local-min-length"
          type="number"
          min={8}
          max={128}
          aria-invalid={errors.minLength ? 'true' : undefined}
          data-testid="local-field-minLength"
          {...register('minLength', { valueAsNumber: true })}
        />
        {errors.minLength && (
          <p role="alert">{t(errors.minLength.message ?? 'auth.local.errors.minLengthMin')}</p>
        )}
      </div>

      {(
        [
          ['requireUppercase', 'auth.local.fields.requireUppercase'],
          ['requireLowercase', 'auth.local.fields.requireLowercase'],
          ['requireDigit', 'auth.local.fields.requireDigit'],
          ['requireSpecial', 'auth.local.fields.requireSpecial'],
        ] as const
      ).map(([field, labelKey]) => (
        <div key={field}>
          <Controller
            name={field}
            control={control}
            render={({ field: f }) => (
              <label>
                <input
                  type="checkbox"
                  checked={f.value}
                  onChange={f.onChange}
                  data-testid={`local-field-${field}`}
                />
                {t(labelKey)}
              </label>
            )}
          />
        </div>
      ))}

      <div>
        <label htmlFor="local-rotation">{t('auth.local.fields.rotationDays')}</label>
        <input
          id="local-rotation"
          type="number"
          min={0}
          aria-invalid={errors.rotationDays ? 'true' : undefined}
          data-testid="local-field-rotationDays"
          {...register('rotationDays', { valueAsNumber: true })}
        />
        {errors.rotationDays && (
          <p role="alert">{t(errors.rotationDays.message ?? 'auth.local.errors.rotationDaysMin')}</p>
        )}
      </div>
    </section>
  );

  return (
    <WizardShell
      open={open}
      title={t('auth.providers.local.name')}
      steps={[{ label: t('auth.local.step1.sectionLabel'), content: stepContent }]}
      currentStep={0}
      onBack={() => {}}
      onNext={() => {}}
      onClose={onClose}
      onTest={handleTest}
      onSave={handleSave}
      isTesting={isTesting}
      testResult={testResult}
      isSaveDisabled={testResult === null || !testResult.success}
      testid="local-provider-wizard"
    />
  );
}
