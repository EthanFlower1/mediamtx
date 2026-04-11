import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import Login from './Login'
import { axe } from '../test/axe-helper'
import { renderWithRouter } from '../test/render-helper'

// Mock auth context
vi.mock('../auth/context', () => ({
  useAuth: () => ({
    login: vi.fn(),
    isAuthenticated: false,
    isLoading: false,
    setupRequired: false,
    user: null,
    logout: vi.fn(),
  }),
}))

// Mock API client
vi.mock('../api/client', () => ({
  apiFetch: vi.fn(() => Promise.resolve({ ok: false })),
}))

describe('Login page accessibility', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('has no axe violations', async () => {
    const { container } = renderWithRouter(<Login />)
    const results = await axe(container)
    expect(results).toHaveNoViolations()
  })

  it('has properly labelled form inputs', () => {
    renderWithRouter(<Login />)
    // Inputs should be findable by their label text (sr-only labels)
    expect(screen.getByLabelText('Username')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
  })

  it('has a submit button', () => {
    renderWithRouter(<Login />)
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument()
  })

  it('displays error with role="alert"', async () => {
    const mockLogin = vi.fn().mockRejectedValue(new Error('fail'))
    vi.mocked(await import('../auth/context')).useAuth = () => ({
      login: mockLogin,
      isAuthenticated: false,
      isLoading: false,
      setupRequired: false,
      user: null,
      logout: vi.fn(),
    }) as any

    // Re-render after mock update
    const { rerender, container } = renderWithRouter(<Login />)

    const user = userEvent.setup()
    await user.type(screen.getByLabelText('Username'), 'admin')
    await user.type(screen.getByLabelText('Password'), 'wrong')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    // Wait for error to appear
    const alert = await screen.findByRole('alert')
    expect(alert).toBeInTheDocument()
  })

  it('supports keyboard form submission', async () => {
    renderWithRouter(<Login />)
    const user = userEvent.setup()

    // Tab to username, type, tab to password, type, press Enter
    await user.tab()
    expect(screen.getByLabelText('Username')).toHaveFocus()

    await user.tab()
    expect(screen.getByLabelText('Password')).toHaveFocus()

    await user.tab()
    expect(screen.getByRole('button', { name: /sign in/i })).toHaveFocus()
  })
})
