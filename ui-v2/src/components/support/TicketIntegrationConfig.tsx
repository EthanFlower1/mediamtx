import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  getTicketHookConfig,
  upsertTicketHookConfig,
  ticketHookConfigQueryKey,
  type TicketHookConfig,
  type TicketHookProvider,
} from '@/api/screenShare';

// KAI-469: Ticket integration configuration panel.
//
// Allows integrator admins to configure which ticket provider (Zendesk,
// Freshdesk, or internal) to use, set the API base URL, toggle auto-create,
// and define tag templates. Configuration is persisted per integrator.

interface TicketIntegrationConfigProps {
  readonly integratorId: string;
}

const PROVIDERS: { value: TicketHookProvider; label: string }[] = [
  { value: 'zendesk', label: 'Zendesk' },
  { value: 'freshdesk', label: 'Freshdesk' },
  { value: 'internal', label: 'Internal' },
];

export function TicketIntegrationConfig({
  integratorId,
}: TicketIntegrationConfigProps): JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const configQuery = useQuery({
    queryKey: ticketHookConfigQueryKey(integratorId),
    queryFn: () => getTicketHookConfig(integratorId),
  });

  const [provider, setProvider] = useState<TicketHookProvider>('internal');
  const [apiBaseUrl, setApiBaseUrl] = useState('');
  const [autoCreate, setAutoCreate] = useState(false);
  const [tagTemplate, setTagTemplate] = useState('kaivue,{{customer_id}}');
  const [enabled, setEnabled] = useState(true);
  const [dirty, setDirty] = useState(false);

  // Populate form when config loads.
  useEffect(() => {
    if (configQuery.data) {
      setProvider(configQuery.data.provider);
      setApiBaseUrl(configQuery.data.apiBaseUrl);
      setAutoCreate(configQuery.data.autoCreate);
      setTagTemplate(configQuery.data.tagTemplate);
      setEnabled(configQuery.data.enabled);
      setDirty(false);
    }
  }, [configQuery.data]);

  const saveMutation = useMutation({
    mutationFn: (config: Omit<TicketHookConfig, 'configId'> & { configId?: string }) =>
      upsertTicketHookConfig(config),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ticketHookConfigQueryKey(integratorId) });
      setDirty(false);
    },
  });

  function handleSave(e: React.FormEvent) {
    e.preventDefault();
    saveMutation.mutate({
      configId: configQuery.data?.configId,
      integratorId,
      provider,
      apiBaseUrl,
      autoCreate,
      tagTemplate,
      enabled,
    });
  }

  function markDirty() {
    setDirty(true);
  }

  return (
    <form
      data-testid="ticket-integration-config"
      onSubmit={handleSave}
      className="rounded-lg border border-slate-200 bg-white shadow-sm"
    >
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-800">
          {t('support.config.heading')}
        </h2>
        <p className="text-xs text-slate-500">{t('support.config.description')}</p>
      </div>

      <div className="p-4 space-y-4">
        {configQuery.isLoading && (
          <p role="status" className="text-xs text-slate-500">
            {t('support.config.loading')}
          </p>
        )}

        {/* Enabled toggle */}
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            data-testid="config-enabled"
            checked={enabled}
            onChange={(e) => {
              setEnabled(e.target.checked);
              markDirty();
            }}
            className="text-blue-600"
          />
          {t('support.config.enabledLabel')}
        </label>

        {/* Provider selector */}
        <fieldset className="space-y-1">
          <legend className="text-xs font-medium text-slate-600">
            {t('support.config.providerLabel')}
          </legend>
          <div className="flex gap-3">
            {PROVIDERS.map((p) => (
              <label key={p.value} className="flex items-center gap-1.5 text-sm">
                <input
                  type="radio"
                  name="provider"
                  value={p.value}
                  checked={provider === p.value}
                  onChange={() => {
                    setProvider(p.value);
                    markDirty();
                  }}
                  className="text-blue-600"
                  data-testid={`config-provider-${p.value}`}
                />
                {p.label}
              </label>
            ))}
          </div>
        </fieldset>

        {/* API Base URL (hidden for internal provider) */}
        {provider !== 'internal' && (
          <label className="block space-y-1">
            <span className="text-xs font-medium text-slate-600">
              {t('support.config.apiBaseUrlLabel')}
            </span>
            <input
              type="url"
              data-testid="config-api-url"
              value={apiBaseUrl}
              onChange={(e) => {
                setApiBaseUrl(e.target.value);
                markDirty();
              }}
              placeholder={
                provider === 'zendesk'
                  ? 'https://yourcompany.zendesk.com'
                  : 'https://yourcompany.freshdesk.com'
              }
              className="block w-full rounded border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </label>
        )}

        {/* Auto-create toggle */}
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            data-testid="config-auto-create"
            checked={autoCreate}
            onChange={(e) => {
              setAutoCreate(e.target.checked);
              markDirty();
            }}
            className="text-blue-600"
          />
          {t('support.config.autoCreateLabel')}
        </label>

        {/* Tag template */}
        <label className="block space-y-1">
          <span className="text-xs font-medium text-slate-600">
            {t('support.config.tagTemplateLabel')}
          </span>
          <input
            type="text"
            data-testid="config-tag-template"
            value={tagTemplate}
            onChange={(e) => {
              setTagTemplate(e.target.value);
              markDirty();
            }}
            placeholder="kaivue,{{customer_id}}"
            className="block w-full rounded border border-slate-300 px-2.5 py-1.5 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <p className="text-xs text-slate-400">
            {t('support.config.tagTemplateHelp')}
          </p>
        </label>

        {/* Actions */}
        <div className="flex items-center gap-2">
          <button
            type="submit"
            data-testid="config-save"
            disabled={!dirty || saveMutation.isPending}
            className="rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 disabled:opacity-40"
          >
            {saveMutation.isPending
              ? t('support.config.saving')
              : t('support.config.saveButton')}
          </button>
          {saveMutation.isSuccess && (
            <span className="text-xs text-green-600">{t('support.config.saved')}</span>
          )}
        </div>

        {saveMutation.isError && (
          <p role="alert" className="text-xs text-red-600">
            {t('support.config.saveError')}
          </p>
        )}
      </div>
    </form>
  );
}
