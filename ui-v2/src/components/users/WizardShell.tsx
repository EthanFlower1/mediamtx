// KAI-325: Multi-step wizard shell shared by all 6 SSO provider wizards.
//
// Renders in: customer admin only (Sign-in Methods tab).
//
// Accessibility: steps use role="group" + aria-label for the step region;
// nav buttons are fully keyboard-navigable; step indicator is aria-live.

import { useTranslation } from 'react-i18next';
import type { ReactNode } from 'react';
import type { TestProviderResult } from '@/api/users';

export interface WizardStep {
  label: string;
  content: ReactNode;
}

interface WizardShellProps {
  open: boolean;
  title: string;
  steps: WizardStep[];
  currentStep: number;
  onBack: () => void;
  onNext: () => void;
  onClose: () => void;
  onTest: () => void;
  onSave: () => void;
  isTesting: boolean;
  testResult: TestProviderResult | null;
  isSaveDisabled: boolean;
  testid?: string;
}

export function WizardShell({
  open,
  title,
  steps,
  currentStep,
  onBack,
  onNext,
  onClose,
  onTest,
  onSave,
  isTesting,
  testResult,
  isSaveDisabled,
  testid,
}: WizardShellProps): JSX.Element | null {
  const { t } = useTranslation('users');

  if (!open) return null;

  const isFirstStep = currentStep === 0;
  const isLastStep = currentStep === steps.length - 1;
  const step = steps[currentStep];

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      data-testid={testid ?? 'sso-wizard'}
    >
      <header>
        <h2>{title}</h2>
        <p
          aria-live="polite"
          aria-atomic="true"
          data-testid="wizard-step-indicator"
        >
          {t('auth.wizard.stepIndicator', {
            step: currentStep + 1,
            total: steps.length,
          })}
        </p>
      </header>

      {/* Step content */}
      <section
        aria-label={step?.label}
        data-testid="wizard-step-content"
      >
        {step?.content}
      </section>

      {/* Test result */}
      {testResult && (
        <div
          role="status"
          aria-live="polite"
          aria-atomic="true"
          data-testid="wizard-test-result"
          data-success={testResult.success ? 'true' : 'false'}
        >
          <span>
            {testResult.success
              ? t('auth.wizard.testResult.success')
              : t('auth.wizard.testResult.failure')}
          </span>
          <p data-testid="wizard-test-message">{testResult.message}</p>
          {testResult.troubleshootingUrl && (
            <a
              href={testResult.troubleshootingUrl}
              target="_blank"
              rel="noreferrer noopener"
              data-testid="wizard-troubleshoot-link"
            >
              {t('auth.wizard.testResult.troubleshoot')}
            </a>
          )}
        </div>
      )}

      {/* Footer navigation */}
      <footer>
        <button
          type="button"
          onClick={onClose}
          data-testid="wizard-cancel"
        >
          {t('auth.wizard.cancel')}
        </button>

        {!isFirstStep && (
          <button
            type="button"
            onClick={onBack}
            data-testid="wizard-back"
          >
            {t('auth.wizard.back')}
          </button>
        )}

        {!isLastStep && (
          <button
            type="button"
            onClick={onNext}
            data-testid="wizard-next"
          >
            {t('auth.wizard.next')}
          </button>
        )}

        {isLastStep && (
          <>
            <button
              type="button"
              onClick={onTest}
              disabled={isTesting}
              aria-disabled={isTesting}
              data-testid="wizard-test-button"
            >
              {isTesting ? t('auth.wizard.testing') : t('auth.wizard.test')}
            </button>
            <button
              type="button"
              onClick={onSave}
              disabled={isSaveDisabled}
              aria-disabled={isSaveDisabled}
              title={isSaveDisabled ? t('auth.wizard.testFirst') : undefined}
              data-testid="wizard-save-button"
            >
              {t('auth.wizard.save')}
            </button>
          </>
        )}
      </footer>
    </div>
  );
}
