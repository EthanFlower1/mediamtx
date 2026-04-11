import { useState, useEffect, useCallback } from 'react'
import { apiFetch } from '../api/client'

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

export type IntegrationCategory = 'access_control' | 'alarm' | 'itsm' | 'comms'

export interface IntegrationDefinition {
  id: string
  name: string
  category: IntegrationCategory
  description: string
  icon: string          // SVG path data for the category icon
  fields: FieldDef[]
}

export interface FieldDef {
  key: string
  label: string
  type: 'text' | 'password' | 'url' | 'select'
  placeholder?: string
  required?: boolean
  options?: { label: string; value: string }[]  // for select type
}

export interface IntegrationConfig {
  id: string
  integration_id: string
  enabled: boolean
  config: Record<string, string>
  status: 'connected' | 'disconnected' | 'error'
  last_tested?: string
  error_message?: string
}

export interface TestResult {
  success: boolean
  message: string
  latency_ms?: number
}

/* ------------------------------------------------------------------ */
/*  Built-in definitions (available integrations)                      */
/* ------------------------------------------------------------------ */

export const CATEGORY_LABELS: Record<IntegrationCategory, string> = {
  access_control: 'Access Control',
  alarm: 'Alarm Panels',
  itsm: 'Incident Management',
  comms: 'Communications',
}

export const INTEGRATION_DEFINITIONS: IntegrationDefinition[] = [
  // Access Control
  {
    id: 'brivo',
    name: 'Brivo',
    category: 'access_control',
    description: 'Cloud-based access control for doors, locks, and gates.',
    icon: 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
    fields: [
      { key: 'api_key', label: 'API Key', type: 'password', required: true, placeholder: 'Enter Brivo API key' },
      { key: 'client_id', label: 'Client ID', type: 'text', required: true, placeholder: 'OAuth client ID' },
      { key: 'client_secret', label: 'Client Secret', type: 'password', required: true, placeholder: 'OAuth client secret' },
      { key: 'base_url', label: 'API Base URL', type: 'url', placeholder: 'https://api.brivo.com' },
    ],
  },
  {
    id: 'openpath',
    name: 'OpenPath',
    category: 'access_control',
    description: 'Mobile-first access control with cloud management.',
    icon: 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
    fields: [
      { key: 'org_id', label: 'Organization ID', type: 'text', required: true, placeholder: 'Your OpenPath org ID' },
      { key: 'api_token', label: 'API Token', type: 'password', required: true, placeholder: 'Bearer token' },
      { key: 'base_url', label: 'API Endpoint', type: 'url', placeholder: 'https://api.openpath.com' },
    ],
  },
  {
    id: 'pdk',
    name: 'PDK',
    category: 'access_control',
    description: 'Cloud-based access control and security management.',
    icon: 'M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z',
    fields: [
      { key: 'panel_id', label: 'Panel ID', type: 'text', required: true, placeholder: 'Cloud node panel ID' },
      { key: 'api_key', label: 'API Key', type: 'password', required: true, placeholder: 'Enter PDK API key' },
    ],
  },

  // Alarm Panels
  {
    id: 'bosch',
    name: 'Bosch',
    category: 'alarm',
    description: 'Bosch alarm panel integration for intrusion detection.',
    icon: 'M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9',
    fields: [
      { key: 'host', label: 'Panel IP Address', type: 'text', required: true, placeholder: '192.168.1.100' },
      { key: 'port', label: 'Port', type: 'text', placeholder: '7700' },
      { key: 'auth_code', label: 'Authorization Code', type: 'password', required: true, placeholder: 'Panel auth code' },
      { key: 'automation_code', label: 'Automation Passcode', type: 'password', placeholder: 'Optional automation passcode' },
    ],
  },
  {
    id: 'dmp',
    name: 'DMP',
    category: 'alarm',
    description: 'DMP alarm systems with remote monitoring.',
    icon: 'M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9',
    fields: [
      { key: 'panel_ip', label: 'Panel IP', type: 'text', required: true, placeholder: '192.168.1.200' },
      { key: 'remote_key', label: 'Remote Key', type: 'password', required: true, placeholder: 'SA remote key' },
      { key: 'account_number', label: 'Account Number', type: 'text', required: true, placeholder: 'DMP account number' },
    ],
  },

  // ITSM
  {
    id: 'pagerduty',
    name: 'PagerDuty',
    category: 'itsm',
    description: 'On-call scheduling and incident escalation.',
    icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
    fields: [
      { key: 'api_token', label: 'API Token', type: 'password', required: true, placeholder: 'v2 REST API key' },
      { key: 'service_id', label: 'Service ID', type: 'text', required: true, placeholder: 'PagerDuty service ID' },
      { key: 'severity', label: 'Default Severity', type: 'select', options: [
        { label: 'Critical', value: 'critical' },
        { label: 'Error', value: 'error' },
        { label: 'Warning', value: 'warning' },
        { label: 'Info', value: 'info' },
      ]},
    ],
  },
  {
    id: 'opsgenie',
    name: 'Opsgenie',
    category: 'itsm',
    description: 'Alert management and incident response by Atlassian.',
    icon: 'M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z',
    fields: [
      { key: 'api_key', label: 'API Key', type: 'password', required: true, placeholder: 'Opsgenie API key' },
      { key: 'team_name', label: 'Team', type: 'text', placeholder: 'Routing team name' },
      { key: 'region', label: 'Region', type: 'select', options: [
        { label: 'US', value: 'us' },
        { label: 'EU', value: 'eu' },
      ]},
    ],
  },

  // Communications
  {
    id: 'slack',
    name: 'Slack',
    category: 'comms',
    description: 'Send alerts and events to Slack channels.',
    icon: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z',
    fields: [
      { key: 'webhook_url', label: 'Webhook URL', type: 'url', required: true, placeholder: 'https://hooks.slack.com/services/...' },
      { key: 'channel', label: 'Channel', type: 'text', placeholder: '#security-alerts' },
      { key: 'username', label: 'Bot Name', type: 'text', placeholder: 'MediaMTX NVR' },
    ],
  },
  {
    id: 'teams',
    name: 'Microsoft Teams',
    category: 'comms',
    description: 'Post notifications to Microsoft Teams channels.',
    icon: 'M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z',
    fields: [
      { key: 'webhook_url', label: 'Incoming Webhook URL', type: 'url', required: true, placeholder: 'https://outlook.office.com/webhook/...' },
      { key: 'card_title', label: 'Card Title', type: 'text', placeholder: 'NVR Alert' },
    ],
  },
]

/* ------------------------------------------------------------------ */
/*  Hook                                                               */
/* ------------------------------------------------------------------ */

export function useIntegrations() {
  const [configs, setConfigs] = useState<IntegrationConfig[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await apiFetch('/integrations')
      if (res.ok) {
        setConfigs(await res.json())
      } else {
        // If the endpoint doesn't exist yet, treat as empty
        if (res.status === 404) {
          setConfigs([])
        } else {
          setError('Failed to load integrations')
        }
      }
    } catch {
      // API not available yet — start with empty
      setConfigs([])
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { refresh() }, [refresh])

  const saveConfig = useCallback(async (
    integrationId: string,
    enabled: boolean,
    config: Record<string, string>,
  ): Promise<{ ok: boolean; error?: string }> => {
    try {
      const existing = configs.find(c => c.integration_id === integrationId)
      const method = existing ? 'PUT' : 'POST'
      const path = existing
        ? `/integrations/${existing.id}`
        : '/integrations'

      const res = await apiFetch(path, {
        method,
        body: JSON.stringify({ integration_id: integrationId, enabled, config }),
      })

      if (res.ok) {
        await refresh()
        return { ok: true }
      }

      const data = await res.json().catch(() => ({}))
      return { ok: false, error: data.error || 'Failed to save' }
    } catch {
      return { ok: false, error: 'Network error' }
    }
  }, [configs, refresh])

  const deleteConfig = useCallback(async (configId: string): Promise<{ ok: boolean; error?: string }> => {
    try {
      const res = await apiFetch(`/integrations/${configId}`, { method: 'DELETE' })
      if (res.ok) {
        await refresh()
        return { ok: true }
      }
      const data = await res.json().catch(() => ({}))
      return { ok: false, error: data.error || 'Failed to delete' }
    } catch {
      return { ok: false, error: 'Network error' }
    }
  }, [refresh])

  const testConnection = useCallback(async (
    integrationId: string,
    config: Record<string, string>,
  ): Promise<TestResult> => {
    try {
      const res = await apiFetch('/integrations/test', {
        method: 'POST',
        body: JSON.stringify({ integration_id: integrationId, config }),
      })

      if (res.ok) {
        return await res.json()
      }

      const data = await res.json().catch(() => ({}))
      return { success: false, message: data.error || `Server returned ${res.status}` }
    } catch {
      return { success: false, message: 'Connection failed — check network' }
    }
  }, [])

  const toggleEnabled = useCallback(async (configId: string, enabled: boolean): Promise<{ ok: boolean; error?: string }> => {
    try {
      const res = await apiFetch(`/integrations/${configId}`, {
        method: 'PATCH',
        body: JSON.stringify({ enabled }),
      })
      if (res.ok) {
        await refresh()
        return { ok: true }
      }
      const data = await res.json().catch(() => ({}))
      return { ok: false, error: data.error || 'Failed to toggle' }
    } catch {
      return { ok: false, error: 'Network error' }
    }
  }, [refresh])

  return {
    definitions: INTEGRATION_DEFINITIONS,
    configs,
    loading,
    error,
    refresh,
    saveConfig,
    deleteConfig,
    testConnection,
    toggleEnabled,
  }
}
