import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import StorageBanner from './StorageBanner'
import { axe } from '../test/axe-helper'

// Mock API client
vi.mock('../api/client', () => ({
  apiFetch: vi.fn(),
}))

const { apiFetch } = await import('../api/client')

describe('StorageBanner accessibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders nothing when storage is healthy', async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ total_bytes: 1e12, used_bytes: 1e11, warning: false, critical: false }),
    } as any)

    const { container } = render(<StorageBanner />)
    // Wait for fetch to complete
    await waitFor(() => expect(apiFetch).toHaveBeenCalled())
    // Should render nothing
    expect(container.innerHTML).toBe('')
  })

  it('uses role="alert" for warning banner', async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ total_bytes: 1e12, used_bytes: 9e11, warning: true, critical: false }),
    } as any)

    render(<StorageBanner />)
    const alert = await screen.findByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert.textContent).toContain('Disk space running low')
  })

  it('uses role="alert" for critical banner', async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ total_bytes: 1e12, used_bytes: 9.8e11, warning: true, critical: true }),
    } as any)

    render(<StorageBanner />)
    const alert = await screen.findByRole('alert')
    expect(alert).toBeInTheDocument()
    expect(alert.textContent).toContain('critically low')
  })

  it('has no axe violations in warning state', async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ total_bytes: 1e12, used_bytes: 9e11, warning: true, critical: false }),
    } as any)

    const { container } = render(<StorageBanner />)
    await screen.findByRole('alert')
    const results = await axe(container)
    expect(results).toHaveNoViolations()
  })
})
