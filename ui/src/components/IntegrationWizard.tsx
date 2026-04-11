import { useState, useEffect, FormEvent, useCallback } from 'react'
import type { IntegrationDefinition, IntegrationConfig, TestResult, FieldDef } from '../hooks/useIntegrations'

/* ------------------------------------------------------------------ */
/*  Props                                                              */
/* ------------------------------------------------------------------ */

interface IntegrationWizardProps {
  definition: IntegrationDefinition
  existingConfig?: IntegrationConfig
  onSave: (enabled: boolean, config: Record<string, string>) => Promise<{ ok: boolean; error?: string }>
  onTest: (config: Record<string, string>) => Promise<TestResult>
  onDelete?: () => Promise<{ ok: boolean; error?: string }>
  onClose: () => void
}

/* ------------------------------------------------------------------ */
/*  Wizard steps                                                       */
/* ------------------------------------------------------------------ */

type WizardStep = 'configure' | 'test' | 'done'

/* ------------------------------------------------------------------ */
/*  Field renderer                                                     */
/* ------------------------------------------------------------------ */

function WizardField({
  field,
  value,
  onChange,
}: {
  field: FieldDef
  value: string
  onChange: (val: string) => void
}) {
  const baseClasses =
    'w-full bg-nvr-bg-input border border-nvr-border rounded-lg px-3 py-2 text-nvr-text-primary placeholder-nvr-text-muted focus:border-nvr-accent focus:ring-1 focus:ring-nvr-accent focus:outline-none transition-colors text-sm'

  if (field.type === 'select' && field.options) {
    return (
      <div>
        <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">
          {field.label}
          {field.required && <span className="text-nvr-danger ml-0.5">*</span>}
        </label>
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className={baseClasses}
          data-testid={`field-${field.key}`}
        >
          <option value="">Select...</option>
          {field.options.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
      </div>
    )
  }

  return (
    <div>
      <label className="block text-sm font-medium text-nvr-text-secondary mb-1.5">
        {field.label}
        {field.required && <span className="text-nvr-danger ml-0.5">*</span>}
      </label>
      <input
        type={field.type === 'password' ? 'password' : 'text'}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={field.placeholder}
        required={field.required}
        className={baseClasses}
        data-testid={`field-${field.key}`}
      />
    </div>
  )
}

/* ------------------------------------------------------------------ */
/*  Status badge                                                       */
/* ------------------------------------------------------------------ */

function StatusBadge({ status }: { status: string }) {
  const map: Record<string, { bg: string; text: string; label: string }> = {
    connected: { bg: 'bg-green-500/10', text: 'text-green-400', label: 'Connected' },
    disconnected: { bg: 'bg-nvr-bg-tertiary', text: 'text-nvr-text-muted', label: 'Disconnected' },
    error: { bg: 'bg-nvr-danger/10', text: 'text-nvr-danger', label: 'Error' },
  }

  const s = map[status] ?? map.disconnected

  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${s.bg} ${s.text}`}>
      <span className={`w-1.5 h-1.5 rounded-full ${status === 'connected' ? 'bg-green-400' : status === 'error' ? 'bg-nvr-danger' : 'bg-nvr-text-muted'}`} />
      {s.label}
    </span>
  )
}

/* ------------------------------------------------------------------ */
/*  Component                                                          */
/* ------------------------------------------------------------------ */

export default function IntegrationWizard({
  definition,
  existingConfig,
  onSave,
  onTest,
  onDelete,
  onClose,
}: IntegrationWizardProps) {
  const isEdit = !!existingConfig
  const [step, setStep] = useState<WizardStep>('configure')
  const [enabled, setEnabled] = useState(existingConfig?.enabled ?? true)
  const [values, setValues] = useState<Record<string, string>>(() => {
    const initial: Record<string, string> = {}
    definition.fields.forEach((f) => {
      initial[f.key] = existingConfig?.config?.[f.key] ?? ''
    })
    return initial
  })
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<TestResult | null>(null)
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  // ESC to close
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [onClose])

  const setField = useCallback((key: string, val: string) => {
    setValues((prev) => ({ ...prev, [key]: val }))
  }, [])

  const handleTest = useCallback(async () => {
    setTesting(true)
    setTestResult(null)
    const result = await onTest(values)
    setTestResult(result)
    setTesting(false)
  }, [onTest, values])

  const handleSave = useCallback(async (e: FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError('')

    const result = await onSave(enabled, values)
    if (result.ok) {
      setStep('done')
    } else {
      setError(result.error || 'Failed to save configuration')
    }
    setSaving(false)
  }, [onSave, enabled, values])

  const handleDelete = useCallback(async () => {
    if (!onDelete) return
    setDeleting(true)
    const result = await onDelete()
    if (result.ok) {
      onClose()
    } else {
      setError(result.error || 'Failed to remove integration')
    }
    setDeleting(false)
  }, [onDelete, onClose])

  /* ---- Done step ---- */
  if (step === 'done') {
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
        <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
        <div
          className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-lg w-full mx-4"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="p-6 text-center">
            <div className="w-12 h-12 rounded-full bg-green-500/10 flex items-center justify-center mx-auto mb-4">
              <svg className="w-6 h-6 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
            </div>
            <h3 className="text-lg font-semibold text-nvr-text-primary mb-1">
              {definition.name} {isEdit ? 'Updated' : 'Configured'}
            </h3>
            <p className="text-sm text-nvr-text-secondary mb-6">
              Integration has been {isEdit ? 'updated' : 'saved'} successfully.
            </p>
            <button
              onClick={onClose}
              className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-6 py-2 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            >
              Done
            </button>
          </div>
        </div>
      </div>
    )
  }

  /* ---- Configure + Test steps ---- */
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" />
      <div
        className="relative bg-nvr-bg-secondary border border-nvr-border rounded-xl shadow-2xl max-w-lg w-full mx-4 max-h-[90vh] overflow-y-auto"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 pt-5 pb-3 border-b border-nvr-border">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-nvr-bg-tertiary flex items-center justify-center">
              <svg className="w-5 h-5 text-nvr-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d={definition.icon} />
              </svg>
            </div>
            <div>
              <h3 className="text-lg font-semibold text-nvr-text-primary">
                {isEdit ? 'Edit' : 'Configure'} {definition.name}
              </h3>
              <p className="text-xs text-nvr-text-muted">{definition.description}</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="w-8 h-8 flex items-center justify-center text-nvr-text-muted hover:text-nvr-text-primary rounded transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
            aria-label="Close"
          >
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        {/* Step indicator */}
        <div className="flex items-center gap-2 px-5 py-3 border-b border-nvr-border/50">
          <button
            onClick={() => setStep('configure')}
            className={`text-xs font-medium px-3 py-1 rounded-full transition-colors ${
              step === 'configure'
                ? 'bg-nvr-accent/10 text-nvr-accent'
                : 'text-nvr-text-muted hover:text-nvr-text-secondary'
            }`}
          >
            1. Configure
          </button>
          <svg className="w-4 h-4 text-nvr-text-muted" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
          </svg>
          <button
            onClick={() => setStep('test')}
            className={`text-xs font-medium px-3 py-1 rounded-full transition-colors ${
              step === 'test'
                ? 'bg-nvr-accent/10 text-nvr-accent'
                : 'text-nvr-text-muted hover:text-nvr-text-secondary'
            }`}
          >
            2. Test & Save
          </button>
        </div>

        {/* Existing status */}
        {isEdit && existingConfig && (
          <div className="px-5 pt-3">
            <div className="flex items-center justify-between">
              <StatusBadge status={existingConfig.status} />
              {existingConfig.last_tested && (
                <span className="text-xs text-nvr-text-muted">
                  Last tested: {new Date(existingConfig.last_tested).toLocaleString()}
                </span>
              )}
            </div>
            {existingConfig.error_message && (
              <p className="text-xs text-nvr-danger mt-1">{existingConfig.error_message}</p>
            )}
          </div>
        )}

        {/* Content */}
        <form onSubmit={handleSave}>
          {step === 'configure' && (
            <div className="p-5 space-y-4">
              {/* Enabled toggle */}
              <div className="flex items-center justify-between">
                <label className="text-sm font-medium text-nvr-text-primary">Enabled</label>
                <button
                  type="button"
                  role="switch"
                  aria-checked={enabled}
                  onClick={() => setEnabled(!enabled)}
                  className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none ${
                    enabled ? 'bg-nvr-accent' : 'bg-nvr-bg-tertiary'
                  }`}
                  data-testid="enabled-toggle"
                >
                  <span
                    className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                      enabled ? 'translate-x-6' : 'translate-x-1'
                    }`}
                  />
                </button>
              </div>

              {/* Fields */}
              {definition.fields.map((field) => (
                <WizardField
                  key={field.key}
                  field={field}
                  value={values[field.key] ?? ''}
                  onChange={(val) => setField(field.key, val)}
                />
              ))}

              {/* Next step */}
              <div className="flex justify-end gap-2 pt-4 border-t border-nvr-border">
                <button
                  type="button"
                  onClick={onClose}
                  className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  Cancel
                </button>
                <button
                  type="button"
                  onClick={() => setStep('test')}
                  className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                >
                  Next: Test Connection
                </button>
              </div>
            </div>
          )}

          {step === 'test' && (
            <div className="p-5 space-y-4">
              {/* Test connection button */}
              <div className="bg-nvr-bg-tertiary/50 border border-nvr-border rounded-lg p-4">
                <p className="text-sm text-nvr-text-secondary mb-3">
                  Test the connection to verify your credentials before saving.
                </p>
                <button
                  type="button"
                  onClick={handleTest}
                  disabled={testing}
                  className="bg-nvr-bg-secondary hover:bg-nvr-border text-nvr-text-primary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  data-testid="test-connection-btn"
                >
                  {testing && (
                    <span className="inline-block w-4 h-4 border-2 border-nvr-text-muted/30 border-t-nvr-text-primary rounded-full animate-spin" />
                  )}
                  {testing ? 'Testing...' : 'Test Connection'}
                </button>

                {/* Test result */}
                {testResult && (
                  <div
                    className={`mt-3 p-3 rounded-lg text-sm ${
                      testResult.success
                        ? 'bg-green-500/10 border border-green-500/20 text-green-400'
                        : 'bg-nvr-danger/10 border border-nvr-danger/20 text-nvr-danger'
                    }`}
                    data-testid="test-result"
                  >
                    <div className="flex items-center gap-2">
                      {testResult.success ? (
                        <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
                        </svg>
                      ) : (
                        <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                          <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                      )}
                      <span>{testResult.message}</span>
                    </div>
                    {testResult.latency_ms !== undefined && (
                      <p className="text-xs mt-1 opacity-70">Latency: {testResult.latency_ms}ms</p>
                    )}
                  </div>
                )}
              </div>

              {error && (
                <div className="bg-nvr-danger/10 border border-nvr-danger/20 rounded-lg p-3 text-sm text-nvr-danger">
                  {error}
                </div>
              )}

              {/* Footer */}
              <div className="flex items-center justify-between pt-4 border-t border-nvr-border">
                <div className="flex gap-2">
                  {isEdit && onDelete && (
                    confirmDelete ? (
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-nvr-danger">Remove this integration?</span>
                        <button
                          type="button"
                          onClick={handleDelete}
                          disabled={deleting}
                          className="bg-nvr-danger hover:bg-nvr-danger-hover text-white font-medium px-3 py-1.5 rounded-lg transition-colors text-xs disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                        >
                          {deleting ? 'Removing...' : 'Confirm'}
                        </button>
                        <button
                          type="button"
                          onClick={() => setConfirmDelete(false)}
                          className="text-xs text-nvr-text-muted hover:text-nvr-text-secondary"
                        >
                          Cancel
                        </button>
                      </div>
                    ) : (
                      <button
                        type="button"
                        onClick={() => setConfirmDelete(true)}
                        className="text-sm text-nvr-danger hover:text-nvr-danger-hover transition-colors focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                        data-testid="remove-integration-btn"
                      >
                        Remove
                      </button>
                    )
                  )}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => setStep('configure')}
                    className="bg-nvr-bg-tertiary hover:bg-nvr-border text-nvr-text-secondary font-medium px-4 py-2 rounded-lg border border-nvr-border transition-colors text-sm focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                  >
                    Back
                  </button>
                  <button
                    type="submit"
                    disabled={saving}
                    className="bg-nvr-accent hover:bg-nvr-accent-hover text-white font-medium px-4 py-2 rounded-lg transition-colors text-sm inline-flex items-center gap-2 disabled:opacity-50 disabled:cursor-not-allowed focus-visible:ring-2 focus-visible:ring-nvr-accent/50 focus-visible:outline-none"
                    data-testid="save-btn"
                  >
                    {saving && (
                      <span className="inline-block w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                    )}
                    {saving ? 'Saving...' : isEdit ? 'Update Integration' : 'Save Integration'}
                  </button>
                </div>
              </div>
            </div>
          )}
        </form>
      </div>
    </div>
  )
}
