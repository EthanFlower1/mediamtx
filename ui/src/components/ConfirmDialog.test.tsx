import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ConfirmDialog from './ConfirmDialog'
import { axe } from '../test/axe-helper'

const defaultProps = {
  open: true,
  title: 'Delete Camera',
  message: 'Are you sure you want to delete this camera?',
  confirmLabel: 'Delete',
  confirmVariant: 'danger' as const,
  onConfirm: vi.fn(),
  onCancel: vi.fn(),
}

describe('ConfirmDialog accessibility', () => {
  it('has no axe violations when open', async () => {
    const { container } = render(<ConfirmDialog {...defaultProps} />)
    const results = await axe(container)
    expect(results).toHaveNoViolations()
  })

  it('renders nothing when closed', () => {
    const { container } = render(<ConfirmDialog {...defaultProps} open={false} />)
    expect(container.innerHTML).toBe('')
  })

  it('has dialog role and aria-modal', () => {
    render(<ConfirmDialog {...defaultProps} />)
    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveAttribute('aria-modal', 'true')
  })

  it('has aria-labelledby and aria-describedby', () => {
    render(<ConfirmDialog {...defaultProps} />)
    const dialog = screen.getByRole('dialog')
    expect(dialog).toHaveAttribute('aria-labelledby', 'confirm-dialog-title')
    expect(dialog).toHaveAttribute('aria-describedby', 'confirm-dialog-message')
  })

  it('closes on Escape key press', async () => {
    const onCancel = vi.fn()
    render(<ConfirmDialog {...defaultProps} onCancel={onCancel} />)
    const user = userEvent.setup()
    await user.keyboard('{Escape}')
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('focuses confirm button on open', () => {
    render(<ConfirmDialog {...defaultProps} />)
    expect(screen.getByRole('button', { name: 'Delete' })).toHaveFocus()
  })

  it('supports keyboard navigation between buttons', async () => {
    render(<ConfirmDialog {...defaultProps} />)
    const user = userEvent.setup()

    // Confirm button should have initial focus
    expect(screen.getByRole('button', { name: 'Delete' })).toHaveFocus()

    // Shift+Tab to Cancel
    await user.tab({ shift: true })
    expect(screen.getByRole('button', { name: 'Cancel' })).toHaveFocus()
  })
})
