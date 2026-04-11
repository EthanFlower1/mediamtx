import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import Integrations from './Integrations'

/* ------------------------------------------------------------------ */
/*  Mocks                                                              */
/* ------------------------------------------------------------------ */

// Mock auth context — always return admin user
vi.mock('../auth/context', () => ({
  useAuth: () => ({
    user: { id: '1', username: 'admin', role: 'admin' },
    isAuthenticated: true,
    isLoading: false,
    setupRequired: false,
    login: vi.fn(),
    logout: vi.fn(),
  }),
}))

// Mock apiFetch
const mockApiFetch = vi.fn()
vi.mock('../api/client', () => ({
  apiFetch: (...args: unknown[]) => mockApiFetch(...args),
}))

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/integrations']}>
      <Integrations />
    </MemoryRouter>,
  )
}

/* ------------------------------------------------------------------ */
/*  Tests                                                              */
/* ------------------------------------------------------------------ */

describe('Integrations page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Default: GET /integrations returns empty array
    mockApiFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [],
    })
  })

  it('renders page title and all integration cards', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('Integrations')).toBeInTheDocument()
    })

    // Should show all 9 integrations
    expect(screen.getByTestId('integration-card-brivo')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-openpath')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-pdk')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-bosch')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-dmp')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-pagerduty')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-opsgenie')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-slack')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-teams')).toBeInTheDocument()
  })

  it('shows category headers', async () => {
    renderPage()

    // Category labels appear in both the filter tabs and the section headers,
    // so use getAllByText and verify at least one instance exists.
    await waitFor(() => {
      expect(screen.getAllByText('Access Control').length).toBeGreaterThanOrEqual(1)
    })

    expect(screen.getAllByText('Alarm Panels').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('Incident Management').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('Communications').length).toBeGreaterThanOrEqual(1)
  })

  it('filters by category when tab clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('integration-card-brivo')).toBeInTheDocument()
    })

    // Click "Alarm Panels" filter
    await user.click(screen.getByRole('button', { name: /Alarm Panels/ }))

    // Should show alarm integrations
    expect(screen.getByTestId('integration-card-bosch')).toBeInTheDocument()
    expect(screen.getByTestId('integration-card-dmp')).toBeInTheDocument()

    // Should hide access control
    expect(screen.queryByTestId('integration-card-brivo')).not.toBeInTheDocument()
    expect(screen.queryByTestId('integration-card-slack')).not.toBeInTheDocument()
  })

  it('shows "Configure" buttons for unconfigured integrations', async () => {
    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('configure-slack')).toBeInTheDocument()
    })

    expect(screen.getByTestId('configure-slack')).toHaveTextContent('Configure')
  })

  it('shows configured integration with status and toggle', async () => {
    mockApiFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [
        {
          id: 'cfg-1',
          integration_id: 'slack',
          enabled: true,
          config: { webhook_url: 'https://hooks.slack.com/test', channel: '#alerts' },
          status: 'connected',
          last_tested: '2026-04-10T12:00:00Z',
        },
      ],
    })

    renderPage()

    await waitFor(() => {
      const card = screen.getByTestId('integration-card-slack')
      expect(within(card).getByText('Active')).toBeInTheDocument()
    })

    // Should show "Edit" instead of "Configure"
    expect(screen.getByTestId('configure-slack')).toHaveTextContent('Edit')

    // Toggle should exist
    expect(screen.getByTestId('toggle-slack')).toBeInTheDocument()
  })

  it('shows error state on configured integration with error', async () => {
    mockApiFetch.mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => [
        {
          id: 'cfg-2',
          integration_id: 'pagerduty',
          enabled: true,
          config: { api_token: 'tok', service_id: 'svc' },
          status: 'error',
          error_message: 'Invalid API token',
        },
      ],
    })

    renderPage()

    await waitFor(() => {
      const card = screen.getByTestId('integration-card-pagerduty')
      expect(within(card).getByText('Invalid API token')).toBeInTheDocument()
    })
  })

  it('opens wizard when Configure button is clicked', async () => {
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('configure-slack')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('configure-slack'))

    // Wizard should open with Slack fields
    await waitFor(() => {
      expect(screen.getByText('Configure Slack')).toBeInTheDocument()
    })

    expect(screen.getByTestId('field-webhook_url')).toBeInTheDocument()
    expect(screen.getByTestId('field-channel')).toBeInTheDocument()
    expect(screen.getByTestId('field-username')).toBeInTheDocument()
  })

  it('can navigate wizard steps and test connection', async () => {
    const user = userEvent.setup()

    // First call: GET integrations (empty)
    // Second call: POST test -> success
    let callCount = 0
    mockApiFetch.mockImplementation(async (path: string, opts?: RequestInit) => {
      callCount++
      if (opts?.method === 'POST' && path === '/integrations/test') {
        return {
          ok: true,
          status: 200,
          json: async () => ({ success: true, message: 'Connection successful', latency_ms: 42 }),
        }
      }
      // GET integrations
      return { ok: true, status: 200, json: async () => [] }
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('configure-pagerduty')).toBeInTheDocument()
    })

    // Open wizard
    await user.click(screen.getByTestId('configure-pagerduty'))

    await waitFor(() => {
      expect(screen.getByText('Configure PagerDuty')).toBeInTheDocument()
    })

    // Fill required fields
    await user.type(screen.getByTestId('field-api_token'), 'test-token')
    await user.type(screen.getByTestId('field-service_id'), 'svc-123')

    // Go to test step
    await user.click(screen.getByText('Next: Test Connection'))

    // Should be on test step
    await waitFor(() => {
      expect(screen.getByTestId('test-connection-btn')).toBeInTheDocument()
    })

    // Test connection
    await user.click(screen.getByTestId('test-connection-btn'))

    await waitFor(() => {
      const result = screen.getByTestId('test-result')
      expect(result).toHaveTextContent('Connection successful')
    })
  })

  it('saves integration end-to-end', async () => {
    const user = userEvent.setup()

    mockApiFetch.mockImplementation(async (path: string, opts?: RequestInit) => {
      if (opts?.method === 'POST' && path === '/integrations') {
        return { ok: true, status: 201, json: async () => ({ id: 'new-1' }) }
      }
      return { ok: true, status: 200, json: async () => [] }
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('configure-slack')).toBeInTheDocument()
    })

    // Open wizard
    await user.click(screen.getByTestId('configure-slack'))

    await waitFor(() => {
      expect(screen.getByText('Configure Slack')).toBeInTheDocument()
    })

    // Fill webhook URL
    await user.type(screen.getByTestId('field-webhook_url'), 'https://hooks.slack.com/services/test')

    // Go to test step
    await user.click(screen.getByText('Next: Test Connection'))

    await waitFor(() => {
      expect(screen.getByTestId('save-btn')).toBeInTheDocument()
    })

    // Click save
    await user.click(screen.getByTestId('save-btn'))

    // Should reach done step
    await waitFor(() => {
      expect(screen.getByText(/Configured/)).toBeInTheDocument()
    })

    // Verify API was called with correct data
    const saveCall = mockApiFetch.mock.calls.find(
      (c: unknown[]) => c[0] === '/integrations' && (c[1] as RequestInit)?.method === 'POST'
    )
    expect(saveCall).toBeDefined()
    const body = JSON.parse((saveCall![1] as RequestInit).body as string)
    expect(body.integration_id).toBe('slack')
    expect(body.enabled).toBe(true)
    expect(body.config.webhook_url).toBe('https://hooks.slack.com/services/test')
  })

  it('shows test failure with error message', async () => {
    const user = userEvent.setup()

    mockApiFetch.mockImplementation(async (path: string, opts?: RequestInit) => {
      if (opts?.method === 'POST' && path === '/integrations/test') {
        return {
          ok: false,
          status: 400,
          json: async () => ({ error: 'Invalid webhook URL' }),
        }
      }
      return { ok: true, status: 200, json: async () => [] }
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('configure-brivo')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('configure-brivo'))

    await waitFor(() => {
      expect(screen.getByText('Configure Brivo')).toBeInTheDocument()
    })

    // Go to test step
    await user.click(screen.getByText('Next: Test Connection'))

    await waitFor(() => {
      expect(screen.getByTestId('test-connection-btn')).toBeInTheDocument()
    })

    await user.click(screen.getByTestId('test-connection-btn'))

    await waitFor(() => {
      const result = screen.getByTestId('test-result')
      expect(result).toHaveTextContent('Invalid webhook URL')
    })
  })

  it('handles toggle enabled/disabled', async () => {
    const user = userEvent.setup()

    mockApiFetch.mockImplementation(async (path: string, opts?: RequestInit) => {
      if (opts?.method === 'PATCH') {
        return { ok: true, status: 200, json: async () => ({}) }
      }
      return {
        ok: true,
        status: 200,
        json: async () => [
          {
            id: 'cfg-1',
            integration_id: 'slack',
            enabled: true,
            config: { webhook_url: 'https://hooks.slack.com/test' },
            status: 'connected',
          },
        ],
      }
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByTestId('toggle-slack')).toBeInTheDocument()
    })

    // Click toggle
    await user.click(screen.getByTestId('toggle-slack'))

    // Should have called PATCH
    await waitFor(() => {
      const patchCall = mockApiFetch.mock.calls.find(
        (c: unknown[]) => (c[1] as RequestInit)?.method === 'PATCH'
      )
      expect(patchCall).toBeDefined()
      const body = JSON.parse((patchCall![1] as RequestInit).body as string)
      expect(body.enabled).toBe(false)
    })
  })

  it('handles API failure gracefully (empty state)', async () => {
    mockApiFetch.mockRejectedValue(new Error('Network error'))

    renderPage()

    // Should still render the page with integration definitions (empty configs)
    await waitFor(() => {
      expect(screen.getByText('Integrations')).toBeInTheDocument()
    })

    // Cards should still render since definitions are hardcoded
    expect(screen.getByTestId('integration-card-slack')).toBeInTheDocument()
  })
})
