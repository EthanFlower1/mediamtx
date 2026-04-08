import { useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  PLAN_CATALOG,
  type BillingMode,
  type CreateCustomerResult,
  type CreateCustomerSpec,
  type PlanId,
  createCustomer,
} from '@/api/customers';

// KAI-309: Create New Customer wizard (modal, multi-step).
//
// Steps:
//   1. Tenant info (name, contact email, timezone, country)
//   2. Billing mode
//   3. Plan
//   4. Setup token (copy-to-clipboard + QR placeholder) — generated
//      automatically when the step is reached by running createCustomer.
//   5. Review + commit (finish button simply closes the modal; the
//      customer is already created in step 4's success path).
//
// We avoid any third-party wizard library to keep the bundle small
// and to keep step state in plain React. Focus trap + ESC close are
// implemented inline to match ImpersonateConfirmDialog's pattern.

export interface CreateCustomerWizardProps {
  readonly open: boolean;
  readonly onClose: () => void;
  readonly onCreated?: (result: CreateCustomerResult) => void;
}

type StepId = 1 | 2 | 3 | 4 | 5;

interface FormState {
  name: string;
  contactEmail: string;
  timezone: string;
  country: string;
  billingMode: BillingMode;
  plan: PlanId;
}

const DEFAULT_STATE: FormState = {
  name: '',
  contactEmail: '',
  timezone: 'America/New_York',
  country: 'US',
  billingMode: 'direct',
  plan: 'starter',
};

const TIMEZONES = [
  'America/New_York',
  'America/Los_Angeles',
  'Europe/London',
  'Europe/Berlin',
  'Asia/Tokyo',
];
const COUNTRIES = ['US', 'CA', 'GB', 'DE', 'FR', 'JP'];

export function CreateCustomerWizard({
  open,
  onClose,
  onCreated,
}: CreateCustomerWizardProps): JSX.Element | null {
  const { t } = useTranslation();
  const headingId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const [step, setStep] = useState<StepId>(1);
  const [state, setState] = useState<FormState>(DEFAULT_STATE);
  const [errors, setErrors] = useState<Partial<Record<keyof FormState, string>>>({});
  const [result, setResult] = useState<CreateCustomerResult | null>(null);
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!open) {
      setStep(1);
      setState(DEFAULT_STATE);
      setErrors({});
      setResult(null);
      setCreating(false);
      setCreateError(null);
      setCopied(false);
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key !== 'Tab') return;
      const root = dialogRef.current;
      if (!root) return;
      const focusables = root.querySelectorAll<HTMLElement>(
        'button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])',
      );
      if (focusables.length === 0) return;
      const first = focusables[0]!;
      const last = focusables[focusables.length - 1]!;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  if (!open) return null;

  function validateStep1(): boolean {
    const next: Partial<Record<keyof FormState, string>> = {};
    if (!state.name.trim()) next.name = t('customers.create.errors.nameRequired');
    if (!state.contactEmail.trim())
      next.contactEmail = t('customers.create.errors.emailRequired');
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(state.contactEmail))
      next.contactEmail = t('customers.create.errors.emailInvalid');
    if (!state.timezone) next.timezone = t('customers.create.errors.timezoneRequired');
    if (!state.country) next.country = t('customers.create.errors.countryRequired');
    setErrors(next);
    return Object.keys(next).length === 0;
  }

  async function goNext() {
    if (step === 1 && !validateStep1()) return;
    if (step === 3) {
      // Commit creation when moving from plan selection to token display.
      setCreating(true);
      setCreateError(null);
      try {
        const spec: CreateCustomerSpec = {
          name: state.name.trim(),
          contactEmail: state.contactEmail.trim(),
          timezone: state.timezone,
          country: state.country,
          billingMode: state.billingMode,
          plan: state.plan,
        };
        const r = await createCustomer(spec);
        setResult(r);
        onCreated?.(r);
        setStep(4);
      } catch (err) {
        setCreateError(
          err instanceof Error ? err.message : t('customers.create.errors.unknown'),
        );
      } finally {
        setCreating(false);
      }
      return;
    }
    setStep((prev) => Math.min(5, (prev + 1) as StepId));
  }

  function goBack() {
    setStep((prev) => Math.max(1, (prev - 1) as StepId));
  }

  function copyToken() {
    if (!result) return;
    void navigator.clipboard?.writeText(result.setupToken.token).then(
      () => setCopied(true),
      () => setCopied(false),
    );
  }

  return (
    <div
      data-testid="create-wizard-backdrop"
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={headingId}
        data-testid="create-wizard"
        className="w-full max-w-lg rounded-lg bg-white p-5 shadow-xl"
      >
        <header className="mb-3 flex items-start justify-between">
          <div>
            <h2 id={headingId} className="text-lg font-semibold text-slate-900">
              {t('customers.create.title')}
            </h2>
            <p className="text-xs text-slate-500" data-testid="create-wizard-step-indicator">
              {t('customers.create.stepIndicator', { current: step, total: 5 })}
            </p>
          </div>
          <button
            type="button"
            data-testid="create-wizard-close"
            onClick={onClose}
            aria-label={t('customers.create.close')}
            className="rounded p-1 text-slate-500 hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-500"
          >
            ×
          </button>
        </header>

        {step === 1 && (
          <section data-testid="create-step-1" className="space-y-3">
            <h3 className="text-sm font-semibold text-slate-800">
              {t('customers.create.step1.heading')}
            </h3>
            <Field
              testId="create-name"
              label={t('customers.create.step1.name')}
              value={state.name}
              error={errors.name}
              onChange={(v) => setState((s) => ({ ...s, name: v }))}
            />
            <Field
              testId="create-email"
              label={t('customers.create.step1.email')}
              type="email"
              value={state.contactEmail}
              error={errors.contactEmail}
              onChange={(v) => setState((s) => ({ ...s, contactEmail: v }))}
            />
            <Select
              testId="create-timezone"
              label={t('customers.create.step1.timezone')}
              value={state.timezone}
              options={TIMEZONES.map((tz) => ({ value: tz, label: tz }))}
              error={errors.timezone}
              onChange={(v) => setState((s) => ({ ...s, timezone: v }))}
            />
            <Select
              testId="create-country"
              label={t('customers.create.step1.country')}
              value={state.country}
              options={COUNTRIES.map((c) => ({ value: c, label: c }))}
              error={errors.country}
              onChange={(v) => setState((s) => ({ ...s, country: v }))}
            />
          </section>
        )}

        {step === 2 && (
          <section data-testid="create-step-2" className="space-y-3">
            <h3 className="text-sm font-semibold text-slate-800">
              {t('customers.create.step2.heading')}
            </h3>
            <fieldset className="space-y-2">
              <legend className="text-xs text-slate-600">
                {t('customers.create.step2.legend')}
              </legend>
              {(['direct', 'via_integrator'] as BillingMode[]).map((mode) => (
                <label
                  key={mode}
                  className="flex cursor-pointer items-start gap-2 rounded border border-slate-200 p-2 hover:bg-slate-50"
                >
                  <input
                    type="radio"
                    name="billing-mode"
                    data-testid={`create-billing-${mode}`}
                    value={mode}
                    checked={state.billingMode === mode}
                    onChange={() => setState((s) => ({ ...s, billingMode: mode }))}
                  />
                  <span>
                    <span className="block text-sm font-medium text-slate-900">
                      {t(`customers.create.step2.${mode}.title`)}
                    </span>
                    <span className="block text-xs text-slate-600">
                      {t(`customers.create.step2.${mode}.description`)}
                    </span>
                  </span>
                </label>
              ))}
            </fieldset>
          </section>
        )}

        {step === 3 && (
          <section data-testid="create-step-3" className="space-y-3">
            <h3 className="text-sm font-semibold text-slate-800">
              {t('customers.create.step3.heading')}
            </h3>
            <div className="grid gap-2 sm:grid-cols-2">
              {PLAN_CATALOG.map((plan) => (
                <label
                  key={plan.id}
                  className={`cursor-pointer rounded border p-2 text-sm ${
                    state.plan === plan.id
                      ? 'border-blue-500 bg-blue-50'
                      : 'border-slate-200 hover:bg-slate-50'
                  }`}
                >
                  <input
                    type="radio"
                    name="plan"
                    data-testid={`create-plan-${plan.id}`}
                    value={plan.id}
                    checked={state.plan === plan.id}
                    onChange={() => setState((s) => ({ ...s, plan: plan.id }))}
                    className="sr-only"
                  />
                  <span className="block font-semibold text-slate-900">
                    {t(`customers.plans.${plan.id}.name`)}
                  </span>
                  <span className="block text-xs text-slate-600">
                    {t(`customers.plans.${plan.id}.summary`, {
                      cameras: plan.maxCameras,
                    })}
                  </span>
                </label>
              ))}
            </div>
            {createError ? (
              <p role="alert" data-testid="create-error" className="text-xs text-red-700">
                {createError}
              </p>
            ) : null}
          </section>
        )}

        {step === 4 && result && (
          <section data-testid="create-step-4" className="space-y-3">
            <h3 className="text-sm font-semibold text-slate-800">
              {t('customers.create.step4.heading')}
            </h3>
            <p className="text-xs text-slate-600">
              {t('customers.create.step4.description')}
            </p>
            <div
              data-testid="setup-qr"
              aria-label={t('customers.create.step4.qrAriaLabel')}
              className="mx-auto grid h-40 w-40 place-items-center rounded border border-slate-300 bg-slate-100 text-xs text-slate-500"
            >
              {/* Placeholder QR — real build uses a QR lib. */}
              QR: {result.setupToken.qrPayload.slice(0, 20)}…
            </div>
            <div className="flex items-center gap-2">
              <input
                type="text"
                readOnly
                data-testid="setup-token-text"
                value={result.setupToken.token}
                className="flex-1 rounded border border-slate-300 px-2 py-1 font-mono text-xs"
                aria-label={t('customers.create.step4.tokenAriaLabel')}
              />
              <button
                type="button"
                data-testid="setup-token-copy"
                onClick={copyToken}
                className="rounded border border-slate-300 px-2 py-1 text-xs text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500"
              >
                {copied
                  ? t('customers.create.step4.copied')
                  : t('customers.create.step4.copy')}
              </button>
            </div>
          </section>
        )}

        {step === 5 && result && (
          <section data-testid="create-step-5" className="space-y-2 text-sm">
            <h3 className="text-sm font-semibold text-slate-800">
              {t('customers.create.step5.heading')}
            </h3>
            <dl className="grid grid-cols-2 gap-1 text-xs text-slate-700">
              <dt className="font-medium">{t('customers.create.step1.name')}</dt>
              <dd data-testid="review-name">{result.customer.name}</dd>
              <dt className="font-medium">{t('customers.create.step1.email')}</dt>
              <dd>{result.customer.contactEmail}</dd>
              <dt className="font-medium">{t('customers.create.step1.timezone')}</dt>
              <dd>{result.customer.timezone}</dd>
              <dt className="font-medium">{t('customers.create.step1.country')}</dt>
              <dd>{result.customer.country}</dd>
              <dt className="font-medium">{t('customers.create.step2.heading')}</dt>
              <dd>{t(`customers.create.step2.${result.customer.billingMode}.title`)}</dd>
              <dt className="font-medium">{t('customers.create.step3.heading')}</dt>
              <dd>{t(`customers.plans.${result.customer.plan}.name`)}</dd>
            </dl>
          </section>
        )}

        <footer className="mt-5 flex items-center justify-between">
          <button
            type="button"
            data-testid="create-wizard-back"
            onClick={goBack}
            disabled={step === 1 || creating || step === 4 || step === 5}
            className="rounded border border-slate-300 px-3 py-1.5 text-sm text-slate-700 hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-40"
          >
            {t('customers.create.back')}
          </button>
          {step < 5 ? (
            <button
              type="button"
              data-testid="create-wizard-next"
              onClick={goNext}
              disabled={creating}
              className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-60"
            >
              {creating
                ? t('customers.create.creating')
                : step === 3
                  ? t('customers.create.createAction')
                  : t('customers.create.next')}
            </button>
          ) : (
            <button
              type="button"
              data-testid="create-wizard-finish"
              onClick={onClose}
              className="rounded bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500"
            >
              {t('customers.create.finish')}
            </button>
          )}
        </footer>
      </div>
    </div>
  );
}

interface FieldProps {
  readonly label: string;
  readonly value: string;
  readonly onChange: (v: string) => void;
  readonly error?: string;
  readonly testId: string;
  readonly type?: string;
}

function Field({ label, value, onChange, error, testId, type = 'text' }: FieldProps): JSX.Element {
  const id = useId();
  const errorId = useId();
  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-slate-800">
        {label}
      </label>
      <input
        id={id}
        type={type}
        data-testid={testId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={error ? 'true' : 'false'}
        aria-describedby={error ? errorId : undefined}
        className="mt-1 w-full rounded border border-slate-300 px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
      />
      {error ? (
        <p id={errorId} role="alert" data-testid={`${testId}-error`} className="mt-1 text-xs text-red-700">
          {error}
        </p>
      ) : null}
    </div>
  );
}

interface SelectProps {
  readonly label: string;
  readonly value: string;
  readonly onChange: (v: string) => void;
  readonly options: readonly { value: string; label: string }[];
  readonly error?: string;
  readonly testId: string;
}

function Select({ label, value, onChange, options, error, testId }: SelectProps): JSX.Element {
  const id = useId();
  return (
    <div>
      <label htmlFor={id} className="block text-sm font-medium text-slate-800">
        {label}
      </label>
      <select
        id={id}
        data-testid={testId}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="mt-1 w-full rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
      {error ? (
        <p role="alert" className="mt-1 text-xs text-red-700">
          {error}
        </p>
      ) : null}
    </div>
  );
}
